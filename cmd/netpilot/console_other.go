//go:build !windows

package main

// setConsoleVisible is a no-op on non-Windows platforms.
func setConsoleVisible(show bool) error {
	return nil
}
