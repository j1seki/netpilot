//go:build !windows && !linux && !darwin

package ttlmgr

import "runtime"

// NewBackend returns the fallback UnsupportedBackend for other platforms.
func NewBackend(Runner) Backend {
	return NewUnsupportedBackend("TTL and Hop Limit configuration is not supported on " + runtime.GOOS)
}
