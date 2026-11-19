package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// autostartFile is an XDG autostart entry, honoured by GNOME, KDE, XFCE, Cinnamon, LXQt and others
// at sign-in. On NixOS the module's programs.net-merge.autostart (a systemd user service) is an
// alternative that doesn't depend on the desktop.
func autostartFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "net-merge.desktop"), nil
}

// trayExe prefers net-merge on PATH: on NixOS that is the capability wrapper in /run/wrappers/bin,
// while os.Executable is the store path, which doesn't carry the capabilities.
func trayExe() string {
	if p, err := exec.LookPath("net-merge"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	exe, _ := os.Executable()
	return exe
}

// desktopQuote quotes an Exec argument as the Desktop Entry spec requires.
func desktopQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\$`") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(s) + `"`
}

// Autostart turns start-at-sign-in "on" or "off", or prints its "status".
func Autostart(action string) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	switch action {
	case "status":
		if AutostartEnabled() {
			fmt.Println("autostart is ON (" + path + ").")
		} else {
			fmt.Println("autostart is OFF.")
		}
		return nil
	case "off":
		if !AutostartEnabled() {
			fmt.Println("autostart is already OFF.")
			return nil
		}
		if err := SetAutostart(false); err != nil {
			return err
		}
		fmt.Println("autostart is OFF.")
		return nil
	}
	if err := SetAutostart(true); err != nil {
		return err
	}
	fmt.Println("autostart is ON: net-merge starts at sign-in (" + path + ").")
	if !IsAdmin() {
		fmt.Println("note: the tray needs CAP_NET_ADMIN to start merging; see `net-merge tray` or the README.")
	}
	return nil
}

// AutostartEnabled reports whether net-merge starts at sign-in.
func AutostartEnabled() bool {
	path, err := autostartFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetAutostart writes or removes the autostart entry. The tray runs as the signed-in user (with
// CAP_NET_ADMIN), so that user's config directory is the right one.
func SetAutostart(on bool) error {
	path, err := autostartFile()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=net-merge\n" +
		"Comment=Use all your network adapters at once\n" +
		"Exec=" + desktopQuote(trayExe()) + " tray\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(path, []byte(entry), 0o644)
}

func desktopAutostartFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "netpilot.desktop"), nil
}

// DesktopAutostartEnabled reports whether NetPilot starts at desktop sign-in.
func DesktopAutostartEnabled() bool {
	path, err := desktopAutostartFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetDesktopAutostart starts the desktop app in standby at sign-in.
func SetDesktopAutostart(on bool) error {
	path, err := desktopAutostartFile()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := " -start-hidden"
	if image := os.Getenv("APPIMAGE"); filepath.IsAbs(image) {
		exe = image
		args = " --appimage-extract-and-run" + args
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	command := desktopQuote(strings.ReplaceAll(exe, "%", "%%")) + args
	entry := "[Desktop Entry]\nType=Application\nName=NetPilot\nExec=" + command + "\nTerminal=false\n"
	return os.WriteFile(path, []byte(entry), 0o644)
}
