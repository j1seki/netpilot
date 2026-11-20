package engine

import (
	"sync"
	"sync/atomic"
	"time"
)

type trafficCounter struct {
	bytes atomic.Int64
	flows int // guarded by processTraffic.mu
	idle  time.Time
}

func (c *trafficCounter) add(n int) {
	if c != nil && n > 0 {
		c.bytes.Add(int64(n))
	}
}

type processTraffic struct {
	mu      sync.Mutex
	byPID   map[int]*trafficCounter
	last    time.Time
	rates   map[int]float64
	windows []trafficWindow
}

type trafficWindow struct {
	start time.Time
	end   time.Time
	bytes map[int]int64
}

func (b *Balancer) trackProcess(pid int) (*trafficCounter, func()) {
	if pid <= 0 {
		return nil, func() {}
	}
	p := &b.processTraffic
	p.mu.Lock()
	if p.byPID == nil {
		p.byPID = make(map[int]*trafficCounter)
		p.last = time.Now()
	}
	c := p.byPID[pid]
	if c == nil {
		c = &trafficCounter{}
		p.byPID[pid] = c
	}
	c.flows++
	c.idle = time.Time{}
	p.mu.Unlock()
	return c, func() { p.mu.Lock(); c.flows--; p.mu.Unlock() }
}

// ProcessRates reports payload in both directions for flows with a known PID.
// Published maps are immutable, so callers can share a sample without copying it.
func (b *Balancer) ProcessRates(now time.Time) map[int]float64 {
	p := &b.processTraffic
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.last.IsZero() && now.Sub(p.last) >= 800*time.Millisecond {
		sample := trafficWindow{start: p.last, end: now, bytes: make(map[int]int64)}
		p.rates = make(map[int]float64, len(p.byPID))
		for pid, c := range p.byPID {
			sample.bytes[pid] = c.bytes.Swap(0)
			if c.flows == 0 {
				if c.idle.IsZero() {
					c.idle = now
				} else if now.Sub(c.idle) >= 15*time.Second {
					delete(p.byPID, pid)
					delete(sample.bytes, pid)
				}
			}
		}
		// Average several intervals so the UI doesn't miss a short flow when it
		// polls less often than an elevated engine writes its one-second samples.
		cutoff := now.Add(-3 * time.Second)
		for len(p.windows) > 0 && !p.windows[0].end.After(cutoff) {
			p.windows = p.windows[1:]
		}
		p.windows = append(p.windows, sample)
		elapsed := now.Sub(p.windows[0].start).Seconds()
		for pid := range sample.bytes {
			var total int64
			for _, window := range p.windows {
				total += window.bytes[pid]
			}
			p.rates[pid] = float64(total) * 8 / 1e6 / elapsed
		}
		p.last = now
	}
	return p.rates
}
