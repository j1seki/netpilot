package update

import (
	"fmt"
	"io"
	"os"
)

// CanAutoInstall reports whether this build can replace itself with a downloaded
// release, along with a human-readable hint when it cannot.
func CanAutoInstall() (bool, string) { return canAutoInstall() }

// CurrentAsset picks the downloadable package for this OS/architecture and
// install mode (for example the Windows installer versus the portable ZIP).
func CurrentAsset(r *Release) (*Asset, error) { return currentAsset(r) }

// Install starts the platform installer or a small helper that replaces the app
// after this process exits. It returns as soon as the helper is running; the
// caller is responsible for quitting NetPilot.
func Install(assetPath string, rel *Release) error {
	if rel == nil {
		return fmt.Errorf("no release to install")
	}
	if _, err := os.Stat(assetPath); err != nil {
		return fmt.Errorf("download missing: %w", err)
	}
	return install(assetPath, rel)
}

// copyFile writes src to dst with the given permissions, replacing dst.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Chmod(dst, perm)
}

// ensureWritableDir checks a directory by creating and removing a small file.
func ensureWritableDir(dir string) error {
	f, err := os.CreateTemp(dir, ".netpilot-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// writeExecutableScript creates a temporary shell script for the Unix helpers.
func writeExecutableScript(contents string) (string, error) {
	f, err := os.CreateTemp("", "netpilot-update-*.sh")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(contents); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Chmod(name, 0o755); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}
