// Package apps discovers real running desktop applications and system processes on the host.
package apps

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcessInfo represents an actual running application on the operating system.
type ProcessInfo struct {
	PID                int       `json:"pid"`
	Name               string    `json:"name"`
	Executable         string    `json:"executable"`
	CommandLine        string    `json:"commandLine"`
	IconHint           string    `json:"iconHint"`
	BytesSent          int64     `json:"bytesSent"`
	BytesReceived      int64     `json:"bytesReceived"`
	CurrentMbps        float64   `json:"currentMbps"`
	AssignedNIC        string    `json:"assignedNic"`
	RuleMatched        string    `json:"ruleMatched,omitempty"`
	LastActive         time.Time `json:"lastActive"`
	SocketCount        int       `json:"socketCount"`
	CustomDNS          string    `json:"customDns,omitempty"`
	BandwidthLimitMbps float64   `json:"bandwidthLimitMbps,omitempty"`
	DataQuotaMB        float64   `json:"dataQuotaMb,omitempty"`
	DataUsedMB         float64   `json:"dataUsedMb,omitempty"`
	QuotaExceeded      bool      `json:"quotaExceeded,omitempty"`
}

// Manager tracks real operating system processes.
type Manager struct {
	mu        sync.RWMutex
	processes map[int]*ProcessInfo
}

// NewManager creates a real application manager.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[int]*ProcessInfo),
	}
}

// Discover lists active applications based on real OS inspection.
func (m *Manager) Discover() []ProcessInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	if runtime.GOOS == "linux" {
		m.discoverLinux()
	} else if runtime.GOOS == "windows" {
		m.discoverWindows()
	}

	out := make([]ProcessInfo, 0, len(m.processes))
	for _, p := range m.processes {
		out = append(out, *p)
	}
	return out
}

func (m *Manager) discoverLinux() {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	seen := make(map[int]bool)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}

		pidPath := filepath.Join("/proc", e.Name())

		// Read real process name from /proc/[pid]/comm
		commBytes, err := os.ReadFile(filepath.Join(pidPath, "comm"))
		if err != nil {
			continue
		}
		commName := strings.TrimSpace(string(commBytes))

		// Filter out kernel worker threads
		if strings.HasPrefix(commName, "kworker") || strings.HasPrefix(commName, "rcu_") || strings.HasPrefix(commName, "cpuhp") {
			continue
		}

		// Read exe link if accessible
		exePath, err := os.Readlink(filepath.Join(pidPath, "exe"))
		if err != nil {
			exePath = commName
		}

		// Read cmdline
		cmdBytes, _ := os.ReadFile(filepath.Join(pidPath, "cmdline"))
		cmdLine := strings.ReplaceAll(string(cmdBytes), "\x00", " ")
		cmdLine = strings.TrimSpace(cmdLine)
		if cmdLine == "" {
			cmdLine = commName
		}

		// Count open sockets in /proc/[pid]/fd
		socketCount := 0
		if fdEntries, err := os.ReadDir(filepath.Join(pidPath, "fd")); err == nil {
			for _, fd := range fdEntries {
				if target, err := os.Readlink(filepath.Join(pidPath, "fd", fd.Name())); err == nil {
					if strings.HasPrefix(target, "socket:[") {
						socketCount++
					}
				}
			}
		}

		seen[pid] = true
		if p, ok := m.processes[pid]; ok {
			p.LastActive = time.Now()
			p.SocketCount = socketCount
		} else {
			m.processes[pid] = &ProcessInfo{
				PID:         pid,
				Name:        commName,
				Executable:  exePath,
				CommandLine: cmdLine,
				IconHint:    inferIconHint(commName),
				SocketCount: socketCount,
				AssignedNIC: "eth0", // default live uplink
				LastActive:  time.Now(),
			}
		}
	}

	// Purge dead processes
	for pid := range m.processes {
		if !seen[pid] {
			delete(m.processes, pid)
		}
	}
}

// GetSocketCount returns the total number of network sockets opened by tracked processes.
func (m *Manager) TotalSockets() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, p := range m.processes {
		total += p.SocketCount
	}
	return total
}

func inferIconHint(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "chrome") || strings.Contains(lower, "chromium") || strings.Contains(lower, "firefox") || strings.Contains(lower, "browser"):
		return "globe"
	case strings.Contains(lower, "tailscale") || strings.Contains(lower, "vpn") || strings.Contains(lower, "tunnel"):
		return "shield"
	case strings.Contains(lower, "curl") || strings.Contains(lower, "wget") || strings.Contains(lower, "git"):
		return "terminal"
	case strings.Contains(lower, "node") || strings.Contains(lower, "python") || strings.Contains(lower, "go"):
		return "code"
	case strings.Contains(lower, "ssh") || strings.Contains(lower, "dbus") || strings.Contains(lower, "daemon"):
		return "server"
	default:
		return "cpu"
	}
}
