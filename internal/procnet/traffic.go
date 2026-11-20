package procnet

import (
	"context"
	"sync"
)

// TrafficSnapshot reports host traffic attributed to local PIDs by the OS.
type TrafficSnapshot struct {
	Source string          `json:"source,omitempty"`
	Detail string          `json:"detail"`
	Rates  map[int]float64 `json:"-"`
}

type TrafficMonitor struct {
	mu       sync.RWMutex
	snapshot TrafficSnapshot
}

func (m *TrafficMonitor) Run(ctx context.Context) {
	m.publish(TrafficSnapshot{Detail: "Starting host traffic measurement…"})
	m.run(ctx)
}

func (m *TrafficMonitor) Snapshot() TrafficSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Maps are replaced when a new sample is published.
	return m.snapshot
}

func (m *TrafficMonitor) publish(snapshot TrafficSnapshot) {
	m.mu.Lock()
	m.snapshot = snapshot
	m.mu.Unlock()
}
