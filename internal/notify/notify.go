// Package notify sends best-effort native desktop notifications.
//
// Notifications are advisory: they must never block the data path or fail an operation, so
// callers can continue when the OS facility is unavailable.
package notify

import "strings"

// Send shows a desktop notification. A missing notification daemon is not an error the
// caller needs to act on, but it is returned for diagnostics.
func Send(title, body string) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		title = "NetPilot"
	}
	return send(title, body)
}

// Available checks for a native notification backend.
func Available() bool { return available() }

// OpenSettings opens the desktop notification preferences where supported.
func OpenSettings() error { return openSettings() }
