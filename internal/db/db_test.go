package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *Database {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRuleRoundTrip(t *testing.T) {
	d := openTestDB(t)
	rule := RuleRecord{
		ID:                 "rule-1",
		AppName:            "Browser",
		PreferredInterface: "eth0",
		Priority:           10,
		Enabled:            true,
	}
	if err := d.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}
	rules, err := d.ListRules()
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != "rule-1" || !rules[0].Enabled {
		t.Fatalf("ListRules = %+v", rules)
	}

	rule.Enabled = false
	if err := d.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule update: %v", err)
	}
	rules, _ = d.ListRules()
	if len(rules) != 1 || rules[0].Enabled {
		t.Fatalf("update did not disable rule: %+v", rules)
	}
}

func TestAdapterLabels(t *testing.T) {
	d := openTestDB(t)
	if err := d.SaveAdapterLabel("eth0", "Home Fiber"); err != nil {
		t.Fatalf("SaveAdapterLabel: %v", err)
	}
	labels, err := d.ListAdapterLabels()
	if err != nil {
		t.Fatalf("ListAdapterLabels: %v", err)
	}
	if len(labels) != 1 || labels[0].Label != "Home Fiber" {
		t.Fatalf("labels = %+v", labels)
	}

	if err := d.SaveAdapterLabel("eth0", "   "); err != nil {
		t.Fatalf("clear label: %v", err)
	}
	labels, _ = d.ListAdapterLabels()
	if len(labels) != 0 {
		t.Fatalf("empty label did not clear record: %+v", labels)
	}

	if err := d.SaveAdapterLabel("", "nope"); err == nil {
		t.Fatal("empty adapter name was accepted")
	}
}

func TestResetUsageScopes(t *testing.T) {
	d := openTestDB(t)
	if err := d.SaveAdapterCap(AdapterCapRecord{
		InterfaceName: "eth0",
		RxLimitMbps:   100,
		DataQuotaMB:   1000,
		DataUsedMB:    512,
		Enabled:       true,
	}); err != nil {
		t.Fatalf("SaveAdapterCap: %v", err)
	}
	if err := d.RecordDailyUsage("2026-01-01", "eth0", "system", 1024, 2048); err != nil {
		t.Fatalf("RecordDailyUsage: %v", err)
	}
	if err := d.RecordMonthlyUsage("2026-01", "eth0", "system", 1024, 2048); err != nil {
		t.Fatalf("RecordMonthlyUsage: %v", err)
	}

	res, err := d.ResetUsage("adapter", "eth0")
	if err != nil {
		t.Fatalf("ResetUsage adapter: %v", err)
	}
	if res.AdapterCounters != 1 {
		t.Fatalf("adapter counters reset = %d, want 1", res.AdapterCounters)
	}
	caps, _ := d.GetAdapterCaps()
	if len(caps) != 1 || caps[0].DataUsedMB != 0 {
		t.Fatalf("data used not reset: %+v", caps)
	}
	daily, _ := d.GetDailyHistory("2026-01-01", "2026-01-01")
	monthly, _ := d.GetMonthlyHistory("2026-01")
	if len(daily) != 1 || len(monthly) != 1 {
		t.Fatalf("adapter reset touched history: daily=%d monthly=%d", len(daily), len(monthly))
	}

	if _, err := d.ResetUsage("bogus", ""); err == nil {
		t.Fatal("invalid reset scope accepted")
	}

	res, err = d.ResetUsage("all", "")
	if err != nil {
		t.Fatalf("ResetUsage all: %v", err)
	}
	if res.DailyRows != 1 || res.MonthlyRows != 1 {
		t.Fatalf("reset all rows = %+v", res)
	}
	daily, _ = d.GetDailyHistory("2026-01-01", "2026-01-01")
	monthly, _ = d.GetMonthlyHistory("2026-01")
	if len(daily) != 0 || len(monthly) != 0 {
		t.Fatalf("history not cleared: daily=%d monthly=%d", len(daily), len(monthly))
	}
}

func TestResetUsageInterfaceFilter(t *testing.T) {
	d := openTestDB(t)
	for _, iface := range []string{"eth0", "wlan0"} {
		if err := d.RecordMonthlyUsage("2026-02", iface, "system", 10, 20); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.ResetUsage("monthly", "eth0"); err != nil {
		t.Fatal(err)
	}
	eth, _ := d.GetMonthlyHistory("2026-02")
	if len(eth) != 1 || eth[0].InterfaceName != "wlan0" {
		t.Fatalf("interface filter reset wrong rows: %+v", eth)
	}
}

func TestExportMonthlyCSVEscapesFields(t *testing.T) {
	d := openTestDB(t)
	if err := d.RecordMonthlyUsage("2026-03", "eth,0", `App "Beta"`, 10, 20); err != nil {
		t.Fatal(err)
	}
	csv, err := d.ExportMonthlyCSV("2026-03")
	if err != nil {
		t.Fatalf("ExportMonthlyCSV: %v", err)
	}
	if !strings.Contains(csv, `"eth,0"`) {
		t.Fatalf("interface with comma was not quoted:\n%s", csv)
	}
	if !strings.Contains(csv, `"App ""Beta"""`) {
		t.Fatalf("application with quotes was not escaped:\n%s", csv)
	}
}
