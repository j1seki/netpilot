//go:build windows

package procnet

import (
	"encoding/binary"
	"net"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
var getExtendedTCPTable = iphlpapi.NewProc("GetExtendedTcpTable")
var getExtendedUDPTable = iphlpapi.NewProc("GetExtendedUdpTable")

// Read a fresh owner table for new flows. The previous one-second netstat
// cache missed newly opened connections and could never match unconnected UDP.
func lookupPID(network string, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	if localIP == nil || localPort == 0 {
		return 0, false
	}
	udp := network == "udp"
	if localIP.To4() != nil {
		if pid, ok := lookupWindowsTable(udp, false, localIP, localPort, remoteIP, remotePort); ok {
			return pid, ok
		}
	}
	return lookupWindowsTable(udp, true, localIP, localPort, remoteIP, remotePort)
}

func lookupWindowsTable(udp, v6 bool, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	proc, class := getExtendedTCPTable, uintptr(5) // TCP_TABLE_OWNER_PID_ALL
	if udp {
		proc, class = getExtendedUDPTable, 1
	} // UDP_TABLE_OWNER_PID
	af := uintptr(windows.AF_INET)
	if v6 {
		af = windows.AF_INET6
	}
	var size uint32
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
	for i := 0; i < 3 && size >= 4 && size <= 16<<20; i++ {
		data := make([]byte, size)
		code, _, _ := proc.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)), 0, af, class, 0)
		switch code {
		case 0:
			return ownerFromTable(data, udp, v6, localIP, localPort, remoteIP, remotePort)
		case uintptr(windows.ERROR_INSUFFICIENT_BUFFER):
			continue
		default:
			return 0, false
		}
	}
	return 0, false
}

func ownerFromTable(data []byte, udp, v6 bool, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	if len(data) < 4 {
		return 0, false
	}
	count := int(binary.LittleEndian.Uint32(data[:4]))
	stride, ipSize, local, lport, remote, rport, state, owner := 24, 4, 4, 8, 12, 16, 0, 20
	switch {
	case udp && v6:
		stride, ipSize, local, lport, owner = 28, 16, 0, 20, 24
	case udp:
		stride, local, lport, owner = 12, 0, 4, 8
	case v6:
		stride, ipSize, local, lport, remote, rport, state, owner = 56, 16, 0, 20, 24, 44, 48, 52
	}
	rows := data[4:]
	if count > len(rows)/stride {
		return 0, false
	}
	var exact, wildcard int
	for i := 0; i < count; i++ {
		r := rows[i*stride : (i+1)*stride]
		if binary.BigEndian.Uint16(r[lport:]) != localPort {
			continue
		}
		pid := int(binary.LittleEndian.Uint32(r[owner:]))
		if pid <= 0 {
			continue
		}
		lip := net.IP(r[local : local+ipSize])
		if !lip.Equal(localIP) {
			if udp && lip.IsUnspecified() {
				wildcard = uniqueOwner(wildcard, pid)
			}
			continue
		}
		if !udp {
			// Closed/listening/time-wait rows do not identify the live peer.
			st := binary.LittleEndian.Uint32(r[state:])
			if st <= 2 || st >= 11 || !net.IP(r[remote:remote+ipSize]).Equal(remoteIP) || binary.BigEndian.Uint16(r[rport:]) != remotePort {
				continue
			}
		}
		exact = uniqueOwner(exact, pid)
	}
	if exact != 0 {
		return exact, exact > 0
	}
	return wildcard, wildcard > 0
}

func uniqueOwner(previous, pid int) int {
	if previous == 0 || previous == pid {
		return pid
	}
	return -1
}

func peerPID(conn net.Conn) (int, bool) {
	local, okL := conn.LocalAddr().(*net.TCPAddr)
	remote, okR := conn.RemoteAddr().(*net.TCPAddr)
	if !okL || !okR || !local.IP.IsLoopback() || !remote.IP.IsLoopback() {
		return 0, false
	}
	return lookupPID("tcp", remote.IP, uint16(remote.Port), local.IP, uint16(local.Port))
}

func processInfo(pid int) (string, string) {
	if name, path, ok := cachedProcess(pid); ok {
		return name, path
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) != nil {
		return "", ""
	}
	path := windows.UTF16ToString(buf[:size])
	name := filepath.Base(path)
	storeProcess(pid, name, path)
	return name, path
}
