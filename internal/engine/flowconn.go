package engine

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"

	"netpilot/internal/nics"
)

// slotConn counts a pinned TCP flow's bytes and releases its slot on Close. It exposes
// CloseRead/CloseWrite because the TUN relay half-closes each direction separately.
type slotConn struct {
	*net.TCPConn
	sl          *Slot
	fam         Family
	s           *Scheduler
	once        sync.Once
	activity    *flowActivity
	limiter     *flowLimiter
	policy      FlowPolicy
	usageIn     atomic.Int64
	usageOut    atomic.Int64
	onClose     func(policy FlowPolicy, in, out int64)
	traffic     *trafficCounter
	endTraffic  func()
	trafficOnce sync.Once
}

func (c *slotConn) Read(p []byte) (int, error) {
	if pair := c.sl.limit.Load(); pair != nil && len(p) > pair.chunk {
		p = p[:pair.chunk] // hand out a chunk at a time, so a limited flow stays smooth
	}
	n, err := c.TCPConn.Read(p)
	if n > 0 {
		c.sl.bytesIn[c.fam].Add(int64(n))
		c.usageIn.Add(int64(n))
		c.traffic.add(n)
		if c.activity != nil {
			c.activity.bytes.Add(int64(n))
		}
		c.sl.throttle(true, n)
		c.limiter.read(n)
	}
	return n, err
}

func (c *slotConn) Write(p []byte) (int, error) {
	c.limiter.write(len(p))
	n, err := c.sl.writePaced(c.TCPConn, p, false)
	c.sl.bytesOut[c.fam].Add(int64(n))
	c.usageOut.Add(int64(n))
	c.traffic.add(n)
	if c.activity != nil {
		c.activity.bytes.Add(int64(n))
	}
	return n, err
}

// release frees the slot's balancing count. It happens at the first half-close, not at the final
// Close: the relay can keep a finished flow open for up to a minute waiting for the peer's FIN,
// and counting those as active would steer new flows away from the NIC that just finished.
func (c *slotConn) release() {
	c.once.Do(func() {
		c.s.untrack(c.sl, c.fam, c.activity)
		c.s.ReleaseFor(c.sl, c.fam)
	})
}

func (c *slotConn) CloseRead() error  { c.release(); return c.TCPConn.CloseRead() }
func (c *slotConn) CloseWrite() error { c.release(); return c.TCPConn.CloseWrite() }
func (c *slotConn) Close() error {
	c.release()
	err := c.TCPConn.Close()
	c.trafficOnce.Do(func() {
		if c.onClose != nil {
			c.onClose(c.policy, c.usageIn.Load(), c.usageOut.Load())
		}
		if c.endTraffic != nil {
			c.endTraffic()
		}
	})
	return err
}

// DialPinned is DialTCP for front-ends that hand the conn to someone else to pump.
func (b *Balancer) DialPinned(ctx context.Context, host string, port int) (net.Conn, error) {
	c, sl, f, err := b.DialTCP(ctx, host, port)
	if err != nil {
		return nil, err
	}
	return &slotConn{TCPConn: c.(*net.TCPConn), sl: sl, fam: f, s: b.S, activity: b.S.track(sl, f)}, nil
}

type slotPacketConn struct {
	net.PacketConn
	sl          *Slot
	fam         Family
	balancer    *Balancer
	policy      FlowPolicy
	limiter     *flowLimiter
	usageIn     atomic.Int64
	usageOut    atomic.Int64
	dnsOriginal netip.AddrPort
	dnsUpstream netip.AddrPort
	traffic     *trafficCounter
	endTraffic  func()
	trafficOnce sync.Once
}

