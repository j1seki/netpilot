package hotspot

import "testing"

func TestIsTetherRecognisesCommonNames(t *testing.T) {
	yes := []string{"enx001122334455", "usb0", "wwan0", "rmnet_data0", "enp0s20u1", "RNDIS", "Ethernet 4 (Mobile Broadband)"}
	no := []string{"eth0", "enp39s0", "Wi-Fi", "wlan0", "lo", "docker0", "tailscale0"}
	for _, n := range yes {
		if !IsTether(n) {
			t.Errorf("%q should be detected as a tether", n)
		}
	}
	for _, n := range no {
		if IsTether(n) {
			t.Errorf("%q should not be detected as a tether", n)
		}
	}
}

func TestProfilesHaveUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Profiles() {
		if seen[p.ID] {
			t.Fatalf("duplicate profile id %q", p.ID)
		}
		seen[p.ID] = true
		if p.MTU <= 0 {
			t.Fatalf("profile %q has no MTU", p.ID)
		}
	}
	if _, ok := Get("hotspot-bypass"); !ok {
		t.Fatal("hotspot-bypass profile missing")
	}
}
