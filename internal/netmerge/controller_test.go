package netmerge

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"netpilot/internal/engine"
	"netpilot/internal/nics"
	"netpilot/internal/state"
)

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
