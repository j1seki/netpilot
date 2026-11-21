package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/proxy"

	"netpilot/internal/db"
	"netpilot/internal/engine"
	"netpilot/internal/nettools"
	"netpilot/internal/nics"
	"netpilot/internal/rules"
	"netpilot/internal/speedtest"
)

func checkPolicyDNS() (TestStatus, string) {
	dns, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return StatusBlocked, "Loopback DNS listener: " + err.Error()
	}
	defer dns.Close()
	tcpDNS, err := net.Listen("tcp4", dns.LocalAddr().String())
	if err != nil {
		return StatusBlocked, "Loopback TCP DNS listener: " + err.Error()
	}
	defer tcpDNS.Close()
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback destination: " + err.Error()
	}
	defer target.Close()
	published := []byte("custom DNS selected")
	go func() {
		for _, listener := range []net.Listener{target, tcpDNS} {
			if conn, err := listener.Accept(); err == nil {
				_, _ = conn.Write(published)
				_ = conn.Close()
			}
		}
	}()
	go serveFixtureDNS(dns)

	database, err := db.Open(":memory:")
	if err != nil {
		return StatusFail, err.Error()
	}
	defer database.Close()
	store, err := rules.NewEngine(database)
	if err != nil {
		return StatusFail, err.Error()
	}
	rule := db.RuleRecord{ID: "dns", AppName: "diagnostic-client", DomainPattern: "*.fixture.test",
		PreferredInterface: "fixture", DataQuotaMB: 1, CustomDNS: dns.LocalAddr().String(), Enabled: true}
	if err := store.ValidateRule(rule); err != nil {
		return StatusFail, err.Error()
	}
	if err := database.SaveRule(rule); err != nil {
		return StatusFail, err.Error()
	}
	if err := store.Reload(); err != nil {
		return StatusFail, err.Error()
	}
	loopback := net.IPv4(127, 0, 0, 1)
	scheduler := engine.NewScheduler([]nics.Nic{{Name: "default", IP: loopback, Weight: 1}, {Name: "fixture", IP: loopback, Weight: 1}})
	scheduler.SetMode(engine.ModeBackup, "default", "fixture")
	balancer := engine.NewBalancer(scheduler)
	balancer.SetV6Mode(engine.V6Off)
	balancer.SetPolicyResolver(diagnosticPolicies{store})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	socks, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback proxy: " + err.Error()
	}
	defer socks.Close()
	go engine.ServeSocks5Context(ctx, socks, balancer)
	dialer, err := proxy.SOCKS5("tcp", socks.Addr().String(), nil, &net.Dialer{})
	if err != nil {
		return StatusFail, err.Error()
	}
	_, port, _ := net.SplitHostPort(target.Addr().String())
	conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", net.JoinHostPort("host.fixture.test", port))
	if err != nil {
		return StatusFail, "SOCKS hostname through custom DNS: " + err.Error()
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	data, err := io.ReadAll(conn)
	_ = conn.Close()
	if err != nil || !bytes.Equal(data, published) {
		return StatusFail, fmt.Sprintf("SOCKS DNS route reached the wrong endpoint: %v", err)
	}

	original := netip.MustParseAddrPort("192.0.2.1:53")
	policy := balancer.ResolvePolicy("diagnostic-client", "", original.Addr().String())
	conn, err = balancer.DialPinnedFor(ctx, original.Addr().String(), 53, policy)
	if err != nil {
		return StatusFail, "TCP DNS redirect: " + err.Error()
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	data, err = io.ReadAll(conn)
	_ = conn.Close()
	if err != nil || !bytes.Equal(data, published) {
		return StatusFail, fmt.Sprintf("TCP DNS redirect reached the wrong endpoint: %v", err)
	}

	policy.PreferredNIC = "missing"
	if udp, err := balancer.ListenUDPWithPolicy(ctx, engine.Fam4, original, policy); err == nil {
		_ = udp.Close()
		return StatusFail, "UDP escaped a strict pin without a fallback"
	}
	policy.FallbackNIC = "fixture"
	udp, err := balancer.ListenUDPWithPolicy(ctx, engine.Fam4, original, policy)
	if err != nil {
		return StatusFail, "UDP DNS fallback: " + err.Error()
	}
	defer udp.Close()
	_ = udp.SetDeadline(time.Now().Add(time.Second))
	question := dnsmessage.Question{Name: dnsmessage.MustNewName("host.fixture.test."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}
	query, _ := (&dnsmessage.Message{Header: dnsmessage.Header{ID: 17}, Questions: []dnsmessage.Question{question}}).Pack()
	if _, err := udp.WriteTo(query, net.UDPAddrFromAddrPort(original)); err != nil {
		return StatusFail, "UDP DNS request: " + err.Error()
	}
	packet := make([]byte, 512)
	n, source, err := udp.ReadFrom(packet)
	if err != nil || source.String() != original.String() {
		return StatusFail, fmt.Sprintf("UDP DNS reply did not retain the original resolver address: %v, %v", source, err)
	}
	var response dnsmessage.Message
	if err := response.Unpack(packet[:n]); err != nil || response.ID != 17 || len(response.Answers) != 1 {
		return StatusFail, "Custom DNS response was missing or malformed"
	}
	_ = udp.Close()
	list, err := database.ListRules()
	if err != nil || len(list) != 1 || list[0].DataUsedMB <= 0 {
		return StatusFail, "The matched rule's usage was not recorded"
	}
	store.RecordRuleUsage(rule.ID, 1<<20, 0)
	blocked := balancer.ResolvePolicy("diagnostic-client", "", original.Addr().String())
	if conn, err = balancer.DialPinnedFor(ctx, original.Addr().String(), 53, blocked); err == nil {
		_ = conn.Close()
		return StatusFail, "Reached rule quota allowed a new flow"
	}
	policy.CustomDNS = "2001:db8::53"
	if conn, err = balancer.DialPinnedFor(ctx, original.Addr().String(), 53, policy); err == nil {
		_ = conn.Close()
		return StatusFail, "IPv6-off policy allowed an IPv6 DNS resolver"
	}
	for _, slot := range scheduler.Snapshot() {
		if slot.Name == "default" && slot.Total != 0 {
			return StatusFail, "Rule traffic used the engine default instead of the pinned link"
		}
	}
	return StatusPass, "SOCKS hostname resolution and TCP/UDP DNS forwarding reached the configured DNS server, strict pin and fallback held, and quota and IPv6 policy blocked new flows"
}

func serveFixtureDNS(listener *net.UDPConn) {
	buffer := make([]byte, 2048)
	for {
		n, from, err := listener.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		var query dnsmessage.Message
		if query.Unpack(buffer[:n]) != nil || len(query.Questions) != 1 {
			continue
		}
		q := query.Questions[0]
		response := dnsmessage.Message{
			Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
			Questions: query.Questions,
		}
		if q.Type == dnsmessage.TypeA {
			response.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
			}}
		}
		if packet, err := response.Pack(); err == nil {
			_, _ = listener.WriteToUDP(packet, from)
		}
	}
}

