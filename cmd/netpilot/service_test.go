package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
	"netpilot/internal/netmerge"
	"netpilot/internal/rules"
)

func TestDryRunBlocksNetworkChanges(t *testing.T) {
	svc, _ := newTestService(t)
	svc.modeCtrl = mode.NewController(capability.Report{})
	if _, err := svc.SetExecutionMode(string(mode.ModeDryRun), false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{
		{"netmerge/toggle", `{"on":true}`},
		{"netmerge/toggle", `{"on":false}`},
		{"netmerge/socks", `{"action":"start"}`},
		{"netmerge/socks", `{"action":"stop"}`},
		{"netmerge/autostart", `{"enabled":true}`},
		{"netmerge/limit", `{"adapter":"eth0","mbps":1}`},
		{"netmerge/cleanup", `{}`},
		{"ttl/apply", `{"dryRun":false}`},
		{"ttl/restore", `{"dryRun":false}`},
		{"tools/wol", `{}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/"+tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		svc.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "switch to Live Mode") {
			t.Errorf("%s %s returned %d %s", tc.path, tc.body, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ttl/apply", strings.NewReader(`{"dryRun":true}`))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "TTL manager not initialized") {
		t.Fatalf("preview did not reach the TTL handler: %s", rec.Body.String())
	}
}

func TestDryRunTransitionRequiresForwardingToStop(t *testing.T) {
	svc, _ := newTestService(t)
	svc.modeCtrl = mode.NewController(capability.Report{})
	svc.netmergeCtrl = netmerge.NewController(t.TempDir())
	t.Cleanup(func() { _ = svc.netmergeCtrl.StopOwned() })
	if err := svc.StartNetMergeSocks("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	if st, err := svc.SetExecutionMode(string(mode.ModeDryRun), false); err == nil || st.CurrentMode != mode.ModeLive {
		t.Fatalf("transition allowed with an active proxy: %+v, %v", st, err)
	}
	if err := svc.StopNetMergeSocks(); err != nil {
		t.Fatal(err)
	}
	if st, err := svc.SetExecutionMode(string(mode.ModeDryRun), false); err != nil || st.CurrentMode != mode.ModeDryRun {
		t.Fatalf("transition with proxy stopped: %+v, %v", st, err)
	}
}

func newTestService(t *testing.T) (*NetPilotService, *db.Database) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "netpilot.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ruleEngine, err := rules.NewEngine(database)
	if err != nil {
		t.Fatalf("rules.NewEngine: %v", err)
	}
	return &NetPilotService{
		database:   database,
		ringLog:    logger.NewRingLogger(128),
		ruleEngine: ruleEngine,
	}, database
}

func TestConfigBundleRoundTrip(t *testing.T) {
	svc, database := newTestService(t)

	rule := db.RuleRecord{
		ID:                 "rule-1",
		AppName:            "Browser",
		PreferredInterface: "eth0",
		Priority:           20,
		Enabled:            true,
	}
	if err := database.SaveRule(rule); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveAdapterCap(db.AdapterCapRecord{
		InterfaceName: "eth0",
		RxLimitMbps:   50,
		DataQuotaMB:   500,
		DataUsedMB:    123,
		Enabled:       true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveAdapterLabel("eth0", "Home Fiber"); err != nil {
		t.Fatal(err)
	}

	bundle, err := svc.ExportConfigBundle()
	if err != nil {
		t.Fatalf("ExportConfigBundle: %v", err)
	}
	if bundle.Schema != ConfigBundleSchema {
		t.Fatalf("schema = %q", bundle.Schema)
	}
	if len(bundle.Rules) != 1 || len(bundle.AdapterCaps) != 1 || len(bundle.AdapterLabels) != 1 {
		t.Fatalf("exported bundle = %+v", bundle)
	}

	// Change everything, then restore from the exported snapshot.
	if err := database.SaveAdapterLabel("eth0", "Changed"); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveAdapterCap(db.AdapterCapRecord{
		InterfaceName: "eth0",
		RxLimitMbps:   1,
		DataUsedMB:    0,
		Enabled:       true,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ImportConfigBundle(bundle)
	if err != nil {
		t.Fatalf("ImportConfigBundle: %v", err)
	}
	if result.RulesImported != 1 || result.AdapterCapsImported != 1 || result.LabelsImported != 1 {
		t.Fatalf("import result = %+v", result)
	}

	labels, _ := database.ListAdapterLabels()
	if len(labels) != 1 || labels[0].Label != "Home Fiber" {
		t.Fatalf("label not restored: %+v", labels)
	}
	caps, _ := database.GetAdapterCaps()
	if len(caps) != 1 || caps[0].RxLimitMbps != 50 || caps[0].DataUsedMB != 123 {
		t.Fatalf("cap not restored: %+v", caps)
	}
	rules, _ := database.ListRules()
	if len(rules) != 1 || rules[0].ID != "rule-1" {
		t.Fatalf("rule not restored: %+v", rules)
	}
}

func TestImportConfigBundleRejectsInvalidWithoutWriting(t *testing.T) {
	svc, database := newTestService(t)

	bundle := ConfigBundle{
		Schema: ConfigBundleSchema,
		Rules: []db.RuleRecord{{
			ID:      "bad-rule",
			AppName: "Browser",
			// PreferredInterface intentionally empty.
		}},
		AdapterCaps: []db.AdapterCapRecord{{InterfaceName: "eth0", RxLimitMbps: 10}},
	}
	if _, err := svc.ImportConfigBundle(bundle); err == nil {
		t.Fatal("invalid rule was accepted")
	}
	rules, _ := database.ListRules()
	if len(rules) != 0 {
		t.Fatalf("invalid import wrote rules: %+v", rules)
	}
	caps, _ := database.GetAdapterCaps()
	if len(caps) != 0 {
		t.Fatalf("invalid import wrote caps: %+v", caps)
	}

	bundle = ConfigBundle{Schema: "someone-else/v9"}
	if _, err := svc.ImportConfigBundle(bundle); err == nil {
		t.Fatal("unknown schema was accepted")
	}
}

func TestResetDataUsageService(t *testing.T) {
	svc, database := newTestService(t)
	if err := database.SaveAdapterCap(db.AdapterCapRecord{
		InterfaceName: "eth0",
		DataUsedMB:    99,
		Enabled:       true,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ResetDataUsage("adapter", "eth0")
	if err != nil {
		t.Fatalf("ResetDataUsage: %v", err)
	}
	if res.AdapterCounters != 1 {
		t.Fatalf("reset result = %+v", res)
	}
	caps, _ := database.GetAdapterCaps()
	if len(caps) != 1 || caps[0].DataUsedMB != 0 {
		t.Fatalf("counter not cleared: %+v", caps)
	}
}
