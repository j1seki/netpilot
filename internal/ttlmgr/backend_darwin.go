//go:build darwin

package ttlmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type darwinBackend struct{}

// NewDarwinBackend returns the macOS sysctl-backed TTL manager.
func NewDarwinBackend() Backend {
	return &darwinBackend{}
}

func (d *darwinBackend) Name() string { return "darwin-sysctl" }

func (d *darwinBackend) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Platform:  "darwin",
		Supported: true,
		Readable:  true,
		Notes: []string{
			"IPv4 packet TTL is read from and written to net.inet.ip.ttl via sysctl.",
			"IPv6 packet hop limit is read from and written to net.inet.ip6.hlim via sysctl.",
		},
	}
}

func (d *darwinBackend) Read(ctx context.Context, ref SettingRef) (Observed, error) {
	if !ref.Kind.IsConfigurable() {
		return Observed{}, newError(CodeUnsupported, string(ref.Kind)+" is not configurable on macOS", "Only packet TTL and hop limit are configurable")
	}

	key := "net.inet.ip.ttl"
	if ref.Family == FamilyIPv6 {
		key = "net.inet.ip6.hlim"
	}

	cmd := exec.CommandContext(ctx, "sysctl", "-n", key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Observed{}, newError(CodeReadFailed, "sysctl "+key+" failed: "+string(out), "Check macOS permissions", err.Error())
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return Observed{}, newError(CodeReadFailed, "invalid integer from sysctl: "+string(out), "sysctl output was not an integer")
	}

	return Observed{
		Value:    intPtr(val),
		Explicit: true,
		Origin:   "sysctl:" + key,
	}, nil
}

func (d *darwinBackend) Write(ctx context.Context, ref SettingRef, value int) error {
	if value < 1 || value > 255 {
		return newError(CodeConfigInvalid, fmt.Sprintf("TTL/Hop Limit must be 1-255, got %d", value), "Valid range is 1 to 255")
	}

	key := "net.inet.ip.ttl"
	if ref.Family == FamilyIPv6 {
		key = "net.inet.ip6.hlim"
	}

	cmd := exec.CommandContext(ctx, "sysctl", "-w", fmt.Sprintf("%s=%d", key, value))
	if out, err := cmd.CombinedOutput(); err != nil {
		return newError(CodePermission, "failed to set "+key+": "+string(out), "Requires administrator/sudo privileges", err.Error())
	}
	return nil
}

func (d *darwinBackend) Unset(ctx context.Context, ref SettingRef) error {
	defaultVal := 64
	return d.Write(ctx, ref, defaultVal)
}
