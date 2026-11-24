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
	e.UpdateNICHealth(map[string]bool{"eth0": false, "wlan0": false, "wwan0": true})
	d = e.Evaluate("game.exe", "", "", "wwan0")
	if !d.Blocked || d.SelectedInterface != "" || d.IsFallback {
		t.Fatalf("strict rule should block when neither adapter is available: %+v", d)
	}
}

func TestBundleRuleMatchesNestedHelpers(t *testing.T) {
	rule := db.RuleRecord{AppPath: "/Applications/Browser.app"}
	if match := matchingCriterion(rule, "Browser Helper", "/applications/browser.app/contents/frameworks/helper", ""); match != "application bundle" {
		t.Fatalf("bundle match = %q", match)
	}
	if match := matchingCriterion(rule, "Browser Other", "/applications/browser.app.backup/contents/macos/browser", ""); match != "" {
		t.Fatalf("similarly named app matched the bundle: %q", match)
	}
}

func TestDomainPatternPlainNameIsExactAndWildcardUsesLabelBoundary(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	e, err := NewEngine(database)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		pattern string
		dest    string
		want    bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "evil-example.com", false},
		{"example.com", "notexample.com", false},
		{"*.example.com", "www.example.com", true},
		{"*.example.com", "a.b.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "evil-example.com", false},
	} {
		t.Run(tc.pattern+"/"+tc.dest, func(t *testing.T) {
			if err := database.SaveRule(db.RuleRecord{
				ID: "domain", DomainPattern: tc.pattern, PreferredInterface: "eth0", Enabled: true, Priority: 1,
			}); err != nil {
				t.Fatal(err)
			}
			if err := e.Reload(); err != nil {
				t.Fatal(err)
			}
			got := e.Evaluate("", "", tc.dest, "default").MatchedRuleID != ""
			if got != tc.want {
				t.Fatalf("pattern %q matched %q = %v, want %v", tc.pattern, tc.dest, got, tc.want)
			}
			if err := database.DeleteRule("domain"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecordUsageDoesNotChargeSubstringApplicationNames(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveRule(db.RuleRecord{ID: "chrome", AppName: "chrome", DataQuotaMB: 100, Enabled: true, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(database)
	if err != nil {
		t.Fatal(err)
	}
	e.RecordUsage("chromedriver", 5<<20, 0)
	e.RecordUsage("chrome helper", 5<<20, 0)
	for _, app := range []string{"chromedriver", "chrome helper"} {
		if d := e.Evaluate(app, "", "", ""); d.MatchedRuleID != "" {
			t.Fatalf("unrelated process %q matched rule %q", app, d.MatchedRuleID)
		}
	}
	if d := e.Evaluate(`C:\Program Files\Browser\CHROME.EXE`, "", "", ""); d.MatchedRuleID != "chrome" {
		t.Fatalf("executable basename did not match: %+v", d)
	}
	rules, err := database.ListRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].DataUsedMB != 0 {
		t.Fatalf("unrelated apps were charged to chrome quota: %+v", rules)
	}
	e.RecordUsage("chrome", 1<<20, 0)
	rules, err = database.ListRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].DataUsedMB < 0.99 || rules[0].DataUsedMB > 1.01 {
		t.Fatalf("exact app was not charged: %+v", rules)
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

	e.RecordUsage("game", 6<<20, 0)
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
	e.RecordUsage("vpn", 11<<20, 0)
	d := e.Evaluate("vpn", "", "example.com", "")
	if !d.QuotaExceeded {
		t.Fatalf("quota should be exceeded after 11 MB: %+v", d)
	}
	if d.DataUsedMB < 10.9 || d.DataUsedMB > 11.1 {
		t.Fatalf("decision usage = %v, want ~11 MB", d.DataUsedMB)
	}
}
