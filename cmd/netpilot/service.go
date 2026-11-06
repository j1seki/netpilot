package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
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
	ip  string
	isp string
	loc string
	at  time.Time
}

type cachedLatency struct {
	routerMs float64
	webMs    float64
	at       time.Time
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
	latencyMu    sync.RWMutex
	latencyCache map[string]cachedLatency
	latencyOnce  sync.Once
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
		latencyCache: make(map[string]cachedLatency),
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
	rules, err := s.database.ListRules()
	if rules == nil {
		// An empty table must serialize as [] rather than null: the frontend treats null
		// as "no backend data" and refuses to render the whole dashboard batch.
		rules = []db.RuleRecord{}
	}
	return rules, err
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
	IsUp            bool      `json:"isUp"`
	IsHardware      bool      `json:"isHardware"`
	IsDefault       bool      `json:"isDefault"`  // carries the OS default route
	IsVirtual       bool      `json:"isVirtual"`  // VPN / tunnel / software adapter
	LatencyRouterMs float64   `json:"latencyRouterMs"`
	LatencyWebMs    float64   `json:"latencyWebMs"`
	RxMbps          float64   `json:"rxMbps"`
	TxMbps          float64   `json:"txMbps"`
	LinkState       string    `json:"linkState"`
	LastProbeAt     time.Time `json:"lastProbeAt"`
	RxLimitMbps     float64   `json:"rxLimitMbps"`
	TxLimitMbps     float64   `json:"txLimitMbps"`
	DataQuotaMB     float64   `json:"dataQuotaMb"`
	DataUsedMB      float64   `json:"dataUsedMb"`
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

var pingTimeRe = regexp.MustCompile(`(?i)time[=<]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

// defaultRouteLocalIP asks the OS which source address it would use for a remote
// connection. A UDP dial does not send a packet; it only resolves the route, so
// this is a cheap cross-platform way to identify the current default adapter.
func defaultRouteLocalIP() net.IP {
	for _, target := range []string{"1.1.1.1:443", "[2606:4700:4700::1111]:443"} {
		conn, err := net.DialTimeout("udp", target, 500*time.Millisecond)
		if err != nil {
			continue
		}
		var ip net.IP
		if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok && ua.IP != nil && !ua.IP.IsUnspecified() {
			ip = ua.IP
		}
		_ = conn.Close()
		if ip != nil {
			return ip
		}
	}
	return nil
}

// probeWebLatency measures a TCP handshake to a well-known anycast resolver,
// bound to one adapter, so each row can show its own internet latency.
func probeWebLatency(idx int, ip net.IP) (float64, bool) {
	if ip == nil {
		return 0, false
	}
	v6 := ip.To4() == nil
	network, target := "tcp4", "1.1.1.1:443"
	if v6 {
		network, target = "tcp6", "[2606:4700:4700::1111]:443"
	}
	d := net.Dialer{
		LocalAddr: &net.TCPAddr{IP: ip},
		Control:   nics.ForceEgress(idx, v6),
		Timeout:   1500 * time.Millisecond,
	}
	start := time.Now()
	conn, err := d.Dial(network, target)
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000.0, true
}

// pingLatency measures round-trip time to the adapter's own gateway. ICMP is used
// because routers rarely expose a TCP listener on a predictable port.
func pingLatency(gw, src net.IP, _ string) (float64, bool) {
	if gw == nil {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	cmdName := "ping"
	var args []string
	v6 := gw.To4() == nil
	switch runtime.GOOS {
	case "windows":
		args = []string{"-n", "1", "-w", "1000"}
		if v6 {
			args = append([]string{"-6"}, args...)
		} else {
			args = append([]string{"-4"}, args...)
		}
		if src != nil && (src.To4() == nil) == v6 {
			args = append(args, "-S", src.String())
		}
	case "darwin":
		if v6 {
			cmdName = "ping6"
			args = []string{"-n", "-c", "1", "-W", "1000"}
		} else {
			args = []string{"-n", "-c", "1", "-W", "1000"}
		}
		if src != nil && (src.To4() == nil) == v6 {
			args = append(args, "-S", src.String())
		}
	default:
		if v6 {
			args = []string{"-6", "-n", "-c", "1", "-W", "1"}
		} else {
			args = []string{"-n", "-c", "1", "-W", "1"}
		}
		if src != nil && (src.To4() == nil) == v6 {
			args = append(args, "-I", src.String())
		}
	}
	args = append(args, gw.String())

	out, err := exec.CommandContext(ctx, cmdName, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return 0, false
	}
	m := pingTimeRe.FindSubmatch(out)
	if m == nil {
		return 0, false
	}
	ms, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0, false
	}
	return ms, true
}

func (s *NetPilotService) lookupLatency(name string) (routerMs, webMs float64) {
	s.latencyMu.RLock()
	defer s.latencyMu.RUnlock()
	c, ok := s.latencyCache[name]
	if !ok || time.Since(c.at) > 2*time.Minute {
		return 0, 0
	}
	return c.routerMs, c.webMs
}

// sampleLatencies refreshes the per-adapter router/web latency cache and feeds the
// failover state machine so "FASTEST" rules finally have live RTT data.
func (s *NetPilotService) sampleLatencies() {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	virtualMap := nics.VirtualAdapterMap()
	var wg sync.WaitGroup
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		var ip net.IP
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			if v4 := ipn.IP.To4(); v4 != nil {
				ip = v4
				break
			}
			if ip == nil && ipn.IP.To16() != nil {
				ip = ipn.IP
			}
		}
		if ip == nil {
			continue
		}
		_, virtualByPlatform := virtualMap[ifc.Index]
		isVirtual := virtualByPlatform || nics.IsVirtualName(ifc.Name) || len(ifc.HardwareAddr) == 0
		idx, name, localIP := ifc.Index, ifc.Name, ip
		wg.Add(1)
		go func() {
			defer wg.Done()
			webMs, webOK := probeWebLatency(idx, localIP)
			routerMs := 0.0
			if gw := nics.GatewayIP(idx); gw != nil {
				if ms, ok := pingLatency(gw, localIP, name); ok {
					routerMs = ms
				}
			}
			s.latencyMu.Lock()
			s.latencyCache[name] = cachedLatency{routerMs: routerMs, webMs: webMs, at: time.Now()}
			s.latencyMu.Unlock()

			// Feed the FASTEST rule selector, but never let a VPN/tunnel adapter
			// become the "lowest latency" physical link.
			if s.failoverSM != nil && !isVirtual {
				if webOK {
					_ = s.failoverSM.RecordProbe(name, true, time.Duration(webMs*float64(time.Millisecond)), "")
				} else {
					_ = s.failoverSM.RecordProbe(name, false, 0, "web latency probe failed")
				}
			}
		}()
	}
	wg.Wait()
}

func (s *NetPilotService) startLatencyProbes() {
	s.latencyOnce.Do(func() {
		go func() {
			s.sampleLatencies()
			ticker := time.NewTicker(12 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				s.sampleLatencies()
			}
		}()
	})
}

// GetInterfaces returns real system interfaces and live throughput.
func (s *NetPilotService) GetInterfaces() []InterfaceInfo {
	// Always return a non-nil slice: the frontend treats a JSON null as "backend has no
	// data" and would refuse to render, which previously hid every adapter (and, because
	// the dashboard loads its data in one batch, every process too) on a host whose
	// adapters were all filtered out.
	out := make([]InterfaceInfo, 0, 8)
	s.startLatencyProbes()
	defaultIP := defaultRouteLocalIP()
	virtualMap := nics.VirtualAdapterMap()

	// Read real host network counters first
	_ = s.analytics.SampleHostNICs()

	nicSnap, _ := s.analytics.Snapshot()
	nicMetrics := make(map[string]analytics.NICMetric)
	for _, m := range nicSnap {
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
		var localV4, localV6 net.IP
		// Prefer a routable IPv4 address for the WAN probe, but fall back to an IPv6
		// address so IPv6-only adapters (common on Windows/macOS tethering and some
		// VPN setups) are still listed instead of silently disappearing.
		var ipv6Str string
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			if v4 := ipn.IP.To4(); v4 != nil {
				if localV4 == nil {
					localV4 = v4
					ipStr = ipn.String()
				}
			} else if ipv6Str == "" && ipn.IP.To16() != nil {
				localV6 = ipn.IP
				ipv6Str = ipn.String()
			}
		}
		if ipStr == "" {
			ipStr = ipv6Str
		}

		// Keep every adapter the OS says is up. An adapter can be up before it has an
		// address (DHCP in progress, VPN connecting), and hiding it makes the UI look
		// broken; only discard interfaces that are both down and unaddressed.
		isUp := ifc.Flags&net.FlagUp != 0
		if !isUp && ipStr == "" {
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

		_, virtualByPlatform := virtualMap[ifc.Index]
		isVirtual := virtualByPlatform || nics.IsVirtualName(ifc.Name) || len(ifc.HardwareAddr) == 0
		isDefault := false
		if defaultIP != nil {
			isDefault = (localV4 != nil && localV4.Equal(defaultIP)) ||
				(localV6 != nil && localV6.Equal(defaultIP))
		}
		routerMs, webMs := s.lookupLatency(ifc.Name)

		out = append(out, InterfaceInfo{
			Name:            ifc.Name,
			IP:              ipStr,
			WanIP:           wanIP,
			ISP:             isp,
			Location:        loc,
			IsUp:            isUp,
			IsHardware:      len(ifc.HardwareAddr) > 0 && !isVirtual,
			IsDefault:       isDefault,
			IsVirtual:       isVirtual,
			LatencyRouterMs: routerMs,
			LatencyWebMs:    webMs,
			RxMbps:          rxMbps,
			TxMbps:          txMbps,
			LinkState:       linkSt,
			RxLimitMbps:     rxLimit,
			TxLimitMbps:     txLimit,
			DataQuotaMB:     quotaMb,
			DataUsedMB:      usedMb,
		})
	}
	return out
}

// DiscoverApplications returns currently active applications with matched policies.
// When net-merge is NOT active, the default adapter is "System Default" and NetPilot does nothing
// (pure standby — no custom routing rules are enforced). When net-merge IS active, the default
// adapter is "net-merge" and active routing rules are evaluated.
func (s *NetPilotService) DiscoverApplications() []apps.ProcessInfo {
	procs := s.appManager.Discover()
	isActive := s.netmergeCtrl != nil && s.netmergeCtrl.IsActive()
	defaultAdapter := "System Default"
	if isActive {
		defaultAdapter = "net-merge"
	}
	for i := range procs {
		procs[i].AssignedNIC = defaultAdapter
		if !isActive {
			continue
		}
		decision := s.ruleEngine.Evaluate(procs[i].Name, procs[i].Executable, "", defaultAdapter)
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
	_ = s.analytics.SampleHostNICs()
	nics, appStats := s.analytics.Snapshot()
	history := s.analytics.GetHistory()
	var pt analytics.TimeSeriesPoint
	if len(history) > 0 {
		pt = history[len(history)-1]
	}

	return map[string]interface{}{
		"nics":        nics,
		"apps":        appStats,
		"latestPoint": pt,
		"history":     history,
	}
}

// QueryLogs searches memory ring logs with filtering.
func (s *NetPilotService) QueryLogs(opts logger.FilterOptions) []logger.LogEntry {
	entries := s.ringLog.Query(opts)
	if entries == nil {
		entries = []logger.LogEntry{}
	}
	return entries
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
// When net-merge is not active, default is "System Default" and NetPilot stands by without altering routes.
func (s *NetPilotService) EvaluateRoute(appName, path, destination string) rules.MatchDecision {
	if s.netmergeCtrl == nil || !s.netmergeCtrl.IsActive() {
		return rules.MatchDecision{
			AppName:           appName,
			Destination:       destination,
			SelectedInterface: "System Default",
			Reason:            "Net-Merge is inactive (Standby Mode): using System Default adapter; NetPilot is not modifying routes.",
			EvaluatedAt:       time.Now(),
		}
	}
	return s.ruleEngine.Evaluate(appName, path, destination, "net-merge")
}

// GetAdapterCaps returns configured adapter caps.
func (s *NetPilotService) GetAdapterCaps() ([]db.AdapterCapRecord, error) {
	caps, err := s.database.GetAdapterCaps()
	if caps == nil {
		caps = []db.AdapterCapRecord{}
	}
	return caps, err
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
	hist, err := s.database.GetMonthlyHistory(yearMonth)
	if hist == nil {
		hist = []db.MonthlyUsageRecord{}
	}
	return hist, err
}

// GetMonthlySummaries returns high-level monthly summaries.
func (s *NetPilotService) GetMonthlySummaries() ([]db.MonthlySummary, error) {
	summaries, err := s.database.GetAllMonthlySummaries()
	if summaries == nil {
		summaries = []db.MonthlySummary{}
	}
	return summaries, err
}

// UsageHistoryRecord is one period/adapter/application row in the usage view.
type UsageHistoryRecord struct {
	Period        string    `json:"period"`
	InterfaceName string    `json:"interfaceName"`
	AppName       string    `json:"appName"`
	BytesIn       int64     `json:"bytesIn"`
	BytesOut      int64     `json:"bytesOut"`
	RxMB          float64   `json:"rxMb"`
	TxMB          float64   `json:"txMb"`
	TotalMB       float64   `json:"totalMb"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// UsageHistorySummary aggregates one bucket (a day or a month).
type UsageHistorySummary struct {
	Period    string  `json:"period"`
	TotalRxMB float64 `json:"totalRxMb"`
	TotalTxMB float64 `json:"totalTxMb"`
	TotalMB   float64 `json:"totalMb"`
}

// UsageHistoryResponse powers the Today / Last 7 Days / This Month / All Months
// toggles. Daily buckets come from daily_history; month/all come from the
// existing monthly_history so existing installs keep their history.
type UsageHistoryResponse struct {
	Period    string                `json:"period"`
	From      string                `json:"from"`
	To        string                `json:"to"`
	Records   []UsageHistoryRecord  `json:"records"`
	Summaries []UsageHistorySummary `json:"summaries"`
	TotalRxMB float64               `json:"totalRxMb"`
	TotalTxMB float64               `json:"totalTxMb"`
	TotalMB   float64               `json:"totalMb"`
}

func usageRecord(period, iface, app string, bytesIn, bytesOut int64, updated time.Time) UsageHistoryRecord {
	rx := float64(bytesIn) / (1024 * 1024)
	tx := float64(bytesOut) / (1024 * 1024)
	return UsageHistoryRecord{
		Period:        period,
		InterfaceName: iface,
		AppName:       app,
		BytesIn:       bytesIn,
		BytesOut:      bytesOut,
		RxMB:          rx,
		TxMB:          tx,
		TotalMB:       rx + tx,
		UpdatedAt:     updated,
	}
}

// GetUsageHistory returns usage rows for a human time range.
func (s *NetPilotService) GetUsageHistory(period string) UsageHistoryResponse {
	period = strings.ToLower(strings.TrimSpace(period))
	now := time.Now()
	resp := UsageHistoryResponse{
		Period:    period,
		Records:   []UsageHistoryRecord{},
		Summaries: []UsageHistorySummary{},
	}

	addRecord := func(r UsageHistoryRecord) {
		resp.Records = append(resp.Records, r)
		resp.TotalRxMB += r.RxMB
		resp.TotalTxMB += r.TxMB
		resp.TotalMB += r.TotalMB
	}

	switch period {
	case "today":
		day := now.Format("2006-01-02")
		resp.From, resp.To = day, day
		recs, _ := s.database.GetDailyHistory(day, day)
		for _, r := range recs {
			addRecord(usageRecord(day, r.InterfaceName, r.AppName, r.BytesIn, r.BytesOut, r.UpdatedAt))
		}
		resp.Summaries = append(resp.Summaries, UsageHistorySummary{
			Period: day, TotalRxMB: resp.TotalRxMB, TotalTxMB: resp.TotalTxMB, TotalMB: resp.TotalMB,
		})

	case "week":
		from := now.AddDate(0, 0, -6).Format("2006-01-02")
		to := now.Format("2006-01-02")
		resp.From, resp.To = from, to
		recs, _ := s.database.GetDailyHistory(from, to)
		byDay := make(map[string]*UsageHistorySummary)
		for _, r := range recs {
			rec := usageRecord(r.Day, r.InterfaceName, r.AppName, r.BytesIn, r.BytesOut, r.UpdatedAt)
			addRecord(rec)
			sum := byDay[r.Day]
			if sum == nil {
				sum = &UsageHistorySummary{Period: r.Day}
				byDay[r.Day] = sum
			}
			sum.TotalRxMB += rec.RxMB
			sum.TotalTxMB += rec.TxMB
			sum.TotalMB += rec.TotalMB
		}
		for day := to; day >= from; {
			if sum, ok := byDay[day]; ok {
				resp.Summaries = append(resp.Summaries, *sum)
			}
			t, err := time.Parse("2006-01-02", day)
			if err != nil {
				break
			}
			day = t.AddDate(0, 0, -1).Format("2006-01-02")
		}

	case "month":
		ym := now.Format("2006-01")
		resp.From, resp.To = ym, ym
		recs, _ := s.database.GetMonthlyHistory(ym)
		for _, r := range recs {
			addRecord(usageRecord(r.YearMonth, r.InterfaceName, r.AppName, r.BytesIn, r.BytesOut, r.UpdatedAt))
		}
		resp.Summaries = append(resp.Summaries, UsageHistorySummary{
			Period: ym, TotalRxMB: resp.TotalRxMB, TotalTxMB: resp.TotalTxMB, TotalMB: resp.TotalMB,
		})

	default: // "all"
		resp.Period = "all"
		resp.From, resp.To = "all", "all"
		recs, _ := s.database.GetMonthlyHistory("all")
		for _, r := range recs {
			addRecord(usageRecord(r.YearMonth, r.InterfaceName, r.AppName, r.BytesIn, r.BytesOut, r.UpdatedAt))
		}
		sums, _ := s.database.GetAllMonthlySummaries()
		for _, sum := range sums {
			resp.Summaries = append(resp.Summaries, UsageHistorySummary{
				Period: sum.YearMonth, TotalRxMB: sum.TotalRxMB, TotalTxMB: sum.TotalTxMB, TotalMB: sum.TotalMB,
			})
		}
	}
	return resp
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

func (s *NetPilotService) SetNetMergeEngineSource(source, customPath string) (netmerge.BinaryInfo, error) {
	if s.netmergeCtrl == nil {
		return netmerge.BinaryInfo{}, fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SetEngineSource(source, customPath)
}

func (s *NetPilotService) SetNetMergeRoutingMode(mode, primary, secondary string) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	s.ringLog.Log(logger.LogEntry{
		Level:     logger.LevelInfo,
		Component: "NetMerge",
		Message:   fmt.Sprintf("Routing mode updated to %s (primary=%s, secondary=%s)", mode, primary, secondary),
	})
	return s.netmergeCtrl.SetRoutingMode(mode, primary, secondary)
}

func (s *NetPilotService) GetCloseAction() string {
	if s.netmergeCtrl == nil {
		return "minimize"
	}
	return s.netmergeCtrl.GetCloseAction()
}

func (s *NetPilotService) SetCloseAction(action string) error {
	if s.netmergeCtrl == nil {
		return fmt.Errorf("net-merge controller not initialized")
	}
	return s.netmergeCtrl.SetCloseAction(action)
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

	cfg := ttlmgr.DefaultConfig()
	cfg.Enabled = true
	cfg.BumpIPv4 = bumpIPv4
	cfg.BumpIPv6 = bumpIPv6
	cfg.Policy = pol
	cfg.IncludeIfaces = ifaceList
	cfg.Updated = time.Now()

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
	hist, err := s.ttlManager.Store().History(100)
	if hist == nil {
		hist = []ttlmgr.Event{}
	}
	return hist, err
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

	case path == "usage-history":
		_ = json.NewEncoder(w).Encode(s.GetUsageHistory(r.URL.Query().Get("period")))

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
				Source string `json:"source"`
				Path   string `json:"path"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			var info netmerge.BinaryInfo
			var err error
			if body.Source != "" {
				info, err = s.SetNetMergeEngineSource(body.Source, body.Path)
			} else {
				info, err = s.SetNetMergeBinaryPath(body.Path)
			}
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

	case path == "netmerge/routing-mode":
		var body struct {
			Mode      string `json:"mode"`
			Primary   string `json:"primary"`
			Secondary string `json:"secondary"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := s.SetNetMergeRoutingMode(body.Mode, body.Primary, body.Secondary); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "mode": body.Mode})

	case path == "system/close-action":
		if r.Method == http.MethodPost {
			var body struct {
				Action string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if err := s.SetCloseAction(body.Action); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"closeAction": s.GetCloseAction()})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]string{"closeAction": s.GetCloseAction()})
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
