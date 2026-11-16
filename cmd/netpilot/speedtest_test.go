package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunSpeedtestMeasuresSystemDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/down", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 128<<10))
	})
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, _ := newTestService(t)
	results := svc.RunSpeedtest(context.Background(), SpeedtestRequest{
		DurationSec: 1,
		Streams:     1,
		DownURL:     srv.URL + "/down",
		UpURL:       srv.URL + "/up",
		PingURL:     srv.URL + "/down",
		MaxMB:       1,
	})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one system-default result", results)
	}
	r := results[0]
	if r.Adapter != "System Default" || r.DownMbps <= 0 || r.UpMbps <= 0 || r.Error != "" {
		t.Fatalf("unexpected result: %+v", r)
	}
}
