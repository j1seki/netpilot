package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticExportUsesChosenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chosen.json")
	prompted := 0
	service := &NetPilotService{promptReportPath: func(name string) (string, error) {
		prompted++
		if !strings.HasPrefix(name, "netpilot-tests-") || !strings.HasSuffix(name, ".json") {
			t.Fatalf("unexpected suggested filename %q", name)
		}
		return path, nil
	}}
	for _, tc := range []struct {
		method string
		body   string
		status int
	}{
		{http.MethodGet, "", http.StatusMethodNotAllowed},
		{http.MethodPost, `{"kind":"tests","report":null}`, http.StatusBadRequest},
		{http.MethodPost, `{"kind":"tests","report":{"passed":1,"failed":0}}`, http.StatusOK},
	} {
		response := httptest.NewRecorder()
		service.ServeHTTP(response, httptest.NewRequest(tc.method, "/api/testrunner/export", strings.NewReader(tc.body)))
		if response.Code != tc.status {
			t.Fatalf("%s: got %d (%s), want %d", tc.method, response.Code, response.Body, tc.status)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]int
	if err := json.Unmarshal(data, &report); err != nil || report["passed"] != 1 || prompted != 1 {
		t.Fatalf("got report %q and %d save prompts (decode: %v)", data, prompted, err)
	}
}
