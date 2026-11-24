package netmerge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"netpilot/internal/engine"
)

const (
	policyBridgeTimeout = time.Second
	policyMessageLimit  = 32 << 10
)

type policyLookup struct {
	Name, Path, Destination string
}

type policyUsage struct {
	RuleID  string
	In, Out int64
}

type policyActivity struct {
	PID     int
	Entries []engine.ProcessActivity
}

type policyServer struct {
	socket string
	server *http.Server

	mu          sync.Mutex
	activity    policyActivity
	inspectTill time.Time
}

// The Linux desktop keeps ownership of its rule database. The privileged tunnel
// asks it for decisions over a Unix socket inside a user-private directory.
func startPolicyServer(resolver engine.PolicyResolver) (*policyServer, error) {
	dir, err := os.MkdirTemp("/tmp", "netpilot-policy-")
	if err != nil {
		return nil, err
	}
	socket := filepath.Join(dir, "rules.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		_ = os.Remove(dir)
		return nil, err
	}
	bridge := &policyServer{socket: socket}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /resolve", func(w http.ResponseWriter, r *http.Request) {
		var lookup policyLookup
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, policyMessageLimit)).Decode(&lookup); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(resolver.Resolve(lookup.Name, lookup.Path, lookup.Destination))
	})
	mux.HandleFunc("POST /usage", func(w http.ResponseWriter, r *http.Request) {
		var usage policyUsage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, policyMessageLimit)).Decode(&usage); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resolver.ReportUsage(engine.FlowPolicy{RuleID: usage.RuleID}, usage.In, usage.Out)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /activity", func(w http.ResponseWriter, r *http.Request) {
		var activity policyActivity
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&activity); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(bridge.updateActivity(activity))
	})
	bridge.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  policyBridgeTimeout,
		WriteTimeout: policyBridgeTimeout,
		IdleTimeout:  15 * time.Second,
	}
	go bridge.server.Serve(listener)
	return bridge, nil
}

func (s *policyServer) close() {
	_ = s.server.Close()
	_ = os.Remove(filepath.Dir(s.socket))
}

func (s *policyServer) inspect(pid int) []engine.ProcessActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.activity.PID != pid || !now.Before(s.inspectTill) {
		s.activity = policyActivity{PID: pid}
	}
	s.inspectTill = now.Add(12 * time.Second)
	return append([]engine.ProcessActivity{}, s.activity.Entries...)
}

func (s *policyServer) updateActivity(activity policyActivity) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !time.Now().Before(s.inspectTill) {
		s.activity = policyActivity{}
	} else if activity.PID == s.activity.PID {
		s.activity = activity
	}
	return s.activity.PID
}

// PolicyClient lets a separately elevated NetPilot runner enforce the
// launching desktop's policies. All requests stay on the supplied Unix socket.
type PolicyClient struct {
	client       *http.Client
	inspectedPID int // only accessed by SyncActivity on the runner's status loop
}

func OpenPolicyClient(socket string) (*PolicyClient, error) {
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socket)
		},
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     10 * time.Second,
	}
	client := &PolicyClient{client: &http.Client{
		Transport: transport,
		Timeout:   policyBridgeTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	if err := client.Check(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *PolicyClient) Check() error {
	return c.request(http.MethodGet, "/ready", nil, nil)
}

// SyncActivity exchanges the selected PID and recent observations with the
// desktop. Traffic is observed only while the inspector renews its selection.
func (c *PolicyClient) SyncActivity(b *engine.Balancer) {
	activity := policyActivity{PID: c.inspectedPID}
	if activity.PID > 0 {
		activity.Entries = b.InspectProcess(activity.PID)
	}
	var pid int
	if c.request(http.MethodPost, "/activity", activity, &pid) == nil && pid != activity.PID {
		b.InspectProcess(pid)
		c.inspectedPID = pid
	}
}

func (c *PolicyClient) Resolve(name, path, destination string) engine.FlowPolicy {
	var policy engine.FlowPolicy
	if err := c.request(http.MethodPost, "/resolve", policyLookup{name, path, destination}, &policy); err != nil {
		return engine.FlowPolicy{Unavailable: true}
	}
	return policy
}

func (c *PolicyClient) ReportUsage(policy engine.FlowPolicy, in, out int64) {
	if in > 0 || out > 0 {
		_ = c.request(http.MethodPost, "/usage", policyUsage{policy.RuleID, in, out}, nil)
	}
}

func (c *PolicyClient) request(method, path string, payload, result any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, "http://netpilot"+path, body)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if result == nil && resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if result != nil && resp.StatusCode == http.StatusOK {
		return json.NewDecoder(io.LimitReader(resp.Body, policyMessageLimit)).Decode(result)
	}
	return fmt.Errorf("policy server returned HTTP %d", resp.StatusCode)
}
