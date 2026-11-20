package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"netpilot/internal/apps"
	"netpilot/internal/engine"
	"netpilot/internal/procnet"
)

type processInspection struct {
	At              time.Time                `json:"at"`
	Process         apps.ProcessInfo         `json:"process"`
	Sockets         []procnet.Socket         `json:"sockets"`
	SocketError     string                   `json:"socketError,omitempty"`
	DNSEndpoints    []string                 `json:"dnsEndpoints"`
	SystemResolvers []string                 `json:"systemResolvers"`
	SavedRuleDNS    string                   `json:"savedRuleDns,omitempty"`
	Activity        []engine.ProcessActivity `json:"activity"`
	ActivityEnabled bool                     `json:"activityEnabled"`
}

func (s *NetPilotService) serveProcessInspection(w http.ResponseWriter, r *http.Request) {
	var selection struct {
		PID        int    `json:"pid"`
		Name       string `json:"name"`
		Executable string `json:"executable"`
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
	for _, p := range s.DiscoverApplications() {
		if p.PID != selection.PID {
			continue
		}
		same := p.Name == selection.Name && p.Executable == selection.Executable
		if runtime.GOOS == "windows" {
			same = strings.EqualFold(p.Name, selection.Name) && strings.EqualFold(p.Executable, selection.Executable)
		}
		if same {
			_ = json.NewEncoder(w).Encode(s.inspectProcess(r.Context(), p))
			return
		}
		break
	}
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "process exited or changed; select it again from Active Processes"})
}

func (s *NetPilotService) inspectProcess(ctx context.Context, p apps.ProcessInfo) processInspection {
	ctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	// Avoid returning launch arguments, which can contain passwords or tokens.
	p.CommandLine = ""
	snapshot := processInspection{At: time.Now().UTC(), Process: p, DNSEndpoints: []string{}, Activity: []engine.ProcessActivity{}}
	resolvers := procnet.SystemResolvers()
	sort.Strings(resolvers)
	snapshot.SystemResolvers = append([]string{}, slices.Compact(resolvers)...)
	sockets, err := procnet.Sockets(ctx, p.PID)
	snapshot.Sockets = sockets
	if err != nil {
		snapshot.SocketError = "Socket table incomplete: " + err.Error()
	}
	if s.ruleEngine != nil {
		snapshot.SavedRuleDNS = s.ruleEngine.Evaluate(p.Name, p.Executable, "", p.AssignedNIC).CustomDNS
	}
	if s.netmergeCtrl != nil {
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
