package engine

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"
)

// FlowPolicy is the per-application decision applied to one new flow. It is produced by the
// caller's rule engine, so the data path stays independent of the database and UI packages.
type FlowPolicy struct {
	PID                int     `json:"-"`
	AppName            string  `json:"appName,omitempty"`
	AppPath            string  `json:"appPath,omitempty"`
	PreferredNIC       string  `json:"preferredNic,omitempty"`
	CustomDNS          string  `json:"customDns,omitempty"`
	FallbackNIC        string  `json:"fallbackNic,omitempty"`
	BandwidthLimitMbps float64 `json:"bandwidthLimitMbps,omitempty"`
	QuotaExceeded      bool    `json:"quotaExceeded,omitempty"`
	RuleID             string  `json:"ruleId,omitempty"`
	Unavailable        bool    `json:"-"`
}

func (p FlowPolicy) blockError() error {
	if p.Unavailable {
		return &dialError{rep: repFailure, err: errors.New("application routing policy is unavailable")}
	}
	if p.QuotaExceeded {
		return &dialError{rep: repFailure, err: errors.New("application data quota exceeded")}
	}
	return nil
}

// PolicyResolver maps application traffic to a rule and receives byte deltas
// for enforcing that rule's quota.
type PolicyResolver interface {
	Resolve(appName, appPath, destination string) FlowPolicy
	ReportUsage(policy FlowPolicy, bytesIn, bytesOut int64)
}

// SetPolicyResolver installs (or clears, with nil) the per-application policy source.
func (b *Balancer) SetPolicyResolver(r PolicyResolver) {
	b.policyMu.Lock()
	b.policy = r
	b.policyMu.Unlock()
}

func (b *Balancer) HasPolicyResolver() bool {
	b.policyMu.RLock()
	defer b.policyMu.RUnlock()
	return b.policy != nil
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

func (b *Balancer) reportUsage(policy FlowPolicy, in, out int64) {
	b.policyMu.RLock()
	r := b.policy
	b.policyMu.RUnlock()
	if r != nil && policy.RuleID != "" {
		r.ReportUsage(policy, in, out)
	}
}

// ResolvePolicy exposes the installed resolver to front-ends that identify an application
// before dialing (the TUN path matches packets to a process and then calls this).
func (b *Balancer) ResolvePolicy(appName, appPath, destination string) FlowPolicy {
	return b.resolvePolicy(appName, appPath, destination)
}

// flowLimiter paces the two directions independently.
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

type ruleMeter struct {
	id      string
	in, out atomic.Int64

	// Guarded by Balancer.meterMu. Each flow keeps the limiter it was assigned.
	rate  float64
	limit *flowLimiter
}

func (m *ruleMeter) add(down bool, n int) {
	if m == nil || n <= 0 {
		return
	}
	if down {
		m.in.Add(int64(n))
	} else {
		m.out.Add(int64(n))
	}
}

func (b *Balancer) meterFor(policy FlowPolicy) (*ruleMeter, *flowLimiter) {
	if policy.RuleID == "" {
		return nil, newFlowLimiter(policy.BandwidthLimitMbps)
	}
	b.meterMu.Lock()
	defer b.meterMu.Unlock()
	if b.meters == nil {
		b.meters = make(map[string]*ruleMeter)
	}
	meter := b.meters[policy.RuleID]
	if meter == nil {
		meter = &ruleMeter{id: policy.RuleID}
		b.meters[policy.RuleID] = meter
	}
	if meter.rate != policy.BandwidthLimitMbps {
		meter.rate = policy.BandwidthLimitMbps
		meter.limit = newFlowLimiter(meter.rate)
	}
	return meter, meter.limit
}

func (b *Balancer) flushRuleMeter(meter *ruleMeter) {
	if meter == nil {
		return
	}
	in, out := meter.in.Swap(0), meter.out.Swap(0)
	if in != 0 || out != 0 {
		b.reportUsage(FlowPolicy{RuleID: meter.id}, in, out)
	}
}

// FlushPolicyUsage publishes bytes from still-open flows. Closing a flow also
// flushes its rule, and atomic swaps ensure each byte is reported only once.
func (b *Balancer) FlushPolicyUsage() {
	b.meterMu.Lock()
	meters := make([]*ruleMeter, 0, len(b.meters))
	for _, meter := range b.meters {
		meters = append(meters, meter)
	}
	b.meterMu.Unlock()
	for _, meter := range meters {
		b.flushRuleMeter(meter)
	}
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
// first, then an explicitly configured fallback.
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
	b.recordProcessTCP(policy.PID, host, port, sl.Name, policy.RuleID)
	meter, limiter := b.meterFor(policy)
	return &slotConn{
		TCPConn:    tcp,
		sl:         sl,
		fam:        f,
		s:          b.S,
		activity:   b.S.track(sl, f),
		limiter:    limiter,
		meter:      meter,
		flushUsage: b.flushRuleMeter,
		traffic:    traffic,
		endTraffic: endTraffic,
	}, nil
}

// acquirePolicySlot offers the preferred target first and the explicit fallback next.
// The default pool keeps its usual recovery behavior. Explicit targets require a usable link.
func (b *Balancer) acquirePolicySlot(f Family, policy FlowPolicy, tried map[string]struct{}) *Slot {
	for _, target := range []string{policy.PreferredNIC, policy.FallbackNIC} {
		switch target {
		case "", "ALL", "net-merge":
			if target == "" && policy.PreferredNIC != "" {
				continue
			}
			if sl := b.S.AcquireFor(f, tried); sl != nil {
				return sl
			}
		case "FASTEST":
			if sl := b.S.acquireFor(f, nil, tried, true, true); sl != nil {
				return sl
			}
		default:
			if sl := b.S.acquireFor(f, map[string]struct{}{target: {}}, tried, true, false); sl != nil {
				return sl
			}
		}
	}
	return nil
}

const relayFailCooldown = 15 * time.Second

var errNotTCP = errors.New("balancer returned a non-TCP connection")
