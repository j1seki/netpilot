package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"netpilot/internal/analytics"
	"netpilot/internal/apps"
	"netpilot/internal/branding"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/diagnostics"
	"netpilot/internal/failover"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/netmerge"
	"netpilot/internal/nics"
	"netpilot/internal/rules"
	"netpilot/internal/scenarios"
	"netpilot/internal/testrunner"
	"netpilot/internal/ttlmgr"
)

type cachedWAN struct {
	ip   string
	isp  string
	loc  string
	at   time.Time
}

// NetPilotService is the backend RPC service exposed to the Wails 3 React frontend.
type NetPilotService struct {
	mu           sync.RWMutex
	database     *db.Database
	ringLog      *logger.RingLogger
	capDetector  *capability.Detector
	modeCtrl     *mode.Controller
	ruleEngine   *rules.Engine
	failoverSM   *failover.StateMachine
	appManager   *apps.Manager
	analytics    *analytics.Collector
	scenarios    *scenarios.Engine
	diagnostics  *diagnostics.Generator
	testRunner   *testrunner.Runner
	netmergeCtrl *netmerge.Controller
	ttlManager   *ttlmgr.Manager
	wanMu        sync.RWMutex
	wanCache     map[string]cachedWAN
}

// NewNetPilotService constructs the centralized backend service.
func NewNetPilotService(
	database *db.Database,
	ringLog *logger.RingLogger,
	capDetector *capability.Detector,
	modeCtrl *mode.Controller,
	ruleEngine *rules.Engine,
	failoverSM *failover.StateMachine,
	appManager *apps.Manager,
	analytics *analytics.Collector,
	scenarios *scenarios.Engine,
	diagnostics *diagnostics.Generator,
	testRunner *testrunner.Runner,
	netmergeCtrl *netmerge.Controller,
	ttlManager *ttlmgr.Manager,
) *NetPilotService {
	return &NetPilotService{
		database:     database,
		ringLog:      ringLog,
		capDetector:  capDetector,
		modeCtrl:     modeCtrl,
		ruleEngine:   ruleEngine,
		failoverSM:   failoverSM,
		appManager:   appManager,
		analytics:    analytics,
		scenarios:    scenarios,
		diagnostics:  diagnostics,
		testRunner:   testRunner,
		netmergeCtrl: netmergeCtrl,
		ttlManager:   ttlManager,
		wanCache:     make(map[string]cachedWAN),
	}
}

// GetBrandingInfo returns application branding metadata.
func (s *NetPilotService) GetBrandingInfo() map[string]string {
	return branding.Info()
}

// GetCapabilities returns the full capability detection matrix.
func (s *NetPilotService) GetCapabilities() capability.Report {
	return s.capDetector.Detect()
}

// GetExecutionMode returns the current active execution mode and safety details.
func (s *NetPilotService) GetExecutionMode() mode.State {
	return s.modeCtrl.GetState()
}

// SetExecutionMode switches between Simulation, Dry-Run, and Live modes.
func (s *NetPilotService) SetExecutionMode(targetMode string, forceLive bool) (mode.State, error) {
	op := s.ringLog.StartOperation("SetExecutionMode", "ModeController", string(s.modeCtrl.Current()), "common", "")
	op.Step(logger.PhaseValidationStarted, "Validating mode transition request to "+targetMode, nil)

	st, err := s.modeCtrl.SetMode(mode.ExecutionMode(targetMode), forceLive)
	if err != nil {
		op.Fail(logger.PhasePolicyCompleted, "SafetyGating", err, "Remain in current safe mode", "NONE", nil)
		return st, err
	}

	op.Complete("SUCCESS", map[string]interface{}{"newMode": targetMode})
	return st, nil
}

// ListRules returns all configured routing rules.
func (s *NetPilotService) ListRules() ([]db.RuleRecord, error) {
	return s.database.ListRules()
}

