package netmerge

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"netpilot/internal/execx"
	"netpilot/internal/netmerge/embedded"
	"netpilot/internal/state"
)

//go:embed VERSION
var embeddedVersionRaw string

// EmbeddedVersion returns the version of the built-in net-merge engine compiled into NetPilot.
func EmbeddedVersion() string {
	v := strings.TrimSpace(embeddedVersionRaw)
	if v == "" {
		return "0.3.6"
	}
	return v
}

// BinaryInfo reports the resolution and verification status of the net-merge binary.
type BinaryInfo struct {
	Path         string `json:"path"`
	TrayPath     string `json:"trayPath,omitempty"`
	CustomPath   string `json:"customPath"`
	EngineSource string `json:"engineSource"` // "builtin" or "external"
	Exists       bool   `json:"exists"`
	Source       string `json:"source"` // "builtin", "custom", "same-folder", "path", "system", "none"
	Version      string `json:"version"`
	IsValid      bool   `json:"isValid"`
	Error        string `json:"error,omitempty"`
}

type cachedBinaryValidation struct {
	modTime   time.Time
	size      int64
	version   string
	err       error
	checkedAt time.Time
}

var (
	validationCacheMu sync.Mutex
	validationCache   = make(map[string]cachedBinaryValidation)
)

// GetBinaryInfo returns the current resolved net-merge binary info.
func (c *Controller) GetBinaryInfo() BinaryInfo {
	c.mu.RLock()
	source := c.config.EngineSource
	custom := c.config.BinaryPath
	stateDir := c.stateDir
	c.mu.RUnlock()
	return ResolveBinaryWithSource(source, custom, stateDir)
}

// SetCustomBinaryPath updates the user-specified binary path and engine source in configuration.
func (c *Controller) SetCustomBinaryPath(path string) (BinaryInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" || strings.EqualFold(path, "builtin") {
		c.config.EngineSource = "builtin"
		c.config.BinaryPath = ""
		_ = c.saveConfigLocked()
		return ResolveBinaryWithSource("builtin", "", c.stateDir), nil
	}

	info := ResolveBinaryWithSource("external", path, c.stateDir)
	if !info.IsValid {
		return info, fmt.Errorf("invalid net-merge binary at %q: %s", path, info.Error)
	}

	c.config.EngineSource = "external"
	c.config.BinaryPath = path
	_ = c.saveConfigLocked()
	return info, nil
}

// SetEngineSource sets whether NetPilot uses the built-in embedded net-merge or an external binary path.
func (c *Controller) SetEngineSource(source string, customPath string) (BinaryInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	source = strings.ToLower(strings.TrimSpace(source))
	customPath = strings.TrimSpace(customPath)
	if source != "external" {
		source = "builtin"
	}

	if source == "external" && customPath != "" {
		info := ResolveBinaryWithSource("external", customPath, c.stateDir)
		if !info.IsValid {
			return info, fmt.Errorf("invalid external net-merge binary at %q: %s", customPath, info.Error)
		}
		c.config.EngineSource = "external"
		c.config.BinaryPath = customPath
		_ = c.saveConfigLocked()
		return info, nil
	}

	c.config.EngineSource = source
	if customPath != "" {
		c.config.BinaryPath = customPath
	}
	_ = c.saveConfigLocked()
	return ResolveBinaryWithSource(c.config.EngineSource, c.config.BinaryPath, c.stateDir), nil
}

// ValidateBinary checks if a file at path exists, is executable, and responds to `version`.
// It caches results keyed by (path, modTime, size) and always uses execx.PrepareCommand
// so Windows never flashes a console window during background status polls.
func ValidateBinary(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("binary path is empty")
	}
	cleaned := filepath.Clean(os.ExpandEnv(path))
	fi, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is a directory, not an executable file")
	}
	if runtime.GOOS != "windows" && fi.Mode()&0111 == 0 {
		return "", fmt.Errorf("file is not executable (chmod +x required)")
	}

	validationCacheMu.Lock()
	if cached, ok := validationCache[cleaned]; ok {
		if cached.modTime.Equal(fi.ModTime()) && cached.size == fi.Size() && time.Since(cached.checkedAt) < 5*time.Minute {
			validationCacheMu.Unlock()
			return cached.version, cached.err
		}
	}
	validationCacheMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cleaned, "version")
	execx.PrepareCommand(cmd)
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))

	var resVer string
	var resErr error
	if err != nil && !strings.Contains(strings.ToLower(outStr), "net-merge") {
		resErr = fmt.Errorf("binary failed execution: %w (output: %q)", err, outStr)
	} else if !strings.Contains(strings.ToLower(outStr), "net-merge") {
		resErr = fmt.Errorf("binary responded with unexpected output: %q", outStr)
	} else {
		resVer = outStr
	}

	validationCacheMu.Lock()
	validationCache[cleaned] = cachedBinaryValidation{
		modTime:   fi.ModTime(),
		size:      fi.Size(),
		version:   resVer,
		err:       resErr,
		checkedAt: time.Now(),
	}
	validationCacheMu.Unlock()

	return resVer, resErr
}

