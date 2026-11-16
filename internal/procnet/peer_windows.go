//go:build windows

package procnet

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"netpilot/internal/execx"
)

var winTable = struct {
	sync.Mutex
	at  time.Time
	row []flowRow
}{}

type flowRow struct {
	network string
	local   net.IP
	lport   uint16
	remote  net.IP
	rport   uint16
	pid     int
}

const winTableTTL = time.Second

// runSilent executes a Windows console tool without briefly showing a CMD window.
// This is the same CREATE_NO_WINDOW guarantee the rest of NetPilot uses through execx.
func runSilent(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	execx.PrepareCommand(cmd)
	return cmd.Output()
}

func parseWinEndpoint(s string) (net.IP, uint16, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "*:*" {
		return nil, 0, false
	}
	host, portStr := "", ""
	if strings.HasPrefix(s, "[") {
		end := strings.LastIndex(s, "]:")
		if end < 0 {
			return nil, 0, false
		}
		host, portStr = s[1:end], s[end+2:]
	} else {
		end := strings.LastIndex(s, ":")
		if end < 0 {
			return nil, 0, false
		}
		host, portStr = s[:end], s[end+1:]
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return nil, 0, false
	}
	return ip, uint16(port), true
}

func winRows() []flowRow {
	winTable.Lock()
	defer winTable.Unlock()
	if winTable.row != nil && time.Since(winTable.at) < winTableTTL {
		return winTable.row
	}
	out, err := runSilent("netstat", "-ano")
	var rows []flowRow
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			proto := strings.ToLower(fields[0])
			if proto != "tcp" && proto != "udp" {
				continue
			}
			lip, lport, ok := parseWinEndpoint(fields[1])
			if !ok {
				continue
			}
			rip, rport, _ := parseWinEndpoint(fields[2])
			pidStr := fields[len(fields)-1]
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}
			rows = append(rows, flowRow{network: proto, local: lip, lport: lport, remote: rip, rport: rport, pid: pid})
		}
	}
	winTable.row, winTable.at = rows, time.Now()
	return rows
}

func lookupPID(network string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	for _, row := range winRows() {
		if row.network != network {
			continue
		}
		if row.lport != localPort || !sameIP(row.local, localIP) {
			continue
		}
		if remoteIP != nil && remotePort != 0 && (row.rport != remotePort || !sameIP(row.remote, remoteIP)) {
			continue
		}
		if row.pid > 0 {
			return row.pid, true
		}
	}
	return 0, false
}

func peerPID(conn net.Conn) (int, bool) {
	local, okL := conn.LocalAddr().(*net.TCPAddr)
	remote, okR := conn.RemoteAddr().(*net.TCPAddr)
	if !okL || !okR {
		return 0, false
	}
	// The accepted connection's remote endpoint is the client's local endpoint, and our
	// local endpoint is its remote endpoint.
	return lookupPID("tcp", remote.IP, uint16(remote.Port), local.IP, uint16(local.Port))
}

func sameIP(a, b net.IP) bool { return a != nil && b != nil && a.Equal(b) }

func processInfo(pid int) (string, string) {
	if name, path, ok := cachedProcess(pid); ok {
		return name, path
	}
	out, err := runSilent("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	if err != nil {
		return "", ""
	}
	line := strings.TrimSpace(string(out))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return "", ""
	}
	fields := strings.Split(line, "\",\"")
	if len(fields) < 2 {
		return "", ""
	}
	name := strings.Trim(fields[0], "\"")
	if name == "" {
		return "", ""
	}
	storeProcess(pid, name, "")
	return name, ""
}