func checkBenchmark() (TestStatus, string) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback benchmark server: " + err.Error()
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.Copy(io.Discard, r.Body)
			return
		}
		_, _ = w.Write(make([]byte, 64<<10))
	})}
	go server.Serve(listener)
	defer server.Close()
	url := "http://" + listener.Addr().String()

	opts := speedtest.Options{
		Duration: 1500 * time.Millisecond, Streams: 2,
		DownURL: url, UpURL: url, PingURL: url, MaxBytes: 1 << 20,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := speedtest.RunAdapter(ctx, speedtest.Adapter{Name: "fixture", IP: net.IPv4(127, 0, 0, 1)}, opts)
	if result.Error != "" || result.DownMbps <= 0 || result.UpMbps <= 0 || result.Grade == "" {
		return StatusFail, fmt.Sprintf("Download, upload or bufferbloat measurement missing: %+v", result)
	}
	socks, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback proxy listener: " + err.Error()
	}
	defer socks.Close()
	b := engine.NewBalancer(engine.NewScheduler([]nics.Nic{{Name: "fixture", IP: net.IPv4(127, 0, 0, 1), Weight: 1}}))
	go engine.ServeSocks5Context(ctx, socks, b)
	result = speedtest.RunProxy(ctx, socks.Addr().String(), opts)
	if result.Error != "" || result.DownMbps <= 0 || result.UpMbps <= 0 || result.Grade == "" {
		return StatusFail, fmt.Sprintf("SOCKS benchmark measurement missing: %+v", result)
	}
	return StatusPass, "Measured throughput and bufferbloat against a loopback HTTP server through direct and SOCKS paths"
}

func checkPortAndWake() (TestStatus, string) {
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, err.Error()
	}
	defer tcp.Close()
	port := tcp.Addr().(*net.TCPAddr).Port
	result := nettools.CheckPort(context.Background(), "127.0.0.1", port, time.Second)
	if !result.Open {
		return StatusFail, "TCP port tool did not detect an open loopback port"
	}
	_ = tcp.Close()
	if nettools.CheckPort(context.Background(), "127.0.0.1", port, time.Second).Open {
		return StatusFail, "TCP port tool detected a closed port as open"
	}
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return StatusBlocked, err.Error()
	}
	defer udp.Close()
	_ = udp.SetReadDeadline(time.Now().Add(time.Second))
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	if err := nettools.WakeOnLAN(mac, "127.0.0.1", udp.LocalAddr().(*net.UDPAddr).Port); err != nil {
		return StatusFail, err.Error()
	}
	packet := make([]byte, 128)
	n, _, err := udp.ReadFromUDP(packet)
	if err != nil || n != 102 || !bytes.Equal(packet[:6], bytes.Repeat([]byte{0xff}, 6)) ||
		!bytes.Equal(packet[6:n], bytes.Repeat(mac, 16)) {
		return StatusFail, fmt.Sprintf("Wrong Wake-on-LAN payload on loopback: %d bytes (%v)", n, err)
	}
	return StatusPass, "Distinguished an open and closed TCP port and received a correctly formed Wake-on-LAN packet on loopback"
}

func checkPing() (TestStatus, string) {
	if _, err := exec.LookPath("ping"); err != nil {
		return StatusBlocked, "The host ping command is unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := nettools.Ping(ctx, "127.0.0.1", 1, time.Second)
	if result.Error != "" || result.Received != 1 {
		return StatusBlocked, fmt.Sprintf("Loopback ICMP unavailable in this environment: %+v", result)
	}
	return StatusPass, "Sent and received a real loopback ICMP echo"
}
