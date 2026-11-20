package testrunner

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"netpilot/internal/branding"
	"netpilot/internal/db"
	"netpilot/internal/engine"
	"netpilot/internal/nettools"
	"netpilot/internal/nics"
	"netpilot/internal/procnet"
	"netpilot/internal/rules"
	"netpilot/internal/ttlmgr"
	"netpilot/internal/update"
)

type diagnosticPolicies struct{ rules *rules.Engine }

func (p diagnosticPolicies) Resolve(appName, appPath, destination string) engine.FlowPolicy {
	decision := p.rules.Evaluate(appName, appPath, destination, "default")
	return engine.FlowPolicy{PreferredNIC: decision.SelectedInterface, FallbackNIC: decision.FallbackInterface,
		BandwidthLimitMbps: decision.BandwidthLimitMbps, QuotaExceeded: decision.QuotaExceeded, RuleID: decision.MatchedRuleID}
}

func (p diagnosticPolicies) ReportUsage(app string, in, out int64) { p.rules.RecordUsage(app, in, out) }

func checkProxyRulesAndCap() (TestStatus, string) {
	database, err := db.Open(":memory:")
	if err != nil {
		return StatusFail, err.Error()
	}
	defer database.Close()
	const capMbps = 2
	rule := db.RuleRecord{ID: "diagnostic-rule", Enabled: true, DomainPattern: "127.0.0.1",
		PreferredInterface: "preferred", BandwidthLimitMbps: capMbps}
	if err := database.SaveRule(rule); err != nil {
		return StatusFail, err.Error()
	}
	policies, err := rules.NewEngine(database)
	if err != nil {
		return StatusFail, err.Error()
	}
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback listener: " + err.Error()
	}
	defer target.Close()
	boundProxy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "SOCKS listener: " + err.Error()
	}
	defer boundProxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const payloadSize = 128 << 10
	go func() {
		c, err := target.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		data := make([]byte, payloadSize)
		_ = c.SetWriteDeadline(time.Now().Add(4 * time.Second))
		_, _ = c.Write(data)
	}()

	loopback := net.IPv4(127, 0, 0, 1)
	sched := engine.NewScheduler([]nics.Nic{
		{Name: "default", IP: loopback, Weight: 1},
		{Name: "preferred", IP: loopback, Weight: 1},
	})
	sched.SetMode(engine.ModeBackup, "default", "preferred")
	balancer := engine.NewBalancer(sched)
	balancer.SetPolicyResolver(diagnosticPolicies{policies})
	go engine.ServeSocks5Context(ctx, boundProxy, balancer)
	dialer, err := proxy.SOCKS5("tcp", boundProxy.Addr().String(), nil, &net.Dialer{Timeout: 2 * time.Second})
	if err != nil {
		return StatusFail, err.Error()
	}
	begin := time.Now()
	conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", target.Addr().String())
	if err != nil {
		return StatusFail, "SOCKS CONNECT: " + err.Error()
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	size, err := io.Copy(io.Discard, conn)
	duration := time.Since(begin)
	if err != nil || size != payloadSize {
		return StatusFail, fmt.Sprintf("SOCKS delivered %d/%d bytes: %v", size, payloadSize, err)
	}
	for _, slot := range sched.Snapshot() {
		if slot.Name != "preferred" || slot.Total != 1 {
			continue
		}
		if duration < 330*time.Millisecond {
			return StatusFail, fmt.Sprintf("rule selected the preferred link, but the %d Mbit/s cap did not pace %d bytes (%s)", capMbps, size, duration)
		}
		return StatusPass, fmt.Sprintf("SOCKS CONNECT followed a saved routing rule and transferred %d bytes through its preferred adapter in %d ms with a %d Mbit/s cap", size, duration.Milliseconds(), capMbps)
	}
	return StatusFail, "SOCKS worked, but the preferred route from the stored rule did not carry the transfer"
}

