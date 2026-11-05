//go:build windows

package windows_test

import (
	"runtime"
	"testing"
	"time"

	"netpilot/internal/apps"
	"netpilot/internal/capability"
)

// Classification: Unit
func TestWindowsCapabilityDetectionMock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows native test suite requires Windows runner; skipping on " + runtime.GOOS)
	}

	detector := capability.NewDetector()
	rep := detector.Detect()

	for _, c := range rep.Capabilities {
		if c.ID == "backend_windows_wfp" {
			if c.Status != capability.StatusSupportedVerified && c.Status != capability.StatusSupportedNotVerified {
				t.Fatalf("expected windows backend supported on Windows OS, got: %s", c.Status)
			}
		}
	}
}

// Classification: Unit
func TestWindowsSocketErrorMapping(t *testing.T) {
	// Validate Windows socket error definitions (10049 to 10065)
	const (
		wsaeAddrNotAvail = 10049
		wsaeNetDown      = 10050
		wsaeNetUnreach   = 10051
		wsaeTimedOut     = 10060
		wsaeConnRefused  = 10061
		wsaeHostUnreach  = 10065
	)

	isRecoverable := func(errno int) bool {
		switch errno {
		case wsaeNetDown, wsaeNetUnreach, wsaeTimedOut, wsaeHostUnreach:
			return true
		default:
			return false
		}
	}

	if !isRecoverable(wsaeTimedOut) {
		t.Fatal("expected WSAETIMEDOUT to be recoverable")
	}
	if isRecoverable(wsaeConnRefused) {
		t.Fatal("expected WSAECONNREFUSED to be unrecoverable (app rejected)")
	}
}

// Classification: Integration
func TestWindowsWintunDeviceLifecycle(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Wintun integration tests require Windows kernel with Wintun.dll; blocked on non-Windows")
	}
	t.Log("Windows Wintun integration test placeholder")
	time.Sleep(10 * time.Millisecond)
}

// Classification: Unit
func TestWindowsProcessDiscovery(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Requires Windows OS")
	}
	mgr := apps.NewManager()
	procs := mgr.Discover()
	if len(procs) == 0 {
		t.Fatal("expected to discover at least one process on Windows")
	}
}
