//go:build linux

package procnet

import (
	"net"
	"os"
	"testing"
)

func TestPeerPIDIdentifiesLoopbackClient(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	client, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()

	pid, ok := PeerPID(server)
	if !ok {
		t.Skip("peer credentials unavailable in this environment")
	}
	if pid != os.Getpid() {
		t.Fatalf("PeerPID = %d, want %d", pid, os.Getpid())
	}
	if id := Identify(pid); id.Resolved == false || id.Name == "" {
		t.Fatalf("Identify(%d) did not resolve a name: %+v", pid, id)
	}
}

func TestLookupPIDMatchesLoopbackSocket(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	cl := client.LocalAddr().(*net.TCPAddr)
	sv := server.LocalAddr().(*net.TCPAddr)
	pid, ok := LookupPID("tcp", cl.IP, uint16(cl.Port), sv.IP, uint16(sv.Port))
	if !ok {
		t.Skip("process socket table is not readable in this environment")
	}
	if pid != os.Getpid() {
		t.Fatalf("LookupPID = %d, want %d", pid, os.Getpid())
	}
}

func TestParseProcEndpoint(t *testing.T) {
	ip, port, err := parseProcEndpoint("0100007F:1F90", false)
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "127.0.0.1" || port != 8080 {
		t.Fatalf("got %s:%d", ip, port)
	}
}
