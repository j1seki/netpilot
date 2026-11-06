//go:build linux

package netmerge

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLinuxElevationTargetUsesOriginalAppImage(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "NetPilot.AppImage")
	if err := os.WriteFile(image, []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", image)
	target, args, appImage, err := linuxElevationTarget("/tmp/.mount-netpilot/usr/bin/netpilot", []string{"run", "-state-dir", "/tmp/state"})
	if err != nil {
		t.Fatal(err)
	}
	if target != image || appImage != image {
		t.Fatalf("target=%q appImage=%q, want %q", target, appImage, image)
	}
	want := []string{"--appimage-extract-and-run", "run", "-state-dir", "/tmp/state"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q, want %q", args, want)
	}
}

func TestLinuxElevationTargetUsesBinaryOutsideAppImage(t *testing.T) {
	t.Setenv("APPIMAGE", "")
	target, args, appImage, err := linuxElevationTarget("/usr/bin/netpilot", []string{"run"})
	if err != nil {
		t.Fatal(err)
	}
	if target != "/usr/bin/netpilot" || appImage != "" || !reflect.DeepEqual(args, []string{"run"}) {
		t.Fatalf("unexpected target %q args %q appimage %q", target, args, appImage)
	}
}
