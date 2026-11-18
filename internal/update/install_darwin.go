//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func canAutoInstall() (bool, string) {
	app, err := appBundle()
	if err != nil {
		return false, err.Error()
	}
	if err := ensureWritableDir(filepath.Dir(app)); err != nil {
		return false, "NetPilot is not in a writable location. Move NetPilot.app to Applications or install the DMG manually."
	}
	return true, ""
}

func currentAsset(r *Release) (*Asset, error) {
	return findAsset(r, "-macos-universal.dmg")
}

// appBundle derives .../NetPilot.app from .../NetPilot.app/Contents/MacOS/netpilot.
func appBundle() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	app := exe
	for i := 0; i < 3; i++ {
		app = filepath.Dir(app)
	}
	if !strings.HasSuffix(app, ".app") {
		return "", fmt.Errorf("NetPilot is not running from a .app bundle; install the DMG manually")
	}
	if strings.Contains(app, "/AppTranslocation/") {
		return "", fmt.Errorf("macOS is running NetPilot from a temporary location. Move NetPilot.app to Applications first")
	}
	return app, nil
}

// install mounts the DMG after NetPilot exits, stages the new app next to the
// old one, swaps them, and reopens the app. The old bundle is kept until the new
// one is safely in place.
func install(assetPath string, rel *Release) error {
	app, err := appBundle()
	if err != nil {
		return err
	}
	script, err := writeExecutableScript(`#!/bin/sh
set -eu
pid="$1"
dmg="$2"
app="$3"

while kill -0 "$pid" 2>/dev/null; do sleep 1; done

mnt=$(mktemp -d "${TMPDIR:-/tmp}/netpilot-dmg.XXXXXX")
success=0
cleanup() {
  # Any failure before the new bundle is in place must bring the old app back.
  if [ "$success" -eq 0 ]; then
    open "$app" >/dev/null 2>&1 || true
  fi
  hdiutil detach "$mnt" >/dev/null 2>&1 || true
  rmdir "$mnt" >/dev/null 2>&1 || true
}
trap cleanup EXIT

hdiutil attach "$dmg" -nobrowse -readonly -mountpoint "$mnt" >/dev/null

stage="${app}.update"
old="${app}.old"
rm -rf "$stage" "$old"
ditto "$mnt/NetPilot.app" "$stage"
xattr -dr com.apple.quarantine "$stage" >/dev/null 2>&1 || true

if ! mv "$app" "$old"; then
  exit 1
fi
if ! mv "$stage" "$app"; then
  mv "$old" "$app" || true
  exit 1
fi

success=1
open "$app"

# Keep a rollback copy briefly, then clean up the download.
sleep 5
rm -rf "$old"
rm -f "$dmg"
`)
	if err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", script, fmt.Sprint(os.Getpid()), assetPath, app)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		os.Remove(script)
		return fmt.Errorf("start the update helper: %w", err)
	}
	return cmd.Process.Release()
}
