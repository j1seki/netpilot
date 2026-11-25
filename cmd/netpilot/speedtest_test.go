package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunSpeedtestMeasuresSystemDefault(t *testing.T) {
	const maxMB = 1
	var downloadConnections sync.Map
	var uploaded atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/down", func(w http.ResponseWriter, r *http.Request) {
		downloadConnections.Store(r.RemoteAddr, struct{}{})
		_, _ = w.Write(make([]byte, 128<<10))
	})
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) {
		if _, reused := downloadConnections.Load(r.RemoteAddr); reused {
			t.Error("upload reused a connection from the download phase")
		}
		n, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			t.Errorf("read upload body: %v", err)
		}
		uploaded.Add(n)
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
		MaxMB:       maxMB,
	})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one system-default result", results)
	}
	r := results[0]
	if r.Adapter != "System Default" || r.DownMbps <= 0 || r.UpMbps <= 0 || r.Error != "" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if got := uploaded.Load(); got != maxMB<<20 {
		t.Fatalf("server received %d upload bytes, want %d", got, maxMB<<20)
	}
}
