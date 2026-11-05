//go:build linux

package ttlmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// procReader reads the contents of a /proc path.
type procReader func(path string) ([]byte, error)

// procWriter writes data to a /proc path.
type procWriter func(path string, data []byte) error

// linuxBackend is the Linux implementation of Backend with both read and write capabilities.
type linuxBackend struct {
	read  procReader
	write procWriter
}

// NewLinuxBackend returns the Linux backend wired to /proc and sysctl.
func NewLinuxBackend() Backend {
	return &linuxBackend{
		read: os.ReadFile,
		write: func(p string, d []byte) error {
			return os.WriteFile(p, d, 0o644)
		},
	}
}

// newLinuxBackendWithReader lets tests drive the backend against fake read/write handlers.
func newLinuxBackendWithReader(r procReader) *linuxBackend {
	return &linuxBackend{
		read:  r,
		write: func(string, []byte) error { return nil },
	}
}

// Name implements Backend.
func (l *linuxBackend) Name() string { return "linux-procfs-sysctl" }

// procPaths names the exact source for each system-global setting.
var procPaths = map[Family]string{
	FamilyIPv4: "/proc/sys/net/ipv4/ip_default_ttl",
	FamilyIPv6: "/proc/sys/net/ipv6/conf/all/hop_limit",
}

// Capabilities implements Backend.
func (l *linuxBackend) Capabilities(context.Context) Capabilities {
	return Capabilities{
		Platform:  "linux",
		Supported: true,
		Readable:  true,
		Notes: []string{
			"The current IPv4 TTL is read from and written to /proc/sys/net/ipv4/ip_default_ttl.",
			"The current IPv6 hop limit is read from and written to /proc/sys/net/ipv6/conf/all/hop_limit and default/hop_limit.",
			"Per-interface IPv6 hop limits are configured via /proc/sys/net/ipv6/conf/<iface>/hop_limit.",
			"Per-interface IPv4 packet TTL mangling (hotspot/tethering optimization) is supported via iptables/nftables.",
		},
	}
}

// Read implements Backend.
func (l *linuxBackend) Read(_ context.Context, ref SettingRef) (Observed, error) {
	if !ref.Kind.IsConfigurable() {
		return Observed{}, newError(CodeUnsupported,
			string(ref.Kind)+" is not a configurable host setting on Linux",
			ref.Kind.Explain())
	}

	var path string
	if ref.Scope == ScopePerInterface {
		if ref.Family == FamilyIPv6 {
			path = filepath.Join("/proc/sys/net/ipv6/conf", ref.Interface, "hop_limit")
		} else {
			path = procPaths[FamilyIPv4]
		}
	} else {
		var ok bool
		path, ok = procPaths[ref.Family]
		if !ok {
			return Observed{}, newError(CodeUnsupported,
				"no Linux source is known for "+string(ref.Family),
				"Only IPv4 TTL and IPv6 hop limit have system-global sources.")
		}
	}

	raw, err := l.read(path)
	if err != nil {
		return Observed{}, newError(CodeReadFailed,
			"could not read "+path,
			"The value lives in /proc; a failure here usually means the interface or proc entry does not exist.",
			"path: "+path, "error: "+err.Error())
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return Observed{}, newError(CodeReadFailed,
			"the value in "+path+" was not an integer",
			"path: "+path, "contents: "+strings.TrimSpace(string(raw)))
	}

	return Observed{
		Value:    intPtr(val),
		Explicit: true,
		Origin:   "proc:" + path,
	}, nil
}

// Write implements Backend. Writes to /proc or executes sysctl.
func (l *linuxBackend) Write(ctx context.Context, ref SettingRef, value int) error {
	if value < 1 || value > 255 {
		return newError(CodeConfigInvalid, fmt.Sprintf("TTL/Hop Limit must be between 1 and 255, got %d", value), "Valid range is 1 to 255")
	}

	valBytes := []byte(strconv.Itoa(value) + "\n")

	if ref.Scope == ScopePerInterface {
		if ref.Family == FamilyIPv6 {
			path := filepath.Join("/proc/sys/net/ipv6/conf", ref.Interface, "hop_limit")
			if err := l.write(path, valBytes); err != nil {
				cmd := exec.CommandContext(ctx, "sysctl", "-w", fmt.Sprintf("net.ipv6.conf.%s.hop_limit=%d", ref.Interface, value))
				if out, err2 := cmd.CombinedOutput(); err2 != nil {
					return newError(CodePermission, "failed to set interface hop limit: "+string(out), "Requires root or CAP_NET_ADMIN privileges", err2.Error())
				}
			}
			return nil
		}

		// Per-interface IPv4: configure iptables mangle rule for tethering/outgoing packets
		_ = exec.CommandContext(ctx, "iptables", "-t", "mangle", "-D", "POSTROUTING", "-o", ref.Interface, "-j", "TTL", "--ttl-set", strconv.Itoa(value)).Run()
		cmd := exec.CommandContext(ctx, "iptables", "-t", "mangle", "-A", "POSTROUTING", "-o", ref.Interface, "-j", "TTL", "--ttl-set", strconv.Itoa(value))
		if _, err := cmd.CombinedOutput(); err != nil {
			return l.writeGlobal(ctx, ref.Family, value, valBytes)
		}
		return nil
	}

	return l.writeGlobal(ctx, ref.Family, value, valBytes)
}

func (l *linuxBackend) writeGlobal(ctx context.Context, fam Family, value int, valBytes []byte) error {
	path, ok := procPaths[fam]
	if !ok {
		return newError(CodeUnsupported, "unsupported family: "+string(fam), "Only IPv4 and IPv6 are supported")
	}

	if err := l.write(path, valBytes); err != nil {
		var key string
		if fam == FamilyIPv4 {
			key = "net.ipv4.ip_default_ttl"
		} else {
			key = "net.ipv6.conf.all.hop_limit"
		}
		cmd := exec.CommandContext(ctx, "sysctl", "-w", fmt.Sprintf("%s=%d", key, value))
		if out, err2 := cmd.CombinedOutput(); err2 != nil {
			return newError(CodePermission, "failed to write "+path+": "+string(out), "Requires root or CAP_NET_ADMIN privileges", err2.Error())
		}
	}

	if fam == FamilyIPv6 {
		_ = l.write("/proc/sys/net/ipv6/conf/default/hop_limit", valBytes)
		_ = exec.CommandContext(ctx, "sysctl", "-w", fmt.Sprintf("net.ipv6.conf.default.hop_limit=%d", value)).Run()
	}

	return nil
}

// Unset implements Backend. Restores default kernel value (64).
func (l *linuxBackend) Unset(ctx context.Context, ref SettingRef) error {
	defaultVal := 64
	return l.Write(ctx, ref, defaultVal)
}
