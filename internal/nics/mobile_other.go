//go:build !windows

package nics

// MobileAdapterHints returns driver-level hints where the OS exposes them.
func MobileAdapterHints() map[int]bool { return nil }
