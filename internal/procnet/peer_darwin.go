//go:build darwin

package procnet

import (
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func peerPID(conn net.Conn) (int, bool) {
	sc, ok := conn.(syscall.Conn)
	if ok {
		if raw, err := sc.SyscallConn(); err == nil {
			var pid int
			var got bool
			_ = raw.Control(func(fd uintptr) {
				// LOCAL_PEERPID is only supported for AF_UNIX sockets. Keep it
				// for local transports, but the SOCKS listener is TCP and must
				// fall back to matching its endpoints in lsof's TCP table.
				v, err := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
				if err == nil && v > 0 {
					pid, got = v, true
				}
			})
			if got {
				return pid, true
			}
		}
	}
	local, okL := conn.LocalAddr().(*net.TCPAddr)
	remote, okR := conn.RemoteAddr().(*net.TCPAddr)
	if !okL || !okR {
		return 0, false
	}
	// On the accepted side, the client's local endpoint is our remote endpoint;
	// the client's remote endpoint is our local endpoint.
	return lookupPID("tcp", remote.IP, uint16(remote.Port), local.IP, uint16(local.Port))
}

func processInfo(pid int) (string, string) {
	if name, path, ok := cachedProcess(pid); ok {
		return name, path
	}
	out, err := exec.Command("ps", "-p", itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", ""
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", ""
	}
	storeProcess(pid, name, name)
	return name, name
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

var tableCache = struct {
	sync.Mutex
	at  time.Time
	net string
	row []flowRow
}{}

type flowRow struct {
	local, remote string
	pid           int
}

const tableCacheTTL = time.Second

func lsofRows(network string) []flowRow {
	tableCache.Lock()
	defer tableCache.Unlock()
	if tableCache.row != nil && tableCache.net == network && time.Since(tableCache.at) < tableCacheTTL {
		return tableCache.row
	}
	proto := "-iTCP"
	args := []string{"-nP"}
	if network == "udp" {
		proto = "-iUDP"
		args = append(args, proto, "-F", "pcn")
	} else {
		args = append(args, proto, "-sTCP:ESTABLISHED", "-F", "pcn")
	}
	out, err := exec.Command("lsof", args...).Output()
	var rows []flowRow
	if err == nil {
		pid := 0
		for _, line := range strings.Split(string(out), "\n") {
			if len(line) < 2 {
				continue
			}
			switch line[0] {
			case 'p':
				pid = atoi(line[1:])
			case 'n':
				name := line[1:]
				parts := strings.SplitN(name, "->", 2)
				if len(parts) == 2 {
					rows = append(rows, flowRow{local: parts[0], remote: parts[1], pid: pid})
				}
			}
		}
	}
	tableCache.row, tableCache.at, tableCache.net = rows, time.Now(), network
	return rows
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func lookupPID(network string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	return lookupRows(lsofRows(network), localIP, localPort, remoteIP, remotePort)
}

func lookupRows(rows []flowRow, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	wantLocal := endpointString(localIP, localPort)
	wantRemote := endpointString(remoteIP, remotePort)
	for _, row := range rows {
		if !strings.EqualFold(row.local, wantLocal) {
			continue
		}
		if remoteIP != nil && remotePort != 0 && !strings.EqualFold(row.remote, wantRemote) {
			continue
		}
		if row.pid > 0 {
			return row.pid, true
		}
	}
	return 0, false
}

func endpointString(ip net.IP, port uint16) string {
	host := "*"
	if ip != nil {
		host = ip.String()
	}
	return host + ":" + uitoa(int(port))
}

func uitoa(n int) string { return itoa(n) }
