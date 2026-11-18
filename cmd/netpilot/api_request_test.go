package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidActionRequestsStopBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "netmerge/cleanup", "", 405},
		{"DELETE", "hotspot/revert", "", 405},
		{"POST", "netmerge/config", "null", 400},
		{"POST", "netmerge/toggle", "", 400},
		{"POST", "ttl/restore", `{"dryRun":"yes"}`, 400},
		{"POST", "usage/reset", `{"scope":"all"} {}`, 400},
		{"POST", "adapter-labels", `{"label":`, 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			new(NetPilotService).ServeHTTP(w, httptest.NewRequest(tc.method, "/api/"+tc.path, strings.NewReader(tc.body)))
			var response struct{ Error string }
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error == "" {
				t.Fatalf("got HTTP %d: %s", w.Code, w.Body)
			}
			if w.Code == http.StatusMethodNotAllowed && w.Header().Get("Allow") != "POST" {
				t.Fatal("missing Allow: POST")
			}
		})
	}
}
