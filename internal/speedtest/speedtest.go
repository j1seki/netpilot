// Package speedtest measures per-adapter and merged throughput and bufferbloat.
//
// Each stream is pinned to one adapter with the same source binding and egress forcing the
// data path uses, so a result reflects what that link can actually carry rather than what the
// routing table happens to choose. The merged test goes through the running SOCKS5 front-end
// and therefore measures the combined pool.
package speedtest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"

	"netpilot/internal/nics"
)

// Adapter is one link to measure. Index 0 or a nil IP means "let the OS choose".
type Adapter struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
	IP    net.IP `json:"-"`
}

// Options controls a run. Zero values are replaced by DefaultOptions.
type Options struct {
	Duration time.Duration `json:"durationSec"`
	Streams  int           `json:"streams"`
	DownURL  string        `json:"downUrl"`
	UpURL    string        `json:"upUrl"`
	PingURL  string        `json:"pingUrl"`
	MaxBytes int64         `json:"maxBytes"`
}

// DefaultOptions returns a short, low-data benchmark suitable for a UI button.
func DefaultOptions() Options {
	return Options{
		Duration: 5 * time.Second,
		Streams:  4,
		DownURL:  "https://speed.cloudflare.com/__down?bytes=10000000",
		UpURL:    "https://speed.cloudflare.com/__up",
		PingURL:  "https://speed.cloudflare.com/__down?bytes=1",
		MaxBytes: 25 << 20,
	}
}

func (o Options) normalized() Options {
	d := DefaultOptions()
	if o.Duration <= 0 || o.Duration > 30*time.Second {
		o.Duration = d.Duration
	}
	if o.Streams <= 0 || o.Streams > 16 {
		o.Streams = d.Streams
	}
	if o.DownURL == "" {
		o.DownURL = d.DownURL
	}
	if o.UpURL == "" {
		o.UpURL = d.UpURL
	}
	if o.PingURL == "" {
		o.PingURL = d.PingURL
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = d.MaxBytes
	}
	return o
}

// Result is one adapter's (or the merged pool's) benchmark outcome.
type Result struct {
	Adapter       string    `json:"adapter"`
	DownMbps      float64   `json:"downMbps"`
	UpMbps        float64   `json:"upMbps"`
	IdleMs        float64   `json:"idleMs"`
	LoadedMs      float64   `json:"loadedMs"`
	BufferbloatMs float64   `json:"bufferbloatMs"`
	Grade         string    `json:"grade"`
	Error         string    `json:"error,omitempty"`
	At            time.Time `json:"at"`
}

// RunAdapter benchmarks one pinned adapter.
func RunAdapter(ctx context.Context, a Adapter, opts Options) Result {
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		var local net.Addr
		if a.IP != nil {
			local = &net.TCPAddr{IP: a.IP}
		}
		d := net.Dialer{
			LocalAddr: local,
			Control:   nics.ForceEgress(a.Index, false),
			Timeout:   8 * time.Second,
		}
		return d.DialContext(ctx, network, addr)
	}
	return run(ctx, a.Name, dial, opts)
}

// RunProxy benchmarks the merged pool through the local SOCKS5 front-end.
func RunProxy(ctx context.Context, socksAddr string, opts Options) Result {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, &net.Dialer{Timeout: 8 * time.Second})
	if err != nil {
		return Result{Adapter: "net-merge", Error: err.Error(), At: time.Now()}
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return Result{Adapter: "net-merge", Error: "SOCKS5 dialer does not support context", At: time.Now()}
	}
	return run(ctx, "net-merge", cd.DialContext, opts)
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func run(parent context.Context, name string, dial dialFunc, opts Options) Result {
	opts = opts.normalized()
	res := Result{Adapter: name, At: time.Now()}
	if name == "" {
		res.Adapter = "direct"
	}

	ctx, cancel := context.WithTimeout(parent, opts.Duration)
	defer cancel()

	transport := &http.Transport{
		DialContext:         dial,
		DisableCompression:  true,
		MaxIdleConnsPerHost: opts.Streams,
	}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	res.IdleMs = ping(ctx, client, opts.PingURL)

	var downloaded atomic.Int64
	budget := opts.MaxBytes
	var wg sync.WaitGroup
	downStart := time.Now()
	loadedCh := make(chan float64, opts.Streams)
	for i := 0; i < opts.Streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loadedCh <- downloadStream(ctx, client, opts.DownURL, &downloaded, &budget)
		}()
	}
	wg.Wait()
	close(loadedCh)
	downElapsed := time.Since(downStart).Seconds()
	sumLoaded, n := 0.0, 0
	for v := range loadedCh {
		if v > 0 {
			sumLoaded += v
			n++
		}
	}
	if n > 0 {
		res.LoadedMs = sumLoaded / float64(n)
	}
	res.DownMbps = mbps(downloaded.Load(), downElapsed)

	// Use a separate connection pool for uploads. A stale download connection
	// can fail as a POST starts, and the transport cannot safely replay its body.
	transport.CloseIdleConnections()
	uploadTransport := transport.Clone()
	defer uploadTransport.CloseIdleConnections()
	uploadClient := &http.Client{Transport: uploadTransport}

	upCtx, upCancel := context.WithTimeout(parent, opts.Duration)
	defer upCancel()
	var up sum
	upBudget := opts.MaxBytes
	upStart := time.Now()
	for i := 0; i < opts.Streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			up.n.Add(uploadStream(upCtx, uploadClient, opts.UpURL, &upBudget))
		}()
	}
	wg.Wait()
	res.UpMbps = mbps(up.n.Load(), time.Since(upStart).Seconds())

	if res.IdleMs <= 0 || res.LoadedMs <= 0 {
		res.Error = "No bufferbloat grade: idle or loaded latency was not measured"
		return res
	}
	if res.UpMbps <= 0 {
		res.Error = "Upload throughput was not measured"
		return res
	}
	res.BufferbloatMs = res.LoadedMs - res.IdleMs
	if res.BufferbloatMs < 0 {
		res.BufferbloatMs = 0
	}
	res.Grade = Grade(res.BufferbloatMs)
	return res
}

