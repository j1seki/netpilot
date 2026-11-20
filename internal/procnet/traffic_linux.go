package procnet

import (
	"context"
	"strconv"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type tcpTraffic struct {
	pid   int
	bytes uint64
}

func (m *TrafficMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous map[[2]uint32]tcpTraffic
	var at time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			sockets, err4 := netlink.SocketDiagTCPInfo(unix.AF_INET)
			sockets6, err6 := netlink.SocketDiagTCPInfo(unix.AF_INET6)
			if err4 != nil && err6 != nil {
				m.publish(TrafficSnapshot{Detail: "Linux TCP counters are unavailable: " + err4.Error()})
				previous = nil
				continue
			}
			owners := inodePIDMap(false)
			current := make(map[[2]uint32]tcpTraffic)
			rates := make(map[int]float64)
			elapsed := now.Sub(at).Seconds()
			for _, row := range append(sockets, sockets6...) {
				sock := row.InetDiagMsg
				if sock == nil || row.TCPInfo == nil || sock.INode == 0 || sock.ID.DestinationPort == 0 {
					continue
				}
				pid := owners[strconv.FormatUint(uint64(sock.INode), 10)]
				if pid <= 0 {
					continue
				}
				key := sock.ID.Cookie
				count := row.TCPInfo.Bytes_acked + row.TCPInfo.Bytes_received
				current[key] = tcpTraffic{pid: pid, bytes: count}
				if old, ok := previous[key]; ok && old.pid == pid && count >= old.bytes && elapsed > 0 {
					rates[pid] += float64(count-old.bytes) * 8 / 1e6 / elapsed
				}
			}
			previous, at = current, now
			m.publish(TrafficSnapshot{Source: "linux-tcp", Detail: "Linux measures visible TCP sockets. UDP and sockets that close between samples may be missing.", Rates: rates})
		}
	}
}
