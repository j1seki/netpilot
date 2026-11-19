package procnet

import (
	"encoding/binary"
	"net"
	"os"
	"testing"
)

func TestWindowsOwnerTableUDPWildcard(t *testing.T) {
	table := make([]byte, 4+2*12)
	binary.LittleEndian.PutUint32(table, 2)
	for i := range 2 {
		row := table[4+i*12:]
		binary.BigEndian.PutUint16(row[4:], 4567)
		binary.LittleEndian.PutUint32(row[8:], uint32(100+i))
	}
	ip := net.IPv4(192, 0, 2, 5)
	if pid, ok := ownerFromTable(table, true, false, ip, 4567, nil, 0); ok {
		t.Fatalf("shared wildcard socket should be unattributed, got PID %d", pid)
	}
	copy(table[16:20], ip.To4())
	if pid, ok := ownerFromTable(table, true, false, ip, 4567, nil, 0); !ok || pid != 101 {
		t.Fatalf("exact UDP binding should win, got %d, %v", pid, ok)
	}
}

func TestWindowsLiveSocketOwners(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if pid, ok := PeerPID(server); !ok || pid != os.Getpid() {
		t.Fatalf("TCP PID: %d, %v", pid, ok)
	}
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	port := uint16(u.LocalAddr().(*net.UDPAddr).Port)
	if pid, ok := LookupPID("udp", net.IPv4(127, 0, 0, 1), port, net.IPv4(192, 0, 2, 1), 443); !ok || pid != os.Getpid() {
		t.Fatalf("UDP PID: %d, %v", pid, ok)
	}
}
