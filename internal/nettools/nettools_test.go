package nettools

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateHost(t *testing.T) {
	valid := []string{"example.com", "1.1.1.1", "2606:4700:4700::1111", "[::1]", "my-host.local."}
	for _, h := range valid {
		if _, err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) = %v, want nil", h, err)
		}
	}
	invalid := []string{"", "   ", "-f", "--help", "example.com; rm -rf /", "a b", "http://example.com"}
	for _, h := range invalid {
		if _, err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) = nil, want error", h)
		}
	}
}

func TestParsePingOutput(t *testing.T) {
	linux := "64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=12.3 ms\n" +
		"64 bytes from 1.1.1.1: icmp_seq=2 ttl=57 time=11.1 ms\n" +
		"64 bytes from 1.1.1.1: icmp_seq=3 ttl=57 time=13.7 ms\n"
	received, samples := ParsePingOutput(linux, 4)
	if received != 3 {
		t.Fatalf("received = %d, want 3", received)
	}
	if len(samples) != 3 || samples[0] != 12.3 || samples[2] != 13.7 {
		t.Fatalf("samples = %v", samples)
	}

	windows := "Reply from 8.8.8.8: bytes=32 time=24ms TTL=118\r\nReply from 8.8.8.8: bytes=32 time<1ms TTL=118\r\n"
	received, samples = ParsePingOutput(windows, 2)
	if received != 2 || samples[0] != 24 || samples[1] != 1 {
		t.Fatalf("windows parse = %d %v", received, samples)
	}

	received, samples = ParsePingOutput("Request timed out.", 3)
	if received != 0 || len(samples) != 0 {
		t.Fatalf("timeout parse = %d %v", received, samples)
	}
}

func TestParseMAC(t *testing.T) {
	for _, in := range []string{"00:11:22:33:44:55", "00-11-22-33-44-55", "0011.2233.4455", "00 11 22 33 44 55"} {
		mac, err := ParseMAC(in)
		if err != nil {
			t.Fatalf("ParseMAC(%q) error: %v", in, err)
		}
		if mac.String() != "00:11:22:33:44:55" {
			t.Fatalf("ParseMAC(%q) = %s", in, mac)
		}
	}
	for _, in := range []string{"", "00:11:22", "zz:11:22:33:44:55"} {
		if _, err := ParseMAC(in); err == nil {
			t.Errorf("ParseMAC(%q) = nil error, want failure", in)
		}
	}
}

func TestBuildMagicPacket(t *testing.T) {
	mac, _ := ParseMAC("00:11:22:33:44:55")
	packet, err := BuildMagicPacket(mac)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 102 {
		t.Fatalf("packet length = %d, want 102", len(packet))
	}
	for i := 0; i < 6; i++ {
		if packet[i] != 0xff {
			t.Fatalf("header byte %d = %#x, want 0xff", i, packet[i])
		}
	}
	for rep := 1; rep <= 16; rep++ {
		start := rep * 6
		if string(packet[start:start+6]) != string(mac) {
			t.Fatalf("repetition %d = %x, want %x", rep, packet[start:start+6], []byte(mac))
		}
	}
	if _, err := BuildMagicPacket(make(net.HardwareAddr, 4)); err == nil {
		t.Fatal("BuildMagicPacket accepted a short MAC")
	}
}

func TestCheckPortAgainstLocalListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	res := CheckPort(context.Background(), "127.0.0.1", port, time.Second)
	if !res.Open || res.Error != "" {
		t.Fatalf("CheckPort open = %+v", res)
	}

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	res = CheckPort(context.Background(), "127.0.0.1", closedPort, 250*time.Millisecond)
	if res.Open {
		t.Fatalf("CheckPort on closed port = %+v", res)
	}

	if got := CheckPort(context.Background(), "127.0.0.1", 0, time.Second); got.Error == "" {
		t.Fatal("CheckPort accepted port 0")
	}
}

func TestLookupDNSLocalhost(t *testing.T) {
	res := LookupDNS(context.Background(), "localhost", "A")
	if res.Error != "" {
		t.Fatalf("LookupDNS(localhost) error: %s", res.Error)
	}
	found := false
	for _, r := range res.Records {
		if strings.Contains(r, "127.0.0.1") || r == "::1" {
			found = true
		}
	}
	if !found && len(res.Records) == 0 {
		t.Logf("no localhost records returned (resolver configuration): %+v", res)
	}

	if res := LookupDNS(context.Background(), "localhost", "BOGUS"); res.Error == "" {
		t.Fatal("LookupDNS accepted an unsupported record type")
	}
}

func TestPingValidation(t *testing.T) {
	res := Ping(context.Background(), "-c", 1, time.Second)
	if res.Error == "" {
		t.Fatal("Ping accepted a flag-like host")
	}
	if res.Sent != 1 {
		t.Fatalf("Sent = %d, want 1", res.Sent)
	}
}

func TestClampTimeout(t *testing.T) {
	if got := clampTimeout(0, 3*time.Second, 30*time.Second); got != 3*time.Second {
		t.Fatalf("clampTimeout default = %v", got)
	}
	if got := clampTimeout(-5, 3*time.Second, 30*time.Second); got != 3*time.Second {
		t.Fatalf("clampTimeout negative = %v", got)
	}
	if got := clampTimeout(60000, 3*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("clampTimeout max = %v", got)
	}
	if got := clampTimeout(100, 3*time.Second, 30*time.Second); got != 100*time.Millisecond {
		t.Fatalf("clampTimeout explicit = %v", got)
	}
	_ = strconv.Itoa(0)
}
