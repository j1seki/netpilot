//go:build darwin

package procnet

import (
	"net"
	"testing"
)

func TestLookupRowsMatchesTCPPeerEndpoints(t *testing.T) {
	rows := []flowRow{{local: "127.0.0.1:42000", remote: "127.0.0.1:1080", pid: 1234}}
	pid, ok := lookupRows(rows, net.ParseIP("127.0.0.1"), 42000, net.ParseIP("127.0.0.1"), 1080)
	if !ok || pid != 1234 {
		t.Fatalf("lookupRows = (%d, %v), want (1234, true)", pid, ok)
	}
	if _, ok := lookupRows(rows, net.ParseIP("127.0.0.1"), 42001, net.ParseIP("127.0.0.1"), 1080); ok {
		t.Fatal("lookupRows matched a different local port")
	}
}
