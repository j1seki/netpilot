package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"netpilot/internal/branding"
	"netpilot/internal/notify"
	"netpilot/internal/update"
)

// UpdateStatus is the complete state of the updater for the UI. It intentionally
// keeps the original current/latest/updateAvailable fields so older clients keep
// working while exposing channel, download and install progress to newer ones.
type UpdateStatus struct {
	Current          string     `json:"current"`
	Channel          string     `json:"channel"`
	AutoCheck        bool       `json:"autoCheck"`
	Latest           string     `json:"latest"`
	ReleaseName      string     `json:"releaseName,omitempty"`
	Prerelease       bool       `json:"prerelease"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
	ReleasesURL      string     `json:"releasesUrl"`
	CheckedAt        *time.Time `json:"checkedAt,omitempty"`
	State            string     `json:"state"`
	AssetName        string     `json:"assetName,omitempty"`
	AssetSize        int64      `json:"assetSize,omitempty"`
	Downloaded       int64      `json:"downloaded,omitempty"`
	Progress         float64    `json:"progress,omitempty"`
	ChecksumVerified bool       `json:"checksumVerified"`
	InstallSupported bool       `json:"installSupported"`
	InstallNote      string     `json:"installNote,omitempty"`
	Message          string     `json:"message,omitempty"`
	Error            string     `json:"error,omitempty"`
}

const (
	updateRetryAfterFailure = 15 * time.Minute
	updateCheckInterval     = 6 * time.Hour
	updateInitialDelay      = 20 * time.Second
	updateCacheTTL          = 10 * time.Minute
	updateInstallTimeout    = 30 * time.Minute
)

func (s *NetPilotService) initUpdates() {
	s.updateDir = update.DefaultSettingsDir()
	s.updateSettings = update.LoadSettings(s.updateDir, branding.AppVersion)
	s.updateStatus = UpdateStatus{
		Current:     branding.AppVersion,
		Channel:     string(s.updateSettings.Channel),
		AutoCheck:   s.updateSettings.AutoCheck,
		ReleasesURL: update.ReleasesURL,
		State:       "idle",
	}
}

// GetUpdateStatus returns the cached updater state. It never performs a network
// request, so the UI can poll it while a download is in progress.
func (s *NetPilotService) GetUpdateStatus() UpdateStatus {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateStatusLocked()
}

func (s *NetPilotService) updateStatusLocked() UpdateStatus {
	st := s.updateStatus
	st.Current = branding.AppVersion
	st.Channel = string(s.updateSettings.Channel)
	st.AutoCheck = s.updateSettings.AutoCheck
	if st.State == "" {
		st.State = "idle"
	}
	st.InstallSupported, st.InstallNote = update.CanAutoInstall()
	if st.InstallSupported && st.UpdateAvailable && st.AssetName == "" {
		st.InstallSupported = false
		if st.InstallNote == "" {
			st.InstallNote = "No matching package was found for this platform. Use the release page to download it."
		}
	}
	return st
}

// SetUpdateChannel switches between stable and beta and clears the cached check
// so the next check cannot show a release from the previous channel.
func (s *NetPilotService) SetUpdateChannel(channel string) (UpdateStatus, error) {
	ch, ok := update.NormalizeChannel(channel)
	if !ok {
		return s.GetUpdateStatus(), fmt.Errorf("unknown update channel %q", channel)
	}
	if !s.updateInstallMu.TryLock() {
		return s.GetUpdateStatus(), errors.New("wait for the current update before changing channels")
	}
	defer s.updateInstallMu.Unlock()

	s.updateMu.Lock()
	previous := s.updateSettings
	next := previous
	next.Channel = ch
	if err := update.SaveSettings(s.updateDir, next); err != nil {
		st := s.updateStatusLocked()
		s.updateMu.Unlock()
		return st, fmt.Errorf("save update settings: %w", err)
	}
	if s.updateSettings.Channel != ch {
		s.updateSettings = next
		s.updateRelease = nil
		s.updateStatus.Latest = ""
		s.updateStatus.ReleaseName = ""
		s.updateStatus.Prerelease = false
		s.updateStatus.PublishedAt = nil
		s.updateStatus.UpdateAvailable = false
		s.updateStatus.CheckedAt = nil
		s.updateStatus.AssetName = ""
		s.updateStatus.AssetSize = 0
		s.updateStatus.Downloaded = 0
		s.updateStatus.Progress = 0
		s.updateStatus.ChecksumVerified = false
		s.updateStatus.State = "idle"
		s.updateStatus.Message = ""
		s.updateStatus.Error = ""
	}
	st := s.updateStatusLocked()
	s.updateMu.Unlock()
	if previous.Channel != ch {
		s.wakeUpdateLoop()
	}
	return st, nil
}

// SetAutoUpdateCheck turns the background check and notification on or off.
func (s *NetPilotService) SetAutoUpdateCheck(enabled bool) (UpdateStatus, error) {
	s.updateMu.Lock()
	previous := s.updateSettings
	s.updateSettings.AutoCheck = enabled
	if err := update.SaveSettings(s.updateDir, s.updateSettings); err != nil {
		s.updateSettings = previous
		st := s.updateStatusLocked()
		s.updateMu.Unlock()
		return st, fmt.Errorf("save update settings: %w", err)
	}
	st := s.updateStatusLocked()
	s.updateMu.Unlock()
	if enabled {
		s.wakeUpdateLoop()
	}
	return st, nil
}

// SetUpdateQuitFunc lets the desktop entry point shut down cleanly after an
// installer has been started. It is set once during startup.
func (s *NetPilotService) SetUpdateQuitFunc(fn func()) {
	s.updateMu.Lock()
	s.updateQuit = fn
	s.updateMu.Unlock()
}

// CheckForUpdate forces a GitHub check on the selected channel.
func (s *NetPilotService) CheckForUpdate(ctx context.Context) UpdateStatus {
	return s.checkForUpdate(ctx, true)
}

func (s *NetPilotService) checkForUpdate(ctx context.Context, force bool) UpdateStatus {
	// Serialize checks instead of returning a cached result for the previous
	// channel: switching Stable -> Beta while the startup check is in flight
	// must not leave Beta showing the old stable answer.
	s.updateCheckMu.Lock()
	defer s.updateCheckMu.Unlock()
	if ctx.Err() != nil {
		return s.GetUpdateStatus()
	}

	s.updateMu.Lock()
	if s.updateStatus.State == "downloading" || s.updateStatus.State == "installing" {
		st := s.updateStatusLocked()
		s.updateMu.Unlock()
		return st
	}
	if !force {
		if !s.updateSettings.AutoCheck {
			st := s.updateStatusLocked()
			s.updateMu.Unlock()
			return st
		}
		if s.updateStatus.CheckedAt != nil && time.Since(*s.updateStatus.CheckedAt) < updateCacheTTL {
			st := s.updateStatusLocked()
			s.updateMu.Unlock()
			return st
		}
	}
	channel := s.updateSettings.Channel
	s.updateStatus.State = "checking"
	s.updateStatus.Error = ""
	s.updateStatus.Message = ""
	s.updateMu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	rel, err := update.LatestRelease(checkCtx, channel)
	now := time.Now().UTC()

	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	// A channel change or an install may have superseded this check in flight.
	if s.updateSettings.Channel != channel || s.updateStatus.State != "checking" {
		return s.updateStatusLocked()
	}
	s.updateStatus.CheckedAt = &now
	if err != nil {
		s.updateStatus.State = "error"
		s.updateStatus.Error = err.Error()
		s.updateStatus.Message = "Could not check for updates. GitHub may be unreachable or rate-limiting requests."
		return s.updateStatusLocked()
	}

	s.updateRelease = rel
	s.updateStatus.Latest = rel.Tag
	s.updateStatus.ReleaseName = rel.Name
	s.updateStatus.Prerelease = rel.IsPrerelease()
	if !rel.PublishedAt.IsZero() {
		published := rel.PublishedAt.UTC()
		s.updateStatus.PublishedAt = &published
	} else {
		s.updateStatus.PublishedAt = nil
	}
	s.updateStatus.ReleasesURL = rel.PageURL()
	s.updateStatus.UpdateAvailable = update.Newer(branding.AppVersion, rel.Tag)
	s.updateStatus.AssetName = ""
	s.updateStatus.AssetSize = 0
	s.updateStatus.Downloaded = 0
	s.updateStatus.Progress = 0
	s.updateStatus.ChecksumVerified = false
	s.updateStatus.InstallNote = ""
	s.updateStatus.Error = ""

	if asset, aerr := update.CurrentAsset(rel); aerr == nil {
		s.updateStatus.AssetName = asset.Name
		s.updateStatus.AssetSize = asset.Size
	} else if s.updateStatus.UpdateAvailable {
		s.updateStatus.InstallNote = aerr.Error()
	}

	if s.updateStatus.UpdateAvailable {
		s.updateStatus.State = "available"
		s.updateStatus.Message = fmt.Sprintf("NetPilot %s is available on the %s channel.", rel.Tag, channelLabel(channel))
	} else {
		s.updateStatus.State = "idle"
		if update.IsVersion(branding.AppVersion) {
			s.updateStatus.Message = fmt.Sprintf("You are on the newest %s release.", channelLabel(channel))
		} else {
			s.updateStatus.Message = "This is a development build; update checks are informational."
		}
	}
	return s.updateStatusLocked()
}

// InstallUpdate downloads the selected release, verifies its published checksum,
// and starts the platform installer. NetPilot is asked to quit afterwards so the
// helper can replace the running files and relaunch the new build.
func (s *NetPilotService) InstallUpdate(ctx context.Context) (UpdateStatus, error) {
	// Reserve the install path before the (possibly slow) release check so two
	// clicks cannot start two downloads. TryLock makes the second caller fail
	// immediately instead of queueing behind a multi-minute transfer.
	if !s.updateInstallMu.TryLock() {
		return s.GetUpdateStatus(), errors.New("an update is already in progress")
	}
	defer s.updateInstallMu.Unlock()

	// A download has no overall deadline of its own (large installers on slow
	// links are legitimate), but it must not hang forever on a dead connection.
	ctx, cancel := context.WithTimeout(ctx, updateInstallTimeout)
	defer cancel()

	s.updateMu.Lock()
	rel := s.updateRelease
	st := s.updateStatusLocked()
	s.updateMu.Unlock()

	if rel == nil || !st.UpdateAvailable || st.Latest == "" {
		st = s.checkForUpdate(ctx, true)
		if st.Error != "" {
			return st, errors.New(st.Error)
		}
		if !st.UpdateAvailable {
			return st, errors.New("no update is available")
		}
		s.updateMu.Lock()
		rel = s.updateRelease
		s.updateMu.Unlock()
	}
	if rel == nil {
		return st, errors.New("no release to install")
	}

	asset, err := update.CurrentAsset(rel)
	if err != nil {
		return s.failUpdate(err)
	}
	if supported, note := update.CanAutoInstall(); !supported {
		return s.failUpdate(errors.New(note))
	}

	s.updateMu.Lock()
	s.updateStatus.State = "downloading"
	s.updateStatus.Error = ""
	s.updateStatus.AssetName = asset.Name
	s.updateStatus.AssetSize = asset.Size
	s.updateStatus.Downloaded = 0
	s.updateStatus.Progress = 0
	s.updateStatus.ChecksumVerified = false
	s.updateStatus.Message = fmt.Sprintf("Downloading %s…", asset.Name)
	s.updateMu.Unlock()

	dir, err := os.MkdirTemp("", "netpilot-update-")
	if err != nil {
		return s.failUpdate(err)
	}
	// Keep the directory when the installer has been started (the helper reads
	// the package and later removes it); remove partial downloads on failure.
	installed := false
	defer func() {
		if !installed {
			os.RemoveAll(dir)
		}
	}()
	dest := filepath.Join(dir, filepath.Base(asset.Name))
	err = update.Download(ctx, *asset, dest, func(done, total int64) {
		s.updateMu.Lock()
		s.updateStatus.Downloaded = done
		if total > 0 {
			s.updateStatus.AssetSize = total
			s.updateStatus.Progress = float64(done) / float64(total)
		}
		s.updateMu.Unlock()
	})
	if err != nil {
		return s.failUpdate(err)
	}

	verified, err := update.VerifyAsset(ctx, rel, *asset, dest)
	if err != nil {
		return s.failUpdate(err)
	}

	s.updateMu.Lock()
	s.updateStatus.ChecksumVerified = verified
	s.updateStatus.State = "installing"
	s.updateStatus.Progress = 1
	s.updateStatus.Message = "Starting the installer…"
	quit := s.updateQuit
	s.updateMu.Unlock()

	if err := update.Install(dest, rel); err != nil {
		return s.failUpdate(err)
	}
	installed = true

	s.updateMu.Lock()
	s.updateStatus.State = "restarting"
	s.updateStatus.Message = "NetPilot will restart to finish the update."
	if !verified {
		s.updateStatus.Message += " No published checksum was found for this release."
	}
	st = s.updateStatusLocked()
	s.updateMu.Unlock()

	if quit != nil {
		go func() {
			time.Sleep(3 * time.Second)
			quit()
		}()
	} else {
		s.updateMu.Lock()
		s.updateStatus.State = "ready"
		s.updateStatus.Message = "Update staged. Restart NetPilot to finish installing."
		s.updateMu.Unlock()
	}
	return st, nil
}

func (s *NetPilotService) failUpdate(err error) (UpdateStatus, error) {
	s.updateMu.Lock()
	s.updateStatus.State = "error"
	s.updateStatus.Error = err.Error()
	s.updateStatus.Message = "The update could not be installed."
	st := s.updateStatusLocked()
	s.updateMu.Unlock()
	return st, err
}

func (s *NetPilotService) wakeUpdateLoop() {
	select {
	case s.updateWake <- struct{}{}:
	default:
	}
}

func (s *NetPilotService) updateSettingsSnapshot() update.Settings {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	return s.updateSettings
}

// updateLoop checks in the background and sends one desktop notification per
// release. Channels are read on every iteration so changing the setting takes
// effect immediately instead of after the six-hour sleep.
func (s *NetPilotService) updateLoop(ctx context.Context) {
	delay := updateInitialDelay
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.updateWake:
			timer.Stop()
		case <-timer.C:
		}

		settings := s.updateSettingsSnapshot()
		if !settings.AutoCheck {
			delay = updateCheckInterval
			continue
		}
		st := s.checkForUpdate(ctx, false)
		if st.UpdateAvailable && st.Latest != "" {
			if settings.LastNotified != st.Latest {
				body := fmt.Sprintf("NetPilot %s is available on the %s channel. Open NetPilot to install it.",
					st.Latest, channelLabel(update.Channel(st.Channel)))
				if err := notify.Send("NetPilot update available", body); err == nil {
					s.markUpdateNotified(st.Latest)
				}
			}
			delay = updateCheckInterval
		} else if st.Error != "" {
			delay = updateRetryAfterFailure
		} else {
			delay = updateCheckInterval
		}
	}
}

func (s *NetPilotService) markUpdateNotified(tag string) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.updateSettings.LastNotified = tag
	_ = update.SaveSettings(s.updateDir, s.updateSettings)
}

func channelLabel(ch update.Channel) string {
	if ch == update.ChannelBeta {
		return "Beta"
	}
	return "Stable"
}
