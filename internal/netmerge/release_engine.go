package netmerge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"netpilot/internal/netmerge/embedded"
	"netpilot/internal/netmerge/releases"
	"netpilot/internal/state"
)

func (c *Controller) engineDir() (string, error) {
	if c.configFile == "" {
		return "", fmt.Errorf("could not locate the NetPilot user settings folder")
	}
	return filepath.Join(filepath.Dir(c.configFile), "engines"), nil
}

func (c *Controller) resolve(source, value string) BinaryInfo {
	tag := embedded.ReleaseTag()
	if source != "bundled" && source != "release" {
		info := ResolveBinaryWithSource(source, value, c.stateDir)
		info.BundledVersion = tag
		return info
	}
	info := BinaryInfo{EngineSource: source, Source: source, BundledVersion: tag}
	if source == "release" {
		tag = value
	}
	info.ReleaseTag = tag
	if !releases.ValidTag(tag) {
		info.Error = "no compatible release is installed"
		return info
	}
	dir, err := c.engineDir()
	if err == nil {
		if source == "bundled" {
			info.Path, err = embedded.ReleaseCLI(dir)
		} else {
			info.Path = filepath.Join(dir, tag, releases.CLIName(runtime.GOOS))
		}
	}
	info.Exists = fileExists(info.Path)
	if err == nil {
		info.Version, err = ValidateBinary(info.Path)
		if err == nil && !versionMatchesTag(info.Version, tag) {
			err = fmt.Errorf("installed engine reported %q instead of %s", info.Version, tag)
		}
	}
	if err != nil {
		info.Error = err.Error()
	}
	info.IsValid = err == nil
	return info
}

func versionMatchesTag(version, tag string) bool {
	number, ok := strings.CutPrefix(version, "net-merge ")
	return ok && strings.TrimPrefix(number, "v") == strings.TrimPrefix(tag, "v")
}

// InstallRelease downloads an official release without replacing the bundled fallback.
// Selection is saved only after the binary has been verified.
func (c *Controller) InstallRelease(ctx context.Context, tag string) (BinaryInfo, error) {
	if !releases.ValidTag(tag) {
		return BinaryInfo{}, fmt.Errorf("select a release version")
	}
	if c.IsForwarding() {
		return BinaryInfo{}, fmt.Errorf("stop net-merge and its SOCKS proxy before changing engines")
	}
	c.installMu.Lock()
	defer c.installMu.Unlock()
	if info := c.resolve("release", tag); info.IsValid {
		return c.SetEngineSource("release", tag)
	}
	dir, err := c.engineDir()
	if err != nil {
		return BinaryInfo{}, err
	}
	_, files, err := releases.Download(ctx, tag, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return BinaryInfo{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BinaryInfo{}, err
	}
	tmp, err := os.MkdirTemp(dir, ".download-")
	if err != nil {
		return BinaryInfo{}, err
	}
	removeStaging := true
	defer func() {
		if removeStaging {
			_ = os.RemoveAll(tmp)
		}
	}()
	staged := filepath.Join(tmp, "engine")
	if err := os.Mkdir(staged, 0o700); err != nil {
		return BinaryInfo{}, err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(staged, name), data, 0o700); err != nil {
			return BinaryInfo{}, err
		}
	}
	version, err := ValidateBinary(filepath.Join(staged, releases.CLIName(runtime.GOOS)))
	if err != nil {
		return BinaryInfo{}, err
	}
	if !versionMatchesTag(version, tag) {
		return BinaryInfo{}, fmt.Errorf("downloaded engine reported unexpected version %q", version)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, running := state.Read(c.stateDir)
	if c.tunOn || c.socksRunning || running {
		return BinaryInfo{}, fmt.Errorf("stop net-merge and its SOCKS proxy before changing engines")
	}
	target := filepath.Join(dir, tag)
	previous := filepath.Join(tmp, "previous")
	hadPrevious := false
	if err := os.Rename(target, previous); err == nil {
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return BinaryInfo{}, fmt.Errorf("could not replace the existing %s engine: %w", tag, err)
	}
	if err := os.Rename(staged, target); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(previous, target); restoreErr != nil {
				removeStaging = false
				err = errors.Join(err, fmt.Errorf("previous engine saved at %s: %w", previous, restoreErr))
			}
		}
		return BinaryInfo{}, fmt.Errorf("could not install %s: %w", tag, err)
	}
	return c.setEngineSourceLocked("release", tag)
}
