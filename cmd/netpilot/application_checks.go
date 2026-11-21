package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"netpilot/internal/apps"
	"netpilot/internal/db"
	"netpilot/internal/logger"
	"netpilot/internal/netmerge"
	"netpilot/internal/rules"
	"netpilot/internal/testrunner"
)

func (s *NetPilotService) applicationChecks() []testrunner.Check {
	return []testrunner.Check{
		{Name: "App Inspection API", Run: checkAppInspection},
		{Name: "Application rule editing and persistence", Run: checkRuleEditor},
		{Name: "Connected mode switch, restart status and saved profiles", Run: checkModeAndProfiles},
		{Name: "Usage history, CSV and selective reset", Run: checkMetricsHistory},
		{Name: "Adapter quota and manual speed cap", Run: checkAdapterQuota},
		{Name: "Notification preferences and in-app delivery", Run: checkNotificationPreferences},
		{Name: "Desktop notification facility", Run: checkNativeNotificationFacility},
		{Name: "Mobile profile and routing settings", Run: checkHotspotProfile},
		{Name: "Configuration backup, restore and persistence", Run: checkConfigRoundTrip},
		{Name: "JSON report export", Run: checkReportExport},
		{Name: "MCP start, authentication and stop", Run: s.checkMCP},
		{Name: "Desktop process network sort", Run: s.desktopTests.checkSort},
		{Name: "Desktop Metrics units", Run: s.desktopTests.checkMetricUnits},
		{Name: "Desktop Developer Tools binding", Run: s.checkDevTools},
	}
}

func checkAppInspection() (testrunner.TestStatus, string) {
	svc := NetPilotService{appManager: apps.NewManager()}
	var pid apps.ProcessInfo
	for _, process := range svc.DiscoverApplications() {
		if process.PID == os.Getpid() {
			pid = process
			break
		}
	}
	if pid.Name == "" {
		return testrunner.StatusFail, "Active Processes did not include the running NetPilot executable"
	}
	database, err := db.Open(":memory:")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer database.Close()
	for _, rule := range []db.RuleRecord{
		{ID: "paused", AppName: pid.Name, PreferredInterface: "paused-link", Priority: 90},
		{ID: "destination", DomainPattern: "*.fixture.test", PreferredInterface: "test-link", Priority: 80, Enabled: true, CustomDNS: "1.1.1.1"},
		{ID: "app", AppName: pid.Name, PreferredInterface: "net-merge", Priority: 10, Enabled: true},
	} {
		if err := database.SaveRule(rule); err != nil {
			return testrunner.StatusFail, err.Error()
		}
	}
	svc.ruleEngine, err = rules.NewEngine(database)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return testrunner.StatusBlocked, err.Error()
	}
	defer target.Close()
	conn, err := net.DialTimeout("tcp4", target.Addr().String(), time.Second)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer conn.Close()

	body, _ := json.Marshal(map[string]any{
		"pid": pid.PID, "name": pid.Name, "executable": pid.Executable, "destination": "host.fixture.test:443",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/applications/inspect", bytes.NewReader(body))
	response := httptest.NewRecorder()
	svc.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		return testrunner.StatusFail, fmt.Sprintf("Inspector returned HTTP %d: %s", response.Code, response.Body.String())
	}
	var report processInspection
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	if report.Process.PID != pid.PID || report.Process.CommandLine != "" || report.DNSEndpoints == nil || report.SystemResolvers == nil || report.Activity == nil {
		return testrunner.StatusFail, "Inspector returned an unexpected process, private arguments or missing fields"
	}
	matches := report.RuleMatches
	if report.RuleDestination != "host.fixture.test" || report.RuleScope != "standby" || report.SavedRuleDNS != "1.1.1.1" ||
		len(matches) != 3 || matches[0].Rule.ID != "paused" || matches[0].Result != "disabled" ||
		matches[1].Rule.ID != "destination" || matches[1].Result != "selected" || matches[1].MatchedBy != "destination" ||
		matches[2].Rule.ID != "app" || matches[2].Result != "shadowed" ||
		svc.Resolve(pid.Name, pid.Executable, report.RuleDestination).RuleID != matches[1].Rule.ID {
		return testrunner.StatusFail, "Inspector rule preview did not agree with the engine's match order"
	}
	for _, socket := range report.Sockets {
		if socket.Protocol == "TCP" && socket.Local == conn.LocalAddr().String() && socket.Remote == target.Addr().String() {
			return testrunner.StatusPass, fmt.Sprintf("Inspector found the test TCP connection, reported %d system DNS resolver address(es), and verified selected, disabled, and lower-priority rules", len(report.SystemResolvers))
		}
	}
	if report.SocketError != "" {
		return testrunner.StatusBlocked, report.SocketError
	}
	return testrunner.StatusFail, "Inspector API did not see the live test connection"
}

