//go:build linux

package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const busName = "org.freedesktop.Notifications"

func available() bool {
	return desktopCall("GetServerInformation") == nil
}

func send(title, body string) error {
	return desktopCall("Notify", "NetPilot", uint32(0), "netpilot", title, body,
		[]string{}, map[string]dbus.Variant{}, int32(-1))
}

func desktopCall(method string, args ...any) error {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && os.Getenv("XDG_RUNTIME_DIR") == "" {
		return fmt.Errorf("desktop session bus is unavailable")
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("desktop session bus: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return conn.Object(busName, "/org/freedesktop/Notifications").
		CallWithContext(ctx, busName+"."+method, 0, args...).Err
}

func openSettings() error {
	desktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	var command []string
	switch {
	case strings.Contains(desktop, "kde"):
		command = []string{"systemsettings", "kcm_notifications"}
	case strings.Contains(desktop, "xfce"):
		command = []string{"xfce4-notifyd-config"}
	case strings.Contains(desktop, "cinnamon"):
		command = []string{"cinnamon-settings", "notifications"}
	case strings.Contains(desktop, "mate"):
		command = []string{"mate-notification-properties"}
	case strings.Contains(desktop, "gnome"), strings.Contains(desktop, "unity"):
		command = []string{"gnome-control-center", "notifications"}
	default:
		return fmt.Errorf("open Notifications in your desktop's system settings")
	}
	cmd := exec.Command(command[0], command[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open Notifications in your desktop's system settings: %w", err)
	}
	go cmd.Wait()
	return nil
}
