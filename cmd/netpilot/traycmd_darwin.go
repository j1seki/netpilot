//go:build darwin

package main

import (
	"os"
	"os/exec"
)

// nmCmdTray on macOS starts the NetPilot GUI hidden, so its native Wails system tray
// provides the same menu. Linking fyne's systray into the same binary as Wails would
// duplicate the Objective-C MenuItem class and fail to link.
func nmCmdTray(args []string) {
	exe, err := os.Executable()
	if err != nil {
		nmDie(err)
	}
	cmd := exec.Command(exe, "-start-hidden")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		nmDie(err)
	}
}