// SaveRule inserts or updates a routing rule.
func (s *NetPilotService) SaveRule(r db.RuleRecord) error {
	op := s.ringLog.StartOperation("SaveRule", "RulesEngine", string(s.modeCtrl.Current()), "common", "")
	op.Step(logger.PhaseValidationStarted, "Validating rule semantics for "+r.AppName, nil)

	if err := s.ruleEngine.ValidateRule(r); err != nil {
		op.Fail(logger.PhaseValidationCompleted, "ValidationError", err, "Reject rule modification", "NONE", nil)
		return err
	}

	if err := s.database.SaveRule(r); err != nil {
		op.Fail(logger.PhaseBackendStarted, "DatabaseError", err, "Rollback", "NONE", nil)
		return err
	}

	_ = s.ruleEngine.Reload()
	op.Complete("SUCCESS", map[string]interface{}{"ruleId": r.ID})
	return nil
}

// DeleteRule removes a rule.
func (s *NetPilotService) DeleteRule(id string) error {
	op := s.ringLog.StartOperation("DeleteRule", "RulesEngine", string(s.modeCtrl.Current()), "common", "")
	if err := s.database.DeleteRule(id); err != nil {
		op.Fail(logger.PhaseBackendStarted, "DatabaseError", err, "None", "NONE", nil)
		return err
	}
	_ = s.ruleEngine.Reload()
	op.Complete("SUCCESS", map[string]interface{}{"ruleId": id})
	return nil
}

// InterfaceInfo summarizes an adapter's status.
type InterfaceInfo struct {
	Name        string    `json:"name"`
	IP          string    `json:"ip"`
	WanIP       string    `json:"wanIp"`
	ISP         string    `json:"isp,omitempty"`
	Location    string    `json:"location,omitempty"`
	IsUp        bool      `json:"isUp"`
	IsHardware  bool      `json:"isHardware"`
	RxMbps      float64   `json:"rxMbps"`
	TxMbps      float64   `json:"txMbps"`
	LinkState   string    `json:"linkState"`
	LastProbeAt time.Time `json:"lastProbeAt"`
	RxLimitMbps float64   `json:"rxLimitMbps"`
	TxLimitMbps float64   `json:"txLimitMbps"`
	DataQuotaMB float64   `json:"dataQuotaMb"`
	DataUsedMB  float64   `json:"dataUsedMb"`
}

func (s *NetPilotService) lookupWanIP(ifaceName string, ifaceIdx int, localIP net.IP) (string, string, string) {
	if localIP == nil || localIP.IsLoopback() || localIP.IsLinkLocalUnicast() {
		return "", "", ""
	}
	s.wanMu.RLock()
	c, ok := s.wanCache[ifaceName]
	s.wanMu.RUnlock()

	if ok && time.Since(c.at) < 2*time.Minute {
		return c.ip, c.isp, c.loc
	}

	// Probe asynchronously so GetInterfaces() returns in < 2ms
	go func(name string, idx int, ip net.IP) {
		s.wanMu.Lock()
		if existing, exists := s.wanCache[name]; exists && time.Since(existing.at) < 30*time.Second {
			s.wanMu.Unlock()
			return
		}
		// Mark in-flight timestamp
		s.wanCache[name] = cachedWAN{ip: c.ip, isp: c.isp, loc: c.loc, at: time.Now()}
		s.wanMu.Unlock()

		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{IP: ip},
			Control:   nics.ForceEgress(idx, false),
			Timeout:   2500 * time.Millisecond,
		}
		tr := &http.Transport{
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		}
		client := &http.Client{
			Transport: tr,
			Timeout:   3000 * time.Millisecond,
		}

		// Try ipinfo.io/json first for rich WAN IP + ISP + Country
		resp, err := client.Get("https://ipinfo.io/json")
		if err == nil && resp != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			var data struct {
				IP      string `json:"ip"`
				Org     string `json:"org"`
				City    string `json:"city"`
				Country string `json:"country"`
			}
			if json.Unmarshal(b, &data) == nil && data.IP != "" {
				loc := data.Country
				if data.City != "" && data.Country != "" {
					loc = data.City + ", " + data.Country
				}
				s.wanMu.Lock()
				s.wanCache[name] = cachedWAN{
					ip:  strings.TrimSpace(data.IP),
					isp: strings.TrimSpace(data.Org),
					loc: loc,
					at:  time.Now(),
				}
				s.wanMu.Unlock()
				return
			}
		}

		// Fallback to Cloudflare trace or ipify
		for _, url := range []string{"https://api.ipify.org", "https://icanhazip.com"} {
			resp2, err2 := client.Get(url)
			if err2 == nil && resp2 != nil {
				b, _ := io.ReadAll(io.LimitReader(resp2.Body, 256))
				resp2.Body.Close()
				wanStr := strings.TrimSpace(string(b))
				if net.ParseIP(wanStr) != nil {
					s.wanMu.Lock()
					s.wanCache[name] = cachedWAN{ip: wanStr, at: time.Now()}
					s.wanMu.Unlock()
					return
				}
			}
		}
	}(ifaceName, ifaceIdx, localIP)

	return c.ip, c.isp, c.loc
}

