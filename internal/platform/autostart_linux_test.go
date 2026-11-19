package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAutostart(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if AutostartEnabled() {
		t.Fatal("on before SetAutostart")
	}
	if err := SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if !AutostartEnabled() {
		t.Fatal("still off after SetAutostart(true)")
	}
	for i := 0; i < 2; i++ { // turning off twice is fine
		if err := SetAutostart(false); err != nil {
			t.Fatal(err)
		}
	}
	if AutostartEnabled() {
		t.Fatal("still on after SetAutostart(false)")
	}
}

func TestDesktopAutostartUsesOriginalAppImage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPIMAGE", filepath.Join(dir, "NetPilot test.AppImage"))
	if err := SetDesktopAutostart(true); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dir, "autostart", "netpilot.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), `Exec="`+filepath.Join(dir, "NetPilot test.AppImage")+`" --appimage-extract-and-run -start-hidden`) {
		t.Fatalf("desktop entry points to the wrong executable: %s", entry)
	}
	if !DesktopAutostartEnabled() || AutostartEnabled() {
		t.Fatal("desktop autostart must be independent of standalone net-merge")
	}
	if err := SetDesktopAutostart(false); err != nil {
		t.Fatal(err)
	}
	if DesktopAutostartEnabled() {
		t.Fatal("desktop autostart still enabled")
	}
}
