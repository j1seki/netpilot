package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"netpilot/internal/nics"
)

// Balancer dials outbound flows through the Scheduler. Front-ends (SOCKS5 now, TUN later)
// only call DialTCP and Relay, so the balancing logic lives in one place.
type Balancer struct {
	S              *Scheduler
	ConnectTimeout time.Duration
	FailCooldown   time.Duration
	v6mode         atomic.Int32

	policyMu          sync.RWMutex
	policy            PolicyResolver
	meterMu           sync.Mutex
	meters            map[string]*ruleMeter
	processTraffic    processTraffic
	processInspection processInspection
	inspectedPID      atomic.Int64
}

// V6Mode decides what happens to IPv6 flows entering the TUN.
type V6Mode int32

const (
	// V6Auto balances IPv6 only when every adapter has it. When adapters differ, IPv6 flows are
	// refused so apps fall back to IPv4, which spreads over all adapters. (Apps that prefer IPv6
	// would otherwise funnel onto the few adapters that have it.)
	V6Auto V6Mode = iota
	// V6On always allows IPv6, balanced across the adapters that have it.
	V6On
	// V6Off refuses IPv6 entirely.
	V6Off
)

func ParseV6Mode(s string) (V6Mode, bool) {
	switch s {
	case "auto":
		return V6Auto, true
	case "on":
		return V6On, true
	case "off":
		return V6Off, true
	}
	return V6Auto, false
}

func (b *Balancer) SetV6Mode(m V6Mode) { b.v6mode.Store(int32(m)) }
func (b *Balancer) V6Mode() V6Mode     { return V6Mode(b.v6mode.Load()) }

// V6Allowed is consulted for every new IPv6 flow.
func (b *Balancer) V6Allowed() bool {
	switch b.V6Mode() {
	case V6Off:
		return false
	case V6On:
		return b.S.V6Usable()
	}
	return b.S.V6CoversAll()
}

// V6State describes the current IPv6 behaviour for `status` and the tray.
func (b *Balancer) V6State() string {
	switch b.V6Mode() {
	case V6Off:
		return "off (refused, apps use IPv4)"
	case V6On:
		return "on (balanced over the adapters that have IPv6)"
	}
	if b.S.V6CoversAll() {
		return "auto: balanced (every adapter has IPv6)"
	}
	return "auto: refused, apps fall back to IPv4 so every adapter is used (not every adapter has IPv6)"
}

func NewBalancer(s *Scheduler) *Balancer {
	return &Balancer{S: s, ConnectTimeout: 8 * time.Second, FailCooldown: 15 * time.Second}
}

// Windows socket error codes; the portable syscall.E* constants don't match them on Windows.
const (
	wsaeAddrNotAvail syscall.Errno = 10049
	wsaeNetDown      syscall.Errno = 10050
	wsaeNetUnreach   syscall.Errno = 10051
	wsaeTimedOut     syscall.Errno = 10060
	wsaeConnRefused  syscall.Errno = 10061
	wsaeHostUnreach  syscall.Errno = 10065
)

// SOCKS5 reply codes (also used as the generic failure reason).
const (
	repOK              = 0x00
	repFailure         = 0x01
	repNetUnreachable  = 0x03
	repHostUnreachable = 0x04
	repRefused         = 0x05
	repCmdUnsupported  = 0x07
	repAtypUnsupported = 0x08
)

type dialClass struct {
	rep      byte
	retry    bool // worth trying another NIC
	linkDown bool // this NIC itself is broken: sideline it
}

func classify(err error) dialClass {
	is := func(wsa, std syscall.Errno) bool { return errors.Is(err, wsa) || errors.Is(err, std) }
	var dns *net.DNSError
	var ne net.Error
	switch {
	case errors.As(err, &dns):
		return dialClass{rep: repHostUnreachable}
	case is(wsaeAddrNotAvail, syscall.EADDRNOTAVAIL), is(wsaeNetDown, syscall.ENETDOWN):
		return dialClass{rep: repNetUnreachable, retry: true, linkDown: true}
	case is(wsaeNetUnreach, syscall.ENETUNREACH):
		return dialClass{rep: repNetUnreachable, retry: true, linkDown: true}
	case is(wsaeHostUnreach, syscall.EHOSTUNREACH), is(wsaeTimedOut, syscall.ETIMEDOUT):
		return dialClass{rep: repHostUnreachable, retry: true}
	case is(wsaeConnRefused, syscall.ECONNREFUSED):
		return dialClass{rep: repRefused}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return dialClass{rep: repHostUnreachable, retry: true}
	}
	return dialClass{rep: repFailure}
}

// DialTCP connects to host:port over one NIC, failing over to the others on link-level errors.
// On success the caller owns the returned slot and must Relay or Release it.
func (b *Balancer) DialTCP(ctx context.Context, host string, port int) (net.Conn, *Slot, Family, error) {
	appName, appPath := appFromContext(ctx)
	return b.DialTCPFor(ctx, host, port, FlowPolicy{AppName: appName, AppPath: appPath})
}

