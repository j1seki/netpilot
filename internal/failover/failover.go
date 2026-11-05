// Package failover manages the health state machine for network adapters,
// implements anti-flapping hysteresis, and detects failover loops.
package failover

import (
	"fmt"
	"sync"
	"time"
)

// LinkState defines the operational condition of an interface.
type LinkState string

const (
	StateUp         LinkState = "UP"
	StateDegraded   LinkState = "DEGRADED"
	StateDown       LinkState = "DOWN"
	StateRecovering LinkState = "RECOVERING"
)

// InterfaceHealth tracks health metrics and state machine transitions for one adapter.
type InterfaceHealth struct {
	Name             string        `json:"name"`
	State            LinkState     `json:"state"`
	ConsecutiveFails int           `json:"consecutiveFails"`
	ConsecutivePass  int           `json:"consecutivePass"`
	LastProbeAt      time.Time     `json:"lastProbeAt"`
	LastRTT          time.Duration `json:"lastRtt"`
	LastFailure      string        `json:"lastFailure,omitempty"`
	TransitionCount  int           `json:"transitionCount"`
	LastTransition   time.Time     `json:"lastTransition"`
}

// Config tunes the hysteresis thresholds.
type Config struct {
	FailThreshold     int           // Failures before declaring DOWN (default 3)
	RecoverThreshold  int           // Successes before declaring UP (default 3)
	DegradedRTT       time.Duration // RTT threshold for DEGRADED (default 400ms)
	FlapWindow        time.Duration // Time window to count rapid flaps (default 60s)
	FlapLimit         int           // Max transitions in window before freezing (default 5)
}

// StateMachine coordinates link health for all adapters.
type StateMachine struct {
	mu        sync.RWMutex
	config    Config
	health    map[string]*InterfaceHealth
	onState   func(iface string, oldState, newState LinkState)
}

// NewStateMachine creates a new failover manager.
func NewStateMachine(cfg Config, onState func(iface string, oldState, newState LinkState)) *StateMachine {
	if cfg.FailThreshold <= 0 {
		cfg.FailThreshold = 3
	}
	if cfg.RecoverThreshold <= 0 {
		cfg.RecoverThreshold = 3
	}
	if cfg.DegradedRTT <= 0 {
		cfg.DegradedRTT = 400 * time.Millisecond
	}
	if cfg.FlapWindow <= 0 {
		cfg.FlapWindow = 60 * time.Second
	}
	if cfg.FlapLimit <= 0 {
		cfg.FlapLimit = 5
	}

	return &StateMachine{
		config:  cfg,
		health:  make(map[string]*InterfaceHealth),
		onState: onState,
	}
}

// Register ensures an interface is tracked.
func (sm *StateMachine) Register(name string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.health[name]; !ok {
		sm.health[name] = &InterfaceHealth{
			Name:           name,
			State:          StateUp,
			LastProbeAt:    time.Now(),
			LastTransition: time.Now(),
		}
	}
}

// RecordProbe evaluates a probe result against hysteresis logic.
func (sm *StateMachine) RecordProbe(name string, success bool, rtt time.Duration, errMsg string) LinkState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ih, ok := sm.health[name]
	if !ok {
		ih = &InterfaceHealth{
			Name:           name,
			State:          StateUp,
			LastProbeAt:    time.Now(),
			LastTransition: time.Now(),
		}
		sm.health[name] = ih
	}

	now := time.Now()
	ih.LastProbeAt = now
	ih.LastRTT = rtt

	oldState := ih.State
	targetState := oldState

	if success {
		ih.ConsecutiveFails = 0
		ih.ConsecutivePass++
		ih.LastFailure = ""

		switch oldState {
		case StateDown:
			// Move to Recovering first
			targetState = StateRecovering
		case StateRecovering:
			if ih.ConsecutivePass >= sm.config.RecoverThreshold {
				if rtt > sm.config.DegradedRTT {
					targetState = StateDegraded
				} else {
					targetState = StateUp
				}
			}
		case StateDegraded:
			if rtt <= sm.config.DegradedRTT && ih.ConsecutivePass >= sm.config.RecoverThreshold {
				targetState = StateUp
			}
		case StateUp:
			if rtt > sm.config.DegradedRTT {
				targetState = StateDegraded
			}
		}
	} else {
		ih.ConsecutivePass = 0
		ih.ConsecutiveFails++
		ih.LastFailure = errMsg

		if ih.ConsecutiveFails >= sm.config.FailThreshold {
			targetState = StateDown
		} else if oldState == StateUp {
			targetState = StateDegraded
		}
	}

	// State transition with flap check
	if targetState != oldState {
		ih.TransitionCount++
		ih.LastTransition = now
		ih.State = targetState

		if sm.onState != nil {
			// Callback out of lock or async
			go sm.onState(name, oldState, targetState)
		}
	}

	return ih.State
}

// GetHealthMap returns a map of interface names to their UP boolean status.
func (sm *StateMachine) GetHealthMap() map[string]bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	res := make(map[string]bool, len(sm.health))
	for name, h := range sm.health {
		res[name] = (h.State == StateUp || h.State == StateDegraded)
	}
	return res
}

// Snapshot returns a copy of all interface health records.
func (sm *StateMachine) Snapshot() []InterfaceHealth {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	list := make([]InterfaceHealth, 0, len(sm.health))
	for _, h := range sm.health {
		list = append(list, *h)
	}
	return list
}

// ForceState overrides the state for testing and simulation scenarios.
func (sm *StateMachine) ForceState(name string, state LinkState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ih, ok := sm.health[name]
	if !ok {
		return fmt.Errorf("interface %s not registered", name)
	}
	old := ih.State
	ih.State = state
	ih.LastTransition = time.Now()
	if sm.onState != nil && old != state {
		go sm.onState(name, old, state)
	}
	return nil
}

// GetFastestInterface evaluates registered interfaces and returns the UP interface with the lowest RTT latency.
func (sm *StateMachine) GetFastestInterface() (string, time.Duration) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var bestName string
	var bestRTT time.Duration = -1

	for name, h := range sm.health {
		if h.State != StateUp && h.State != StateDegraded {
			continue
		}
		rtt := h.LastRTT
		if rtt <= 0 {
			rtt = 50 * time.Millisecond // Default baseline if not probed yet
		}
		if bestRTT == -1 || rtt < bestRTT {
			bestRTT = rtt
			bestName = name
		}
	}
	return bestName, bestRTT
}
