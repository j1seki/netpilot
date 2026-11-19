package netmerge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"netpilot/internal/netmerge/embedded"
	"netpilot/internal/netmerge/releases"
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
		if err == nil && info.Version != "net-merge "+strings.TrimPrefix(tag, "v") {
			err = fmt.Errorf("installed engine reported %q instead of %s", info.Version, tag)
		}
	}
	if err != nil {
		info.Error = err.Error()
	}
	info.IsValid = err == nil
	return info
}

// InstallRelease downloads an official release without replacing the bundled fallback.
// Selection is saved only after the binary has been verified.
func (c *Controller) InstallRelease(ctx context.Context, tag string) (BinaryInfo, error) {
	if !releases.ValidTag(tag) {
		return BinaryInfo{}, fmt.Errorf("select a release version")
	}
	if c.IsActive() {
		return BinaryInfo{}, fmt.Errorf("stop the running net-merge engine before changing versions")
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
	defer os.RemoveAll(tmp)
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o700); err != nil {
			return BinaryInfo{}, err
		}
	}
	version, err := ValidateBinary(filepath.Join(tmp, releases.CLIName(runtime.GOOS)))
	if err != nil {
		return BinaryInfo{}, err
	}
	if version != "net-merge "+strings.TrimPrefix(tag, "v") {
		return BinaryInfo{}, fmt.Errorf("downloaded engine reported unexpected version %q", version)
	}
	if err := os.Rename(tmp, filepath.Join(dir, tag)); err != nil {
		return BinaryInfo{}, fmt.Errorf("could not install %s: %w", tag, err)
	}
	return c.SetEngineSource("release", tag)
}
