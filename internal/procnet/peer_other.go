//go:build !linux && !darwin && !windows

package procnet

import "net"

func peerPID(net.Conn) (int, bool) { return 0, false }

func lookupPID(string, net.IP, uint16, net.IP, uint16) (int, bool) { return 0, false }

func processInfo(int) (string, string) { return "", "" }
