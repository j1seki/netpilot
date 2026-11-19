//go:build linux

package procnet

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func peerPID(conn net.Conn) (int, bool) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return 0, false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return 0, false
	}
	var pid int
	var got bool
	err = raw.Control(func(fd uintptr) {
		ucred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err == nil && ucred.Pid > 0 {
			pid, got = int(ucred.Pid), true
		}
	})
	if err != nil {
		return 0, false
	}
	return pid, got
}

func processInfo(pid int) (string, string) {
	if name, path, ok := cachedProcess(pid); ok {
		return name, path
	}
	base := filepath.Join("/proc", strconv.Itoa(pid))
	name := ""
	if b, err := os.ReadFile(filepath.Join(base, "comm")); err == nil {
		name = strings.TrimSpace(string(b))
	}
	path := ""
	if p, err := os.Readlink(filepath.Join(base, "exe")); err == nil {
		path = p
		if name == "" {
			name = filepath.Base(p)
		}
	}
	if name == "" {
		if b, err := os.ReadFile(filepath.Join(base, "cmdline")); err == nil {
			fields := strings.Fields(strings.ReplaceAll(string(b), "\x00", " "))
			if len(fields) > 0 {
				name = filepath.Base(fields[0])
			}
		}
	}
	if name != "" {
		storeProcess(pid, name, path)
	}
	return name, path
}

var inodeCache = struct {
	sync.Mutex
	at time.Time
	m  map[string]int
}{}

const inodeCacheTTL = 2 * time.Second

func inodePIDMap() map[string]int {
	inodeCache.Lock()
	defer inodeCache.Unlock()
	if inodeCache.m != nil && time.Since(inodeCache.at) < inodeCacheTTL {
		return inodeCache.m
	}
	out := make(map[string]int)
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", p.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
				out[target[len("socket:["):len(target)-1]] = pid
			}
		}
	}
	inodeCache.m, inodeCache.at = out, time.Now()
	return out
}

func lookupPID(network string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	var best, owner int
	if network == "udp" {
		files = []string{"/proc/net/udp", "/proc/net/udp6"}
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		v6 := strings.HasSuffix(file, "6")
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 10 || !strings.Contains(fields[0], ":") {
				continue
			}
			lip, lport, err := parseProcEndpoint(fields[1], v6)
			if err != nil || lport != localPort {
				continue
			}
			score := 1
			if sameIP(lip, localIP) {
				score++
			} else if network != "udp" || !lip.IsUnspecified() {
				continue
			}
			rip, rport, err := parseProcEndpoint(fields[2], v6)
			if err != nil {
				continue
			}
			if remoteIP != nil && remotePort != 0 {
				if rport == remotePort && sameIP(rip, remoteIP) {
					score += 2
				} else if network != "udp" || rport != 0 || !rip.IsUnspecified() {
					continue
				}
			}
			if pid, ok := inodePIDMap()[fields[9]]; ok {
				if network != "udp" {
					return pid, true
				}
				if score > best {
					best, owner = score, pid
				} else if score == best && owner != pid {
					owner = -1
				}
			}
		}
	}
	return max(owner, 0), owner > 0
}

func sameIP(a, b net.IP) bool {
	return a != nil && b != nil && a.Equal(b)
}

func parseProcEndpoint(s string, v6 bool) (net.IP, uint16, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return nil, 0, strconv.ErrSyntax
	}
	raw, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		return nil, 0, err
	}
	ipHex := parts[0]
	if v6 {
		if len(ipHex) != 32 {
			return nil, 0, strconv.ErrSyntax
		}
		out := make([]byte, 16)
		for w := 0; w < 4; w++ {
			for i := 0; i < 4; i++ {
				v, err := strconv.ParseUint(ipHex[w*8+i*2:w*8+i*2+2], 16, 8)
				if err != nil {
					return nil, 0, err
				}
				out[w*4+3-i] = byte(v)
			}
		}
		return net.IP(out), uint16(raw), nil
	}
	if len(ipHex) != 8 {
		return nil, 0, strconv.ErrSyntax
	}
	out := make([]byte, 4)
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(ipHex[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, 0, err
		}
		out[3-i] = byte(v)
	}
	return net.IP(out), uint16(raw), nil
}
