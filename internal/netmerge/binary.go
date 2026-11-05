package netmerge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BinaryInfo reports the resolution and verification status of the net-merge binary.
type BinaryInfo struct {
	Path       string `json:"path"`
	CustomPath string `json:"customPath"`
	Exists     bool   `json:"exists"`
	Source     string `json:"source"` // "custom", "same-folder", "path", "system", "none"
	Version    string `json:"version"`
	IsValid    bool   `json:"isValid"`
	Error      string `json:"error,omitempty"`
}

// GetBinaryInfo returns the current resolved net-merge binary info.
func (c *Controller) GetBinaryInfo() BinaryInfo {
	c.mu.RLock()
	custom := c.config.BinaryPath
	c.mu.RUnlock()
	return ResolveBinary(custom)
}

// SetCustomBinaryPath updates the user-specified binary path in configuration.
func (c *Controller) SetCustomBinaryPath(path string) (BinaryInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	path = strings.TrimSpace(path)
	info := ResolveBinary(path)
	if path != "" && !info.IsValid {
		return info, fmt.Errorf("invalid net-merge binary at %q: %s", path, info.Error)
	}

	c.config.BinaryPath = path
	_ = c.saveConfigLocked()
	return info, nil
}

// ValidateBinary checks if a file at path exists, is executable, and responds to `version`.
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

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cleaned, "version")
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil && !strings.Contains(strings.ToLower(outStr), "net-merge") {
		return "", fmt.Errorf("binary failed execution: %w (output: %q)", err, outStr)
	}
	if !strings.Contains(strings.ToLower(outStr), "net-merge") {
		return "", fmt.Errorf("binary responded with unexpected output: %q", outStr)
	}
	return outStr, nil
}

// ResolveBinary probes candidate locations in priority order:
// 1. User custom path (if provided)
// 2. Same folder as NetPilot executable (or AppImage bundle host directory)
// 3. Current working directory
// 4. System PATH ($PATH / %PATH%)
// 5. Standard operating system installation paths
func ResolveBinary(customPath string) BinaryInfo {
	binName := "net-merge"
	if runtime.GOOS == "windows" {
		binName = "net-merge.exe"
	}

	// 1. User-specified custom path
	if custom := strings.TrimSpace(customPath); custom != "" {
		expanded := filepath.Clean(os.ExpandEnv(custom))
		ver, err := ValidateBinary(expanded)
		if err == nil {
			return BinaryInfo{
				Path:       expanded,
				CustomPath: custom,
				Exists:     true,
				Source:     "custom",
				Version:    ver,
				IsValid:    true,
			}
		}
		// Custom path was explicitly provided but failed validation
		return BinaryInfo{
			Path:       expanded,
			CustomPath: custom,
			Exists:     fileExists(expanded),
			Source:     "custom",
			IsValid:    false,
			Error:      err.Error(),
		}
	}

	// 2. Default: Same folder as NetPilot executable
	sameFolderCandidates := getSameFolderCandidates(binName)
	for _, cand := range sameFolderCandidates {
		if ver, err := ValidateBinary(cand); err == nil {
			return BinaryInfo{
				Path:       cand,
				CustomPath: "",
				Exists:     true,
				Source:     "same-folder",
				Version:    ver,
				IsValid:    true,
			}
		}
	}

	// 3. Current working directory
	cwd, err := os.Getwd()
	if err == nil {
		cwdPath := filepath.Join(cwd, binName)
		if ver, err := ValidateBinary(cwdPath); err == nil {
			return BinaryInfo{
				Path:       cwdPath,
				CustomPath: "",
				Exists:     true,
				Source:     "same-folder",
				Version:    ver,
				IsValid:    true,
			}
		}
	}

	// 4. System PATH
	if look, err := exec.LookPath(binName); err == nil {
		if ver, err := ValidateBinary(look); err == nil {
			return BinaryInfo{
				Path:       look,
				CustomPath: "",
				Exists:     true,
				Source:     "path",
				Version:    ver,
				IsValid:    true,
			}
		}
	}

	// 5. Standard platform locations
	standardPaths := getStandardPlatformPaths(binName)
	for _, p := range standardPaths {
		if ver, err := ValidateBinary(p); err == nil {
			return BinaryInfo{
				Path:       p,
				CustomPath: "",
				Exists:     true,
				Source:     "system",
				Version:    ver,
				IsValid:    true,
			}
		}
	}

	return BinaryInfo{
		Path:       "",
		CustomPath: "",
		Exists:     false,
		Source:     "none",
		IsValid:    false,
		Error:      "net-merge executable not found in same folder, PATH, or standard locations",
	}
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func getSameFolderCandidates(binName string) []string {
	var candidates []string

	// Main executable directory
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, binName))
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(exeDir, "net-merge-tray.exe"))
		}
	}

	// AppImage environment support
	if appimage := os.Getenv("APPIMAGE"); appimage != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(appimage), binName))
	}
	if owd := os.Getenv("OWD"); owd != "" {
		candidates = append(candidates, filepath.Join(owd, binName))
	}

	return candidates
}

func getStandardPlatformPaths(binName string) []string {
	var paths []string
	home := os.Getenv("HOME")

	switch runtime.GOOS {
	case "windows":
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			paths = append(paths, filepath.Join(pf, "NetPilot", binName))
			paths = append(paths, filepath.Join(pf, "net-merge", binName))
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			paths = append(paths, filepath.Join(local, "net-merge", binName))
		}
	case "darwin":
		paths = append(paths,
			"/usr/local/bin/net-merge",
			"/opt/net-merge/bin/net-merge",
			"/opt/homebrew/bin/net-merge",
		)
		if home != "" {
			paths = append(paths, filepath.Join(home, ".local/bin/net-merge"))
		}
	case "linux":
		paths = append(paths,
			"/usr/local/bin/net-merge",
			"/usr/bin/net-merge",
			"/opt/net-merge/bin/net-merge",
		)
		if home != "" {
			paths = append(paths, filepath.Join(home, ".local/bin/net-merge"))
			paths = append(paths, filepath.Join(home, "bin/net-merge"))
		}
	}

	return paths
}