func (c *slotPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, a, err := c.PacketConn.ReadFrom(p)
	for err == nil && c.dnsUpstream.IsValid() {
		if source, ok := a.(*net.UDPAddr); ok && netip.AddrPortFrom(source.AddrPort().Addr().Unmap(), uint16(source.Port)) == c.dnsUpstream {
			a = net.UDPAddrFromAddrPort(c.dnsOriginal)
			break
		}
		n, a, err = c.PacketConn.ReadFrom(p)
	}
	if n > 0 {
		c.sl.bytesIn[c.fam].Add(int64(n))
		c.usageIn.Add(int64(n))
		c.traffic.add(n)
		c.sl.throttlePacket(true, n)
		c.limiter.read(n)
	}
	return n, a, err
}

func (c *slotPacketConn) WriteTo(p []byte, a net.Addr) (int, error) {
	c.limiter.write(len(p))
	c.sl.throttlePacket(false, len(p))
	target := a
	if c.dnsUpstream.IsValid() {
		target = net.UDPAddrFromAddrPort(c.dnsUpstream)
	}
	n, err := c.PacketConn.WriteTo(p, target)
	c.sl.bytesOut[c.fam].Add(int64(n))
	c.usageOut.Add(int64(n))
	c.traffic.add(n)
	if n > 0 {
		c.balancer.recordProcessUDP(c.policy.PID, target, c.sl.Name, c.policy.RuleID, p[:n])
	}
	return n, err
}

func (c *slotPacketConn) Close() error {
	err := c.PacketConn.Close()
	c.trafficOnce.Do(func() {
		c.balancer.reportUsage(c.policy, c.usageIn.Load(), c.usageOut.Load())
		if c.endTraffic != nil {
			c.endTraffic()
		}
	})
	return err
}

// ListenUDP opens a UDP socket pinned to one NIC (a UDP "flow" = the socket, so every datagram
// of that flow leaves the same way). UDP flows are short and numerous (DNS), so they don't hold
// a balancing slot: the pick still spreads them via the scheduler's tie-breaking.
func (b *Balancer) ListenUDP(ctx context.Context, f Family) (net.PacketConn, error) {
	return b.ListenUDPFor(ctx, f, 0)
}

func (b *Balancer) ListenUDPFor(ctx context.Context, f Family, pid int) (net.PacketConn, error) {
	return b.ListenUDPWithPolicy(ctx, f, netip.AddrPort{}, FlowPolicy{PID: pid})
}

// ListenUDPWithPolicy pins a TUN UDP flow and can redirect an ordinary DNS query.
func (b *Balancer) ListenUDPWithPolicy(ctx context.Context, f Family, dst netip.AddrPort, policy FlowPolicy) (net.PacketConn, error) {
	if policy.QuotaExceeded {
		return nil, quotaError()
	}
	var upstream netip.AddrPort
	if dst.Port() == 53 && policy.CustomDNS != "" {
		server, err := b.dnsAddress(policy.CustomDNS)
		if err != nil {
			return nil, err
		}
		upstream = server
		f = Fam4
		if server.Addr().Is6() {
			f = Fam6
		}
	}
	network := "udp4"
	if f == Fam6 {
		network = "udp6"
	}
	tried := map[string]struct{}{}
	lastErr := errors.New("no usable network adapter")
	for {
		sl := b.acquirePolicySlot(f, policy, tried)
		if sl == nil {
			return nil, lastErr
		}
		tried[sl.key] = struct{}{}
		lc := net.ListenConfig{Control: nics.ForceEgress(sl.Index, f == Fam6)}
		pc, err := lc.ListenPacket(ctx, network, net.JoinHostPort(sl.ip(f).String(), "0"))
		b.S.ReleaseFor(sl, f)
		if err != nil {
			if classify(err).linkDown {
				b.S.Fail(sl, f, b.FailCooldown)
			}
			lastErr = err
			continue
		}
		traffic, end := b.trackProcess(policy.PID)
		return &slotPacketConn{
			PacketConn: pc, sl: sl, fam: f, balancer: b, policy: policy,
			limiter: newFlowLimiter(policy.BandwidthLimitMbps), traffic: traffic, endTraffic: end,
			dnsUpstream: upstream, dnsOriginal: dst,
		}, nil
	}
}
