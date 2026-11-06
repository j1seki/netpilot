//go:build windows

package tun

import "testing"

func TestContainsRestoreEntry(t *testing.T) {
	entries := []string{"ipv4 12 weakhostsend=enabled forwarding=disabled"}
	if !containsRestoreEntry(entries, entries[0]) {
		t.Fatal("existing restoration entry was not detected")
	}
	if containsRestoreEntry(entries, "ipv6 12 weakhostsend=enabled forwarding=enabled") {
		t.Fatal("different family/settings must not be treated as a duplicate")
	}
}
