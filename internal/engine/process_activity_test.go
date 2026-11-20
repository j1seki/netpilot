package engine

import (
	"net"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestProcessInspectionRecordsOnlySelectedPIDAndDNSQuestion(t *testing.T) {
	var b Balancer
	b.recordProcessTCP(17, "before.example", 443, "eth0")
	b.InspectProcess(17)
	b.recordProcessTCP(18, "unselected.example", 443, "eth0")
	b.recordProcessTCP(17, "visited.example", 443, "eth0")
	query := dnsmessage.Message{Questions: []dnsmessage.Question{{
		Name: dnsmessage.MustNewName("dns.example."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET,
	}}}
	payload, err := query.Pack()
	if err != nil {
		t.Fatal(err)
	}
	destination := &net.UDPAddr{IP: net.ParseIP("192.0.2.53"), Port: 53}
	b.recordProcessUDP(17, destination, "wlan0", payload)
	b.recordProcessUDP(17, destination, "wlan0", payload)
	seen := b.InspectProcess(17)
	if len(seen) != 2 || seen[0].Destination != "192.0.2.53:53" || seen[0].DNSQuestion != "dns.example. TypeA" ||
		seen[0].Adapter != "wlan0" || seen[1].Destination != "visited.example:443" {
		t.Fatalf("unexpected activity: %+v", seen)
	}
	b.processInspection.expires = time.Now().Add(-time.Second)
	b.recordProcessTCP(17, "after.example", 443, "eth0")
	if seen = b.InspectProcess(17); len(seen) != 0 {
		t.Fatalf("expired inspector retained activity: %+v", seen)
	}
}
