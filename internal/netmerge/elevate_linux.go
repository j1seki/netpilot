//go:build linux

package netmerge

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"netpilot/internal/execx"
)

// startElevatedLinux starts the net-merge runner with CAP_NET_ADMIN through an
// already-authorised sudo session or Polkit. An AppImage has an important special
// case: os.Executable() points at a FUSE-mounted file under /tmp/.mount*, which a
// root process spawned by pkexec cannot reliably access. Re-run the original
// AppImage instead and let it create a root-owned temporary extraction.
func startElevatedLinux(binaryPath string, args []string) error {
	target, targetArgs, appImage, err := linuxElevationTarget(binaryPath, args)
	if err != nil {
		return err
	}

	if sudoPath, err := exec.LookPath("sudo"); err == nil {
		checkCmd := exec.Command(sudoPath, "-n", "true")
		execx.PrepareCommand(checkCmd)
		if checkCmd.Run() == nil {
			cmd := exec.Command(sudoPath, append([]string{"-n", target}, targetArgs...)...)
			execx.PrepareCommand(cmd)
			return cmd.Start()
		}
	}

	pkexecPath, err := exec.LookPath("pkexec")
	if err != nil {
		if appImage != "" {
			return fmt.Errorf("NetPilot AppImage needs administrator permission to create the net-merge TUN adapter. Install a Polkit agent (pkexec) or run from a terminal: sudo %q --appimage-extract-and-run run", appImage)
		}
		return fmt.Errorf("CAP_NET_ADMIN or root required; install pkexec, run with sudo, or grant CAP_NET_ADMIN to an installed (non-AppImage) binary")
	}

	cmd := exec.Command(pkexecPath, append([]string{target}, targetArgs...)...)
	execx.PrepareCommand(cmd)
	return cmd.Start()
}

// linuxElevationTarget is separated for deterministic tests. It returns the
// executable and arguments that must cross the privilege boundary.
func linuxElevationTarget(binaryPath string, args []string) (target string, targetArgs []string, appImage string, err error) {
	if image := strings.TrimSpace(os.Getenv("APPIMAGE")); image != "" {
		info, statErr := os.Stat(image)
		if statErr != nil {
			return "", nil, image, fmt.Errorf("cannot access the AppImage at %q: %w", image, statErr)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", nil, image, fmt.Errorf("the AppImage at %q is not executable; run chmod +x on the AppImage file", image)
		}
		return image, append([]string{"--appimage-extract-and-run"}, args...), image, nil
	}
	return binaryPath, args, "", nil
}
