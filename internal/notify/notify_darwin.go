//go:build darwin

package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func available() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

func send(title, body string) error {
	path, err := exec.LookPath("osascript")
	if err != nil {
		return err
	}
	const script = "on run argv\ndisplay notification (item 1 of argv) with title (item 2 of argv)\nend run"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, path, "-e", script, body, title).CombinedOutput(); err != nil {
		return fmt.Errorf("macOS notification: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func openSettings() error {
	return exec.Command("open", "x-apple.systempreferences:com.apple.Notifications-Settings.extension").Run()
}