func checkHotspotProfile() (testrunner.TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-test-hotspot-")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	controller := netmerge.NewController(dir)
	defer controller.Close()
	svc := NetPilotService{netmergeCtrl: controller, hotspotFile: filepath.Join(dir, "hotspot.json")}
	initial := controller.GetConfig()
	initial.MTU, initial.IPv6Mode = 1500, "on"
	initial.RoutingMode, initial.PrimaryAdapter, initial.SecondaryAdapter = "backup", "diag-primary", "diag-tether"
	if err := controller.SaveConfig(initial); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	ctx := context.Background()
	adapters := []string{initial.SecondaryAdapter}
	preview, err := svc.ApplyHotspotTo(ctx, "metered-saver", true, adapters)
	if err != nil || preview.MTU != 1280 || !slices.Equal(preview.Adapters, adapters) || controller.GetConfig().MTU != initial.MTU || svc.hotspot.Enabled {
		return testrunner.StatusFail, fmt.Sprintf("Mobile profile preview changed state or returned an unexpected plan: %v", err)
	}
	active, err := svc.ApplyHotspotTo(ctx, "metered-saver", false, adapters)
	cfg := controller.GetConfig()
	if err != nil || !active.Enabled || cfg.MTU != 1280 || cfg.IPv6Mode != "off" ||
		cfg.RoutingMode != initial.RoutingMode || cfg.PrimaryAdapter != initial.PrimaryAdapter || cfg.SecondaryAdapter != initial.SecondaryAdapter {
		return testrunner.StatusFail, fmt.Sprintf("Mobile profile failed to set MTU/IPv6 while preserving Backup mode and its adapter choices: %v", err)
	}
	reverted, err := svc.RevertHotspot(ctx)
	cfg = controller.GetConfig()
	if err != nil || reverted.Enabled || cfg.MTU != initial.MTU || cfg.IPv6Mode != initial.IPv6Mode || cfg.RoutingMode != initial.RoutingMode {
		return testrunner.StatusFail, fmt.Sprintf("Mobile profile did not restore the prior tunnel settings: %v", err)
	}
	if _, err := svc.SetHotspotAutoApply(true); err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defaults, err := svc.resetHotspot(true)
	cfg = controller.GetConfig()
	if err != nil || defaults.AutoApply || defaults.Enabled || cfg.MTU != 9000 || cfg.IPv6Mode != "auto" ||
		cfg.PrimaryAdapter != initial.PrimaryAdapter || cfg.RoutingMode != "backup" {
		return testrunner.StatusFail, "Mobile defaults changed other settings or left auto-apply enabled"
	}
	settings := cfg.MobileSettings()
	settings.MTU, settings.IPv6Mode, settings.Scheduling = 1375, "on", "low-latency"
	settings.ProbeTargetsV4 = []string{"192.0.2.10:443"}
	if _, err := svc.saveMobileSettings(mobileSettingsRequest{MobileSettings: settings}); err != nil {
		return testrunner.StatusFail, "Apply customized Mobile settings: " + err.Error()
	}
	custom := controller.GetConfig()
	if custom.MTU != settings.MTU || custom.IPv6Mode != settings.IPv6Mode || custom.Scheduling != settings.Scheduling ||
		!slices.Equal(custom.ProbeTargetsV4, settings.ProbeTargetsV4) || custom.RoutingMode != initial.RoutingMode {
		return testrunner.StatusFail, "Combined Mobile settings did not save together or changed Backup mode"
	}
	if _, err := svc.RevertHotspot(ctx); err != nil {
		return testrunner.StatusFail, "Restore customized Mobile settings: " + err.Error()
	}
	restored := controller.GetConfig()
	if restored.MTU != cfg.MTU || restored.IPv6Mode != cfg.IPv6Mode || restored.Scheduling != cfg.Scheduling ||
		!slices.Equal(restored.ProbeTargetsV4, cfg.ProbeTargetsV4) {
		return testrunner.StatusFail, "Restore did not restore all Mobile fields"
	}
	return testrunner.StatusPass, "Verified preset preview, customized MTU, IPv6, scheduling and probes, and restoration without changing the Backup adapters"
}

