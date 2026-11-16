package rules

import (
	"testing"

	"netpilot/internal/db"
)

func TestDecisionExposesFallbackInterface(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SaveRule(db.RuleRecord{
		ID: "fb", AppName: "game", PreferredInterface: "eth0", FallbackInterface: "wlan0",
		Enabled: true, Priority: 1,
	}); err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(database)
	if err != nil {
		t.Fatal(err)
	}
	e.UpdateNICHealth(map[string]bool{"eth0": true, "wlan0": true})
	d := e.Evaluate("game.exe", "", "", "")
	if d.SelectedInterface != "eth0" || d.FallbackInterface != "wlan0" {
		t.Fatalf("decision = %+v, want eth0 with wlan0 fallback", d)
	}
	e.UpdateNICHealth(map[string]bool{"eth0": false, "wlan0": true})
	d = e.Evaluate("game.exe", "", "", "")
	if d.SelectedInterface != "wlan0" || !d.IsFallback {
		t.Fatalf("decision = %+v, want fallback to wlan0", d)
	}
}

func TestRecordUsageAccumulatesOnlyQuotaRules(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SaveRule(db.RuleRecord{ID: "1", AppName: "game", DataQuotaMB: 10, Enabled: true, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveRule(db.RuleRecord{ID: "2", AppName: "browser", Enabled: true, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(database)
	if err != nil {
		t.Fatal(err)
	}

	e.RecordUsage("game.exe", 6<<20, 0)
	e.RecordUsage("browser", 6<<20, 0)

	rules, err := database.ListRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		switch r.ID {
		case "1":
			if r.DataUsedMB < 5.9 || r.DataUsedMB > 6.1 {
				t.Fatalf("quota rule usage = %v, want ~6 MB", r.DataUsedMB)
			}
		case "2":
			if r.DataUsedMB != 0 {
				t.Fatalf("non-quota rule should not be updated, got %v", r.DataUsedMB)
			}
		}
	}
}

func TestRecordUsageDrivesQuotaEnforcement(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := database.SaveRule(db.RuleRecord{ID: "q", AppName: "vpn", DataQuotaMB: 10, Enabled: true, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(database)
	if err != nil {
		t.Fatal(err)
	}

	if d := e.Evaluate("vpn.exe", "", "example.com", ""); d.QuotaExceeded {
		t.Fatal("quota should not be exceeded before usage")
	}
	e.RecordUsage("vpn.exe", 11<<20, 0)
	d := e.Evaluate("vpn.exe", "", "example.com", "")
	if !d.QuotaExceeded {
		t.Fatalf("quota should be exceeded after 11 MB: %+v", d)
	}
	if d.DataUsedMB < 10.9 || d.DataUsedMB > 11.1 {
		t.Fatalf("decision usage = %v, want ~11 MB", d.DataUsedMB)
	}
}
