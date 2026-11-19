package engine

import (
	"sync"
	"sync/atomic"
	"time"
)

type trafficCounter struct {
	bytes atomic.Int64
	flows int // guarded by processTraffic.mu
}

func (c *trafficCounter) add(n int) {
	if c != nil && n > 0 {
		c.bytes.Add(int64(n))
	}
}

type processTraffic struct {
	mu    sync.Mutex
	byPID map[int]*trafficCounter
	last  time.Time
	rates map[int]float64
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
	p.mu.Unlock()
	return c, func() { p.mu.Lock(); c.flows--; p.mu.Unlock() }
}

// ProcessRates reports payload in both directions for flows with a known PID.
// Published maps are immutable, so callers can share a sample without copying it.
func (b *Balancer) ProcessRates(now time.Time) map[int]float64 {
	p := &b.processTraffic
	p.mu.Lock()
	defer p.mu.Unlock()
	if elapsed := now.Sub(p.last).Seconds(); !p.last.IsZero() && elapsed >= 0.8 {
		p.rates = make(map[int]float64, len(p.byPID))
		for pid, c := range p.byPID {
			p.rates[pid] = float64(c.bytes.Swap(0)) * 8 / 1e6 / elapsed
			if c.flows == 0 {
				delete(p.byPID, pid)
			}
		}
		p.last = now
	}
	return p.rates
}
