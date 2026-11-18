//go:build linux

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

func canAutoInstall() (bool, string) {
	target := os.Getenv("APPIMAGE")
	if target == "" {
		return false, "Automatic install is available for the AppImage build. Download the new release from GitHub."
	}
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	if _, err := os.Stat(target); err != nil {
		return false, "This AppImage could not be located on disk; download the new release from GitHub."
	}
	if err := ensureWritableDir(filepath.Dir(target)); err != nil {
		return false, "The AppImage directory is not writable. Move NetPilot somewhere writable or download the release manually."
	}
	return true, ""
}

func currentAsset(r *Release) (*Asset, error) {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return findAsset(r, "-linux-"+runtime.GOARCH+".AppImage")
	default:
		return nil, fmt.Errorf("no AppImage is published for linux/%s", runtime.GOARCH)
	}
}

// install hands the replacement to a helper that waits for this process to exit,
// swaps the AppImage, and starts the new one. The running AppImage keeps its old
// inode, so replacing the path is safe.
func install(assetPath string, rel *Release) error {
	target := os.Getenv("APPIMAGE")
	if target == "" {
		return fmt.Errorf("not running from an AppImage")
	}
	// The helper inherits this process's working directory, which is not
	// necessarily the directory the AppImage was launched from.
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("locate the AppImage: %w", err)
	}
	target = abs
	dir := filepath.Dir(target)
	newPath := filepath.Join(dir, "."+filepath.Base(target)+".new")
	if err := copyFile(assetPath, newPath, 0o755); err != nil {
		return fmt.Errorf("stage the new AppImage: %w", err)
	}

	script, err := writeExecutableScript(`#!/bin/sh
set -eu
pid="$1"
target="$2"
new="$3"
old="$4"

# Wait for the old process to release the UI and its loopback API port.
while kill -0 "$pid" 2>/dev/null; do sleep 0.5; done

rm -f "$old"
if ! mv "$target" "$old"; then
  "$target" >/dev/null 2>&1 &
  exit 1
fi
if ! mv "$new" "$target"; then
  mv "$old" "$target"
  "$target" >/dev/null 2>&1 &
  exit 1
fi
chmod 755 "$target"

"$target" >/dev/null 2>&1 &
new_pid=$!

# Keep the old binary until the new one has survived its first seconds.
sleep 3
if kill -0 "$new_pid" 2>/dev/null; then
  rm -f "$old"
fi
`)
	if err != nil {
		os.Remove(newPath)
		return err
	}

	cmd := exec.Command("/bin/sh", script,
		fmt.Sprint(os.Getpid()), target, newPath, target+".old")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		os.Remove(script)
		os.Remove(newPath)
		return fmt.Errorf("start the update helper: %w", err)
	}
	return cmd.Process.Release()
}