// DialTCPFor connects through the matching rule's allowed links. Quota-exceeded
// flows are refused before dialing.
func (b *Balancer) DialTCPFor(ctx context.Context, host string, port int, policy FlowPolicy) (net.Conn, *Slot, Family, error) {
	if err := policy.blockError(); err != nil {
		return nil, nil, Fam4, err
	}
	if port == 53 && policy.CustomDNS != "" {
		server, err := b.dnsAddress(policy.CustomDNS)
		if err != nil {
			return nil, nil, Fam4, &dialError{rep: repHostUnreachable, err: err}
		}
		return b.dialTCPAddress(ctx, server.Addr().String(), int(server.Port()), policy)
	}
	if ip, _ := netip.ParseAddr(host); !ip.IsValid() && policy.CustomDNS != "" {
		resolved, err := b.resolveHost(ctx, host, policy)
		if err != nil {
			return nil, nil, Fam4, &dialError{rep: repHostUnreachable, err: err}
		}
		var lastErr error
		for _, addr := range resolved {
			conn, sl, f, err := b.dialTCPAddress(ctx, addr.String(), port, policy)
			if err == nil {
				return conn, sl, f, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = &dialError{rep: repHostUnreachable, err: errors.New("custom DNS returned no usable addresses")}
		}
		return nil, nil, Fam4, lastErr
	}
	return b.dialTCPAddress(ctx, host, port, policy)
}

func (b *Balancer) dialTCPAddress(ctx context.Context, host string, port int, policy FlowPolicy) (net.Conn, *Slot, Family, error) {
	f, network := Fam4, "tcp4"
	if ip, _ := netip.ParseAddr(host); ip.Unmap().Is6() {
		f, network = Fam6, "tcp6"
	}

	tried := map[string]struct{}{}
	rep := byte(repNetUnreachable)
	lastErr := errors.New("no usable network adapter for the route")
	for {
		sl := b.acquirePolicySlot(f, policy, tried)
		if sl == nil {
			return nil, nil, f, &dialError{rep: rep, err: lastErr}
		}
		tried[sl.key] = struct{}{}
		addr := sl.addr.Load()
		if addr.ip(f) == nil {
			b.S.ReleaseFor(sl, f)
			continue
		}
		d := net.Dialer{
			LocalAddr: &net.TCPAddr{IP: addr.ip(f)},
			Control:   nics.ForceEgress(addr.index, f == Fam6),
			Timeout:   b.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return conn, sl, f, nil
		}
		b.S.ReleaseFor(sl, f)
		c := classify(err)
		if c.linkDown {
			b.S.Fail(sl, f, b.FailCooldown)
		}
		rep, lastErr = c.rep, err
		if !c.retry || ctx.Err() != nil {
			return nil, nil, f, &dialError{rep: rep, err: err}
		}
	}
}

type dialError struct {
	rep byte
	err error
}

func (e *dialError) Error() string { return e.err.Error() }
func (e *dialError) Unwrap() error { return e.err }

var bufPool = sync.Pool{New: func() any { b := make([]byte, 128<<10); return &b }}

// Relay copies both directions until both sides finish, counting bytes on the slot, then
// closes both conns and releases the slot. Half-close is honoured so request/response
// protocols that shut down their write side still receive the reply. It returns bytes
// received from the peer (in) and bytes sent to it (out) for per-application accounting.
func Relay(client, upstream net.Conn, sl *Slot, f Family, s *Scheduler) (in, out int64) {
	return RelayLimited(client, upstream, sl, f, s, 0)
}

// RelayLimited relays a SOCKS flow while applying an optional per-flow cap in
// addition to the adapter-wide cap on its Slot. The limiter is shared by both
// relay directions and remains attached to the concrete upstream TCPConn.
func RelayLimited(client, upstream net.Conn, sl *Slot, f Family, s *Scheduler, limitMbps float64) (in, out int64) {
	return relayWithTraffic(client, upstream, sl, f, s, newFlowLimiter(limitMbps), nil, nil)
}

func relayWithTraffic(client, upstream net.Conn, sl *Slot, f Family, s *Scheduler, limiter *flowLimiter, traffic *trafficCounter, meter *ruleMeter) (in, out int64) {
	activity := s.track(sl, f)
	var wg sync.WaitGroup
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.untrack(sl, f, activity)
			s.ReleaseFor(sl, f)
		})
	} // at the first half-close, see slotConn
	wg.Add(1)
	go func() {
		defer wg.Done()
		n, err := pumpLimited(upstream, client, &sl.bytesOut[f], sl, false, activity, limiter, traffic, meter)
		out = n
		if isLinkDown(err) {
			s.Fail(sl, f, relayFailCooldown)
		}
		release()
	}()
	var err error
	in, err = pumpLimited(client, upstream, &sl.bytesIn[f], sl, true, activity, limiter, traffic, meter)
	if isLinkDown(err) {
		s.Fail(sl, f, relayFailCooldown)
	}
	release()
	wg.Wait()
	client.Close()
	upstream.Close()
	return in, out
}

func isLinkDown(err error) bool {
	return err != nil && err != io.EOF && classify(err).linkDown
}

func pump(dst, src net.Conn, counter *atomic.Int64, sl *Slot, down bool, activity *flowActivity) (int64, error) {
	return pumpLimited(dst, src, counter, sl, down, activity, nil, nil, nil)
}

func pumpLimited(dst, src net.Conn, counter *atomic.Int64, sl *Slot, down bool, activity *flowActivity, flowLimit *flowLimiter, traffic *trafficCounter, meter *ruleMeter) (int64, error) {
	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	buf := *bp
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			total += int64(n)
			activity.bytes.Add(int64(n))
			counter.Add(int64(n))
			traffic.add(n)
			meter.add(down, n)
			if flowLimit != nil {
				if down {
					flowLimit.read(n)
				} else {
					flowLimit.write(n)
				}
			}
			if _, werr := sl.writePaced(dst, buf[:n], down); werr != nil {
				dst.Close()
				src.Close()
				return total, werr
			}
		}
		if err != nil {
			if err == io.EOF {
				if cw, ok := dst.(interface{ CloseWrite() error }); ok {
					cw.CloseWrite()
				} else {
					dst.Close()
				}
			} else {
				dst.Close()
				src.Close()
			}
			return total, err
		}
	}
}
