package testrunner

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"netpilot/internal/hotspot"
	"netpilot/internal/nics"
	"netpilot/internal/procnet"
)

func checkStandbyTraffic() (TestStatus, string) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return StatusBlocked, "This platform has no host traffic sampler"
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, err.Error()
	}
	defer listener.Close()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		return StatusFail, err.Error()
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		return StatusFail, err.Error()
	}
	defer server.Close()
	go func() { _, _ = io.Copy(io.Discard, server) }()

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	var monitor procnet.TrafficMonitor
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitor.Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	var windowsTraffic <-chan bool
	if runtime.GOOS == "windows" {
		reachable := make(chan bool, 1)
		windowsTraffic = reachable
		go func() {
			dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 1200 * time.Millisecond}}
			ok := false
			// Some Windows builds omit loopback I/O from kernel traffic events.
			// Generate a few small TLS handshakes so the same check works off loopback.
			for range 3 {
				req, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
				conn, err := dialer.DialContext(req, "tcp4", "1.1.1.1:443")
				stop()
				if err == nil {
					ok = true
					_ = conn.Close()
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(600 * time.Millisecond):
				}
			}
			reachable <- ok
		}()
	}
	reachedOffLoopback := false
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	payload := make([]byte, 64<<10)
	for {
		select {
		case <-ctx.Done():
			last := monitor.Snapshot()
			if last.Source == "" {
				return StatusBlocked, last.Detail
			}
			if runtime.GOOS == "windows" {
				if reachedOffLoopback {
					return StatusFail, "Windows ETW reported no NetPilot bytes for the automated loopback transfer or external TLS traffic"
				}
				return StatusBlocked, "Windows ETW produced no loopback events and the external traffic probe was unavailable"
			}
			return StatusFail, "Linux did not report bytes from the local test stream"
		case <-done:
			return StatusBlocked, monitor.Snapshot().Detail
		case reachedOffLoopback = <-windowsTraffic:
			windowsTraffic = nil
		case <-ticker.C:
			_ = client.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
			if _, err := client.Write(payload); err != nil {
				return StatusFail, err.Error()
			}
			if got := monitor.Snapshot(); got.Rates[os.Getpid()] > 0 {
				return StatusPass, fmt.Sprintf("%s measured %.3f Mbps for a real TCP transfer without the NetPilot engine", got.Source, got.Rates[os.Getpid()])
			}
		}
	}
}

func checkProcessSockets() (TestStatus, string) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return StatusBlocked, "Loopback listener could not start: " + err.Error()
	}
	defer listener.Close()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		return StatusFail, "Could not establish a loopback TCP connection: " + err.Error()
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := procnet.Sockets(ctx, os.Getpid())
	for _, row := range rows {
		if row.Protocol == "TCP" && row.Local == client.LocalAddr().String() && row.Remote == listener.Addr().String() {
			return StatusPass, "The inspector found both endpoints of a real loopback connection owned by NetPilot"
		}
	}
	if err != nil {
		return StatusBlocked, err.Error()
	}
	return StatusFail, "The inspector did not find the test connection in the current process socket table"
}

func checkTetherLink(adapterName string) (TestStatus, string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return StatusBlocked, "Could not enumerate adapters: " + err.Error()
	}
	var candidates []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if adapterName != "" && iface.Name == adapterName {
			candidates = append(candidates, iface)
		} else if adapterName == "" && iface.Flags&net.FlagUp != 0 && hotspot.IsTether(iface.Name) {
			candidates = append(candidates, iface)
		}
	}
	if len(candidates) == 0 {
		if adapterName != "" {
			return StatusBlocked, "Selected adapter was not found on this host"
		}
		return StatusSkipped, "Auto-detection found no USB or cellular adapter. Select your PC’s Wi-Fi or USB adapter to test a specific hotspot connection."
	}
	if len(candidates) > 1 {
		return StatusSkipped, "Several USB or cellular adapters were detected. Choose the adapter connected to your hotspot to run this check."
	}
	iface := candidates[0]
	if iface.Flags&net.FlagUp == 0 {
		return StatusBlocked, "Selected adapter is down"
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return StatusBlocked, "Could not read the adapter addresses: " + err.Error()
	}
	var source net.IP
	for _, addr := range addrs {
		ip, _, _ := net.ParseCIDR(addr.String())
		if ip = ip.To4(); ip != nil && ip.IsGlobalUnicast() {
			source = ip
			break
		}
	}
	if source == nil {
		return StatusBlocked, "Adapter has no usable IPv4 address for this check"
	}
	dialer := net.Dialer{
		Timeout:   3 * time.Second,
		LocalAddr: &net.TCPAddr{IP: source},
		Control:   nics.ForceEgress(iface.Index, false),
	}
	start := time.Now()
	for _, target := range []string{"1.1.1.1:443", "8.8.8.8:443"} {
		conn, err := dialer.Dial("tcp4", target)
		if err == nil {
			_ = conn.Close()
			kind := "Selected adapter"
			if adapterName == "" {
				kind = "Name-matched candidate"
			}
			return StatusPass, fmt.Sprintf("%s %q reached %s over TCP in %d ms.", kind, iface.Name, target, time.Since(start).Milliseconds())
		}
	}
	return StatusFail, fmt.Sprintf("Adapter %q has IPv4 but could not connect to either IPv4 probe (TCP 443)", strings.TrimSpace(iface.Name))
}
