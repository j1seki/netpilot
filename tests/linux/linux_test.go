//go:build linux

package linux_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"netpilot/internal/analytics"
	"netpilot/internal/apps"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/mode"
)

// Classification: Unit
func TestLinuxCapabilityDetection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test suite; skipping on non-Linux")
	}

	detector := capability.NewDetector()
	rep := detector.Detect()

	if rep.Environment.RuntimeOS != "linux" {
		t.Fatalf("expected runtime OS linux, got %s", rep.Environment.RuntimeOS)
	}

	// Verify detection of OrbStack kernel
	if rep.Environment.IsVM {
		t.Logf("Detected virtualization: %s (Host: %s)", rep.Environment.VMType, rep.Environment.InferredHostOS)
	}
}

// Classification: Unit
func TestLinuxInterfaceSysfsInspection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test suite; skipping on non-Linux")
	}

	links, err := netlink.LinkList()
	if err != nil {
		t.Fatalf("netlink LinkList failed: %v", err)
	}

	if len(links) == 0 {
		t.Fatal("expected at least 1 network link (e.g. lo)")
	}

	for _, l := range links {
		name := l.Attrs().Name
		sys := filepath.Join("/sys/class/net", name)
		_, devErr := os.Stat(filepath.Join(sys, "device"))
		hasHwBacking := devErr == nil
		t.Logf("Link %s: hardware device backing = %v", name, hasHwBacking)
	}
}

// Classification: Unit
func TestLiveLinuxApplicationsDiscovery(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	mgr := apps.NewManager()
	procs := mgr.Discover()
	if len(procs) == 0 {
		t.Fatal("expected real running Linux processes from /proc, got 0")
	}

	t.Logf("Discovered %d real running Linux processes", len(procs))
	foundLive := false
	for _, p := range procs {
		if p.PID > 1 && p.Name != "" && p.CommandLine != "" {
			foundLive = true
			t.Logf("  Live Process: PID=%d Name=%s Sockets=%d Exe=%s", p.PID, p.Name, p.SocketCount, p.Executable)
			break
		}
	}
	if !foundLive {
		t.Fatal("failed to find valid live Linux process")
	}
}

// Classification: Unit
func TestLiveLinuxNetDevCounters(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	memDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer memDB.Close()

	col := analytics.NewCollector(memDB)
	err = col.SampleLinuxNetDev()
	if err != nil {
		t.Fatalf("SampleLinuxNetDev failed: %v", err)
	}

	nics, _ := col.Snapshot()
	if len(nics) == 0 {
		t.Fatal("expected network interfaces from /proc/net/dev, got 0")
	}

	foundEth0 := false
	for _, n := range nics {
		t.Logf("  Live Interface: %s | BytesIn=%d | BytesOut=%d", n.Name, n.BytesIn, n.BytesOut)
		if n.Name == "eth0" {
			foundEth0 = true
			if n.BytesIn == 0 {
				t.Errorf("expected non-zero BytesIn on live eth0")
			}
		}
	}
	if !foundEth0 {
		t.Fatal("eth0 not found in /proc/net/dev")
	}
}

// Classification: Unit
func TestLiveLinuxModeController(t *testing.T) {
	detector := capability.NewDetector()
	rep := detector.Detect()
	ctrl := mode.NewController(rep)

	if ctrl.Current() != mode.ModeLive {
		t.Fatalf("expected Live mode, got %s", ctrl.Current())
	}
	st := ctrl.GetState()
	if !st.AllowsSystemMod {
		t.Fatalf("expected AllowsSystemMod=true in Live mode")
	}
	t.Logf("Live Mode Active: %s (AllowsSystemMod=%v)", st.CurrentMode, st.AllowsSystemMod)
}

// Classification: Integration
func TestLinuxIsolatedNetworkNamespace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test suite; skipping on non-Linux")
	}

	if os.Geteuid() != 0 {
		t.Skip("Privileged network namespace test requires root / CAP_SYS_ADMIN; marking as SKIPPED in unprivileged test pass")
	}

	origNS, err := os.Open("/proc/self/ns/net")
	if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
		t.Logf("Unshare CLONE_NEWNET blocked by container policy: %v", err)
		return
	}

	// In new netns, only loopback should exist initially
	newLinks, err := netlink.LinkList()
	if err != nil {
		t.Fatalf("LinkList in new netns: %v", err)
	}
	if len(newLinks) != 1 || newLinks[0].Attrs().Name != "lo" {
		t.Fatalf("expected only loopback in fresh namespace, found %d links", len(newLinks))
	}

	// Restore original namespace
	_ = unix.Setns(int(origNS.Fd()), unix.CLONE_NEWNET)
}
