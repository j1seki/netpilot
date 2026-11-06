//go:build windows

package tun

import "testing"

func TestContainsRestoreInterfacePreservesFirstObservedSettings(t *testing.T) {
	entries := []string{"ipv4 12 weakhostsend=enabled forwarding=disabled"}
	if !containsRestoreInterface(entries, "ipv4", "12") {
		t.Fatal("existing interface/family restoration entry was not detected")
	}
	if containsRestoreInterface(entries, "ipv6", "12") {
		t.Fatal("IPv4 restoration entry must not suppress IPv6 restoration")
	}
	if containsRestoreInterface(entries, "ipv4", "13") {
		t.Fatal("different interface index must have its own restoration entry")
	}
}
