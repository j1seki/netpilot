//go:build !windows && !darwin && !linux

package update

import "fmt"

func canAutoInstall() (bool, string) {
	return false, "Automatic install is not supported on this platform. Download the new release from GitHub."
}

func currentAsset(r *Release) (*Asset, error) {
	return nil, fmt.Errorf("no package is published for this platform")
}

func install(assetPath string, rel *Release) error {
	return fmt.Errorf("automatic install is not supported on this platform")
}
