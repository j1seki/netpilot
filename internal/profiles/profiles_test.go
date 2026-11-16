package profiles

import (
	"testing"

	"netpilot/internal/netmerge"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Save(Profile{Name: "Gaming", NetMerge: netmerge.Config{RoutingMode: "speed"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("save did not assign an id")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Get(p.ID)
	if !ok || got.Name != "Gaming" || got.NetMerge.RoutingMode != "speed" {
		t.Fatalf("round trip = %+v, ok=%v", got, ok)
	}
	if err := reopened.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(reopened.List()) != 0 {
		t.Fatal("delete did not persist")
	}
}

func TestSaveRequiresName(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Profile{}); err == nil {
		t.Fatal("expected a name error")
	}
}

func TestBuiltinsApplyOnTopOfCurrentConfig(t *testing.T) {
	cur := netmerge.Config{
		RoutingMode: "speed",
		Scheduling:  "low-latency",
		MTU:         1500,
		Weights:     map[string]float64{"eth0": 2},
	}
	var lowLatency Profile
	for _, p := range Builtins(cur) {
		if p.ID == "builtin-low-latency" {
			lowLatency = p
		}
	}
	if lowLatency.Name == "" {
		t.Fatal("low-latency builtin missing")
	}
	if lowLatency.NetMerge.Weights["eth0"] != 2 || lowLatency.NetMerge.MTU != 1500 {
		t.Fatalf("builtin did not preserve current settings: %+v", lowLatency.NetMerge)
	}
}
