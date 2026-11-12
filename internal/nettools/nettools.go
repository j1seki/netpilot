// Package nettools provides safe, dependency-free network diagnostics used by the
// NetPilot Tools view: ping, DNS lookup, TCP port checks and Wake-on-LAN.
//
// Everything here is read-only unless the caller explicitly asks to send a magic
// packet. Hosts are validated before they reach an external command so a crafted
// value cannot be interpreted as a ping flag on any platform.
package nettools

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"netpilot/internal/execx"
)

// PingResult summarizes one ICMP echo run.
type PingResult struct {
	Host       string  `json:"host"`
	Sent       int     `json:"sent"`
	Received   int     `json:"received"`
	PacketLoss float64 `json:"packetLoss"`
	MinMs      float64 `json:"minMs"`
	AvgMs      float64 `json:"avgMs"`
	MaxMs      float64 `json:"maxMs"`
	DurationMs float64 `json:"durationMs"`
	Output     string  `json:"output,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// DNSResult is the result of a name-resolution request.
type DNSResult struct {
	Host       string   `json:"host"`
	Type       string   `json:"type"`
	Records    []string `json:"records"`
	DurationMs float64  `json:"durationMs"`
	Error      string   `json:"error,omitempty"`
}

// PortResult reports whether a TCP port accepted a connection.
type PortResult struct {
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	Open      bool    `json:"open"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
}

