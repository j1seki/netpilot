//go:build !windows

package apps

func (m *Manager) discoverWindows() {
	// No-op on non-Windows platforms
}