// GetInterfaces returns real system interfaces and live throughput.
func (s *NetPilotService) GetInterfaces() []InterfaceInfo {
	var out []InterfaceInfo

	// Read real host network counters first
	_ = s.analytics.SampleHostNICs()

	nics, _ := s.analytics.Snapshot()
	nicMetrics := make(map[string]analytics.NICMetric)
	for _, m := range nics {
		nicMetrics[m.Name] = m
	}

	// Fetch configured adapter caps
	capsList, _ := s.database.GetAdapterCaps()
	capsMap := make(map[string]db.AdapterCapRecord)
	for _, c := range capsList {
		capsMap[c.InterfaceName] = c
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}

	healthMap := s.failoverSM.Snapshot()
	stateMap := make(map[string]failover.InterfaceHealth)
	for _, h := range healthMap {
		stateMap[h.Name] = h
	}

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		ipStr := ""
		var localV4 net.IP
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				ipStr = ipn.String()
				localV4 = ipn.IP.To4()
				break
			}
		}

		isUp := ifc.Flags&net.FlagUp != 0 && ipStr != ""
		if !isUp && ipStr == "" {
			// Skip completely disconnected / unconfigured ghost adapters
			continue
		}

		wanIP, isp, loc := "", "", ""
		if isUp && localV4 != nil {
			wanIP, isp, loc = s.lookupWanIP(ifc.Name, ifc.Index, localV4)
		}

		linkSt := "UP"
		if h, ok := stateMap[ifc.Name]; ok {
			linkSt = string(h.State)
		} else if !isUp {
			linkSt = "DOWN"
		}

		rxMbps := 0.0
		txMbps := 0.0
		if m, ok := nicMetrics[ifc.Name]; ok {
			rxMbps = m.CurrentRxMbps
			txMbps = m.CurrentTxMbps
		}

		rxLimit := 0.0
		txLimit := 0.0
		quotaMb := 0.0
		usedMb := 0.0
		if capRec, ok := capsMap[ifc.Name]; ok && capRec.Enabled {
			rxLimit = capRec.RxLimitMbps
			txLimit = capRec.TxLimitMbps
			quotaMb = capRec.DataQuotaMB
			usedMb = capRec.DataUsedMB
		}

		out = append(out, InterfaceInfo{
			Name:        ifc.Name,
			IP:          ipStr,
			WanIP:       wanIP,
			ISP:         isp,
			Location:    loc,
			IsUp:        isUp,
			IsHardware:  len(ifc.HardwareAddr) > 0,
			RxMbps:      rxMbps,
			TxMbps:      txMbps,
			LinkState:   linkSt,
			RxLimitMbps: rxLimit,
			TxLimitMbps: txLimit,
			DataQuotaMB: quotaMb,
			DataUsedMB:  usedMb,
		})
	}
	return out
}

// DiscoverApplications returns currently active applications with matched policies.
func (s *NetPilotService) DiscoverApplications() []apps.ProcessInfo {
	procs := s.appManager.Discover()
	for i := range procs {
		decision := s.ruleEngine.Evaluate(procs[i].Name, procs[i].Executable, "", "eth0")
		if decision.MatchedRuleID != "" {
			procs[i].AssignedNIC = decision.SelectedInterface
			procs[i].RuleMatched = decision.MatchedRuleID
			procs[i].CustomDNS = decision.CustomDNS
			procs[i].BandwidthLimitMbps = decision.BandwidthLimitMbps
			procs[i].DataQuotaMB = decision.DataQuotaMB
			procs[i].DataUsedMB = decision.DataUsedMB
			procs[i].QuotaExceeded = decision.QuotaExceeded
		}
	}
	return procs
}

