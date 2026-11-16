package engine

import (
	"context"
	"errors"
	"testing"
)

type fakeResolver struct {
	policy FlowPolicy
	in     int64
	out    int64
}

func (f *fakeResolver) Resolve(appName, appPath, destination string) FlowPolicy {
	return f.policy
}

func (f *fakeResolver) ReportUsage(appName string, in, out int64) {
	f.in += in
	f.out += out
}

func TestPolicyResolverIsUsedForFlows(t *testing.T) {
	b := NewBalancer(NewScheduler(nil))
	fr := &fakeResolver{policy: FlowPolicy{PreferredNIC: "eth0", BandwidthLimitMbps: 3, RuleID: "r1"}}
	b.SetPolicyResolver(fr)
	got := b.ResolvePolicy("game", "/games/game", "example.com")
	if got.PreferredNIC != "eth0" || got.BandwidthLimitMbps != 3 {
		t.Fatalf("policy = %+v", got)
	}
	b.reportUsage("game", 10, 20)
	if fr.in != 10 || fr.out != 20 {
		t.Fatalf("usage not reported: %+v", fr)
	}
}

func TestQuotaExceededFlowIsRefusedBeforeDialing(t *testing.T) {
	b := NewBalancer(NewScheduler(nil))
	_, _, _, err := b.DialTCPFor(context.Background(), "192.0.2.1", 443, FlowPolicy{QuotaExceeded: true})
	if err == nil {
		t.Fatal("expected refusal")
	}
	var de *dialError
	if !errors.As(err, &de) {
		t.Fatalf("expected dialError, got %T", err)
	}
}

func TestFlowLimiterIsOptional(t *testing.T) {
	if newFlowLimiter(0) != nil {
		t.Fatal("zero cap should not install a limiter")
	}
	lim := newFlowLimiter(10)
	if lim == nil || lim.in == nil || lim.out == nil {
		t.Fatal("cap should install both directions")
	}
}
