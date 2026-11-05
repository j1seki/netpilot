// Package branding provides centralized identity metadata for the NetPilot desktop application.
// All user-facing strings, application identifiers, window titles, and packaging metadata
// must reference this package so rebranding can be accomplished in a single place.
package branding

const (
	// AppName is the primary human-readable product name.
	// Working name: NetPilot.
	AppName = "NetPilot"

	// AppID is the unique reverse-DNS identifier used for desktop packaging,
	// settings directories, DBUS, and Wails registration.
	AppID = "com.netpilot.desktop"

	// AppVersion is the current semver release.
	AppVersion = "0.1.0"

	// AppDescription describes the application's core function.
	AppDescription = "Advanced Multi-Network Management, Application Steering & Failover"

	// VendorName is the organization or maintainer name.
	VendorName = "NetPilot Project"

	// CopyrightNotice specifies the legal copyright string.
	CopyrightNotice = "© 2026 NetPilot Project. All rights reserved."

	// SupportURL is the documentation and issue tracking link.
	SupportURL = "https://github.com/netpilot-project/netpilot"

	// WindowTitle is the default desktop application window title.
	WindowTitle = "NetPilot — Multi-Network Manager"
)

// Info returns a map of all branding metadata for frontend serialization.
func Info() map[string]string {
	return map[string]string{
		"name":        AppName,
		"id":          AppID,
		"version":     AppVersion,
		"description": AppDescription,
		"vendor":      VendorName,
		"copyright":   CopyrightNotice,
		"supportUrl":  SupportURL,
		"windowTitle": WindowTitle,
	}
}
