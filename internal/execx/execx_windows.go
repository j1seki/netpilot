//go:build windows

package execx

import (
	"os/exec"
	"syscall"
)

// createNoWindow (CREATE_NO_WINDOW = 0x08000000) prevents Windows from creating a console
// window for console-subsystem child processes (net-merge.exe, powershell.exe, netsh.exe, route.exe)
// when spawned from a GUI-subsystem parent process (-H=windowsgui).
const createNoWindow = 0x08000000

// PrepareCommand stops Windows from flashing a console window for every probe and write. A
// windowless build has no console to inherit, so without this each child invocation would
// briefly steal focus or flash a silent CMD window.
func PrepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
