//go:build windows

package procnet

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
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
	data, err := readWindowsTable(udp, v6)
	if err != nil {
		return 0, false
	}
	return ownerFromTable(data, udp, v6, localIP, localPort, remoteIP, remotePort)
}

func readWindowsTable(udp, v6 bool) ([]byte, error) {
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
			return data, nil
		case uintptr(windows.ERROR_INSUFFICIENT_BUFFER):
			continue
		default:
			return nil, windows.Errno(code)
		}
	}
	return nil, fmt.Errorf("could not read Windows socket table (size %d)", size)
}

type tableLayout struct {
	stride, ipSize, local, lport, remote, rport, state, owner int
}

func windowsTableLayout(udp, v6 bool) tableLayout {
	switch {
	case udp && v6:
		return tableLayout{stride: 28, ipSize: 16, lport: 20, owner: 24}
	case udp:
		return tableLayout{stride: 12, ipSize: 4, lport: 4, owner: 8}
	case v6:
		return tableLayout{56, 16, 0, 20, 24, 44, 48, 52}
	}
	return tableLayout{24, 4, 4, 8, 12, 16, 0, 20}
}

func (l tableLayout) rows(data []byte) []byte {
	if len(data) < 4 {
		return nil
	}
	count := int(binary.LittleEndian.Uint32(data[:4]))
	rows := data[4:]
	if count < 0 || count > len(rows)/l.stride {
		return nil
	}
	return rows[:count*l.stride]
}

func ownerFromTable(data []byte, udp, v6 bool, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16) (int, bool) {
	l := windowsTableLayout(udp, v6)
	rows := l.rows(data)
	var exact, wildcard int
	for i := 0; i < len(rows); i += l.stride {
		r := rows[i : i+l.stride]
		if binary.BigEndian.Uint16(r[l.lport:]) != localPort {
			continue
		}
		pid := int(binary.LittleEndian.Uint32(r[l.owner:]))
		if pid <= 0 {
			continue
		}
		lip := net.IP(r[l.local : l.local+l.ipSize])
		if !lip.Equal(localIP) {
			if udp && lip.IsUnspecified() {
				wildcard = uniqueOwner(wildcard, pid)
			}
			continue
		}
		if !udp {
			// Closed/listening/time-wait rows do not identify the live peer.
			st := binary.LittleEndian.Uint32(r[l.state:])
			if st <= 2 || st >= 11 || !net.IP(r[l.remote:l.remote+l.ipSize]).Equal(remoteIP) || binary.BigEndian.Uint16(r[l.rport:]) != remotePort {
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

func processSockets(ctx context.Context, pid int) ([]Socket, error) {
	var sockets []Socket
	var firstErr error
	for _, udp := range []bool{false, true} {
		for _, v6 := range []bool{false, true} {
			if err := ctx.Err(); err != nil {
				return sockets, err
			}
			data, err := readWindowsTable(udp, v6)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			sockets = append(sockets, windowsProcessSockets(data, udp, v6, pid)...)
		}
	}
	return sockets, firstErr
}

func windowsProcessSockets(data []byte, udp, v6 bool, pid int) []Socket {
	l := windowsTableLayout(udp, v6)
	rows := l.rows(data)
	var sockets []Socket
	for i := 0; i < len(rows); i += l.stride {
		r := rows[i : i+l.stride]
		if int(binary.LittleEndian.Uint32(r[l.owner:])) != pid {
			continue
		}
		local := net.IP(r[l.local : l.local+l.ipSize])
		row := Socket{Protocol: "TCP", Local: net.JoinHostPort(local.String(), strconv.Itoa(int(binary.BigEndian.Uint16(r[l.lport:]))))}
		if udp {
			row.Protocol = "UDP"
		} else if port := binary.BigEndian.Uint16(r[l.rport:]); port != 0 {
			remote := net.IP(r[l.remote : l.remote+l.ipSize])
			row.Remote = net.JoinHostPort(remote.String(), strconv.Itoa(int(port)))
		}
		sockets = append(sockets, row)
	}
	return sockets
}

// SystemResolvers reports DNS servers configured on active Windows interfaces.
func SystemResolvers() []string {
	size := uint32(32 << 10)
	flags := uint32(windows.GAA_FLAG_SKIP_UNICAST | windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST)
	for range 4 {
		if size == 0 || size > 16<<20 {
			break
		}
		data := make([]byte, size)
		adapters := (*windows.IpAdapterAddresses)(unsafe.Pointer(&data[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, adapters, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			break
		}
		var servers []string
		for a := adapters; a != nil; a = a.Next {
			if a.OperStatus != windows.IfOperStatusUp {
				continue
			}
			for d := a.FirstDnsServerAddress; d != nil; d = d.Next {
				if d.Address.Sockaddr != nil {
					if ip := d.Address.IP(); ip != nil {
						servers = append(servers, ip.String())
					}
				}
			}
		}
		return servers
	}
	return nil
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
