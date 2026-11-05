//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole = kernel32.NewProc("AllocConsole")
	procFreeConsole  = kernel32.NewProc("FreeConsole")
)

// setConsoleVisible attaches/allocates or detaches/frees the Windows console window.
func setConsoleVisible(show bool) error {
	if show {
		r, _, err := procAllocConsole.Call()
		if r == 0 && err != nil && err != windows.ERROR_ACCESS_DENIED {
			return err
		}
		os.Stdout = os.NewFile(uintptr(windows.Stdout), "/dev/stdout")
		os.Stderr = os.NewFile(uintptr(windows.Stderr), "/dev/stderr")
		return nil
	}
	r, _, err := procFreeConsole.Call()
	if r == 0 && err != nil {
		return err
	}
	return nil
}