// ResolveBinary preserves backwards-compatible resolution when only customPath is passed.
func ResolveBinary(customPath string) BinaryInfo {
	source := "builtin"
	if strings.TrimSpace(customPath) != "" {
		source = "external"
	}
	return ResolveBinaryWithSource(source, customPath, state.DefaultDir())
}

// ResolveBinaryWithSource resolves either the built-in embedded net-merge + net-merge-tray
// or a user-specified external binary path.
func ResolveBinaryWithSource(engineSource, customPath, stateDir string) BinaryInfo {
	engineSource = strings.ToLower(strings.TrimSpace(engineSource))
	if engineSource == "" {
		if strings.TrimSpace(customPath) != "" {
			engineSource = "external"
		} else {
			engineSource = "builtin"
		}
	}

	binName := "net-merge"
	trayName := "net-merge-tray"
	if runtime.GOOS == "windows" {
		binName = "net-merge.exe"
		trayName = "net-merge-tray.exe"
	}

	// If external mode is explicitly selected with a custom path, validate that path first.
	if engineSource == "external" && strings.TrimSpace(customPath) != "" {
		custom := strings.TrimSpace(customPath)
		expanded := filepath.Clean(os.ExpandEnv(custom))
		ver, err := ValidateBinary(expanded)
		trayCand := filepath.Join(filepath.Dir(expanded), trayName)
		trayFound := ""
		if fileExists(trayCand) {
			trayFound = trayCand
		}
		if err == nil {
			return BinaryInfo{
				Path:         expanded,
				TrayPath:     trayFound,
				CustomPath:   custom,
				EngineSource: "external",
				Exists:       true,
				Source:       "custom",
				Version:      ver,
				IsValid:      true,
			}
		}
		return BinaryInfo{
			Path:         expanded,
			TrayPath:     trayFound,
			CustomPath:   custom,
			EngineSource: "external",
			Exists:       fileExists(expanded),
			Source:       "custom",
			IsValid:      false,
			Error:        err.Error(),
		}
	}

	// Built-in mode (default):
	// 1. Check embedded binaries extracted into stateDir/bin
	embCLI, embTray := embedded.ExtractBinaries(stateDir)
	if embCLI != "" && fileExists(embCLI) {
		return BinaryInfo{
			Path:         embCLI,
			TrayPath:     embTray,
			CustomPath:   customPath,
			EngineSource: "builtin",
			Exists:       true,
			Source:       "builtin",
			Version:      "net-merge " + EmbeddedVersion() + " (built-in + tray)",
			IsValid:      true,
		}
	}

	// 2. Check bundled binaries in same folder as NetPilot executable / AppImage
	sameFolderCandidates := getSameFolderCandidates(binName)
	for _, cand := range sameFolderCandidates {
		if fileExists(cand) {
			trayCand := filepath.Join(filepath.Dir(cand), trayName)
			trayFound := ""
			if fileExists(trayCand) {
				trayFound = trayCand
			}
			ver, err := ValidateBinary(cand)
			if err == nil {
				return BinaryInfo{
					Path:         cand,
					TrayPath:     trayFound,
					CustomPath:   customPath,
					EngineSource: engineSource,
					Exists:       true,
					Source:       "builtin",
					Version:      ver + " (bundled)",
					IsValid:      true,
				}
			}
		}
	}

	// 3. In-process embedded engine is always compiled into NetPilot
	exe, _ := os.Executable()
	return BinaryInfo{
		Path:         exe,
		TrayPath:     exe,
		CustomPath:   customPath,
		EngineSource: "builtin",
		Exists:       true,
		Source:       "builtin",
		Version:      "net-merge " + EmbeddedVersion() + " (embedded in NetPilot)",
		IsValid:      true,
	}
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func getSameFolderCandidates(binName string) []string {
	var candidates []string

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, binName))
	}

	if appdir := os.Getenv("APPDIR"); appdir != "" {
		candidates = append(candidates, filepath.Join(appdir, "usr", "bin", binName))
	}
	if appimage := os.Getenv("APPIMAGE"); appimage != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(appimage), binName))
	}
	if owd := os.Getenv("OWD"); owd != "" {
		candidates = append(candidates, filepath.Join(owd, binName))
	}

	return candidates
}
