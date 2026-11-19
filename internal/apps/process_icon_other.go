//go:build !linux && !windows && (!darwin || !cgo)

package apps

func nativeIcon(string) []byte { return nil }
