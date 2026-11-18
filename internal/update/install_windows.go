//go:build windows

package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"netpilot/internal/execx"
)

func canAutoInstall() (bool, string) {
	if portableInstall() {
		return true, "The portable build will be updated in place; NetPilot will restart."
	}
	return true, "The installer will ask for administrator permission, then NetPilot will restart."
}

func currentAsset(r *Release) (*Asset, error) {
	if runtime.GOARCH != "amd64" {
		return nil, fmt.Errorf("no Windows package is published for windows/%s", runtime.GOARCH)
	}
	for _, suffix := range []string{
		"-windows-amd64-portable.zip",
		"-installer.exe",
	} {
		if asset, err := findAsset(r, suffix); err == nil {
			return asset, nil
		}
	}
	return nil, fmt.Errorf("no Windows package is published for windows/%s", runtime.GOARCH)
}

// portableInstall is true when NetPilot is not running from the default
// Program Files installation and its directory is writable. Those builds are
// updated from the portable ZIP instead of installing a second copy.
func portableInstall() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return false
	}
	for _, env := range []string{"ProgramW6432", "ProgramFiles"} {
		dir := strings.TrimSpace(os.Getenv(env))
		if dir == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(exe), strings.ToLower(dir)+string(os.PathSeparator)) {
			return false
		}
	}
	return ensureWritableDir(filepath.Dir(exe)) == nil
}

func install(assetPath string, rel *Release) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	if portableInstall() {
		return installPortable(assetPath, exe)
	}
	return installWithSetup(assetPath, exe)
}

// installWithSetup waits for NetPilot to exit, runs the signed Inno Setup
// installer with elevation, and relaunches the installed binary. If the user
// cancels the UAC prompt, the helper starts the old binary again.
func installWithSetup(installerPath, exe string) error {
	script, err := os.CreateTemp("", "netpilot-update-*.ps1")
	if err != nil {
		return err
	}
	name := script.Name()
	contents := `param(
  [int]$TargetPid,
  [string]$Installer,
  [string]$AppExe
)
while (Get-Process -Id $TargetPid -ErrorAction SilentlyContinue) {
  Start-Sleep -Milliseconds 500
}
try {
  Start-Process -FilePath $Installer ` +
		`-ArgumentList '/SILENT','/SUPPRESSMSGBOXES','/NORESTART','/CLOSEAPPLICATIONS' ` +
		`-Verb RunAs -Wait
} catch {
  # The user declined the UAC prompt; bring the old build back up.
} finally {
  Start-Process -FilePath $AppExe
}
`
	if _, err := script.WriteString(contents); err != nil {
		script.Close()
		os.Remove(name)
		return err
	}
	if err := script.Close(); err != nil {
		os.Remove(name)
		return err
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-File", name,
		strconv.Itoa(os.Getpid()), installerPath, exe)
	execx.PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		os.Remove(name)
		return fmt.Errorf("start the update helper: %w", err)
	}
	return cmd.Process.Release()
}

// installPortable extracts the ZIP next to a temporary helper, then waits for
// NetPilot to exit before replacing the executable and its DLLs in place.
func installPortable(zipPath, exe string) error {
	payload, err := os.MkdirTemp("", "netpilot-portable-*")
	if err != nil {
		return err
	}
	if err := extractPortable(zipPath, payload); err != nil {
		return err
	}

	script, err := os.CreateTemp("", "netpilot-update-*.cmd")
	if err != nil {
		return err
	}
	name := script.Name()
	contents := `@echo off
setlocal
set "PID=%~1"
set "SRC=%~2"
set "EXE=%~3"
set "WINTUN=%~4"
set "ICON=%~5"

:wait
tasklist /FI "PID eq %PID%" 2>nul | findstr /C:"%PID%" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)

copy /Y "%SRC%\netpilot.exe" "%EXE%" >nul
if errorlevel 1 (
  start "" "%EXE%"
  exit /b 1
)
if exist "%SRC%\wintun.dll" copy /Y "%SRC%\wintun.dll" "%WINTUN%" >nul
if exist "%SRC%\netpilot.ico" copy /Y "%SRC%\netpilot.ico" "%ICON%" >nul
start "" "%EXE%"
`
	if _, err := script.WriteString(contents); err != nil {
		script.Close()
		os.Remove(name)
		return err
	}
	if err := script.Close(); err != nil {
		os.Remove(name)
		return err
	}

	dir := filepath.Dir(exe)
	cmd := exec.Command("cmd.exe", "/C", name, strconv.Itoa(os.Getpid()), payload,
		exe, filepath.Join(dir, "wintun.dll"), filepath.Join(dir, "netpilot.ico"))
	execx.PrepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		os.Remove(name)
		return fmt.Errorf("start the update helper: %w", err)
	}
	return cmd.Process.Release()
}

func extractPortable(zipPath, dir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		switch {
		case strings.EqualFold(base, "netpilot.exe"),
			strings.EqualFold(base, "wintun.dll"),
			strings.EqualFold(base, "wintun-LICENSE.txt"),
			strings.EqualFold(base, "netpilot.ico"):
		default:
			continue
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(dir, base), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			in.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			in.Close()
			out.Close()
			return err
		}
		in.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "netpilot.exe")); err != nil {
		return fmt.Errorf("portable package does not contain netpilot.exe")
	}
	return nil
}
