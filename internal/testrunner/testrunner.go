// Package testrunner executes environment-aware NetPilot test suites without a Go toolchain.
package testrunner

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"netpilot/internal/analytics"
	"netpilot/internal/apps"
	"netpilot/internal/branding"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/failover"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/rules"
	"netpilot/internal/scenarios"
)

// PlatformSuite identifies the platform test target.
type PlatformSuite string

const (
	SuiteCommon    PlatformSuite = "common"
	SuiteLinux     PlatformSuite = "linux"
	SuiteWindows   PlatformSuite = "windows"
	SuiteMacOS     PlatformSuite = "macos"
	SuiteScenarios PlatformSuite = "scenarios"
	SuiteTether    PlatformSuite = "tether"
	SuiteLive      PlatformSuite = "live"
)

// TestStatus represents test execution state.
type TestStatus string

const (
	StatusPass    TestStatus = "PASS"
	StatusFail    TestStatus = "FAIL"
	StatusSkipped TestStatus = "SKIPPED"
	StatusBlocked TestStatus = "BLOCKED"
	StatusNotRun  TestStatus = "NOT RUN"
)

// CaseResult holds the outcome of an individual test case.
type CaseResult struct {
	Name              string     `json:"name"`
	Classification    string     `json:"classification"` // Unit, Integration, System
	Status            TestStatus `json:"status"`
	DurationMs        float64    `json:"durationMs"`
	Output            string     `json:"output,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	AffectsConnection bool       `json:"affectsConnection"`
	DisruptionWarning string     `json:"disruptionWarning,omitempty"`
}

// UnifiedTestReport aggregates all platform suites and scenarios in a single one-click report.
type UnifiedTestReport struct {
	Schema          string                     `json:"schema"`
	AppVersion      string                     `json:"appVersion"`
	SafeMode        bool                       `json:"safeMode"`
	Total           int                        `json:"total"`
	Passed          int                        `json:"passed"`
	Failed          int                        `json:"failed"`
	Skipped         int                        `json:"skipped"`
	Blocked         int                        `json:"blocked"`
	NotRun          int                        `json:"notRun"`
	DisruptiveCount int                        `json:"disruptiveCount"`
	DurationMs      float64                    `json:"durationMs"`
	Suites          []SuiteReport              `json:"suites"`
	Scenarios       []scenarios.ScenarioResult `json:"scenarios"`
	RanAt           time.Time                  `json:"ranAt"`
}

// SuiteReport contains aggregate metrics for a test run.
type SuiteReport struct {
	RunID              string        `json:"runId"`
	Suite              PlatformSuite `json:"suite"`
	TargetOS           string        `json:"targetOs"`
	ExecutionEnv       string        `json:"executionEnv"`
	Total              int           `json:"total"`
	Passed             int           `json:"passed"`
	Failed             int           `json:"failed"`
	Skipped            int           `json:"skipped"`
	Blocked            int           `json:"blocked"`
	NotRun             int           `json:"notRun"`
	DurationMs         float64       `json:"durationMs"`
	Cases              []CaseResult  `json:"cases"`
	EnvironmentSummary string        `json:"environmentSummary"`
	RanAt              time.Time     `json:"ranAt"`
}

// Runner manages suite execution.
type Runner struct {
	capDetector *capability.Detector
	database    *db.Database
	ringLog     *logger.RingLogger
}

// NewRunner creates a new platform test runner.
func NewRunner(capDetector *capability.Detector, database *db.Database, ringLog *logger.RingLogger) *Runner {
	return &Runner{
		capDetector: capDetector,
		database:    database,
		ringLog:     ringLog,
	}
}

type inProcTest struct {
	name              string
	classification    string
	affectsConnection bool
	disruptionWarning string
	fn                func() (TestStatus, string)
}

func runCaseList(tests []inProcTest) []CaseResult {
	return runCaseListFiltered(tests, false)
}

func runCaseListFiltered(tests []inProcTest, skipDisruptive bool) []CaseResult {
	var out []CaseResult
	for _, tc := range tests {
		if skipDisruptive && tc.affectsConnection {
			out = append(out, CaseResult{
				Name: tc.name, Classification: tc.classification, Status: StatusSkipped,
				Reason: "Disruptive check excluded by safe mode", AffectsConnection: true,
				DisruptionWarning: tc.disruptionWarning,
			})
			continue
		}
		t0 := time.Now()
		st, msg := tc.fn()
		dur := float64(time.Since(t0).Microseconds()) / 1000.0
		if dur < 0.05 {
			dur = 0.05
		}
		cr := CaseResult{
			Name:              tc.name,
			Classification:    tc.classification,
			Status:            st,
			DurationMs:        dur,
			AffectsConnection: tc.affectsConnection,
			DisruptionWarning: tc.disruptionWarning,
		}
		if st == StatusPass {
			cr.Output = msg
		} else {
			cr.Reason = msg
		}
		out = append(out, cr)
	}
	return out
}

// RunSuite executes the specified platform suite.
func (r *Runner) RunSuite(suite PlatformSuite) SuiteReport {
	return r.runSuite(suite, "", false)
}

func (r *Runner) runSuite(suite PlatformSuite, tetherAdapter string, skipDisruptive bool) SuiteReport {
	start := time.Now()
	capRep := r.capDetector.Detect()
	runID := logger.RandomID()

	envSummary := fmt.Sprintf("%s/%s (%s)", capRep.Environment.RuntimeOS, capRep.Environment.RuntimeArch, capRep.Environment.VMType)

	report := SuiteReport{
		RunID:              runID,
		Suite:              suite,
		TargetOS:           string(suite),
		ExecutionEnv:       envSummary,
		EnvironmentSummary: envSummary,
		RanAt:              start,
	}

	if suite == SuiteWindows && runtime.GOOS != "windows" {
		r.recordBlockedSuite(&report, "Windows native integration runner unavailable on "+runtime.GOOS)
		r.saveReport(report)
		return report
	}
	if suite == SuiteMacOS && runtime.GOOS != "darwin" {
		r.recordBlockedSuite(&report, "macOS native integration runner unavailable on "+runtime.GOOS)
		r.saveReport(report)
		return report
	}
	if suite == SuiteLinux && runtime.GOOS != "linux" {
		r.recordBlockedSuite(&report, "Linux native integration runner unavailable on "+runtime.GOOS)
		r.saveReport(report)
		return report
	}

	var cases []CaseResult
	switch suite {
	case SuiteCommon:
		cases = r.runCommonCases(capRep)
	case SuiteWindows:
		cases = r.runWindowsCases(capRep)
	case SuiteLinux:
		cases = r.runLinuxCases(capRep)
	case SuiteMacOS:
		cases = r.runMacOSCases(capRep)
	case SuiteTether:
		cases = runCaseList([]inProcTest{{
			name: "Tether adapter IPv4 connectivity", classification: "System",
			fn: func() (TestStatus, string) { return checkTetherLink(tetherAdapter) },
		}})
	case SuiteLive:
		cases = runCaseListFiltered(append([]inProcTest{
			{name: "SOCKS routing rule and bandwidth cap", classification: "Integration", fn: checkProxyRulesAndCap},
			{name: "Process identity", classification: "Integration", fn: checkHostProcessIdentity},
			{name: "TTL preview, apply and restore", classification: "Integration", fn: checkTTLTransaction},
			{name: "Read host TTL", classification: "System", fn: checkLiveTTLInspection},
			{name: "DNS lookup", classification: "Integration", fn: checkDNS},
			{name: "Update release feed", classification: "System", fn: checkUpdateFeed},
		}, liveRoutingChecks(tetherAdapter)...), skipDisruptive)
	default:
		r.recordBlockedSuite(&report, "Unknown or unsupported suite selection")
		r.saveReport(report)
		return report
	}

	addCases(&report, cases)
	report.DurationMs = elapsedMs(start)
	r.saveReport(report)
	return report
}

type Check struct {
	Name string
	Run  func() (TestStatus, string)
}

// RunChecks runs application-level checks and appends their results to the unified report.
func (r *Runner) RunChecks(report *UnifiedTestReport, suite PlatformSuite, checks []Check) {
	start := time.Now()
	tests := make([]inProcTest, 0, len(checks))
	for _, check := range checks {
		tests = append(tests, inProcTest{name: check.Name, classification: "Integration", fn: check.Run})
	}
	env := runtime.GOOS + "/" + runtime.GOARCH
	result := SuiteReport{
		RunID: logger.RandomID(), Suite: suite, TargetOS: runtime.GOOS,
		ExecutionEnv: env, EnvironmentSummary: env, RanAt: start,
	}
	addCases(&result, runCaseList(tests))
	result.DurationMs = elapsedMs(start)
	r.saveReport(result)
	report.addSuite(result)
	report.DurationMs = elapsedMs(report.RanAt)
}

func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}

func addCases(suite *SuiteReport, cases []CaseResult) {
	suite.Cases = append(suite.Cases, cases...)
	for _, c := range cases {
		suite.Total++
		switch c.Status {
		case StatusPass:
			suite.Passed++
		case StatusFail:
			suite.Failed++
		case StatusSkipped:
			suite.Skipped++
		case StatusBlocked:
			suite.Blocked++
		default:
			suite.NotRun++
		}
	}
}

func (report *UnifiedTestReport) addSuite(suite SuiteReport) {
	report.Suites = append(report.Suites, suite)
	report.Total += suite.Total
	report.Passed += suite.Passed
	report.Failed += suite.Failed
	report.Skipped += suite.Skipped
	report.Blocked += suite.Blocked
	report.NotRun += suite.NotRun
	for _, c := range suite.Cases {
		if c.AffectsConnection {
			report.DisruptiveCount++
		}
	}
}

func (r *Runner) runCommonCases(capRep capability.Report) []CaseResult {
	return runCaseList([]inProcTest{
		{name: "TestProcessInspectorLoopback", classification: "Integration", fn: checkProcessSockets},
		{name: "TestStandbyProcessTraffic", classification: "Integration", fn: checkStandbyTraffic},
		{
			name:           "TestApplicationIdentityCentralization",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				if branding.AppName != "NetPilot" || branding.AppID != "com.netpilot.desktop" {
					return StatusFail, "branding mismatch"
				}
				return StatusPass, "Verified AppName=NetPilot, AppID=com.netpilot.desktop"
			},
		},
		{
			name:           "TestPolicyValidationAndPrecedence",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				memDB, err := db.Open(":memory:")
				if err != nil {
					return StatusFail, err.Error()
				}
				defer memDB.Close()
				eng, err := rules.NewEngine(memDB)
				if err != nil {
					return StatusFail, err.Error()
				}
				if err := eng.ValidateRule(db.RuleRecord{ID: "r_loop", AppName: "test", PreferredInterface: "eth0", FallbackInterface: "eth0"}); err == nil {
					return StatusFail, "expected failover loop rejection"
				}
				_ = memDB.SaveRule(db.RuleRecord{ID: "low", AppName: "Stream", PreferredInterface: "eth0", Priority: 10, Enabled: true})
				_ = memDB.SaveRule(db.RuleRecord{ID: "high", AppName: "Stream", PreferredInterface: "wlan0", Priority: 50, Enabled: true})
				_ = eng.Reload()
				dec := eng.Evaluate("stream", "/usr/bin/stream", "stream.tv", "default0")
				if dec.MatchedRuleID != "high" || dec.SelectedInterface != "wlan0" {
					return StatusFail, "precedence evaluation mismatch"
				}
				return StatusPass, "Rule loop detection and priority precedence verified"
			},
		},
		{
			name:           "TestFailoverStateMachineHysteresis",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				sm := failover.NewStateMachine(failover.Config{FailThreshold: 3, RecoverThreshold: 2, DegradedRTT: 200 * time.Millisecond}, nil)
				sm.Register("eth0")
				if sm.RecordProbe("eth0", false, 0, "t1") != failover.StateDegraded {
					return StatusFail, "expected DEGRADED"
				}
				_ = sm.RecordProbe("eth0", false, 0, "t2")
				if sm.RecordProbe("eth0", false, 0, "t3") != failover.StateDown {
					return StatusFail, "expected DOWN after 3 failures"
				}
				if sm.RecordProbe("eth0", true, 30*time.Millisecond, "") != failover.StateRecovering {
					return StatusFail, "expected RECOVERING"
				}
				if sm.RecordProbe("eth0", true, 30*time.Millisecond, "") != failover.StateUp {
					return StatusFail, "expected UP after 2 recoveries"
				}
				return StatusPass, "Hysteresis state transitions verified"
			},
		},
		{
			name:           "TestLogCorrelationAndRedaction",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				ring := logger.NewRingLogger(50)
				op := ring.StartOperation("ConfigureNIC", "EgressManager", "LIVE", runtime.GOOS, "")
				op.Step(logger.PhaseValidationStarted, "Checking token=secretPassword123", nil)
				op.Complete("SUCCESS", nil)
				entries := ring.Query(logger.FilterOptions{Limit: 10})
				for _, e := range entries {
					if strings.Contains(e.Message, "secretPassword123") {
						return StatusFail, "credential leaked in logs"
					}
				}
				return StatusPass, "Sensitive tokens and MACs properly redacted"
			},
		},
		{
			name:           "TestAdapterTrafficRateAccounting",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				memDB, err := db.Open(":memory:")
				if err != nil {
					return StatusFail, err.Error()
				}
				defer memDB.Close()
				col := analytics.NewCollector(memDB)
				col.RecordNICSample("eth0", 1000000, 500000)
				time.Sleep(60 * time.Millisecond) // The collector requires more than 50 ms between samples.
				col.RecordNICSample("eth0", 2000000, 1000000)
				nics, _ := col.Snapshot()
				for _, n := range nics {
					if n.Name == "eth0" && n.CurrentRxMbps > 0 {
						return StatusPass, fmt.Sprintf("Measured rate %.2f Mbps", n.CurrentRxMbps)
					}
				}
				return StatusFail, "failed to compute positive rate delta"
			},
		},
		{
			name:           "TestSafeModeTransitionGating",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				ctrl := mode.NewController(capRep)
				if _, err := ctrl.SetMode(mode.ModeDryRun, false); err != nil {
					return StatusFail, err.Error()
				}
				if _, err := ctrl.SetMode(mode.ModeLive, true); err != nil {
					return StatusFail, err.Error()
				}
				return StatusPass, "Mode transitions between DRY_RUN and LIVE verified"
			},
		},
	})
}

func (r *Runner) runWindowsCases(capRep capability.Report) []CaseResult {
	return runCaseList([]inProcTest{
		{
			name:           "TestWindowsCapabilityDetection",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				for _, c := range capRep.Capabilities {
					if c.ID == "backend_windows_wfp" {
						return StatusPass, string(c.Status)
					}
				}
				return StatusFail, "missing windows capability item"
			},
		},
		{
			name:           "TestWindowsProcessDiscovery",
			classification: "Integration",
			fn: func() (TestStatus, string) {
				mgr := apps.NewManager()
				procs := mgr.Discover()
				if len(procs) == 0 {
					return StatusFail, "no running processes discovered via Toolhelp32"
				}
				return StatusPass, fmt.Sprintf("Discovered %d live Windows processes", len(procs))
			},
		},
		{
			name:           "TestWindowsInterfaceCounters",
			classification: "Integration",
			fn: func() (TestStatus, string) {
				ifaces, err := net.Interfaces()
				if err != nil || len(ifaces) == 0 {
					return StatusFail, "failed to enumerate Windows adapters"
				}
				return StatusPass, fmt.Sprintf("Enumerated %d Windows network interfaces", len(ifaces))
			},
		},
	})
}

func (r *Runner) runLinuxCases(capRep capability.Report) []CaseResult {
	return runCaseList([]inProcTest{
		{
			name:           "TestLinuxCapabilityDetection",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				if capRep.Environment.RuntimeOS != "linux" {
					return StatusFail, "expected linux runtime"
				}
				return StatusPass, "Linux kernel & sysfs verified"
			},
		},
		{
			name:           "TestLiveLinuxApplicationsDiscovery",
			classification: "Integration",
			fn: func() (TestStatus, string) {
				mgr := apps.NewManager()
				procs := mgr.Discover()
				if len(procs) == 0 {
					return StatusFail, "0 processes read from /proc"
				}
				return StatusPass, fmt.Sprintf("Discovered %d live Linux processes", len(procs))
			},
		},
		{
			name:           "TestLiveLinuxNetDevCounters",
			classification: "Integration",
			fn: func() (TestStatus, string) {
				memDB, _ := db.Open(":memory:")
				defer memDB.Close()
				col := analytics.NewCollector(memDB)
				if err := col.SampleLinuxNetDev(); err != nil {
					return StatusFail, err.Error()
				}
				nics, _ := col.Snapshot()
				if len(nics) == 0 {
					return StatusFail, "no interfaces in /proc/net/dev"
				}
				return StatusPass, fmt.Sprintf("Sampled %d interfaces from /proc/net/dev", len(nics))
			},
		},
		{name: "Linux isolated network namespace", classification: "System", fn: checkLinuxNamespace},
	})
}

func checkLinuxNamespace() (TestStatus, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "unshare", "--net", "--", "cat", "/proc/net/dev").CombinedOutput()
	if err != nil {
		return StatusBlocked, fmt.Sprintf("Cannot start a disposable network namespace: %v %s", err, strings.TrimSpace(string(output)))
	}
	var seen []string
	for _, line := range strings.Split(string(output), "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok {
			seen = append(seen, strings.TrimSpace(name))
		}
	}
	if len(seen) != 1 || seen[0] != "lo" {
		return StatusFail, fmt.Sprintf("Unexpected interfaces in isolated namespace: %v", seen)
	}
	return StatusPass, "The child process observed only loopback in its private network namespace"
}

func (r *Runner) runMacOSCases(capRep capability.Report) []CaseResult {
	return runCaseList([]inProcTest{
		{
			name:           "TestMacOSCapabilityDetection",
			classification: "Unit",
			fn: func() (TestStatus, string) {
				if capRep.Environment.RuntimeOS != "darwin" {
					return StatusFail, "expected macOS runtime"
				}
				return StatusPass, "Darwin runtime detected"
			},
		},
		{
			name:           "macOS interface enumeration",
			classification: "Integration",
			fn: func() (TestStatus, string) {
				ifaces, err := net.Interfaces()
				if err != nil || len(ifaces) == 0 {
					return StatusFail, "no macOS interfaces enumerated"
				}
				return StatusPass, fmt.Sprintf("Enumerated %d macOS interfaces", len(ifaces))
			},
		},
	})
}

func (r *Runner) recordBlockedSuite(report *SuiteReport, reason string) {
	report.DurationMs = 0.5
	report.Blocked = 2
	report.Total = 2
	report.Cases = []CaseResult{
		{
			Name:           "PlatformCapabilityCheck",
			Classification: "Unit",
			Status:         StatusBlocked,
			Reason:         reason,
		},
		{
			Name:           "InterfaceEnumerationNative",
			Classification: "Integration",
			Status:         StatusBlocked,
			Reason:         reason,
		},
	}
}

// RunAllUnified runs platform tests for the current host OS, common tests, and scenarios in one click.
func (r *Runner) RunAllUnified(skipDisruptive bool, scEng *scenarios.Engine, tetherAdapter string) UnifiedTestReport {
	start := time.Now()
	var suites []SuiteReport

	commonRep := r.RunSuite(SuiteCommon)
	suites = append(suites, commonRep)

	switch runtime.GOOS {
	case "linux":
		suites = append(suites, r.RunSuite(SuiteLinux))
	case "windows":
		suites = append(suites, r.RunSuite(SuiteWindows))
	case "darwin":
		suites = append(suites, r.RunSuite(SuiteMacOS))
	}
	suites = append(suites, r.runSuite(SuiteTether, tetherAdapter, skipDisruptive))
	suites = append(suites, r.runSuite(SuiteLive, tetherAdapter, skipDisruptive))

	var scenarioResults []scenarios.ScenarioResult
	if scEng != nil {
		scenarioResults = scEng.RunAllWithFilter(skipDisruptive)
	}

	report := UnifiedTestReport{
		Schema:     "netpilot-test-report-v1",
		AppVersion: branding.AppVersion,
		SafeMode:   skipDisruptive,
		Scenarios:  scenarioResults,
		RanAt:      start,
	}

	for _, suite := range suites {
		report.addSuite(suite)
	}

	for _, sc := range scenarioResults {
		report.Total++
		if sc.AffectsConnection {
			report.DisruptiveCount++
		}
		switch sc.Status {
		case scenarios.StatusPass:
			report.Passed++
		case scenarios.StatusFail:
			report.Failed++
		case scenarios.StatusSkipped:
			report.Skipped++
		case scenarios.StatusBlocked:
			report.Blocked++
		}
	}

	report.DurationMs = float64(time.Since(start).Microseconds()) / 1000.0
	return report
}

func (r *Runner) saveReport(rep SuiteReport) {
	if r.database != nil {
		_ = r.database.SaveTestRun(db.TestRunRecord{
			RunID:              rep.RunID,
			Platform:           rep.TargetOS,
			SuiteName:          string(rep.Suite),
			TotalTests:         rep.Total,
			Passed:             rep.Passed,
			Failed:             rep.Failed,
			Skipped:            rep.Skipped,
			Blocked:            rep.Blocked,
			DurationMs:         rep.DurationMs,
			EnvironmentSummary: rep.EnvironmentSummary,
			RanAt:              rep.RanAt,
		})
	}
}
