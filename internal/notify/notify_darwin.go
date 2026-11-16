//go:build darwin

package notify

import (
	"os/exec"
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
	return exec.Command(path, "-e", script, body, title).Run()
}