func checkLiveTTLInspection() (TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-ttl-preview-")
	if err != nil {
		return StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	manager := ttlmgr.NewManager(ttlmgr.NewStore(dir), ttlmgr.NewBackend(ttlmgr.NewExecRunner()))
	report, err := manager.Inspect(ctx, nil)
	if err != nil {
		return StatusBlocked, "Cannot inspect host TTL settings: " + err.Error()
	}
	if !report.Capabilities.Readable {
		return StatusBlocked, strings.Join(report.Capabilities.Notes, " ")
	}
	for _, setting := range report.Settings {
		if setting.Kind == ttlmgr.KindPacketTTL && setting.Value != nil {
			return StatusPass, fmt.Sprintf("Read IPv4 TTL %d from the host through the platform backend", *setting.Value)
		}
	}
	if len(report.ReadErrors) > 0 {
		return StatusBlocked, report.ReadErrors[0].Message
	}
	return StatusFail, "The backend reported readable TTL but produced no current value"
}

func checkDNS() (TestStatus, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	dns := nettools.LookupDNS(ctx, "localhost", "A")
	if dns.Error != "" {
		return StatusBlocked, "Local DNS lookup: " + dns.Error
	}
	if len(dns.Records) == 0 {
		return StatusFail, "Local DNS succeeded without returning an IPv4 record"
	}
	return StatusPass, fmt.Sprintf("OS resolver returned %d IPv4 record(s) for localhost", len(dns.Records))
}

func checkUpdateFeed() (TestStatus, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	settings := update.DefaultSettings(branding.AppVersion)
	release, err := update.LatestRelease(ctx, settings.Channel)
	if err != nil {
		return StatusBlocked, "Update metadata could not be fetched: " + err.Error()
	}
	if _, ok := release.Version(); !ok {
		return StatusFail, "Release metadata returned an invalid version"
	}
	return StatusPass, fmt.Sprintf("Fetched and validated %s update metadata", settings.Channel)
}

func checkHostProcessIdentity() (TestStatus, string) {
	identity := procnet.Identify(os.Getpid())
	if !identity.Resolved || identity.Name == "" {
		return StatusFail, fmt.Sprintf("Could not resolve the diagnostic process on %s", runtime.GOOS)
	}
	return StatusPass, "Resolved the running NetPilot PID and executable name through the OS process inspector"
}

// ttlTestBackend keeps the write/verify/restore transaction separate from host settings.
// The operating-system backend is exercised by checkLiveTTLInspection.
type ttlTestBackend map[ttlmgr.SettingID]int

func (b ttlTestBackend) Name() string { return "diagnostic-fixture" }

func (b ttlTestBackend) Capabilities(context.Context) ttlmgr.Capabilities {
	return ttlmgr.Capabilities{Platform: runtime.GOOS, Readable: true, Supported: true}
}

func (b ttlTestBackend) Read(_ context.Context, ref ttlmgr.SettingRef) (ttlmgr.Observed, error) {
	value, ok := b[ref.ID]
	if !ok {
		return ttlmgr.Observed{}, nil
	}
	return ttlmgr.Observed{Value: &value, Explicit: true, Origin: b.Name()}, nil
}

func (b ttlTestBackend) Write(_ context.Context, ref ttlmgr.SettingRef, value int) error {
	b[ref.ID] = value
	return nil
}

func (b ttlTestBackend) Unset(_ context.Context, ref ttlmgr.SettingRef) error {
	delete(b, ref.ID)
	return nil
}

func checkTTLTransaction() (TestStatus, string) {
	dir, err := os.MkdirTemp("", "netpilot-ttl-transaction-")
	if err != nil {
		return StatusFail, err.Error()
	}
	defer os.RemoveAll(dir)
	backend := ttlTestBackend{ttlmgr.IDv4Default: 64, ttlmgr.IDv6Default: 128}
	store := ttlmgr.NewStore(dir)
	manager := ttlmgr.NewManager(store, backend)
	cfg := ttlmgr.DefaultConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	preview, err := manager.Apply(ctx, ttlmgr.ApplyRequest{DryRun: true, Override: &cfg})
	if err != nil || len(preview.Changes) != 2 || preview.Applied != 0 || backend[ttlmgr.IDv4Default] != 64 || backend[ttlmgr.IDv6Default] != 128 {
		return StatusFail, fmt.Sprintf("TTL preview returned an unexpected plan or changed values: %v", err)
	}
	applied, err := manager.Apply(ctx, ttlmgr.ApplyRequest{Confirm: true, Override: &cfg})
	if err != nil || applied.Applied != 2 || backend[ttlmgr.IDv4Default] != cfg.BumpIPv4 || backend[ttlmgr.IDv6Default] != cfg.BumpIPv6 {
		return StatusFail, fmt.Sprintf("TTL apply or readback failed: %v", err)
	}
	manager = ttlmgr.NewManager(ttlmgr.NewStore(dir), backend)
	restored, err := manager.Restore(ctx, ttlmgr.RestoreRequest{Confirm: true})
	if err != nil || restored.Applied != 2 || backend[ttlmgr.IDv4Default] != 64 || backend[ttlmgr.IDv6Default] != 128 {
		return StatusFail, fmt.Sprintf("TTL restore did not write the captured values: %v", err)
	}
	return StatusPass, "TTL manager previewed, wrote, verified and restored IPv4 TTL and IPv6 Hop Limit using a simulated adapter backend and a real snapshot file"
}
