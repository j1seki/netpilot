package testrunner

import (
	"net"
	"runtime"
	"testing"

	"netpilot/internal/nics"
)

func TestRoutingChecksUseSocketsAndRecover(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fixture needs Linux's 127.0.0.0/8 loopback addresses")
	}
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	plan := routingTarget{
		host: "127.0.0.1", port: target.Addr().(*net.TCPAddr).Port,
		links: []nics.Nic{
			{Name: "test-primary", IP: net.IPv4(127, 0, 0, 2), Weight: 1},
			{Name: "test-backup", IP: net.IPv4(127, 0, 0, 3), Weight: 1},
		},
	}
	if status, detail := checkSpeed(plan); status != StatusPass {
		t.Fatalf("speed: %s %s", status, detail)
	}
	if status, detail := checkLiveFailover(plan, "test-backup"); status != StatusPass {
		t.Fatalf("failover: %s %s", status, detail)
	}
}

func TestDisruptionFilter(t *testing.T) {
	tests := []inProcTest{{name: "disruption", affectsConnection: true, fn: func() (TestStatus, string) {
		return StatusPass, "executed"
	}}}
	if result := runCaseListFiltered(tests, false)[0]; result.Status != StatusPass || result.Output != "executed" || !result.AffectsConnection {
		t.Fatalf("full run: %+v", result)
	}
	if result := runCaseListFiltered(tests, true)[0]; result.Status != StatusSkipped || !result.AffectsConnection {
		t.Fatalf("safe run: %+v", result)
	}
}
