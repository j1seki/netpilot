//go:build linux

package procnet

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestLookupPIDFindsSocketOpenedAfterOwnerSnapshot(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	inodePIDMap(true)
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	local, remote := client.LocalAddr().(*net.TCPAddr), client.RemoteAddr().(*net.TCPAddr)
	pid, ok := LookupPID("tcp", local.IP, uint16(local.Port), remote.IP, uint16(remote.Port))
	if !ok || pid != os.Getpid() {
		t.Fatalf("got pid %d, found %t, want %d", pid, ok, os.Getpid())
	}
}
