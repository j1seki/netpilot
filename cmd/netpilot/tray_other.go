//go:build !linux

package main

func prepareLinuxTrayEnvironment() {}

func isLinuxTrayHostAvailable() bool {
	return true
}
