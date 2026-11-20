package engine

import (
	"context"
	"errors"
	"net"
	"time"
)

// FlowPolicy is the per-application decision applied to one new flow. It is produced by the
// caller's rule engine, so the data path stays independent of the database and UI packages.
type FlowPolicy struct {
	PID                int     `json:"-"`
	AppName            string  `json:"appName,omitempty"`
	AppPath            string  `json:"appPath,omitempty"`
	PreferredNIC       string  `json:"preferredNic,omitempty"`
	FallbackNIC        string  `json:"fallbackNic,omitempty"`
	BandwidthLimitMbps float64 `json:"bandwidthLimitMbps,omitempty"`
	QuotaExceeded      bool    `json:"quotaExceeded,omitempty"`
	RuleID             string  `json:"ruleId,omitempty"`
}

// PolicyResolver turns an application identity into a FlowPolicy and receives the flow's
// final byte counts so per-application quotas can be maintained.
type PolicyResolver interface {
	Resolve(appName, appPath, destination string) FlowPolicy
	ReportUsage(appName string, bytesIn, bytesOut int64)
}

// SetPolicyResolver installs (or clears, with nil) the per-application policy source.
func (b *Balancer) SetPolicyResolver(r PolicyResolver) {
	b.policyMu.Lock()
	b.policy = r
	b.policyMu.Unlock()
}

func (b *Balancer) resolvePolicy(appName, appPath, destination string) FlowPolicy {
	b.policyMu.RLock()
	r := b.policy
	b.policyMu.RUnlock()
	if r == nil {
		return FlowPolicy{AppName: appName, AppPath: appPath}
	}
	p := r.Resolve(appName, appPath, destination)
	p.AppName, p.AppPath = appName, appPath
	return p
}

func (b *Balancer) reportUsage(appName string, in, out int64) {
	b.policyMu.RLock()
	r := b.policy
	b.policyMu.RUnlock()
	if r != nil && appName != "" {
		r.ReportUsage(appName, in, out)
	}
}

// ResolvePolicy exposes the installed resolver to front-ends that identify an application
// before dialing (the TUN path matches packets to a process and then calls this).
func (b *Balancer) ResolvePolicy(appName, appPath, destination string) FlowPolicy {
	return b.resolvePolicy(appName, appPath, destination)
}

// flowLimiter is a per-flow token bucket pair. Adapter caps are shared by every flow on the
// adapter; this adds the per-application cap on top of it.
type flowLimiter struct {
	in, out *bucket
}

func newFlowLimiter(mbitPerSec float64) *flowLimiter {
	if mbitPerSec <= 0 {
		return nil
	}
	rate := mbitPerSec * bytesPerMbit
	return &flowLimiter{in: newBucket(rate), out: newBucket(rate)}
}

func (l *flowLimiter) read(n int) {
	if l != nil && n > 0 {
		l.in.take(n)
	}
}

func (l *flowLimiter) write(n int) {
	if l != nil && n > 0 {
		l.out.take(n)
	}
}

// limitedConn applies a per-flow cap while still exposing an ordinary net.Conn to the relay.
type limitedConn struct {
	net.Conn
	lim *flowLimiter
}

func (c *limitedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.lim.read(n)
	return n, err
}

func (c *limitedConn) Write(p []byte) (int, error) {
	c.lim.write(len(p))
	return c.Conn.Write(p)
}

func maybeLimited(conn net.Conn, mbitPerSec float64) net.Conn {
	if mbitPerSec <= 0 {
		return conn
	}
	return &limitedConn{Conn: conn, lim: newFlowLimiter(mbitPerSec)}
}

// appIdentity is the context key used when the caller learned the app before dialing.
type appIdentity struct{ name, path string }

func withAppIdentity(ctx context.Context, name, path string) context.Context {
	if name == "" && path == "" {
		return ctx
	}
	return context.WithValue(ctx, appIdentity{}, appIdentity{name: name, path: path})
}

func appFromContext(ctx context.Context) (string, string) {
	v, _ := ctx.Value(appIdentity{}).(appIdentity)
	return v.name, v.path
}

// DialPinnedFor is DialPinned with a per-application policy: the preferred adapter is tried
// first, then the fallback, then the normal pool.
func (b *Balancer) DialPinnedFor(ctx context.Context, host string, port int, policy FlowPolicy) (net.Conn, error) {
	c, sl, f, err := b.DialTCPFor(ctx, host, port, policy)
	if err != nil {
		return nil, err
	}
	tcp, ok := c.(*net.TCPConn)
	if !ok {
		b.S.ReleaseFor(sl, f)
		c.Close()
		return nil, &dialError{rep: repFailure, err: errNotTCP}
	}
	traffic, endTraffic := b.trackProcess(policy.PID)
	b.recordProcessTCP(policy.PID, host, port, sl.Name)
	return &slotConn{
		TCPConn:    tcp,
		sl:         sl,
		fam:        f,
		s:          b.S,
		activity:   b.S.track(sl, f),
		limiter:    newFlowLimiter(policy.BandwidthLimitMbps),
		appName:    policy.AppName,
		onClose:    b.reportUsage,
		traffic:    traffic,
		endTraffic: endTraffic,
	}, nil
}

const relayFailCooldown = 15 * time.Second

var errNotTCP = errors.New("balancer returned a non-TCP connection")