type sum struct{ n atomic.Int64 }

func mbps(bytes int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(bytes) * 8 / seconds / 1e6
}

// Grade turns loaded-latency increase into an A-F bufferbloat grade.
func Grade(extraMs float64) string {
	switch {
	case extraMs < 30:
		return "A"
	case extraMs < 60:
		return "B"
	case extraMs < 100:
		return "C"
	case extraMs < 200:
		return "D"
	default:
		return "F"
	}
}

func ping(ctx context.Context, client *http.Client, url string) float64 {
	var total float64
	var n int
	for i := 0; i < 4; i++ {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, withBuster(url, i), nil)
		if err != nil {
			break
		}
		req.Header.Set("Cache-Control", "no-store")
		resp, err := client.Do(req)
		if err != nil {
			break
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		total += float64(time.Since(start)) / float64(time.Millisecond)
		n++
	}
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

func downloadStream(ctx context.Context, client *http.Client, url string, total *atomic.Int64, budget *int64) float64 {
	var latencyTotal float64
	var latencyN int
	for i := 0; ctx.Err() == nil; i++ {
		reserved := reserveBudget(budget, 1<<20)
		if reserved <= 0 {
			break
		}
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, withBuster(url, i), nil)
		if err != nil {
			atomic.AddInt64(budget, reserved)
			break
		}
		req.Header.Set("Cache-Control", "no-store")
		resp, err := client.Do(req)
		if err != nil {
			atomic.AddInt64(budget, reserved)
			break
		}
		if ctx.Err() == nil {
			latencyTotal += float64(time.Since(start)) / float64(time.Millisecond)
			latencyN++
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			atomic.AddInt64(budget, reserved)
			break
		}
		n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, reserved))
		resp.Body.Close()
		total.Add(n)
		// Return unused reserved bytes. The bytes actually received remain
		// consumed from the shared budget, so concurrent streams cannot exceed it.
		atomic.AddInt64(budget, reserved-n)
		if n == 0 {
			break
		}
	}
	return latencyTotal / float64(max(latencyN, 1))
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func uploadStream(ctx context.Context, client *http.Client, url string, budget *int64) int64 {
	var sent atomic.Int64
	for ctx.Err() == nil {
		reserved := reserveBudget(budget, 1<<20)
		if reserved <= 0 {
			break
		}
		body := io.LimitReader(zeroReader{}, reserved)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, withBuster(url, int(sent.Load())), nil)
		if err != nil {
			atomic.AddInt64(budget, reserved)
			break
		}
		req.Body = io.NopCloser(countingReader{r: body, n: &sent})
		req.ContentLength = reserved
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Cache-Control", "no-store")
		before := sent.Load()
		resp, err := client.Do(req)
		sentNow := sent.Load() - before
		// Request bodies are fully consumed or the request fails. Release the
		// unused reservation in either case; consumed bytes remain charged.
		atomic.AddInt64(budget, reserved-min(reserved, sentNow))
		if err != nil {
			break
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			break
		}
	}
	return sent.Load()
}

// reserveBudget atomically reserves up to maxBytes from a non-negative shared
// test budget. The caller returns any unused portion after the request completes.
func reserveBudget(budget *int64, maxBytes int64) int64 {
	for {
		remaining := atomic.LoadInt64(budget)
		if remaining <= 0 {
			return 0
		}
		reservation := min(remaining, maxBytes)
		if atomic.CompareAndSwapInt64(budget, remaining, remaining-reservation) {
			return reservation
		}
	}
}

func withBuster(raw string, i int) string {
	sep := "?"
	if strings.Contains(raw, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%sr=%d", raw, sep, i)
}
