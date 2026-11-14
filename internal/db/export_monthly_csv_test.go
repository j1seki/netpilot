package db

import (
	"encoding/csv"
	"strings"
	"testing"
)

func TestExportMonthlyCSVEscapesCSVFieldsAndFormulaPrefixes(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	iface := "Wi-Fi, \"Guest\""
	app := `=HYPERLINK("https://attacker.invalid","open")`
	if err := database.RecordMonthlyUsage("2026-11", iface, app, 1024, 2048); err != nil {
		t.Fatal(err)
	}

	content, err := database.ExportMonthlyCSV("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d CSV rows, want header plus one record", len(rows))
	}
	if got := rows[1][1]; got != iface {
		t.Fatalf("interface field = %q, want original value %q", got, iface)
	}
	if got := rows[1][2]; got != "'"+app {
		t.Fatalf("application field = %q, want formula-neutralized value", got)
	}
	if rows[1][3] != "1024" || rows[1][4] != "2048" {
		t.Fatalf("byte counters were not preserved: %v", rows[1][3:5])
	}
}

func TestSpreadsheetSafeCellNeutralizesFormulaPrefixesAfterWhitespace(t *testing.T) {
	for _, value := range []string{
		"=1+1",
		"+SUM(1,2)",
		"-1+2",
		"@SUM(A1:A2)",
		"  \t=1+1",
		"＝1+1",
		"＋1+1",
		"－1+1",
		"＠SUM(A1:A2)",
	} {
		t.Run(value, func(t *testing.T) {
			if got := spreadsheetSafeCell(value); got != "'"+value {
				t.Fatalf("spreadsheetSafeCell(%q) = %q, want apostrophe-prefixed value", value, got)
			}
		})
	}
}

func TestSpreadsheetSafeCellLeavesOrdinaryTextUnchanged(t *testing.T) {
	for _, value := range []string{"Ethernet", "Wi-Fi 2", "", "  ordinary name"} {
		if got := spreadsheetSafeCell(value); got != value {
			t.Fatalf("spreadsheetSafeCell(%q) = %q, want unchanged", value, got)
		}
	}
}
