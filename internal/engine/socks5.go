package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"

	"netpilot/internal/procnet"
)

func ServeSocks5(ln net.Listener, b *Balancer) error {
	return ServeSocks5Context(context.Background(), ln, b)
}

// ServeSocks5Context closes the listener and client connections on cancellation.
func ServeSocks5Context(ctx context.Context, ln net.Listener, b *Balancer) error {
	stopClosingListener := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer stopClosingListener()
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go b.handleSocks5(ctx, c)
	}
}

func socksReply(c net.Conn, rep byte) {
	c.Write([]byte{0x05, rep, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
}

func (b *Balancer) handleSocks5(ctx context.Context, c net.Conn) {
	defer c.Close()
	stopClosingClient := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stopClosingClient()
	c.SetDeadline(time.Now().Add(15 * time.Second))

	app := procnet.Identity{}
	if pid, ok := procnet.PeerPID(c); ok {
		app = procnet.Identify(pid)
	}

	var hdr [2]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil || hdr[0] != 0x05 || hdr[1] == 0 {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	if !bytes.Contains(methods, []byte{0x00}) {
		c.Write([]byte{0x05, 0xff})
		return
	}
	c.Write([]byte{0x05, 0x00})

	var req [4]byte
	if _, err := io.ReadFull(c, req[:]); err != nil || req[0] != 0x05 || req[2] != 0 {
		return
	}
	// Every SOCKS5 command carries an address and port after the request header.
	// Consume the complete request before rejecting unsupported commands; closing a
	// TCP connection with unread request bytes can send a reset and discard the reply.
	var host string
	switch req[3] {
	case 0x01:
		var a [4]byte
		if _, err := io.ReadFull(c, a[:]); err != nil {
			return
		}
		host = net.IP(a[:]).String()
	case 0x03:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil || l[0] == 0 {
			return
		}
		name := make([]byte, l[0])
		if _, err := io.ReadFull(c, name); err != nil {
			return
		}
		host = string(name)
	case 0x04:
		var a [16]byte
		if _, err := io.ReadFull(c, a[:]); err != nil {
			return
		}
		host = net.IP(a[:]).String()
	default:
		socksReply(c, repAtypUnsupported)
		return
	}
	var p [2]byte
	if _, err := io.ReadFull(c, p[:]); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(p[:]))

	if req[1] != 0x01 { // CONNECT only; UDP ASSOCIATE comes with the TUN front-end
		socksReply(c, repCmdUnsupported)
		return
	}

	policy := b.resolvePolicy(app.Name, app.Path, host)
	up, sl, fam, err := b.DialTCPFor(ctx, host, port, policy)
	if err != nil {
		rep := byte(repFailure)
		var de *dialError
		if errors.As(err, &de) {
			rep = de.rep
		}
		socksReply(c, rep)
		return
	}
	stopClosingUpstream := context.AfterFunc(ctx, func() { _ = up.Close() })
	defer stopClosingUpstream()
	c.SetDeadline(time.Time{})
	socksReply(c, repOK)
	traffic, stopTracking := b.trackProcess(app.PID)
	defer stopTracking()
	b.recordProcessTCP(app.PID, host, port, sl.Name)
	in, out := relayWithTraffic(c, up, sl, fam, b.S, policy.BandwidthLimitMbps, traffic)
	b.reportUsage(policy.AppName, in, out)
}