var (
	pingTimeRe = regexp.MustCompile(`(?i)time[=<]\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)
	hostRe     = regexp.MustCompile(`^[A-Za-z0-9_.:\[\]-]+$`)
)

// ValidateHost rejects empty values, leading dashes (command-flag injection) and
// characters that are never valid in a hostname or IP literal.
func ValidateHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("host is required")
	}
	if strings.HasPrefix(host, "-") || !hostRe.MatchString(host) {
		return "", fmt.Errorf("invalid host %q: use a hostname or IP address", host)
	}
	return host, nil
}

func clampTimeout(ms int, def, max time.Duration) time.Duration {
	d := def
	if ms > 0 {
		d = time.Duration(ms) * time.Millisecond
	}
	if d > max {
		d = max
	}
	if d <= 0 {
		d = def
	}
	return d
}

// Ping sends count ICMP echo requests to host and reports the round-trip samples.
// It uses the platform ping binary, which is available without elevated privileges
// on Windows and macOS and via the setuid helper or CAP_NET_RAW on Linux.
func Ping(ctx context.Context, host string, count int, timeout time.Duration) PingResult {
	res := PingResult{Host: strings.TrimSpace(host)}
	if count <= 0 {
		count = 4
	}
	if count > 20 {
		count = 20
	}
	res.Sent = count
	clean, err := ValidateHost(host)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	if timeout > 5*time.Second {
		timeout = 5 * time.Second
	}
	res.Host = clean

	cmdName := "ping"
	var args []string
	timeoutSec := strconv.Itoa(int(timeout / time.Second))
	if timeoutSec == "0" {
		timeoutSec = "1"
	}
	switch runtime.GOOS {
	case "windows":
		args = []string{"-n", strconv.Itoa(count), "-w", strconv.Itoa(int(timeout / time.Millisecond))}
	case "darwin":
		args = []string{"-n", "-c", strconv.Itoa(count), "-W", timeoutSec + "000"}
	default:
		args = []string{"-n", "-c", strconv.Itoa(count), "-W", timeoutSec}
	}
	args = append(args, clean)

	// Give the command enough time for every probe plus process startup.
	runner := &execx.ExecRunner{Timeout: time.Duration(count)*timeout + 5*time.Second}
	start := time.Now()
	out := runner.Run(ctx, cmdName, args...)
	res.DurationMs = float64(time.Since(start).Microseconds()) / 1000.0
	res.Output = out.Output()
	var samples []float64
	res.Received, samples = ParsePingOutput(res.Output, count)
	if len(samples) > 0 {
		res.Received = len(samples)
		sum := 0.0
		res.MinMs = samples[0]
		res.MaxMs = samples[0]
		for _, s := range samples {
			sum += s
			if s < res.MinMs {
				res.MinMs = s
			}
			if s > res.MaxMs {
				res.MaxMs = s
			}
		}
		res.AvgMs = sum / float64(len(samples))
	}
	if res.Sent > 0 {
		res.PacketLoss = (float64(res.Sent-res.Received) / float64(res.Sent)) * 100
		if res.PacketLoss < 0 {
			res.PacketLoss = 0
		}
	}
	if out.Failed() && res.Received == 0 {
		res.Error = strings.TrimSpace(out.Err)
		if res.Error == "" {
			res.Error = execx.FirstLine(out.Output())
		}
		if res.Error == "" {
			res.Error = "ping failed"
		}
	}
	return res
}

// ParsePingOutput extracts every RTT sample from Linux, macOS or Windows ping
// output. It is exported so the parser can be tested without sending packets.
func ParsePingOutput(output string, sent int) (received int, samples []float64) {
	for _, m := range pingTimeRe.FindAllStringSubmatch(output, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			samples = append(samples, v)
		}
	}
	received = len(samples)
	if received > sent {
		received = sent
	}
	return received, samples
}

// LookupDNS resolves host for the requested record type (A, AAAA, CNAME, MX, NS,
// TXT, PTR or ALL). Records are deduplicated and sorted for stable output.
func LookupDNS(ctx context.Context, host, recordType string) DNSResult {
	clean, err := ValidateHost(host)
	res := DNSResult{Host: host, Type: strings.ToUpper(strings.TrimSpace(recordType))}
	if res.Type == "" {
		res.Type = "A"
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Host = clean
	clean = strings.Trim(clean, "[]")
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 8*time.Second {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
	}

	start := time.Now()
	defer func() { res.DurationMs = float64(time.Since(start).Microseconds()) / 1000.0 }()

	add := func(values ...string) {
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				res.Records = append(res.Records, v)
			}
		}
	}

	lookupType := func(kind string) error {
		switch kind {
		case "A", "AAAA":
			network := "ip4"
			if kind == "AAAA" {
				network = "ip6"
			}
			ips, e := net.DefaultResolver.LookupIP(ctx, network, clean)
			if e != nil {
				return e
			}
			for _, ip := range ips {
				add(ip.String())
			}
		case "CNAME":
			cname, e := net.DefaultResolver.LookupCNAME(ctx, clean)
			if e != nil {
				return e
			}
			add(cname)
		case "MX":
			mxs, e := net.DefaultResolver.LookupMX(ctx, clean)
			if e != nil {
				return e
			}
			for _, mx := range mxs {
				add(fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
			}
		case "NS":
			nss, e := net.DefaultResolver.LookupNS(ctx, clean)
			if e != nil {
				return e
			}
			for _, ns := range nss {
				add(strings.TrimSuffix(ns.Host, "."))
			}
		case "TXT":
			txts, e := net.DefaultResolver.LookupTXT(ctx, clean)
			if e != nil {
				return e
			}
			add(txts...)
		case "PTR":
			names, e := net.DefaultResolver.LookupAddr(ctx, clean)
			if e != nil {
				return e
			}
			for _, n := range names {
				add(strings.TrimSuffix(n, "."))
			}
		default:
			return fmt.Errorf("unsupported record type %q", kind)
		}
		return nil
	}

	if res.Type == "ALL" || res.Type == "ANY" {
		if ips, e := net.DefaultResolver.LookupIPAddr(ctx, clean); e == nil {
			for _, ip := range ips {
				add(ip.IP.String())
			}
		}
		for _, kind := range []string{"CNAME", "MX", "NS", "TXT"} {
			_ = lookupType(kind)
		}
	} else if err := lookupType(res.Type); err != nil {
		res.Error = err.Error()
	}

	sort.Strings(res.Records)
	res.Records = dedupe(res.Records)
	if res.Records == nil {
		res.Records = []string{}
	}
	return res
}

func dedupe(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

// CheckPort attempts a TCP connection to host:port and measures the handshake.
func CheckPort(ctx context.Context, host string, port int, timeout time.Duration) PortResult {
	res := PortResult{Host: strings.TrimSpace(host), Port: port}
	clean, err := ValidateHost(host)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if port < 1 || port > 65535 {
		res.Error = "port must be between 1 and 65535"
		return res
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	dialHost := strings.Trim(clean, "[]")
	addr := net.JoinHostPort(dialHost, strconv.Itoa(port))
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, dialErr := d.DialContext(ctx, "tcp", addr)
	res.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
	if dialErr != nil {
		res.Error = dialErr.Error()
		return res
	}
	_ = conn.Close()
	res.Open = true
	return res
}

// ParseMAC accepts colon-, dash-, dot- or space-separated MAC addresses.
func ParseMAC(value string) (net.HardwareAddr, error) {
	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.TrimSpace(value))
	if len(clean) != 12 {
		return nil, fmt.Errorf("invalid MAC address %q: expected 6 bytes", value)
	}
	b, err := hex.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid MAC address %q: %w", value, err)
	}
	return net.HardwareAddr(b), nil
}

// BuildMagicPacket returns the 102-byte Wake-on-LAN payload: six 0xFF bytes
// followed by the target MAC repeated sixteen times.
func BuildMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("invalid MAC length %d: expected 6 bytes", len(mac))
	}
	packet := make([]byte, 6+16*6)
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 1; i <= 16; i++ {
		copy(packet[i*6:(i+1)*6], mac)
	}
	return packet, nil
}

// WakeOnLAN broadcasts a magic packet for mac. An empty broadcast address uses
// 255.255.255.255 and a zero port uses the standard WoL port 9.
func WakeOnLAN(mac net.HardwareAddr, broadcast string, port int) error {
	packet, err := BuildMagicPacket(mac)
	if err != nil {
		return err
	}
	if port <= 0 || port > 65535 {
		port = 9
	}
	broadcast = strings.TrimSpace(broadcast)
	if broadcast == "" {
		broadcast = "255.255.255.255"
	}
	ip := net.ParseIP(strings.Trim(broadcast, "[]"))
	if ip == nil {
		return fmt.Errorf("invalid broadcast address %q", broadcast)
	}
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		return fmt.Errorf("open Wake-on-LAN socket: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write(packet); err != nil {
		return fmt.Errorf("send Wake-on-LAN packet: %w", err)
	}
	return nil
}
