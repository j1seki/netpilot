package embedded

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

//go:embed bin/*
var binFS embed.FS

func ReleaseTag() string {
	b, _ := binFS.ReadFile("bin/release.tag")
	parts := strings.Fields(string(b))
	arch := runtime.GOARCH
	if runtime.GOOS == "darwin" {
		arch = "universal"
	}
	if len(parts) != 2 || parts[1] != runtime.GOOS+"/"+arch {
		return ""
	}
	return parts[0]
}

var releaseMu sync.Mutex
var releasePaths = map[string]string{}

func ReleaseCLI(dir string) (string, error) {
	tag := ReleaseTag()
	if tag == "" {
		return "", fmt.Errorf("no bundled release for this platform")
	}
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if p := releasePaths[dir]; p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	cli := "net-merge"
	if runtime.GOOS == "windows" {
		cli += ".exe"
	}
	dst := filepath.Join(dir, "bundled-"+tag)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" && extractOne(dst, "wintun.dll") == "" {
		return "", fmt.Errorf("bundled Wintun is unavailable")
	}
	p := extractOne(dst, cli)
	if p == "" {
		return "", fmt.Errorf("bundled net-merge executable is unavailable")
	}
	releasePaths[dir] = p
	return p, nil
}

var (
	extractOnce sync.Once
	cliPath     string
	trayPath    string
)

// ExtractBinaries writes any embedded net-merge and net-merge-tray (plus wintun.dll on Windows)
// binaries to stateDir/bin if present in the embedded filesystem, returning their paths.
// On Windows, if wintun.dll is embedded, it is also extracted beside the current executable
// so WireGuard/Wintun can load it directly from a single-file portable netpilot.exe.
func ExtractBinaries(stateDir string) (cli string, tray string) {
	extractOnce.Do(func() {
		if runtime.GOOS == "windows" {
			if exe, err := os.Executable(); err == nil {
				_ = extractOne(filepath.Dir(exe), "wintun.dll")
			}
		}
		if stateDir == "" {
			return
		}
		outDir := filepath.Join(stateDir, "bin")
		_ = os.MkdirAll(outDir, 0o755)

		cliName := "net-merge"
		trayName := "net-merge-tray"
		if runtime.GOOS == "windows" {
			cliName = "net-merge.exe"
			trayName = "net-merge-tray.exe"
		}

		if p := extractOne(outDir, cliName); p != "" {
			cliPath = p
		}
		if p := extractOne(outDir, trayName); p != "" {
			trayPath = p
		}
		if runtime.GOOS == "windows" {
			_ = extractOne(outDir, "wintun.dll")
		}
	})
	return cliPath, trayPath
}

// EnsureWintunDLL ensures wintun.dll is extracted next to the executable (or stateDir/bin) on Windows.
func EnsureWintunDLL(stateDir string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dllPath := filepath.Join(exeDir, "wintun.dll")
		if _, err := os.Stat(dllPath); err == nil {
			return dllPath
		}
		if p := extractOne(exeDir, "wintun.dll"); p != "" {
			return p
		}
	}
	if stateDir != "" {
		outDir := filepath.Join(stateDir, "bin")
		_ = os.MkdirAll(outDir, 0o755)
		if p := extractOne(outDir, "wintun.dll"); p != "" {
			return p
		}
	}
	return ""
}

func extractOne(outDir, name string) string {
	data, err := binFS.ReadFile("bin/" + name)
	if err != nil || len(data) == 0 {
		return ""
	}
	dst := filepath.Join(outDir, name)
	if existing, err := os.ReadFile(dst); err == nil && bytes.Equal(existing, data) {
		if runtime.GOOS != "windows" {
			_ = os.Chmod(dst, 0o755)
		}
		return dst
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return ""
	}
	return dst
}
