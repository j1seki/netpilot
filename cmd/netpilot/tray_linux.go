//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"

	"netpilot/internal/icon"
	"netpilot/internal/state"
)

// prepareLinuxTrayEnvironment ensures that icon files exist in both ~/.local/share/icons
// and the NetPilot state directory, and continually ensures NetPilot's StatusNotifierItem
// is registered with both org.kde.StatusNotifierWatcher and org.freedesktop.StatusNotifierWatcher
// across all Linux desktop environments (KDE, GNOME, XFCE, Cinnamon, MATE, Sway/Hyprland/Waybar, i3).
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

	go func() {
		// Wait briefly for Wails to export /StatusNotifierItem on the session bus
		time.Sleep(800 * time.Millisecond)
		conn, err := dbus.SessionBus()
		if err != nil {
			return
		}
		serviceName := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
		_, _ = conn.RequestName(serviceName, dbus.NameFlagDoNotQueue)

		registerBoth := func() {
			for _, watcher := range []string{
				"org.kde.StatusNotifierWatcher",
				"org.freedesktop.StatusNotifierWatcher",
			} {
				obj := conn.Object(watcher, "/StatusNotifierWatcher")
				// Register by well-known service name (preferred by KDE, XFCE, Waybar, GNOME AppIndicator)
				c := obj.Call(watcher+".RegisterStatusNotifierItem", 0, serviceName)
				if c.Err != nil {
					// Fallback: register by object path
					_ = obj.Call(watcher+".RegisterStatusNotifierItem", 0, "/StatusNotifierItem").Err
				}
			}
		}

		registerBoth()
		// Retry registration periodically in case the DE tray applet starts after NetPilot
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			registerBoth()
		}
	}()
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
