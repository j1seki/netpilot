package netmerge

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netpilot/internal/engine"
	"netpilot/internal/nics"
	"netpilot/internal/state"
)

func TestStopSocks5ClosesClientConnections(t *testing.T) {
	c := NewController(t.TempDir())
	defer close(c.stopCh)
	defer c.StopOwned()
	if err := c.StartSocks5("0.0.0.0:0"); err == nil {
		t.Fatal("SOCKS5 accepted an address outside loopback")
	}
	if err := c.StartSocks5("localhost:0"); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetStatus()
	if err != nil || !st.SocksRunning {
		t.Fatalf("proxy status: %+v, %v", st, err)
	}
	conn, err := net.DialTimeout("tcp4", st.SocksAddress, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil || reply[0] != 5 || reply[1] != 0 {
		t.Fatalf("SOCKS5 handshake: %v, %v", reply, err)
	}
	if err := c.StopSocks5(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(reply); !errors.Is(err, io.EOF) {
		t.Fatalf("client connection is still open: %v", err)
	}
}

func TestStopOwnedLeavesIndependentEngineRunning(t *testing.T) {
	dir := t.TempDir()
	c := NewController(dir)
	defer close(c.stopCh)
	record := state.Status{PID: 12345, Since: time.Now(), Updated: time.Now()}
	state.Write(dir, record)
	if err := c.StopOwned(); err != nil {
		t.Fatal(err)
	}
	if state.TakeMarker(dir, "off") {
		t.Fatal("stop marker sent to an independent engine")
	}
	c.mu.Lock()
	c.launchedPID, c.launchedSince = record.PID, record.Since
	c.mu.Unlock()
	if err := c.StopOwned(); err != nil {
		t.Fatal(err)
	}
	if !state.TakeMarker(dir, "off") {
		t.Fatal("missing stop marker for engine started by this controller")
	}
}

func TestCleanupStaleRefusesIndependentEngine(t *testing.T) {
	dir := t.TempDir()
	c := NewController(dir)
	defer close(c.stopCh)
	state.Write(dir, state.Status{PID: 12345, Updated: time.Now()})
	if err := c.CleanupStale(); err == nil {
		t.Fatal("cleanup accepted while the independent engine is active")
	}
	if current, ok := state.Read(dir); !ok || current.PID != 12345 {
		t.Fatal("cleanup removed the independent engine's status")
	}
}

func TestHasTunnelUplinkRequiresEnabledAddressFamily(t *testing.T) {
	ipv6 := nics.Nic{IP6: net.ParseIP("2001:db8::1")}
	ipv4 := nics.Nic{IP: net.ParseIP("192.0.2.1")}
	if hasTunnelUplink(nil, true) || hasTunnelUplink([]nics.Nic{ipv6}, false) ||
		!hasTunnelUplink([]nics.Nic{ipv6}, true) || !hasTunnelUplink([]nics.Nic{ipv4}, false) {
		t.Fatal("tunnel startup accepted the wrong uplink set")
	}
}

func TestNetMergeController(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "netmerge_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ctrl := NewController(tempDir)
	st, err := ctrl.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if st.Config.SocksAddress != "127.0.0.1:1080" {
		t.Errorf("expected default socks address 127.0.0.1:1080, got %s", st.Config.SocksAddress)
	}

	// Test limit setting
	if err := ctrl.SetLimit("eth0", 25.5); err != nil {
		t.Fatalf("SetLimit failed: %v", err)
	}

	// Verify status sees the limit
	st, _ = ctrl.GetStatus()
	if st.Limits["eth0"] != 25.5 {
		t.Errorf("expected eth0 limit 25.5, got %v", st.Limits["eth0"])
	}

	// Test config save
	cfg := ctrl.GetConfig()
	cfg.AutoPause = false
	cfg.MTU = 9000
	cfg.IPv6Mode = "off"
	if err := ctrl.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	ctrl2 := NewController(tempDir)
	cfg2 := ctrl2.GetConfig()
	if cfg2.AutoPause != false || cfg2.MTU != 9000 || cfg2.IPv6Mode != "off" {
		t.Errorf("persisted config did not match: %+v", cfg2)
	}

	// Keep external status until the daemon has processed the stop marker.
	state.Write(tempDir, state.Status{PID: 84321, Updated: time.Now(), Version: "net-merge 0.3.6", EngineSource: "release"})
	st, _ = ctrl.GetStatus()
	if !st.IsRunning || st.RunningVersion != "net-merge 0.3.6" || st.Binary.EngineSource != "builtin" {
		t.Fatalf("running version must come from the live engine, not settings: %+v", st)
	}
	if _, err := ctrl.SetEngineSource("builtin", ""); err == nil {
		t.Fatal("engine switching must be blocked while the daemon is active")
	}
	if err := ctrl.SetSystemWide(false); err != nil {
		t.Fatalf("SetSystemWide(false) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "ctl", "off")); err != nil {
		t.Errorf("expected 'off' marker file, got: %v", err)
	}
	if _, ok := state.Read(tempDir); !ok {
		t.Fatal("GUI removed the running daemon's status")
	}
	state.Clear(tempDir)

	// SetSystemWide(true) must NOT fake success when no daemon/admin is available.
	// Use an external config pointing at a nonexistent binary so the failure is fast
	// and deterministic (no 12s daemon spawn).
	extCtrl := NewController(tempDir)
	missing := filepath.Join(tempDir, "does-not-exist-net-merge")
	if _, err := extCtrl.SetCustomBinaryPath(missing); err == nil {
		t.Fatal("must not select a nonexistent engine")
	}
	// Reproduce a previously selected executable being removed from disk.
	extCtrl.config.EngineSource, extCtrl.config.BinaryPath = "external", missing
	if err := extCtrl.SetSystemWide(true); err == nil {
		t.Errorf("expected SetSystemWide(true) to return an error for a missing binary, got nil")
	}
	// Ensure the failed attempt left no fake ctl/on marker.
	if _, err := os.Stat(filepath.Join(tempDir, "ctl", "on")); err == nil {
		t.Errorf("expected no 'on' marker after failed SetSystemWide(true), but marker file exists")
	}
	// The controller must record the external config for later checks
	// without having faked tunOn.
	if st, _ := extCtrl.GetStatus(); st.Config.EngineSource != "external" {
		t.Errorf("expected external engine source, got %v", st.Config.EngineSource)
	}

	// Turning off again after a failed start must still succeed.
	if err := extCtrl.SetSystemWide(false); err != nil {
		t.Fatalf("SetSystemWide(false) after failed start failed: %v", err)
	}
}

func TestRoutingConfigDoesNotNeedRuntimeDirectory(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "not-yet-created-var-run-directory")
	settingsDir := filepath.Join(root, "Application Support", "NetPilot")
	c := newController(runtimeDir, settingsDir)
	defer close(c.stopCh)
	if err := c.SetRoutingMode("backup", "primary", "secondary"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runtimeDir); !os.IsNotExist(err) {
		t.Fatalf("GUI must not write to runtime directory: %v", err)
	}
	reopened := newController(runtimeDir, settingsDir)
	defer close(reopened.stopCh)
	if cfg := reopened.GetConfig(); cfg.RoutingMode != "backup" || cfg.PrimaryAdapter != "primary" || cfg.SecondaryAdapter != "secondary" {
		t.Fatalf("routing preference did not survive restart: %+v", cfg)
	}
}

func TestUpdateSchedulerFromDiscoveryDistinguishesEmptyScanFromError(t *testing.T) {
	adapter := nics.Nic{Name: "Ethernet", Index: 4, IP: net.ParseIP("192.0.2.10"), Weight: 1}
	sched := engine.NewScheduler([]nics.Nic{adapter})
	if err := updateSchedulerFromDiscovery(sched, nil, errors.New("temporary enumeration failure")); err == nil {
		t.Fatal("discovery error should be returned")
	}
	if got := sched.Snapshot(); len(got) != 1 || !got[0].Present {
		t.Fatalf("failed discovery must preserve current adapter: %+v", got)
	}
	if err := updateSchedulerFromDiscovery(sched, nil, nil); err != nil {
		t.Fatalf("successful empty discovery: %v", err)
	}
	if got := sched.Snapshot(); len(got) != 0 {
		t.Fatalf("successful empty scan should remove idle adapters: %+v", got)
	}
	if sched.Acquire(nil) != nil {
		t.Fatal("a vanished adapter must not receive a new flow")
	}
	if err := updateSchedulerFromDiscovery(sched, []nics.Nic{adapter}, nil); err != nil {
		t.Fatalf("rediscovery: %v", err)
	}
	if got := sched.Snapshot(); len(got) != 1 || !got[0].Present {
		t.Fatalf("adapter should be re-added after a later non-empty scan: %+v", got)
	}
}

type routeProbePolicy struct{ flow engine.FlowPolicy }

func (p routeProbePolicy) Resolve(string, string, string) engine.FlowPolicy { return p.flow }
func (p routeProbePolicy) ReportUsage(engine.FlowPolicy, int64, int64)      {}

func TestRuleRouteProbeReportsPinnedSocketAndFallback(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	target := listener.Addr().(*net.TCPAddr)
	scheduler := engine.NewScheduler([]nics.Nic{
		{Name: "primary", IP: target.IP, Weight: 1},
		{Name: "backup", IP: target.IP, Weight: 1},
	})
	balancer := engine.NewBalancer(scheduler)
	ctrl := Controller{balancer: balancer, socksRunning: true}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for _, test := range []struct{ preferred, fallback, want string }{
		{"primary", "", "primary"},
		{"missing", "backup", "backup"},
	} {
		balancer.SetPolicyResolver(routeProbePolicy{engine.FlowPolicy{
			RuleID: "fixture", PreferredNIC: test.preferred, FallbackNIC: test.fallback,
		}})
		result, err := ctrl.TestRuleRoute(ctx, "example", "", target.IP.String(), target.Port)
		if err != nil || result.RuleID != "fixture" || result.Adapter != test.want ||
			result.SourceIP != target.IP.String() || result.Peer != target.String() {
			t.Fatalf("live probe for %+v returned %+v, %v", test, result, err)
		}
		if tcp, ok := listener.(*net.TCPListener); ok {
			_ = tcp.SetDeadline(time.Now().Add(time.Second))
		}
		conn, err := listener.Accept()
		if err != nil {
			t.Fatalf("probe did not open an actual connection: %v", err)
		}
		_ = conn.Close()
	}

	balancer.SetPolicyResolver(routeProbePolicy{engine.FlowPolicy{RuleID: "fixture", PreferredNIC: "missing"}})
	if _, err := ctrl.TestRuleRoute(ctx, "example", "", target.IP.String(), target.Port); err == nil {
		t.Fatal("probe silently escaped a missing strict pin")
	}
	ctrl.socksRunning = false
	if _, err := ctrl.TestRuleRoute(ctx, "example", "", target.IP.String(), target.Port); err == nil {
		t.Fatal("probe succeeded with the local forwarder stopped")
	}
}
