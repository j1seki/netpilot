package netmerge

import (
	"os"
	"path/filepath"
	"testing"
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

	// Test marker sending
	if err := ctrl.SetSystemWide(true); err != nil {
		t.Fatalf("SetSystemWide(true) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "ctl", "on")); err != nil {
		t.Errorf("expected 'on' marker file, got: %v", err)
	}

	if err := ctrl.SetSystemWide(false); err != nil {
		t.Fatalf("SetSystemWide(false) failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "ctl", "off")); err != nil {
		t.Errorf("expected 'off' marker file, got: %v", err)
	}
}
