//go:build linux

package netmerge

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"

	"netpilot/internal/engine"
	"netpilot/internal/nics"
)

type bridgeTestResolver struct {
	mu        sync.Mutex
	preferred string
	usedRule  string
	in, out   int64
}

func (r *bridgeTestResolver) Resolve(name, path, destination string) engine.FlowPolicy {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "fixture" && path == "/usr/bin/fixture" && destination == "127.0.0.1" {
		return engine.FlowPolicy{RuleID: "fixture-rule", PreferredNIC: r.preferred}
	}
	return engine.FlowPolicy{}
}

func (r *bridgeTestResolver) ReportUsage(policy engine.FlowPolicy, in, out int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usedRule, r.in, r.out = policy.RuleID, in, out
}

func TestPolicyBridgeResolvesCurrentRulesAndReportsTraffic(t *testing.T) {
	resolver := &bridgeTestResolver{preferred: "loopback"}
	server, err := startPolicyServer(resolver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.close)
	client, err := OpenPolicyClient(server.socket)
	if err != nil {
		t.Fatal(err)
	}
	const pid = 123
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	b := engine.NewBalancer(engine.NewScheduler([]nics.Nic{{Name: "loopback", IP: net.IPv4(127, 0, 0, 1)}}))
	b.SetPolicyResolver(client)
	server.inspect(pid)
	client.SyncActivity(b)
	policy := b.ResolvePolicy("fixture", "/usr/bin/fixture", "127.0.0.1")
	policy.PID = pid
	if policy.RuleID != "fixture-rule" || policy.PreferredNIC != "loopback" {
		t.Fatalf("wrong resolved policy: %+v", policy)
	}
	conn, err := b.DialPinnedFor(t.Context(), "127.0.0.1", listener.Addr().(*net.TCPAddr).Port, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	client.SyncActivity(b)
	if activity := server.inspect(pid); len(activity) != 1 || activity[0].RuleID != policy.RuleID || activity[0].Adapter != "loopback" {
		t.Fatalf("wrong activity: %+v", activity)
	}

	resolver.mu.Lock()
	usedRule, in, out := resolver.usedRule, resolver.in, resolver.out
	resolver.preferred = "backup"
	resolver.mu.Unlock()
	if usedRule != policy.RuleID || in != 0 || out != 4 {
		t.Fatalf("wrong usage for %q: %d in, %d out", usedRule, in, out)
	}
	if next := b.ResolvePolicy("fixture", "/usr/bin/fixture", "127.0.0.1"); next.PreferredNIC != "backup" {
		t.Fatalf("saved rule change did not take effect: %+v", next)
	}
	if activity := server.inspect(pid + 1); len(activity) != 0 {
		t.Fatalf("old PID's activity leaked into new inspection: %+v", activity)
	}
}

func TestPolicyBridgeFailureBlocksNewTCPAndUDPFlows(t *testing.T) {
	server, err := startPolicyServer(&bridgeTestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := OpenPolicyClient(server.socket)
	server.close()
	if err != nil {
		t.Fatal(err)
	}
	b := engine.NewBalancer(engine.NewScheduler([]nics.Nic{{Name: "loopback", IP: net.IPv4(127, 0, 0, 1)}}))
	b.SetPolicyResolver(client)
	policy := b.ResolvePolicy("fixture", "/usr/bin/fixture", "127.0.0.1")
	const blocked = "application routing policy is unavailable"
	ctx := context.Background()
	if conn, slot, _, err := b.DialTCPFor(ctx, "127.0.0.1", 1, policy); conn != nil || slot != nil || err == nil || err.Error() != blocked {
		t.Fatalf("TCP did not fail closed: %v", err)
	}
	target := netip.MustParseAddrPort("127.0.0.1:53")
	if conn, err := b.ListenUDPWithPolicy(ctx, engine.Fam4, target, policy); conn != nil || err == nil || err.Error() != blocked {
		t.Fatalf("UDP did not fail closed: %v", err)
	}
}
