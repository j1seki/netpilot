package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"netpilot/internal/branding"
	"netpilot/internal/procnet"
	"netpilot/internal/testrunner"
)

const diagnosticsPort = "4546"

var tailscaleIPv4 = netip.MustParsePrefix("100.64.0.0/10")

type diagnosticAddress struct {
	IP    string `json:"ip"`
	Label string `json:"label"`
	Mode  string `json:"mode"`
}

type diagnosticMCPStatus struct {
	Running     bool                `json:"running"`
	Endpoint    string              `json:"endpoint,omitempty"`
	Token       string              `json:"token,omitempty"`
	Certificate string              `json:"certificate,omitempty"`
	Addresses   []diagnosticAddress `json:"addresses"`
}

type diagnosticMCP struct {
	mu     sync.Mutex
	server *http.Server
	status diagnosticMCPStatus
}

func diagnosticAddresses() []diagnosticAddress {
	out := []diagnosticAddress{{IP: "127.0.0.1", Label: "This computer", Mode: "loopback"}}
	ifaces, _ := net.Interfaces()
	seen := map[string]bool{"127.0.0.1": true}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err != nil || !prefix.Addr().Is4() {
				continue
			}
			ip := prefix.Addr()
			mode := "lan"
			if tailscaleIPv4.Contains(ip) && strings.Contains(strings.ToLower(iface.Name), "tailscale") {
				mode = "tailscale"
			} else if !ip.IsPrivate() && !ip.IsLinkLocalUnicast() {
				continue
			}
			if !seen[ip.String()] {
				out = append(out, diagnosticAddress{IP: ip.String(), Label: iface.Name, Mode: mode})
				seen[ip.String()] = true
			}
		}
	}
	return out
}

func (m *diagnosticMCP) snapshot() diagnosticMCPStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.status
	status.Addresses = diagnosticAddresses()
	return status
}

func (m *diagnosticMCP) start(s *NetPilotService, address string) error {
	return m.listen(s, address, diagnosticsPort)
}

func (m *diagnosticMCP) listen(s *NetPilotService, address, port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return errors.New("stop the current endpoint before changing its address")
	}
	var bind diagnosticAddress
	for _, candidate := range diagnosticAddresses() {
		if candidate.IP == address {
			bind = candidate
			break
		}
	}
	if bind.IP == "" {
		return errors.New("choose an active local, Tailscale, or LAN address")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(secret[:])
	listener, err := net.Listen("tcp4", net.JoinHostPort(bind.IP, port))
	if err != nil {
		return err
	}
	host := listener.Addr().String()
	scheme, trustPEM := "http", ""
	if bind.Mode == "lan" {
		cert, ca, err := diagnosticTLS(net.ParseIP(bind.IP))
		if err != nil {
			_ = listener.Close()
			return err
		}
		listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		scheme, trustPEM = "https", ca
	}
	handler := s.diagnosticMCPHandler(host, token)
	if bind.Mode == "tailscale" {
		tailnetHandler := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			remote, err := netip.ParseAddrPort(r.RemoteAddr)
			if err != nil || !tailscaleIPv4.Contains(remote.Addr().Unmap()) {
				http.Error(w, "Tailscale peer required", http.StatusForbidden)
				return
			}
			tailnetHandler.ServeHTTP(w, r)
		})
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 85 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	m.server = server
	m.status = diagnosticMCPStatus{Running: true, Endpoint: scheme + "://" + host + "/mcp", Token: token, Certificate: trustPEM}
	go func() {
		_ = server.Serve(listener)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.server == server {
			m.server = nil
			m.status = diagnosticMCPStatus{}
		}
	}()
	return nil
}

func (m *diagnosticMCP) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		_ = m.server.Close()
	}
	m.server = nil
	m.status = diagnosticMCPStatus{}
}

// A short-lived local certificate lets native clients pin the LAN endpoint.
func diagnosticTLS(ip net.IP) (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, "", err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "NetPilot Diagnostics"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), IPAddresses: []net.IP{ip},
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), nil
}

