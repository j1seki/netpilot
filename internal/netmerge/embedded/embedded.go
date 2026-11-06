package embedded

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

//go:embed bin/*
var binFS embed.FS

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
