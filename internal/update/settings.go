package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Settings is the user's update preference, persisted outside the net-merge
// (root-owned) state directory.
type Settings struct {
	Channel Channel `json:"channel"`
	// AutoCheck enables the background check and update notification.
	AutoCheck bool `json:"autoCheck"`
	// LastNotified prevents the same release from notifying on every launch.
	LastNotified string `json:"lastNotified,omitempty"`
}

// DefaultSettings follows the channel of the installed build, so a beta build
// keeps receiving betas until the user deliberately switches to stable.
func DefaultSettings(currentVersion string) Settings {
	return Settings{
		Channel:   ChannelForVersion(currentVersion),
		AutoCheck: true,
	}
}

// DefaultSettingsDir is the per-user configuration directory NetPilot uses for
// update preferences. The net-merge state directory is root-owned on macOS, so
// it cannot be used here.
func DefaultSettingsDir() string {
	if dir := strings.TrimSpace(os.Getenv("NETPILOT_CONFIG_DIR")); dir != "" {
		return dir
	}
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		return filepath.Join(base, "netpilot")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "netpilot")
	}
	return filepath.Join(os.TempDir(), "netpilot")
}

// LoadSettings reads the preferences, falling back to sensible defaults when the
// file is missing or unreadable.
func LoadSettings(dir, currentVersion string) Settings {
	def := DefaultSettings(currentVersion)
	b, err := os.ReadFile(filepath.Join(dir, "update.json"))
	if err != nil {
		return def
	}
	var raw struct {
		Channel      string `json:"channel"`
		AutoCheck    *bool  `json:"autoCheck"`
		LastNotified string `json:"lastNotified"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return def
	}
	if raw.Channel != "" {
		if ch, ok := NormalizeChannel(raw.Channel); ok {
			def.Channel = ch
		}
	}
	if raw.AutoCheck != nil {
		def.AutoCheck = *raw.AutoCheck
	}
	def.LastNotified = strings.TrimSpace(raw.LastNotified)
	return def
}

// SaveSettings writes the preferences atomically.
func SaveSettings(dir string, s Settings) error {
	if ch, ok := NormalizeChannel(string(s.Channel)); ok {
		s.Channel = ch
	} else {
		s.Channel = ChannelStable
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// A unique temporary file keeps two settings writers (channel change and
	// notification bookkeeping) from clobbering each other's partial write.
	f, err := os.CreateTemp(dir, "update.json.tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "update.json"))
}
