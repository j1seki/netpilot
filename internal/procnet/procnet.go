// Package procnet maps network flows back to the process that owns them so NetPilot can
// apply per-application routing, bandwidth and quota policies on the data path.
//
// Peer credentials are available directly on a loopback socket (SO_PEERCRED on Linux,
// LOCAL_PEERPID on macOS, the TCP owner table on Windows). Flows captured by the system-wide
// TUN adapter are matched to the owning socket by their local/remote endpoint instead.
package procnet

import (
	"net"
	"sync"
	"time"
)

// Identity describes the application that owns a flow.
type Identity struct {
	PID      int    `json:"pid"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
	Resolved bool   `json:"resolved"`
}

// PeerPID returns the PID at the other end of an accepted loopback connection. It is the
// cheapest and most reliable way to identify an application that uses the local SOCKS5 proxy.
func PeerPID(conn net.Conn) (int, bool) {
	if conn == nil {
		return 0, false
	}
	return peerPID(conn)
}

// LookupPID finds the process that owns the socket matching the flow's endpoints. The
// network is "tcp" or "udp". It is used by the TUN front-end, which sees packets rather
// than the owning socket. A false result is normal when the table is unreadable (for
// example without elevation on Linux) and simply means the flow is unclassified.
func LookupPID(network string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	return lookupPID(normalizeNetwork(network), localIP, localPort, remoteIP, remotePort)
}

// Identify resolves a PID to a human-readable process identity.
func Identify(pid int) Identity {
	id := Identity{PID: pid}
	if pid <= 0 {
		return id
	}
	id.Name, id.Path = processInfo(pid)
	id.Resolved = id.Name != ""
	return id
}

func normalizeNetwork(network string) string {
	switch network {
	case "udp", "udp4", "udp6":
		return "udp"
	default:
		return "tcp"
	}
}

type nameEntry struct {
	name, path string
	at         time.Time
}

var nameCache = struct {
	sync.Mutex
	m map[int]nameEntry
}{m: make(map[int]nameEntry)}

const nameCacheTTL = 10 * time.Second

func cachedProcess(pid int) (string, string, bool) {
	nameCache.Lock()
	defer nameCache.Unlock()
	e, ok := nameCache.m[pid]
	if !ok || time.Since(e.at) > nameCacheTTL {
		if ok {
			delete(nameCache.m, pid)
		}
		return "", "", false
	}
	return e.name, e.path, true
}

func storeProcess(pid int, name, path string) {
	nameCache.Lock()
	nameCache.m[pid] = nameEntry{name: name, path: path, at: time.Now()}
	nameCache.Unlock()
}