// GetAnalyticsSnapshot returns current metrics and time series points.
func (s *NetPilotService) GetAnalyticsSnapshot() map[string]interface{} {
	_ = s.analytics.SampleLinuxNetDev()
	nics, appStats := s.analytics.Snapshot()
	pt := s.analytics.PushHistoryPoint()
	history := s.analytics.GetHistory()

	return map[string]interface{}{
		"nics":        nics,
		"apps":        appStats,
		"latestPoint": pt,
		"history":     history,
	}
}

// QueryLogs searches memory ring logs with filtering.
func (s *NetPilotService) QueryLogs(opts logger.FilterOptions) []logger.LogEntry {
	return s.ringLog.Query(opts)
}

// ExportDiagnosticReport creates a comprehensive diagnostic bundle JSON string.
func (s *NetPilotService) ExportDiagnosticReport(strictRedact bool) (string, error) {
	b, err := s.diagnostics.ToJSON(strictRedact)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RunScenario executes a deterministic scenario.
func (s *NetPilotService) RunScenario(id string) scenarios.ScenarioResult {
	return s.scenarios.Run(id)
}

// RunAllScenarios runs all deterministic simulation scenarios.
func (s *NetPilotService) RunAllScenarios() []scenarios.ScenarioResult {
	return s.scenarios.RunAll()
}

// RunAllUnified executes all platform suites and scenarios in a single one-click run.
func (s *NetPilotService) RunAllUnified(skipDisruptive bool) testrunner.UnifiedTestReport {
	return s.testRunner.RunAllUnified(skipDisruptive, s.scenarios)
}

// RunPlatformSuite executes a platform test suite.
func (s *NetPilotService) RunPlatformSuite(suite string) testrunner.SuiteReport {
	return s.testRunner.RunSuite(testrunner.PlatformSuite(suite))
}

// ListTestHistory returns historical test run summaries.
func (s *NetPilotService) ListTestHistory(limit int) ([]db.TestRunRecord, error) {
	return s.database.ListTestRuns(limit)
}

// GetFailoverStatus returns current failover state machine snapshots.
func (s *NetPilotService) GetFailoverStatus() []failover.InterfaceHealth {
	return s.failoverSM.Snapshot()
}

// SimFailInterface triggers a simulated interface failure for testing.
func (s *NetPilotService) SimFailInterface(name string) error {
	return s.failoverSM.ForceState(name, failover.StateDown)
}

// SimRecoverInterface triggers a simulated interface recovery for testing.
func (s *NetPilotService) SimRecoverInterface(name string) error {
	return s.failoverSM.ForceState(name, failover.StateUp)
}

// EvaluateRoute computes the routing outcome for a prospective flow.
func (s *NetPilotService) EvaluateRoute(appName, path, destination string) rules.MatchDecision {
	return s.ruleEngine.Evaluate(appName, path, destination, "eth0")
}

// GetAdapterCaps returns configured adapter caps.
func (s *NetPilotService) GetAdapterCaps() ([]db.AdapterCapRecord, error) {
	return s.database.GetAdapterCaps()
}

// SaveAdapterCap persists speed limits and quotas for an interface.
func (s *NetPilotService) SaveAdapterCap(ac db.AdapterCapRecord) error {
	op := s.ringLog.StartOperation("SaveAdapterCap", "FailoverEngine", string(s.modeCtrl.Current()), "common", "")
	err := s.database.SaveAdapterCap(ac)
	if err != nil {
		op.Fail(logger.PhaseBackendStarted, "DatabaseError", err, "Rollback", "NONE", nil)
		return err
	}
	op.Complete("SUCCESS", map[string]interface{}{"interface": ac.InterfaceName})
	return nil
}

// GetMonthlyHistory retrieves historical bandwidth records.
func (s *NetPilotService) GetMonthlyHistory(yearMonth string) ([]db.MonthlyUsageRecord, error) {
	return s.database.GetMonthlyHistory(yearMonth)
}

// GetMonthlySummaries returns high-level monthly summaries.
func (s *NetPilotService) GetMonthlySummaries() ([]db.MonthlySummary, error) {
	return s.database.GetAllMonthlySummaries()
}

// NetMerge Control RPC methods

func (s *NetPilotService) GetNetMergeStatus() (netmerge.StatusInfo, error) {
	if s.netmergeCtrl == nil {
		return netmerge.StatusInfo{}, fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.GetStatus()
}

func (s *NetPilotService) SetNetMergeMode(on bool) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	s.ringLog.Log(logger.LogEntry{
		Level:     logger.LevelInfo,
		Component: "NetMerge",
		Message:   fmt.Sprintf("System-wide aggregation state requested: %v", on),
	})
	return s.netmergeCtrl.SetSystemWide(on)
}

func (s *NetPilotService) SetNetMergeAutostart(enabled bool) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SetAutostart(enabled)
}

