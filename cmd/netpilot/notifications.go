package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"netpilot/internal/notify"
)

type notificationPreferences struct {
	Desktop bool `json:"desktop"`
	Links   bool `json:"links"`
	Quotas  bool `json:"quotas"`
	Updates bool `json:"updates"`
}

type notificationEvent struct {
	Title string    `json:"title"`
	Body  string    `json:"body"`
	At    time.Time `json:"at"`
}

type notificationStatus struct {
	Preferences notificationPreferences `json:"preferences"`
	Platform    string                  `json:"platform"`
	Latest      *notificationEvent      `json:"latest,omitempty"`
}

type notificationDelivery struct {
	Desktop      string `json:"desktop"`
	DesktopError string `json:"desktopError,omitempty"`
}

func defaultNotifications() notificationPreferences {
	return notificationPreferences{Desktop: true, Links: true, Quotas: true, Updates: true}
}

func (s *NetPilotService) initNotifications(path string) {
	s.notificationFile = path
	s.notificationPrefs = defaultNotifications()
	if data, err := os.ReadFile(path); err == nil {
		prefs := s.notificationPrefs
		if json.Unmarshal(data, &prefs) == nil {
			s.notificationPrefs = prefs
		}
	}
}

func (s *NetPilotService) getNotifications() notificationStatus {
	s.notificationMu.Lock()
	defer s.notificationMu.Unlock()
	return notificationStatus{s.notificationPrefs, runtime.GOOS, s.latestNotification}
}

func (s *NetPilotService) setNotifications(prefs notificationPreferences) error {
	s.notificationMu.Lock()
	defer s.notificationMu.Unlock()
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	if err := writePrivateFile(s.notificationFile, data); err != nil {
		return err
	}
	s.notificationPrefs = prefs
	return nil
}

func (s *NetPilotService) publishNotification(category, title, body string) notificationDelivery {
	s.notificationMu.Lock()
	prefs := s.notificationPrefs
	enabled := map[string]bool{"links": prefs.Links, "quotas": prefs.Quotas, "updates": prefs.Updates, "test": true}
	if !enabled[category] {
		s.notificationMu.Unlock()
		return notificationDelivery{Desktop: "off"}
	}
	s.latestNotification = &notificationEvent{Title: title, Body: body, At: time.Now().UTC()}
	s.notificationMu.Unlock()
	if !prefs.Desktop {
		return notificationDelivery{Desktop: "off"}
	}
	if err := notify.Send(title, body); err != nil {
		return notificationDelivery{Desktop: "unavailable", DesktopError: err.Error()}
	}
	return notificationDelivery{Desktop: "submitted"}
}

func writePrivateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".netpilot-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