func (s *NetPilotService) diagnosticMCPHandler(host, token string) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "netpilot-diagnostics", Version: branding.AppVersion}, nil)
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	type empty struct{}
	type statusOutput struct {
		Version    string                  `json:"version"`
		Mode       string                  `json:"mode"`
		Traffic    procnet.TrafficSnapshot `json:"traffic"`
		Forwarding bool                    `json:"forwarding"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "status", Description: "Read NetPilot version, execution mode, forwarding and host traffic source.", Annotations: readOnly},
		func(context.Context, *mcp.CallToolRequest, empty) (*mcp.CallToolResult, statusOutput, error) {
			return nil, statusOutput{branding.AppVersion, string(s.modeCtrl.Current()), s.hostTraffic.Snapshot(), s.netmergeCtrl.IsForwarding()}, nil
		})
	type adapter struct {
		Name      string   `json:"name"`
		Up        bool     `json:"up"`
		MTU       int      `json:"mtu"`
		Addresses []string `json:"addresses,omitempty"`
	}
	type adapterOutput struct {
		Adapters []adapter `json:"adapters"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "adapters", Description: "Read local adapter names, IP addresses, link state and MTU.", Annotations: readOnly},
		func(context.Context, *mcp.CallToolRequest, empty) (*mcp.CallToolResult, adapterOutput, error) {
			ifaces, err := net.Interfaces()
			out := adapterOutput{Adapters: []adapter{}}
			for _, iface := range ifaces {
				item := adapter{Name: iface.Name, Up: iface.Flags&net.FlagUp != 0, MTU: iface.MTU}
				addresses, _ := iface.Addrs()
				for _, addr := range addresses {
					item.Addresses = append(item.Addresses, addr.String())
				}
				out.Adapters = append(out.Adapters, item)
			}
			return nil, out, err
		})
	type processInput struct {
		PID int `json:"pid" jsonschema:"Local process ID from the Active Processes table"`
	}
	type socketOutput struct {
		Sockets []procnet.Socket `json:"sockets"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "process_sockets", Description: "Read TCP and UDP socket endpoints for one local process.", Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, input processInput) (*mcp.CallToolResult, socketOutput, error) {
			ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
			defer cancel()
			rows, err := procnet.Sockets(ctx, input.PID)
			return nil, socketOutput{Sockets: rows}, err
		})
	type testInput struct {
		Adapter  string `json:"adapter,omitempty" jsonschema:"Desktop interface name for the phone tether, or empty for detection"`
		SafeOnly bool   `json:"safeOnly,omitempty" jsonschema:"Skip disruption checks. The default is a full run."`
	}
	var runningTests sync.Mutex
	mcp.AddTool(server, &mcp.Tool{Name: "run_diagnostics", Description: "Run all automated NetPilot checks, including a failover test with real diagnostic sockets and isolated network failures. Returns the exportable report.", Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, input testInput) (*mcp.CallToolResult, testrunner.UnifiedTestReport, error) {
			if !runningTests.TryLock() {
				return nil, testrunner.UnifiedTestReport{}, errors.New("a diagnostic run is already in progress")
			}
			defer runningTests.Unlock()
			return nil, s.RunAllUnified(input.SafeOnly, input.Adapter), nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if r.Host != host || r.Header.Get("Origin") != "" {
			http.Error(w, "Host or origin not allowed", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		handler.ServeHTTP(w, r)
	})
}

func (s *NetPilotService) serveDiagnosticMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodPost {
		var body struct {
			Enabled bool   `json:"enabled"`
			Address string `json:"address"`
		}
		if !decodeAPIObject(w, r, &body) {
			return
		}
		if body.Enabled {
			if err := s.mcp.start(s, body.Address); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Could not start MCP: %v", err)})
				return
			}
		} else {
			s.mcp.stop()
		}
	} else if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(s.mcp.snapshot())
}
