// Package mode implements the execution modes: Live (real system operation) and Dry-Run (inspection only).
package mode

import (
	"errors"
	"fmt"
	"sync"

	"netpilot/internal/capability"
)

// ExecutionMode represents the active operational posture of NetPilot.
type ExecutionMode string

const (
	ModeLive   ExecutionMode = "LIVE"
	ModeDryRun ExecutionMode = "DRY_RUN"
)

var (
	ErrInvalidTransition = errors.New("invalid execution mode transition requested")
)

// State describes the current mode and its safety guarantees.
type State struct {
	CurrentMode     ExecutionMode `json:"currentMode"`
	Description     string        `json:"description"`
	AllowsSystemMod bool          `json:"allowsSystemMod"`
	RequiresRoot    bool          `json:"requiresRoot"`
	ActiveWarning   string        `json:"activeWarning,omitempty"`
}

// Controller manages the application mode.
type Controller struct {
	mu           sync.RWMutex
	current      ExecutionMode
	capabilities capability.Report
}

// NewController initializes the mode controller in LIVE mode.
func NewController(caps capability.Report) *Controller {
	return &Controller{
		current:      ModeLive,
		capabilities: caps,
	}
}

// GetState returns the current execution mode state.
func (c *Controller) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.describe(c.current)
}

// Current returns the current mode directly.
func (c *Controller) Current() ExecutionMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// SetMode transitions to a requested mode.
func (c *Controller) SetMode(target ExecutionMode, forceLive bool) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch target {
	case ModeLive:
		c.current = ModeLive
		return c.describe(c.current), nil

	case ModeDryRun:
		c.current = ModeDryRun
		return c.describe(c.current), nil

	default:
		return c.describe(c.current), fmt.Errorf("%w: %s", ErrInvalidTransition, target)
	}
}

func (c *Controller) describe(m ExecutionMode) State {
	switch m {
	case ModeLive:
		return State{
			CurrentMode:     ModeLive,
			Description:     "Live Mode: System network traffic is inspected and managed across real interfaces and real running processes.",
			AllowsSystemMod: true,
			RequiresRoot:    false,
			ActiveWarning:   "Live real-time operation active on host.",
		}
	case ModeDryRun:
		return State{
			CurrentMode:     ModeDryRun,
			Description:     "Dry-Run Mode: Inspects real system interfaces and processes, but suppresses active route modifications.",
			AllowsSystemMod: false,
			RequiresRoot:    false,
			ActiveWarning:   "Inspection active. Route modifications disabled.",
		}
	default:
		return State{
			CurrentMode: m,
			Description: "Unknown operational mode",
		}
	}
}
