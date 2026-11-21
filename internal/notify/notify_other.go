//go:build !linux && !darwin && !windows

package notify

import "errors"

func available() bool { return false }

func send(title, body string) error {
	return errors.New("desktop notifications are not supported on this platform")
}

func openSettings() error { return errors.New("open notifications in your system settings") }
