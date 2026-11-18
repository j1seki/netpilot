// Package hotspot recognises phone-tethering and cellular adapters and describes the
// one-click profiles that make them usable as a normal internet link.
package hotspot

import (
	"regexp"
	"strings"
)

// Adapter is a detected tether/cellular interface.
type Adapter struct {
	Name  string `json:"name"`
	Index int    `json:"index"`
}

// tetherPattern matches the naming conventions used by USB tethering, mobile broadband
// modems and WWAN drivers on Linux, Windows and macOS. It is intentionally anchored in the
// risky places (`^enx`, `^usb`, `^wwan`, ...) so a normal Ethernet or Wi-Fi adapter never
// matches just because its description happens to contain "mobile".
var tetherPattern = regexp.MustCompile(`(?i)(^enx|^usb\d|^wwan|^rmnet|^cdc|^mbim|^qmi|^pdp|^tether|^android|^iphone|^cell|^modem|^lte|^5g|^4g|^enp\d+s\d+u\d+|rndis|mobile broadband|cellular)`)

// IsTether reports whether an adapter name looks like a phone tether or cellular modem.
func IsTether(name string) bool {
	return tetherPattern.MatchString(strings.TrimSpace(name))
}

// Detect filters an adapter list down to the tether/cellular candidates.
func Detect(names []string) []Adapter {
	out := make([]Adapter, 0, len(names))
	for _, n := range names {
		if IsTether(n) {
			out = append(out, Adapter{Name: n})
		}
	}
	return out
}

// Profile is a named hotspot configuration applied to the detected adapters.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MTU         int    `json:"mtu"`
	IPv6Mode    string `json:"ipv6Mode"`
}

// Profiles returns the built-in hotspot profiles.
func Profiles() []Profile {
	return []Profile{
		{
			ID:          "hotspot-bypass",
			Name:        "Tethering (Automatic IPv6)",
			Description: "Use a 1420 tunnel MTU and automatic IPv6 policy.",
			MTU:         1420,
			IPv6Mode:    "auto",
		},
		{
			ID:          "hotspot-ipv4",
			Name:        "Hotspot (IPv4 only)",
			Description: "Use a 1420 tunnel MTU and route only IPv4 through the tunnel.",
			MTU:         1420,
			IPv6Mode:    "off",
		},
		{
			ID:          "metered-saver",
			Name:        "Low MTU (IPv4 only)",
			Description: "Use a 1280 tunnel MTU for troubleshooting suspected path MTU issues.",
			MTU:         1280,
			IPv6Mode:    "off",
		},
	}
}

// Get finds a profile by ID.
func Get(id string) (Profile, bool) {
	for _, p := range Profiles() {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}
