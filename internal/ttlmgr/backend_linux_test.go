package ttlmgr

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeProc serves a scripted /proc tree so the Linux backend is tested against realistic
// contents instead of the kernel's live layout.
func fakeProc(files map[string]string) procReader {
	return func(path string) ([]byte, error) {
		v, ok := files[path]
		if !ok {
			return nil, errors.New("open " + path + ": no such file or directory")
		}
		return []byte(v), nil
	}
}

func TestLinuxBackendReadsRealProcValues(t *testing.T) {
	files := map[string]string{
		"/proc/sys/net/ipv4/ip_default_ttl":     "64\n",
		"/proc/sys/net/ipv6/conf/all/hop_limit": "64\n",
	}
	l := &linuxBackend{
		read: fakeProc(files),
		write: func(path string, data []byte) error {
			files[path] = string(data)
			return nil
		},
	}
	m, _ := newTestManager(t, l)

	rep, err := m.Inspect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !rep.Capabilities.Supported {
		t.Fatal("the Linux backend must report itself supported")
	}
	if !rep.Capabilities.Readable {
		t.Fatal("the Linux backend must report itself readable")
	}

	sys4, ok := settingByID(rep, IDv4Default)
	if !ok {
		t.Fatal("report is missing the system-global IPv4 setting")
	}
	if sys4.Value == nil || *sys4.Value != 64 {
		t.Fatalf("IPv4 TTL = %v, want 64", sys4.Value)
	}
	if !strings.Contains(sys4.ValueOrigin, "ip_default_ttl") {
		t.Errorf("IPv4 origin = %q, want it to name ip_default_ttl", sys4.ValueOrigin)
	}

	sys6, ok := settingByID(rep, IDv6Default)
	if !ok {
		t.Fatal("report is missing the system-global IPv6 setting")
	}
	if sys6.Value == nil || *sys6.Value != 64 {
		t.Fatalf("IPv6 hop limit = %v, want 64", sys6.Value)
	}
	if !strings.Contains(sys6.ValueOrigin, "conf/all/hop_limit") {
		t.Errorf("IPv6 origin = %q, want it to name conf/all/hop_limit", sys6.ValueOrigin)
	}

	// Verify write works
	ctx := context.Background()
	ref4 := SettingRef{Family: FamilyIPv4, Scope: ScopeSystemGlobal, Kind: KindPacketTTL}
	if err := l.Write(ctx, ref4, 65); err != nil {
		t.Fatalf("Write IPv4 TTL: %v", err)
	}
	obs4, _ := l.Read(ctx, ref4)
	if obs4.Value == nil || *obs4.Value != 65 {
		t.Fatalf("after write, IPv4 TTL = %v, want 65", obs4.Value)
	}
}

// TestLinuxBackendMissingProcFailsHonestly pins that a missing /proc source surfaces as a read
// error with the offending path, rather than a silent zero or an invented default.
func TestLinuxBackendMissingProcFailsHonestly(t *testing.T) {
	l := newLinuxBackendWithReader(fakeProc(map[string]string{}))
	m, _ := newTestManager(t, l)

	rep, err := m.Inspect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Inspect must still succeed when a read fails: %v", err)
	}
	if len(rep.ReadErrors) != 2 {
		t.Fatalf("read errors = %d, want 2 (both system-global settings missing)", len(rep.ReadErrors))
	}
	for _, e := range rep.ReadErrors {
		if e.Code != CodeReadFailed {
			t.Errorf("%s: code = %q, want %q", e.Setting.ID, e.Code, CodeReadFailed)
		}
		if len(e.Details) == 0 || !strings.Contains(strings.Join(e.Details, " "), "/proc/sys/net") {
			t.Errorf("%s: details = %v, want them to name the /proc path", e.Setting.ID, e.Details)
		}
	}
}
