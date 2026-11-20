package main

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

func (s *NetPilotService) serveTestReportExport(w http.ResponseWriter, r *http.Request) {
	if s.promptReportPath == nil {
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "report export requires the desktop app"})
		return
	}
	var body struct {
		Kind   string                     `json:"kind"`
		Report map[string]json.RawMessage `json:"report"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if !decodeAPIObject(w, r, &body) {
		return
	}
	if body.Report == nil || (body.Kind != "tests" && body.Kind != "diagnostics") {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid report"})
		return
	}
	data, err := json.MarshalIndent(body.Report, "", "  ")
	if err == nil {
		var path string
		path, err = s.promptReportPath("netpilot-" + body.Kind + "-" + time.Now().Format("2006-01-02-150405") + ".json")
		if err == nil && path != "" {
			err = os.WriteFile(path, append(data, '\n'), 0o600)
		}
		if err == nil {
			_ = json.NewEncoder(w).Encode(map[string]bool{"saved": path != ""})
			return
		}
	}
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
