package netmerge

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"netpilot/internal/engine"
	"netpilot/internal/nics"
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

	// SetSystemWide(false) always succeeds (clears state + writes ctl/off marker).
	if err := ctrl.SetSystemWide(false); err != nil {
		t.Fatalf("SetSystemWide(false) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "ctl", "off")); err != nil {
		t.Errorf("expected 'off' marker file, got: %v", err)
	}

	// SetSystemWide(true) must NOT fake success when no daemon/admin is available.
	// Use an external config pointing at a nonexistent binary so the failure is fast
	// and deterministic (no 12s daemon spawn).
	extCtrl := NewController(tempDir)
	extCfg := extCtrl.GetConfig()
	extCfg.EngineSource = "external"
	extCfg.BinaryPath = filepath.Join(tempDir, "does-not-exist-net-merge")
	if err := extCtrl.SaveConfig(extCfg); err != nil {
		t.Fatalf("SaveConfig(external) failed: %v", err)
	}
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
