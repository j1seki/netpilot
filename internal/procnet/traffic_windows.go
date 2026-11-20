//go:build windows && (amd64 || arm64)

package procnet

import (
	"context"
	"sync"
	"time"
)

func (m *TrafficMonitor) run(ctx context.Context) {
	type counter struct {
		bytes uint64
		last  time.Time
	}
	var mu sync.Mutex
	counters := make(map[int]counter)
	finished, stop, err := startNetworkTrace(func(pid int, size uint32) {
		mu.Lock()
		item := counters[pid]
		item.bytes += uint64(size)
		item.last = time.Now()
		counters[pid] = item
		mu.Unlock()
	})
	if err != nil {
		m.publish(TrafficSnapshot{Detail: "Windows traffic monitoring requires access to kernel ETW. Run NetPilot as administrator if access is denied. " + err.Error()})
		return
	}
	defer stop()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-finished:
			detail := "Windows network event tracing stopped."
			if err != nil {
				detail += " " + err.Error()
			}
			m.publish(TrafficSnapshot{Detail: detail})
			return
		case now := <-ticker.C:
			elapsed := now.Sub(last).Seconds()
			last = now
			rates := make(map[int]float64)
			mu.Lock()
			for pid, item := range counters {
				if now.Sub(item.last) > 15*time.Second {
					delete(counters, pid)
					continue
				}
				rates[pid] = float64(item.bytes) * 8 / 1e6 / elapsed
				item.bytes = 0
				counters[pid] = item
			}
			mu.Unlock()
			m.publish(TrafficSnapshot{Source: "windows-etw", Detail: "Windows ETW measures attributed TCP and UDP traffic across the host.", Rates: rates})
		}
	}
}
