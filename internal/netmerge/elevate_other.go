//go:build !linux

package netmerge

import "fmt"

// startElevatedLinux is only reachable behind a runtime.GOOS == "linux" guard.
// This stub keeps the controller portable for Windows and macOS builds.
func startElevatedLinux(string, []string) error {
	return fmt.Errorf("Linux elevation is unavailable on this platform")
}
