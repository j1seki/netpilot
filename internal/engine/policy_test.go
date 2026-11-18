package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"netpilot/internal/nics"
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

func TestDialPinnedForWithBandwidthCapKeepsTCPConn(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		c, err := target.Accept()
		if err == nil {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}
	}()

	loopback, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skipf("loopback interface lookup unavailable: %v", err)
	}
	port := target.Addr().(*net.TCPAddr).Port
	sched := NewScheduler([]nics.Nic{{Name: "loopback", Index: loopback.Index, IP: net.ParseIP("127.0.0.1"), Weight: 1}})
	b := NewBalancer(sched)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := b.DialPinnedFor(ctx, "127.0.0.1", port, FlowPolicy{BandwidthLimitMbps: 10})
	if err != nil {
		t.Fatalf("DialPinnedFor with a per-app cap: %v", err)
	}
	sc, ok := conn.(*slotConn)
	if !ok {
		t.Fatalf("connection type = %T, want *slotConn", conn)
	}
	if sc.limiter == nil {
		t.Fatal("per-app bandwidth limiter was not installed")
	}
	if sc.sl.Name != "loopback" {
		t.Fatalf("slot = %q", sc.sl.Name)
	}
	conn.Close()
	if got := sched.Snapshot()[0].Active; got != 0 {
		t.Fatalf("slot leaked after Close: active=%d", got)
	}
}
