package main

import (
	"path/filepath"
	"testing"

	"netpilot/internal/db"
	"netpilot/internal/logger"
	"netpilot/internal/rules"
)

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
