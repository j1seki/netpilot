//go:build !linux && !darwin && !windows

package procnet

import (
	"context"
	"errors"
	"net"
)

func processSockets(context.Context, int) ([]Socket, error) {
	return nil, errors.New("socket inspection is unavailable on this platform")
}

func peerPID(net.Conn) (int, bool) { return 0, false }

func lookupPID(string, net.IP, uint16, net.IP, uint16) (int, bool) { return 0, false }

func processInfo(int) (string, string) { return "", "" }