func (s *NetPilotService) SetNetMergeLimit(adapter string, mbps float64) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SetLimit(adapter, mbps)
}

func (s *NetPilotService) RunNetMergeCleanup() error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	s.ringLog.Log(logger.LogEntry{
		Level:     logger.LevelWarn,
		Component: "NetMerge",
		Message:   "Invoked emergency route and stale interface cleanup",
	})
	return s.netmergeCtrl.CleanupStale()
}

func (s *NetPilotService) StartNetMergeSocks(addr string) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.StartSocks5(addr)
}

func (s *NetPilotService) StopNetMergeSocks() error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.StopSocks5()
}

func (s *NetPilotService) SaveNetMergeConfig(cfg netmerge.Config) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SaveConfig(cfg)
}

func (s *NetPilotService) GetNetMergeBinaryInfo() (netmerge.BinaryInfo, error) {
	if s.netmergeCtrl == nil {
		return netmerge.BinaryInfo{}, fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.GetBinaryInfo(), nil
}

func (s *NetPilotService) SetNetMergeBinaryPath(path string) (netmerge.BinaryInfo, error) {
	if s.netmergeCtrl == nil {
		return netmerge.BinaryInfo{}, fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SetCustomBinaryPath(path)
}

func (s *NetPilotService) SetConsoleWindow(show bool) error {
	return setConsoleVisible(show)
}

// TTL / Hop Limit Management Methods

func (s *NetPilotService) InspectTTL(ctx context.Context) (ttlmgr.Report, error) {
	if s.ttlManager == nil {
		return ttlmgr.Report{}, fmt.Errorf("TTL manager not initialized")
	}
	var ifaces []ttlmgr.IfaceInfo
	netIfaces, _ := net.Interfaces()
	for _, ni := range netIfaces {
		ifaces = append(ifaces, ttlmgr.IfaceInfo{
			Index: ni.Index,
			Name:  ni.Name,
		})
	}
	return s.ttlManager.Inspect(ctx, ifaces)
}

func (s *NetPilotService) ApplyTTL(ctx context.Context, bumpIPv4, bumpIPv6 int, policy string, ifaceList []string, dryRun bool) (ttlmgr.Outcome, error) {
	if s.ttlManager == nil {
		return ttlmgr.Outcome{}, fmt.Errorf("TTL manager not initialized")
	}
	var ifaces []ttlmgr.IfaceInfo
	netIfaces, _ := net.Interfaces()
	for _, ni := range netIfaces {
		ifaces = append(ifaces, ttlmgr.IfaceInfo{
			Index: ni.Index,
			Name:  ni.Name,
		})
	}

	pol := ttlmgr.PolicyInterfaceNone
	if policy == "list" {
		pol = ttlmgr.PolicyInterfaceList
	} else if policy == "all-except" {
		pol = ttlmgr.PolicyInterfaceAllExcept
	}

	cfg := ttlmgr.Config{
		Enabled:       true,
		BumpIPv4:      bumpIPv4,
		BumpIPv6:      bumpIPv6,
		Policy:        pol,
		IncludeIfaces: ifaceList,
		Updated:       time.Now(),
	}

	return s.ttlManager.Apply(ctx, ttlmgr.ApplyRequest{
		Ifaces:   ifaces,
		DryRun:   dryRun,
		Confirm:  true,
		Reason:   "Configured via NetPilot Control Center UI",
		Override: &cfg,
	})
}

func (s *NetPilotService) RestoreTTL(ctx context.Context, dryRun bool) (ttlmgr.Outcome, error) {
	if s.ttlManager == nil {
		return ttlmgr.Outcome{}, fmt.Errorf("TTL manager not initialized")
	}
	var ifaces []ttlmgr.IfaceInfo
	netIfaces, _ := net.Interfaces()
	for _, ni := range netIfaces {
		ifaces = append(ifaces, ttlmgr.IfaceInfo{
			Index: ni.Index,
			Name:  ni.Name,
		})
	}

	return s.ttlManager.Restore(ctx, ttlmgr.RestoreRequest{
		Ifaces:  ifaces,
		DryRun:  dryRun,
		Confirm: true,
	})
}

func (s *NetPilotService) GetTTLHistory() ([]ttlmgr.Event, error) {
	if s.ttlManager == nil {
		return []ttlmgr.Event{}, fmt.Errorf("TTL manager not initialized")
	}
	return s.ttlManager.Store().History(100)
}

// ExportMonthlyCSV returns CSV string representation of monthly network usage.
func (s *NetPilotService) ExportMonthlyCSV(yearMonth string) (string, error) {
	return s.database.ExportMonthlyCSV(yearMonth)
}

// ServeHTTP handles /api requests routed by the Wails asset server.
func (s *NetPilotService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/")
	path = strings.TrimPrefix(path, "api/")

	switch {
	case path == "branding":
		_ = json.NewEncoder(w).Encode(s.GetBrandingInfo())

	case path == "capabilities":
		_ = json.NewEncoder(w).Encode(s.GetCapabilities())

	case path == "mode":
		if r.Method == http.MethodPost {
			var body struct {
				Mode      string `json:"mode"`
				ForceLive bool   `json:"forceLive"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			st, err := s.SetExecutionMode(body.Mode, body.ForceLive)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(st)
		} else {
			_ = json.NewEncoder(w).Encode(s.GetExecutionMode())
		}

	case path == "interfaces":
		_ = json.NewEncoder(w).Encode(s.GetInterfaces())

	case path == "applications":
		_ = json.NewEncoder(w).Encode(s.DiscoverApplications())

	case path == "rules":
		if r.Method == http.MethodPost {
			var rule db.RuleRecord
			_ = json.NewDecoder(r.Body).Decode(&rule)
			if err := s.SaveRule(rule); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		} else if r.Method == http.MethodDelete {
			id := r.URL.Query().Get("id")
			_ = s.DeleteRule(id)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		} else {
			rules, err := s.ListRules()
			if err != nil {
				rules = []db.RuleRecord{}
			}
			_ = json.NewEncoder(w).Encode(rules)
		}

	case path == "scenarios":
		_ = json.NewEncoder(w).Encode(s.RunAllScenarios())

	case path == "failover":
		_ = json.NewEncoder(w).Encode(s.GetFailoverStatus())

	case path == "analytics":
		_ = json.NewEncoder(w).Encode(s.GetAnalyticsSnapshot())

	case path == "diagnostics":
		strict := r.URL.Query().Get("strict") != "false"
		rep, err := s.ExportDiagnosticReport(strict)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rep))

	case path == "logs":
		level := r.URL.Query().Get("level")
		comp := r.URL.Query().Get("component")
		search := r.URL.Query().Get("search")
		errOnly := r.URL.Query().Get("errorsOnly") == "true"
		entries := s.QueryLogs(logger.FilterOptions{
			Level:      level,
			Component:  comp,
			SearchText: search,
			ErrorsOnly: errOnly,
			Limit:      150,
		})
		_ = json.NewEncoder(w).Encode(entries)

	case path == "suite":
		suite := r.URL.Query().Get("suite")
		if suite == "" {
			suite = "common"
		}
		rep := s.RunPlatformSuite(suite)
		_ = json.NewEncoder(w).Encode(rep)

	case path == "testrunner/run-all":
		skipDisruptive := r.URL.Query().Get("skipDisruptive") != "false"
		rep := s.RunAllUnified(skipDisruptive)
		_ = json.NewEncoder(w).Encode(rep)

	case path == "adapter-caps":
		if r.Method == http.MethodPost {
			var capRec db.AdapterCapRecord
			_ = json.NewDecoder(r.Body).Decode(&capRec)
			if err := s.SaveAdapterCap(capRec); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		} else {
			caps, err := s.GetAdapterCaps()
			if err != nil {
				caps = []db.AdapterCapRecord{}
			}
			_ = json.NewEncoder(w).Encode(caps)
		}

	case path == "monthly-history":
		ym := r.URL.Query().Get("month")
		hist, err := s.GetMonthlyHistory(ym)
		if err != nil {
			hist = []db.MonthlyUsageRecord{}
		}
		summaries, _ := s.GetMonthlySummaries()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"records":   hist,
			"summaries": summaries,
		})

	case path == "monthly-history/export":
		ym := r.URL.Query().Get("month")
		csvStr, err := s.ExportMonthlyCSV(ym)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		filename := fmt.Sprintf("netpilot-monthly-%s.csv", ym)
		if ym == "" || ym == "all" {
			filename = "netpilot-monthly-all.csv"
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		_, _ = w.Write([]byte(csvStr))

	case path == "netmerge/status":
		st, err := s.GetNetMergeStatus()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(st)

	case path == "netmerge/toggle":
		var body struct {
			On bool `json:"on"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.SetNetMergeMode(body.On); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case path == "netmerge/autostart":
		var body struct {
			Enabled bool `json:"enabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.SetNetMergeAutostart(body.Enabled); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case path == "netmerge/limit":
		var body struct {
			Adapter string  `json:"adapter"`
			Mbps    float64 `json:"mbps"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.SetNetMergeLimit(body.Adapter, body.Mbps); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case path == "netmerge/cleanup":
		if err := s.RunNetMergeCleanup(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cleaned"})

	case path == "netmerge/socks":
		var body struct {
			Action  string `json:"action"` // "start" or "stop"
			Address string `json:"address"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Action == "start" {
			if err := s.StartNetMergeSocks(body.Address); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		} else {
			_ = s.StopNetMergeSocks()
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case path == "netmerge/config":
		if r.Method == http.MethodPost {
			var cfg netmerge.Config
			_ = json.NewDecoder(r.Body).Decode(&cfg)
			if err := s.SaveNetMergeConfig(cfg); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		} else {
			if s.netmergeCtrl != nil {
				_ = json.NewEncoder(w).Encode(s.netmergeCtrl.GetConfig())
			} else {
				_ = json.NewEncoder(w).Encode(map[string]string{})
			}
		}

	case path == "netmerge/binary":
		if r.Method == http.MethodPost {
			var body struct {
				Path string `json:"path"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			info, err := s.SetNetMergeBinaryPath(body.Path)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":  err.Error(),
					"binary": info,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(info)
		} else {
			info, err := s.GetNetMergeBinaryInfo()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(info)
		}

	case path == "system/console":
		var body struct {
			Show bool `json:"show"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = s.SetConsoleWindow(body.Show)
		_ = json.NewEncoder(w).Encode(map[string]bool{"visible": body.Show})

	case path == "ttl/inspect":
		rep, err := s.InspectTTL(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(rep)

	case path == "ttl/apply":
		var body struct {
			BumpIPv4 int      `json:"bumpIPv4"`
			BumpIPv6 int      `json:"bumpIPv6"`
			Policy   string   `json:"policy"`
			Ifaces   []string `json:"ifaces"`
			DryRun   bool     `json:"dryRun"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		out, err := s.ApplyTTL(r.Context(), body.BumpIPv4, body.BumpIPv6, body.Policy, body.Ifaces, body.DryRun)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(out)

	case path == "ttl/restore":
		var body struct {
			DryRun bool `json:"dryRun"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		out, err := s.RestoreTTL(r.Context(), body.DryRun)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(out)

	case path == "ttl/history":
		hist, err := s.GetTTLHistory()
		if err != nil {
			hist = []ttlmgr.Event{}
		}
		_ = json.NewEncoder(w).Encode(hist)

	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}
}
