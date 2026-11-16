//go:build linux

package notify

import "os/exec"

func available() bool {
	_, err := exec.LookPath("notify-send")
	return err == nil
}

func send(title, body string) error {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		return err
	}
	return exec.Command(path, "--app-name=NetPilot", "--urgency=normal", title, body).Run()
}