func checkConfigRoundTrip() (testrunner.TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-test-config-")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	database, err := db.Open(filepath.Join(dir, "netpilot.db"))
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer database.Close()
	policy, err := rules.NewEngine(database)
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	controller := netmerge.NewController(dir)
	defer controller.Close()
	svc := NetPilotService{database: database, ruleEngine: policy, netmergeCtrl: controller, ringLog: logger.NewRingLogger(4)}
	config := controller.GetConfig()
	config.CloseAction = "close"
	input := ConfigBundle{
		Schema: ConfigBundleSchema, Version: 1, NetMerge: &config,
		Rules:         []db.RuleRecord{{ID: "diag-rule", AppName: "diagnostic", PreferredInterface: "diag-link", Enabled: true}},
		AdapterCaps:   []db.AdapterCapRecord{{InterfaceName: "diag-link", TxLimitMbps: 17, Enabled: true}},
		AdapterLabels: []db.AdapterLabelRecord{{InterfaceName: "diag-link", Label: "Diagnostic link"}},
	}
	restored, err := svc.ImportConfigBundle(input)
	if err != nil || !restored.NetMergeImported {
		return testrunner.StatusFail, fmt.Sprintf("Configuration import failed: %v", err)
	}
	exported, err := svc.ExportConfigBundle()
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	reopened := netmerge.NewController(dir)
	defer reopened.Close()
	if len(exported.Rules) != 1 || exported.Rules[0].ID != input.Rules[0].ID ||
		len(exported.AdapterCaps) != 1 || exported.AdapterCaps[0].TxLimitMbps != 17 ||
		len(exported.AdapterLabels) != 1 || exported.AdapterLabels[0].Label != "Diagnostic link" ||
		exported.NetMerge == nil || exported.NetMerge.CloseAction != "close" || reopened.GetCloseAction() != "close" {
		return testrunner.StatusFail, "Exported configuration or persisted preference differed from the imported values"
	}
	return testrunner.StatusPass, "Imported and exported a rule, cap and adapter label, then reopened the tunnel controller and verified its saved preference"
}

func checkReportExport() (testrunner.TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-test-report-")
	if err != nil {
		return testrunner.StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "report.json")
	svc := NetPilotService{promptReportPath: func(name string) (string, error) {
		if !strings.HasSuffix(name, ".json") {
			return "", fmt.Errorf("export suggested a non-JSON filename")
		}
		return path, nil
	}}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/testrunner/export", strings.NewReader(
		`{"kind":"tests","report":{"schema":"netpilot-test-report-v1","passed":1}}`))
	svc.ServeHTTP(response, req)
	data, err := os.ReadFile(path)
	if response.Code != http.StatusOK || err != nil {
		return testrunner.StatusFail, fmt.Sprintf("Report export returned HTTP %d: %v", response.Code, err)
	}
	var exported testrunner.UnifiedTestReport
	if err := json.Unmarshal(data, &exported); err != nil || exported.Schema != "netpilot-test-report-v1" || exported.Passed != 1 {
		return testrunner.StatusFail, fmt.Sprintf("Saved file did not contain the expected JSON report: %v", err)
	}
	return testrunner.StatusPass, "Saved and parsed JSON through the export API using a temporary destination"
}

func (s *NetPilotService) checkMCP() (testrunner.TestStatus, string) {
	var endpoint diagnosticMCP
	if err := endpoint.listen(s, "127.0.0.1", "0"); err != nil {
		return testrunner.StatusBlocked, err.Error()
	}
	defer endpoint.stop()
	status := endpoint.snapshot()
	client := http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	call := func(token string) (int, []string, error) {
		req, err := http.NewRequest(http.MethodPost, status.Endpoint, strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, nil, nil
		}
		var payload struct {
			Result struct {
				Tools []struct{ Name string }
			}
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return resp.StatusCode, nil, err
		}
		var tools []string
		for _, tool := range payload.Result.Tools {
			tools = append(tools, tool.Name)
		}
		return resp.StatusCode, tools, nil
	}
	if code, _, err := call(""); code != http.StatusUnauthorized || err != nil {
		return testrunner.StatusFail, fmt.Sprintf("MCP did not reject the unauthenticated request: HTTP %d, %v", code, err)
	}
	if code, tools, err := call(status.Token); code != http.StatusOK || !slices.Contains(tools, "run_diagnostics") || err != nil {
		return testrunner.StatusFail, fmt.Sprintf("Authenticated MCP did not advertise diagnostics: HTTP %d, %v", code, err)
	}
	endpoint.stop()
	if _, _, err := call(status.Token); err == nil || endpoint.snapshot().Running {
		return testrunner.StatusFail, "Stopped MCP endpoint continued to accept requests"
	}
	return testrunner.StatusPass, "Started an actual loopback MCP listener, verified bearer authentication and tool listing, then confirmed it stopped accepting requests"
}

func (s *NetPilotService) checkDevTools() (testrunner.TestStatus, string) {
	if s.desktopTests.send == nil {
		return testrunner.StatusBlocked, "The desktop window is unavailable in headless mode"
	}
	options := desktopWindowOptions(false)
	if !options.DevToolsEnabled || options.KeyBindings["F12"] == nil {
		return testrunner.StatusFail, "Desktop window is missing its DevTools setting or F12 binding"
	}
	return testrunner.StatusPass, "Desktop window options enable Developer Tools and register F12"
}
