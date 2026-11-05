//go:build darwin

package ttlmgr

// NewBackend returns the Darwin sysctl TTL backend for macOS.
func NewBackend(Runner) Backend { return NewDarwinBackend() }
