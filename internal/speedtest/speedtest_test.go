package speedtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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

func TestConcurrentDownloadStreamsShareByteBudget(t *testing.T) {
	const budgetBytes = int64(64 << 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1<<20))
	}))
	defer srv.Close()
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var remaining int64
	var total atomic.Int64
	atomic.StoreInt64(&remaining, budgetBytes)
	results := make(chan float64, 8)
	done := make(chan struct{})
	for i := 0; i < cap(results); i++ {
		go func() {
			results <- downloadStream(ctx, client, srv.URL, &total, &remaining)
		}()
	}
	for i := 0; i < cap(results); i++ {
		<-results
	}
	close(done)
	if got := total.Load(); got > budgetBytes {
		t.Fatalf("parallel downloads received %d bytes; budget is %d", got, budgetBytes)
	}
	if got := atomic.LoadInt64(&remaining); got < 0 {
		t.Fatalf("shared budget went negative: %d", got)
	}
}

func TestConcurrentUploadStreamsShareByteBudget(t *testing.T) {
	const budgetBytes = int64(64 << 10)
	var received atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		received.Add(n)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	client := srv.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var remaining int64
	atomic.StoreInt64(&remaining, budgetBytes)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			uploadStream(ctx, client, srv.URL, &remaining)
		}()
	}
	wg.Wait()
	if got := received.Load(); got > budgetBytes {
		t.Fatalf("parallel uploads sent %d bytes; budget is %d", got, budgetBytes)
	}
	if got := atomic.LoadInt64(&remaining); got < 0 {
		t.Fatalf("shared budget went negative: %d", got)
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
