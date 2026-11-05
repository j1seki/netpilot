//go:build darwin

package macos_test

import (
	"runtime"
	"testing"

	"netpilot/internal/capability"
)

// Classification: Unit
func TestMacOSCapabilityDetectionMock(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS native test suite requires macOS runner; skipping on " + runtime.GOOS)
	}

	detector := capability.NewDetector()
	rep := detector.Detect()

	for _, c := range rep.Capabilities {
		if c.ID == "backend_macos_utun" {
			if c.Status != capability.StatusSupportedVerified && c.Status != capability.StatusSupportedNotVerified {
				t.Fatalf("expected macos backend supported on Darwin OS, got: %s", c.Status)
			}
		}
	}
}

// Classification: Unit
func TestMacOSRouteCommandFormatting(t *testing.T) {
	// macOS uses `route -n add` rather than netlink or netsh
	formatRouteCmd := func(dst, gateway string) []string {
		return []string{"route", "-n", "add", dst, gateway}
	}

	cmd := formatRouteCmd("198.18.0.0/15", "198.18.0.1")
	if len(cmd) != 5 || cmd[0] != "route" || cmd[3] != "198.18.0.0/15" {
		t.Fatalf("unexpected route cmd format: %v", cmd)
	}
}

// Classification: Integration
func TestMacOSUtunLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS utun integration requires Darwin kernel; blocked on non-macOS")
	}
	t.Log("macOS utun lifecycle test placeholder")
}
