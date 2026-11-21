package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"netpilot/internal/apps"
	"netpilot/internal/engine"
	"netpilot/internal/nettools"
	"netpilot/internal/procnet"
	"netpilot/internal/rules"
)

type processInspection struct {
	At              time.Time                `json:"at"`
	Process         apps.ProcessInfo         `json:"process"`
	Sockets         []procnet.Socket         `json:"sockets"`
	SocketError     string                   `json:"socketError,omitempty"`
	DNSEndpoints    []string                 `json:"dnsEndpoints"`
	SystemResolvers []string                 `json:"systemResolvers"`
	RuleMatches     []rules.RuleMatch        `json:"ruleMatches"`
	RuleDestination string                   `json:"ruleDestination"`
	RuleScope       string                   `json:"ruleScope"`
	SavedRuleDNS    string                   `json:"savedRuleDns,omitempty"`
	Activity        []engine.ProcessActivity `json:"activity"`
	ActivityEnabled bool                     `json:"activityEnabled"`
}

func (s *NetPilotService) serveProcessInspection(w http.ResponseWriter, r *http.Request, liveTest bool) {
	var selection struct {
		PID         int    `json:"pid"`
		Name        string `json:"name"`
		Executable  string `json:"executable"`
		Destination string `json:"destination"`
	}
	if !decodeAPIObject(w, r, &selection) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if selection.PID <= 0 || selection.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "select a running process"})
		return
	}
	destination, err := rulePreviewDestination(selection.Destination)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if liveTest && destination == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "enter a hostname or IP address for the test"})
		return
	}
	for _, p := range s.DiscoverApplications() {
		if p.PID != selection.PID {
			continue
		}
		same := p.Name == selection.Name && p.Executable == selection.Executable
		if runtime.GOOS == "windows" {
			same = strings.EqualFold(p.Name, selection.Name) && strings.EqualFold(p.Executable, selection.Executable)
		}
		if same {
			if liveTest {
				s.serveRuleRouteTest(w, r, p, destination, selection.Destination)
			} else {
				_ = json.NewEncoder(w).Encode(s.inspectProcess(r.Context(), p, destination))
			}
			return
		}
		break
	}
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "process exited or changed; select it again from Active Processes"})
}

func (s *NetPilotService) serveRuleRouteTest(w http.ResponseWriter, r *http.Request, process apps.ProcessInfo, host, destination string) {
	port := 443
	if _, explicitPort, err := net.SplitHostPort(strings.TrimSpace(destination)); err == nil {
		port, err = strconv.Atoi(explicitPort)
		if err != nil || port < 1 || port > 65535 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "enter a TCP port from 1 to 65535"})
			return
		}
	}
	if s.netmergeCtrl == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "NetPilot forwarding is unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	probe, err := s.netmergeCtrl.TestRuleRoute(ctx, process.Name, process.Executable, host, port)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(probe)
}

func rulePreviewDestination(input string) (string, error) {
	host := strings.TrimSpace(input)
	if host == "" {
		return "", nil
	}
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().String(), nil
	}
	if strings.ContainsAny(host, ":[]") {
		return "", fmt.Errorf("enter a hostname or IP address to verify")
	}
	return nettools.ValidateHost(host)
}

func (s *NetPilotService) inspectProcess(ctx context.Context, p apps.ProcessInfo, destination string) processInspection {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	// Avoid returning launch arguments, which can contain passwords or tokens.
	p.CommandLine = ""
	snapshot := processInspection{
		At: time.Now().UTC(), Process: p, DNSEndpoints: []string{}, Activity: []engine.ProcessActivity{},
		RuleMatches: []rules.RuleMatch{}, RuleDestination: destination, RuleScope: "standby",
	}
	resolvers := procnet.SystemResolvers()
	sort.Strings(resolvers)
	snapshot.SystemResolvers = append([]string{}, slices.Compact(resolvers)...)
	sockets, err := procnet.Sockets(ctx, p.PID)
	snapshot.Sockets = sockets
	if err != nil {
		snapshot.SocketError = "Socket table incomplete: " + err.Error()
	}
	if s.ruleEngine != nil {
		snapshot.RuleMatches = s.ruleEngine.MatchRules(p.Name, p.Executable, destination)
		for _, match := range snapshot.RuleMatches {
			if match.Result == "selected" {
				snapshot.SavedRuleDNS = strings.TrimSpace(match.Rule.CustomDNS)
				break
			}
		}
	}
	if s.netmergeCtrl != nil {
		if status, err := s.netmergeCtrl.GetStatus(); err == nil {
			switch {
			case status.SystemAppRules:
				snapshot.RuleScope = "system"
			case status.SocksRunning:
				snapshot.RuleScope = "socks"
			case status.IsRunning:
				snapshot.RuleScope = "separate"
			}
		}
		if activity, ok := s.netmergeCtrl.InspectProcess(p.PID); ok {
			snapshot.Activity, snapshot.ActivityEnabled = activity, true
		}
	}
	seen := make(map[string]bool)
	addDNS := func(destination string) {
		_, port, err := net.SplitHostPort(destination)
		if err == nil && port == "53" && !seen[destination] {
			seen[destination] = true
			snapshot.DNSEndpoints = append(snapshot.DNSEndpoints, destination)
		}
	}
	for _, socket := range snapshot.Sockets {
		addDNS(socket.Remote)
	}
	for _, request := range snapshot.Activity {
		addDNS(request.Destination)
	}
	return snapshot
}
