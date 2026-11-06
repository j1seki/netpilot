//go:build ignore
// Gluetun proxy probe: checks each Gluetun HTTP proxy gives a distinct public IP.
// Gluetun ports already are HTTP(S) forwarders; this Go helper verifies distinct egress.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	proxies := []struct {
		name, host string
		port       int
	}{
		{"glu1", "192.168.1.151", 18881},
		{"glu2", "192.168.1.151", 18882},
		{"glu3", "192.168.1.151", 18883},
		{"glu4", "192.168.1.151", 18884},
		{"glu5", "192.168.1.151", 18885},
	}
	seen := map[string]string{}
	failed := false
	for _, p := range proxies {
		pu := &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", p.host, p.port)}
		tr := &http.Transport{Proxy: http.ProxyURL(pu)}
		c := &http.Client{Timeout: 10 * time.Second, Transport: tr}
		req, _ := http.NewRequest("GET", "http://ifconfig.me", nil)
		req.Header.Set("User-Agent", "curl/8.0")
		resp, err := c.Do(req)
		if err != nil {
			fmt.Printf("%s %s:%d ERROR %v\n", p.name, p.host, p.port, err)
			failed = true
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		resp.Body.Close()
		ip := strings.TrimSpace(string(body))
		fmt.Printf("%s %s:%d -> %s\n", p.name, p.host, p.port, ip)
		if prev, ok := seen[ip]; ok && ip != "" {
			fmt.Printf("  DUPLICATE IP with %s (%s)\n", prev, ip)
			failed = true
		}
		seen[ip] = p.name
	}
	if len(seen) != 5 {
		fmt.Println("FAIL: expected 5 distinct IPs")
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("OK: 5 distinct public IPs")
}
