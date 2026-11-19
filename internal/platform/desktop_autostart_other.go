//go:build !linux

package platform

func DesktopAutostartEnabled() bool { return AutostartEnabled() }

func SetDesktopAutostart(on bool) error { return SetAutostart(on) }
