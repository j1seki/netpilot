//go:build !darwin

package main

import (
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"netpilot/internal/logx"
	"netpilot/internal/netmerge"
	"netpilot/internal/platform"
	"netpilot/internal/state"
	"netpilot/internal/tray"
	"netpilot/internal/tun"
)

// nmCmdTray runs the standalone net-merge tray (the original net-merge-tray behaviour).
// On macOS this lives in traycmd_darwin.go: linking fyne's systray next to Wails' own
// Objective-C MenuItem class produces duplicate ObjC symbols, so macOS uses the
// NetPilot GUI tray instead.
func nmCmdTray(args []string) {
	fs := flag.NewFlagSet("tray", flag.ExitOnError)
	var c nmCommon
	c.register(fs)
	hidden := fs.Bool("hidden", true, "hide the console window of the elevated process")
	startOff := fs.Bool("off", false, "start with system-wide mode OFF (default: ON)")
	autoPause := fs.Bool("auto-pause", true, "pause when fewer than two connections work")
	fs.Parse(args)

	if !platform.IsAdmin() {
		child := append([]string{"tray"}, args...)
		if c.stateDir != state.DefaultDir() {
			child = append(child, "-state-dir", c.stateDir)
		}
		if err := platform.RelaunchTray(child, *hidden); err != nil {
			nmDie(err)
		}
		return
	}
	if !platform.SingleInstance() {
		return
	}
	if c.logFile == "" {
		c.stats = 0
		if !platform.HasConsole() {
			c.logFile = filepath.Join(c.stateDir, "net-merge.log")
		}
	}
	if err := c.setup(); err != nil {
		nmDie(err)
	}
	if err := platform.CheckDriver(); err != nil {
		nmDie(err)
	}

	t := tun.New(nil)
	t.MTU = c.mtu
	mode := func() string {
		if t.IsOn() {
			return "system-wide (tray)"
		}
		return ""
	}
	sched, bal, err := c.boot(mode)
	if err != nil {
		nmDie(err)
	}
	t.B = bal

	if !platform.WaitForTray(3 * time.Minute) {
		logx.Printf("warning: taskbar not found after 3 minutes, trying anyway\n")
	}
	ready := make(chan struct{})
	go func() {
		select {
		case <-ready:
		case <-time.After(30 * time.Second):
			os.Exit(4)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		t.Close()
		state.Clear(c.stateDir)
		os.Exit(0)
	}()
	tray.Run(t, sched, bal, c.stateDir, netmerge.EmbeddedVersion(), !*startOff, *autoPause, ready)
}
