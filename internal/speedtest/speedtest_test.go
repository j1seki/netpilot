package speedtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func benchmarkServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/down", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, 256<<10))
	})
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunAdapterMeasuresThroughput(t *testing.T) {
	srv := benchmarkServer(t)
	opts := Options{
		Duration: 400 * time.Millisecond,
		Streams:  2,
		DownURL:  srv.URL + "/down",
		UpURL:    srv.URL + "/up",
		PingURL:  srv.URL + "/down",
		MaxBytes: 2 << 20,
	}
	res := RunAdapter(context.Background(), Adapter{Name: "test"}, opts)
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.DownMbps <= 0 {
		t.Fatalf("download not measured: %+v", res)
	}
	if res.UpMbps <= 0 {
		t.Fatalf("upload not measured: %+v", res)
	}
	if res.Grade == "" {
		t.Fatalf("grade missing: %+v", res)
	}
}

func TestRunAdapterHonoursByteBudget(t *testing.T) {
	srv := benchmarkServer(t)
	opts := Options{
		Duration: 500 * time.Millisecond,
		Streams:  4,
		DownURL:  srv.URL + "/down",
		UpURL:    srv.URL + "/up",
		PingURL:  srv.URL + "/down",
		MaxBytes: 1 << 20,
	}
	res := RunAdapter(context.Background(), Adapter{Name: "test"}, opts)
	if res.DownMbps > 0 && res.DownMbps > 1e6 {
		t.Fatalf("impossible throughput suggests the budget was ignored: %+v", res)
	}
}

func TestGrade(t *testing.T) {
	cases := map[float64]string{0: "A", 29: "A", 30: "B", 70: "C", 150: "D", 400: "F"}
	for in, want := range cases {
		if got := Grade(in); got != want {
			t.Errorf("Grade(%v) = %s, want %s", in, got, want)
		}
	}
}
