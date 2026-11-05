package common_test

import (
	"strings"
	"testing"
	"time"

	"netpilot/internal/analytics"
	"netpilot/internal/branding"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/failover"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/rules"
)

// Classification: Unit
func TestApplicationIdentityCentralization(t *testing.T) {
	if branding.AppName != "NetPilot" {
		t.Fatalf("expected AppName NetPilot, got %s", branding.AppName)
	}
	if branding.AppID != "com.netpilot.desktop" {
		t.Fatalf("expected AppID com.netpilot.desktop, got %s", branding.AppID)
	}
	info := branding.Info()
	if info["name"] != "NetPilot" || info["id"] != "com.netpilot.desktop" {
		t.Fatalf("branding.Info mismatch: %+v", info)
	}
}

// Classification: Unit
func TestPolicyValidationAndPrecedence(t *testing.T) {
	memDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	defer memDB.Close()

	eng, err := rules.NewEngine(memDB)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	// 1. Validation error: empty ID
	err = eng.ValidateRule(db.RuleRecord{ID: ""})
	if err == nil {
		t.Fatal("expected error on empty ID")
	}

	// 2. Validation error: failover loop
	err = eng.ValidateRule(db.RuleRecord{
		ID:                 "r_loop",
		AppName:            "test",
		PreferredInterface: "eth0",
		FallbackInterface:  "eth0",
	})
	if err == nil || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("expected failover loop error, got: %v", err)
	}

	// 3. Save lower and higher priority rules
	_ = memDB.SaveRule(db.RuleRecord{
		ID: "low", AppName: "Stream", PreferredInterface: "eth0", Priority: 10, Enabled: true,
	})
	_ = memDB.SaveRule(db.RuleRecord{
		ID: "high", AppName: "Stream", PreferredInterface: "wlan0", Priority: 50, Enabled: true,
	})
	if err := eng.Reload(); err != nil {
		t.Fatalf("reload rules: %v", err)
	}

	dec := eng.Evaluate("StreamingApp", "/usr/bin/stream", "stream.tv", "default0")
	if dec.MatchedRuleID != "high" {
		t.Fatalf("expected high priority rule matched, got: %s", dec.MatchedRuleID)
	}
	if dec.SelectedInterface != "wlan0" {
		t.Fatalf("expected wlan0, got: %s", dec.SelectedInterface)
	}
}

// Classification: Unit
func TestFailoverStateMachineHysteresis(t *testing.T) {
	sm := failover.NewStateMachine(failover.Config{
		FailThreshold:    3,
		RecoverThreshold: 2,
		DegradedRTT:      200 * time.Millisecond,
	}, nil)

	sm.Register("eth0")

	// Fail 1: degraded
	s1 := sm.RecordProbe("eth0", false, 0, "timeout 1")
	if s1 != failover.StateDegraded {
		t.Fatalf("expected StateDegraded after 1 fail, got %s", s1)
	}

	// Fail 2: still degraded
	_ = sm.RecordProbe("eth0", false, 0, "timeout 2")

	// Fail 3: reaches threshold -> DOWN
	s3 := sm.RecordProbe("eth0", false, 0, "timeout 3")
	if s3 != failover.StateDown {
		t.Fatalf("expected StateDown after 3 fails, got %s", s3)
	}

	// Recover probe 1: moves to RECOVERING, not UP yet
	sr1 := sm.RecordProbe("eth0", true, 50*time.Millisecond, "")
	if sr1 != failover.StateRecovering {
		t.Fatalf("expected StateRecovering after 1 success, got %s", sr1)
	}

	// Recover probe 2: satisfies threshold (2) -> UP
	sr2 := sm.RecordProbe("eth0", true, 50*time.Millisecond, "")
	if sr2 != failover.StateUp {
		t.Fatalf("expected StateUp after 2 successes, got %s", sr2)
	}
}

// Classification: Unit
func TestLogCorrelationAndRedaction(t *testing.T) {
	ring := logger.NewRingLogger(100)

	op := ring.StartOperation("ConfigureNIC", "EgressManager", "SIMULATION", "linux", "")
	op.Step(logger.PhaseValidationStarted, "Checking token=secretPassword123 with bearer eyJhbGciOi...", nil)
	op.Step(logger.PhaseBackendStarted, "Connecting to MAC 00:1A:2B:3C:4D:5E", nil)
	op.Complete("SUCCESS", nil)

	entries := ring.Query(logger.FilterOptions{OperationID: op.OperationID()})
	_ = entries

	all := ring.Query(logger.FilterOptions{Limit: 10})
	foundValidation := false
	for _, e := range all {
		if strings.Contains(e.Message, "Checking") {
			foundValidation = true
			if strings.Contains(e.Message, "secretPassword123") {
				t.Fatalf("secret password was NOT redacted: %s", e.Message)
			}
			if !strings.Contains(e.Message, "[REDACTED_CREDENTIAL]") {
				t.Fatalf("expected [REDACTED_CREDENTIAL] marker: %s", e.Message)
			}
		}
	}
	if !foundValidation {
		t.Fatal("validation log entry not found")
	}
}

// Classification: Unit
func TestRateLimitingAndUsageAccounting(t *testing.T) {
	memDB, _ := db.Open(":memory:")
	defer memDB.Close()

	col := analytics.NewCollector(memDB)
	col.RecordNICSample("eth0", 1000000, 500000)
	time.Sleep(100 * time.Millisecond)
	col.RecordNICSample("eth0", 2000000, 1000000)

	nics, _ := col.Snapshot()
	found := false
	for _, n := range nics {
		if n.Name == "eth0" {
			found = true
			if n.CurrentRxMbps <= 0 {
				t.Fatalf("expected positive Rx Mbps, got: %f", n.CurrentRxMbps)
			}
		}
	}
	if !found {
		t.Fatal("eth0 metric not found")
	}
}

// Classification: Unit
func TestSafeModeTransitionGating(t *testing.T) {
	caps := capability.Report{
		Environment: capability.EnvironmentDetails{
			IsVM:             false,
			VMType:           "Linux Host",
			PhysicalNICCount: 2,
			IsElevated:       true,
			HasSudo:          true,
		},
	}

	ctrl := mode.NewController(caps)
	if ctrl.Current() != mode.ModeLive {
		t.Fatalf("expected default to Live mode on host, got %s", ctrl.Current())
	}

	// 1. Dry-Run transition succeeds
	st, err := ctrl.SetMode(mode.ModeDryRun, false)
	if err != nil {
		t.Fatalf("set dry-run: %v", err)
	}
	if st.CurrentMode != mode.ModeDryRun {
		t.Fatalf("expected DRY_RUN, got %s", st.CurrentMode)
	}

	// 2. Return to Live mode succeeds
	stLive, err := ctrl.SetMode(mode.ModeLive, true)
	if err != nil {
		t.Fatalf("set live: %v", err)
	}
	if stLive.CurrentMode != mode.ModeLive {
		t.Fatalf("expected LIVE, got %s", stLive.CurrentMode)
	}
}
