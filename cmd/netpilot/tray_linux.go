//go:build linux

package main

import (
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"

	"netpilot/internal/icon"
	"netpilot/internal/state"
)

// prepareLinuxTrayEnvironment ensures that icon files exist in both ~/.local/share/icons
// and the NetPilot state directory. Wails itself owns and registers the StatusNotifierItem;
// registering it again here produced two icons on desktops which proxy both KDE and
// freedesktop watcher interfaces (notably AppImage hosts running AppIndicator).
func prepareLinuxTrayEnvironment() {
	iconDir := filepath.Join(state.DefaultDir(), "icons")
	_ = os.MkdirAll(iconDir, 0o755)
	_ = os.WriteFile(filepath.Join(iconDir, "netpilot.png"), icon.PNG(true), 0o644)
	_ = os.WriteFile(filepath.Join(iconDir, "netpilot-off.png"), icon.PNG(false), 0o644)

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		hicolor := filepath.Join(home, ".local", "share", "icons", "hicolor", "32x32", "apps")
		if os.MkdirAll(hicolor, 0o755) == nil {
			_ = os.WriteFile(filepath.Join(hicolor, "netpilot.png"), icon.PNG(true), 0o644)
		}
	}

}

// isLinuxTrayHostAvailable checks whether any StatusNotifierWatcher or system tray host is active on DBus.
func isLinuxTrayHostAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	for _, watcher := range []string{
		"org.kde.StatusNotifierWatcher",
		"org.freedesktop.StatusNotifierWatcher",
	} {
		var hasOwner bool
		err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, watcher).Store(&hasOwner)
		if err == nil && hasOwner {
			return true
		}
	}
	return false
}
