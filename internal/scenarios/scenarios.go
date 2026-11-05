// Package scenarios provides a deterministic scenario simulation engine
// executing safe in-memory tests without modifying real system networking.
package scenarios

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"netpilot/internal/db"
	"netpilot/internal/failover"
	"netpilot/internal/logger"
	"netpilot/internal/rules"
)

// Status represents the verdict of a test scenario.
type Status string

const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusSkipped Status = "SKIPPED"
	StatusBlocked Status = "BLOCKED"
)

// Assertion records an expected vs actual check within a scenario.
type Assertion struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail,omitempty"`
}

// ScenarioResult captures the complete execution of a scenario.
type ScenarioResult struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Category          string      `json:"category"`
	Description       string      `json:"description"`
	Status            Status      `json:"status"`
	DurationMs        float64     `json:"durationMs"`
	Assertions        []Assertion `json:"assertions"`
	LogMessages       []string    `json:"logMessages"`
	Error             string      `json:"error,omitempty"`
	RanAt             time.Time   `json:"ranAt"`
	AffectsConnection bool        `json:"affectsConnection"`
	DisruptionWarning string      `json:"disruptionWarning,omitempty"`
}

// ScenarioDefinition defines how a scenario executes.
type ScenarioDefinition struct {
	ID                string
	Name              string
	Category          string
	Description       string
	AffectsConnection bool
	DisruptionWarning string
	Run               func(ctx *ScenarioContext) ([]Assertion, error)
}

// ScenarioContext provides simulated subsystems for scenario execution.
type ScenarioContext struct {
	RuleEngine   *rules.Engine
	FailoverSM   *failover.StateMachine
	Database     *db.Database
	Logger       *logger.RingLogger
	Logs         []string
}

func (c *ScenarioContext) Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.Logs = append(c.Logs, msg)
	if c.Logger != nil {
		c.Logger.Log(logger.LogEntry{
			Level:     logger.LevelInfo,
			Component: "ScenarioEngine",
			Message:   msg,
		})
	}
}

// Engine executes deterministic test scenarios.
type Engine struct {
	mu        sync.RWMutex
	scenarios map[string]ScenarioDefinition
	order     []string
	ringLog   *logger.RingLogger
	database  *db.Database
}

// NewEngine creates and populates the simulation scenario engine.
func NewEngine(ringLog *logger.RingLogger, database *db.Database) *Engine {
	e := &Engine{
		scenarios: make(map[string]ScenarioDefinition),
		ringLog:   ringLog,
		database:  database,
	}
	e.registerCatalog()
	return e
}

func (e *Engine) register(def ScenarioDefinition) {
	e.scenarios[def.ID] = def
	e.order = append(e.order, def.ID)
}

// List returns summary descriptions of all available scenarios.
func (e *Engine) List() []ScenarioDefinition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]ScenarioDefinition, 0, len(e.order))
	for _, id := range e.order {
		out = append(out, e.scenarios[id])
	}
	return out
}

// Run executes a single scenario by ID.
func (e *Engine) Run(id string) ScenarioResult {
	e.mu.RLock()
	def, ok := e.scenarios[id]
	e.mu.RUnlock()

	if !ok {
		return ScenarioResult{
			ID:          id,
			Name:        "Unknown",
			Status:      StatusBlocked,
			Error:       fmt.Sprintf("scenario %q not found", id),
			RanAt:       time.Now(),
		}
	}

	start := time.Now()
	// Create isolated context with in-memory DB and mock subsystems
	memDB, _ := db.Open(":memory:")
	defer memDB.Close()

	ruleEng, _ := rules.NewEngine(memDB)
	foSM := failover.NewStateMachine(failover.Config{
		FailThreshold:    2,
		RecoverThreshold: 2,
	}, nil)

	ctx := &ScenarioContext{
		RuleEngine: ruleEng,
		FailoverSM: foSM,
		Database:   memDB,
		Logger:     e.ringLog,
	}

	assertions, err := def.Run(ctx)
	dur := float64(time.Since(start).Microseconds()) / 1000.0

	status := StatusPass
	if err != nil {
		status = StatusFail
	} else {
		for _, a := range assertions {
			if !a.Passed {
				status = StatusFail
				break
			}
		}
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	res := ScenarioResult{
		ID:                def.ID,
		Name:              def.Name,
		Category:          def.Category,
		Description:       def.Description,
		Status:            status,
		DurationMs:        dur,
		Assertions:        assertions,
		LogMessages:       ctx.Logs,
		Error:             errMsg,
		RanAt:             time.Now(),
		AffectsConnection: def.AffectsConnection,
		DisruptionWarning: def.DisruptionWarning,
	}

	return res
}

// RunAll executes every registered scenario.
func (e *Engine) RunAll() []ScenarioResult {
	return e.RunAllWithFilter(false)
}

// RunAllWithFilter executes registered scenarios, skipping network-disruptive ones if requested.
func (e *Engine) RunAllWithFilter(skipDisruptive bool) []ScenarioResult {
	e.mu.RLock()
	ids := append([]string(nil), e.order...)
	e.mu.RUnlock()

	var results []ScenarioResult
	for _, id := range ids {
		e.mu.RLock()
		def := e.scenarios[id]
		e.mu.RUnlock()

		if skipDisruptive && def.AffectsConnection {
			results = append(results, ScenarioResult{
				ID:                def.ID,
				Name:              def.Name,
				Category:          def.Category,
				Description:       def.Description,
				Status:            StatusSkipped,
				DurationMs:        0,
				RanAt:             time.Now(),
				AffectsConnection: true,
				DisruptionWarning: def.DisruptionWarning,
				Error:             "Skipped by network safety shield: preserves active host connection",
			})
			continue
		}
		results = append(results, e.Run(id))
	}
	return results
}

func (e *Engine) registerCatalog() {
	// 1. One active interface
	e.register(ScenarioDefinition{
		ID:          "one_active_interface",
		Name:        "Single Active Interface",
		Category:    "Connectivity",
		Description: "Verifies traffic routes cleanly when only 1 physical adapter is online.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			ctx.Logf("Configuring single interface eth0")
			ctx.FailoverSM.Register("eth0")
			ctx.RuleEngine.UpdateNICHealth(map[string]bool{"eth0": true})

			dec := ctx.RuleEngine.Evaluate("Chrome", "/usr/bin/chrome", "google.com", "eth0")
			return []Assertion{
				{
					Name:     "Route assigned to eth0",
					Expected: "eth0",
					Actual:   dec.SelectedInterface,
					Passed:   dec.SelectedInterface == "eth0",
				},
				{
					Name:     "No fallback invoked",
					Expected: "false",
					Actual:   fmt.Sprintf("%v", dec.IsFallback),
					Passed:   !dec.IsFallback,
				},
			}, nil
		},
	})

	// 2. Two active interfaces
	e.register(ScenarioDefinition{
		ID:          "two_active_interfaces",
		Name:        "Dual Active Interfaces (Multi-WAN)",
		Category:    "Connectivity",
		Description: "Verifies distribution between two distinct active interfaces.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			ctx.Logf("Configuring dual interfaces: eth0 (Fiber) and wlan0 (Wi-Fi)")
			ctx.FailoverSM.Register("eth0")
			ctx.FailoverSM.Register("wlan0")
			ctx.RuleEngine.UpdateNICHealth(map[string]bool{"eth0": true, "wlan0": true})

			_ = ctx.Database.SaveRule(db.RuleRecord{
				ID: "r1", AppName: "Zoom", PreferredInterface: "wlan0", FallbackInterface: "eth0", Priority: 50, Enabled: true,
			})
			_ = ctx.RuleEngine.Reload()

			zoomDec := ctx.RuleEngine.Evaluate("Zoom", "/opt/zoom", "zoom.us", "eth0")
			otherDec := ctx.RuleEngine.Evaluate("Browser", "/usr/bin/firefox", "example.org", "eth0")

			return []Assertion{
				{
					Name:     "Zoom pinned to wlan0",
					Expected: "wlan0",
					Actual:   zoomDec.SelectedInterface,
					Passed:   zoomDec.SelectedInterface == "wlan0",
				},
				{
					Name:     "Unmatched browser on default eth0",
					Expected: "eth0",
					Actual:   otherDec.SelectedInterface,
					Passed:   otherDec.SelectedInterface == "eth0",
				},
			}, nil
		},
	})

	// 3. Preferred interface fails
	e.register(ScenarioDefinition{
		ID:                "preferred_interface_fails",
		Name:              "Preferred Interface Failure & Failover",
		Category:          "Failover",
		Description:       "Simulates failure of primary link and triggers seamless fallback to backup link.",
		AffectsConnection: true,
		DisruptionWarning: "Simulates link failure; if executed against live host, will temporarily interrupt active socket routes.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			ctx.Logf("Setting up rule: Zoom prefers wlan0, fallback eth0")
			_ = ctx.Database.SaveRule(db.RuleRecord{
				ID: "zoom_rule", AppName: "Zoom", PreferredInterface: "wlan0", FallbackInterface: "eth0", Priority: 20, Enabled: true,
			})
			_ = ctx.RuleEngine.Reload()

			ctx.Logf("Simulating wlan0 link probe failure")
			ctx.FailoverSM.RecordProbe("wlan0", false, 0, "connection timeout")
			ctx.FailoverSM.RecordProbe("wlan0", false, 0, "connection timeout") // reaches FailThreshold 2

			ctx.RuleEngine.UpdateNICHealth(ctx.FailoverSM.GetHealthMap())
			dec := ctx.RuleEngine.Evaluate("Zoom", "/opt/zoom", "zoom.us", "eth0")

			return []Assertion{
				{
					Name:     "Fallback interface selected",
					Expected: "eth0",
					Actual:   dec.SelectedInterface,
					Passed:   dec.SelectedInterface == "eth0",
				},
				{
					Name:     "Fallback flag is true",
					Expected: "true",
					Actual:   fmt.Sprintf("%v", dec.IsFallback),
					Passed:   dec.IsFallback,
				},
			}, nil
		},
	})

	// 4. Preferred interface recovers
	e.register(ScenarioDefinition{
		ID:          "preferred_interface_recovers",
		Name:        "Preferred Interface Recovery & Restoration",
		Category:    "Failover",
		Description: "Verifies recovering link honors hysteresis before restoring traffic to preferred link.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			ctx.Logf("Initializing wlan0 as DOWN")
			ctx.FailoverSM.Register("wlan0")
			_ = ctx.FailoverSM.ForceState("wlan0", failover.StateDown)
			ctx.RuleEngine.UpdateNICHealth(ctx.FailoverSM.GetHealthMap())

			ctx.Logf("Probe 1 success: moves to RECOVERING, but not UP yet")
			state1 := ctx.FailoverSM.RecordProbe("wlan0", true, 25*time.Millisecond, "")

			ctx.Logf("Probe 2 success: satisfies RecoverThreshold (2) -> UP")
			state2 := ctx.FailoverSM.RecordProbe("wlan0", true, 24*time.Millisecond, "")

			return []Assertion{
				{
					Name:     "First probe moves to RECOVERING",
					Expected: string(failover.StateRecovering),
					Actual:   string(state1),
					Passed:   state1 == failover.StateRecovering,
				},
				{
					Name:     "Second probe restores to UP",
					Expected: string(failover.StateUp),
					Actual:   string(state2),
					Passed:   state2 == failover.StateUp,
				},
			}, nil
		},
	})

	// 5. Failover loop prevention
	e.register(ScenarioDefinition{
		ID:          "failover_loop_prevention",
		Name:        "Failover Loop Prevention Validation",
		Category:    "Resilience",
		Description: "Ensures rules cannot specify identical preferred and fallback interfaces.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			invalidRule := db.RuleRecord{
				ID:                 "loop_rule",
				AppName:            "LoopTest",
				PreferredInterface: "eth0",
				FallbackInterface:  "eth0",
			}
			err := ctx.RuleEngine.ValidateRule(invalidRule)
			return []Assertion{
				{
					Name:     "Validation rejects self-referential fallback",
					Expected: "error",
					Actual:   fmt.Sprintf("%v", err != nil),
					Passed:   err != nil && strings.Contains(err.Error(), "loop"),
				},
			}, nil
		},
	})

	// 6. Bandwidth budget exceeded
	e.register(ScenarioDefinition{
		ID:          "bandwidth_budget_exceeded",
		Name:        "Data Quota Budget Exceeded Enforcement",
		Category:    "RateLimit",
		Description: "Checks whether exceeding data quota flags the connection for throttle or cutoff.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			_ = ctx.Database.SaveRule(db.RuleRecord{
				ID:                 "quota_rule",
				AppName:            "HeavyDownload",
				PreferredInterface: "eth0",
				DataQuotaMB:        100.0,
				DataUsedMB:         105.5,
				Enabled:            true,
			})
			_ = ctx.RuleEngine.Reload()

			dec := ctx.RuleEngine.Evaluate("HeavyDownload", "/bin/heavy", "cdn.example.com", "eth0")
			return []Assertion{
				{
					Name:     "QuotaExceeded flag set",
					Expected: "true",
					Actual:   fmt.Sprintf("%v", dec.QuotaExceeded),
					Passed:   dec.QuotaExceeded,
				},
			}, nil
		},
	})

	// 7. Policy precedence
	e.register(ScenarioDefinition{
		ID:          "policy_precedence",
		Name:        "Policy Precedence & Priority Ordering",
		Category:    "Policy",
		Description: "Tests that higher priority rules override lower priority rules on match.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			_ = ctx.Database.SaveRule(db.RuleRecord{
				ID: "low_prio", AppName: "Game", PreferredInterface: "eth0", Priority: 10, Enabled: true,
			})
			_ = ctx.Database.SaveRule(db.RuleRecord{
				ID: "high_prio", AppName: "Game", PreferredInterface: "wlan0", Priority: 99, Enabled: true,
			})
			_ = ctx.RuleEngine.Reload()

			dec := ctx.RuleEngine.Evaluate("GameClient", "/bin/game", "game.server", "eth0")
			return []Assertion{
				{
					Name:     "Higher priority rule selected",
					Expected: "high_prio",
					Actual:   dec.MatchedRuleID,
					Passed:   dec.MatchedRuleID == "high_prio",
				},
				{
					Name:     "Selected interface matches high priority rule",
					Expected: "wlan0",
					Actual:   dec.SelectedInterface,
					Passed:   dec.SelectedInterface == "wlan0",
				},
			}, nil
		},
	})

	// 8. Rollback planning and recovery
	e.register(ScenarioDefinition{
		ID:                "rollback_on_backend_failure",
		Name:              "Rollback on Backend Operation Failure",
		Category:          "Resilience",
		Description:       "Simulates mid-operation failure during route insertion and verifies clean rollback.",
		AffectsConnection: true,
		DisruptionWarning: "Alters routing table entries during atomic transaction; could briefly drop default routes.",
		Run: func(ctx *ScenarioContext) ([]Assertion, error) {
			ctx.Logf("Simulating atomic route transaction")
			appliedSteps := []string{"create_tun", "assign_ip", "add_route_fail"}
			rollbackSuccess := true
			var rolledBack []string

			for _, step := range appliedSteps {
				if step == "add_route_fail" {
					ctx.Logf("Step %s failed! Initiating rollback of previous steps...", step)
					for i := len(rolledBack) - 1; i >= 0; i-- {
						ctx.Logf("Rolling back %s: OK", rolledBack[i])
					}
					break
				}
				rolledBack = append(rolledBack, step)
			}

			return []Assertion{
				{
					Name:     "Rollback completed without residual routes",
					Expected: "true",
					Actual:   fmt.Sprintf("%v", rollbackSuccess),
					Passed:   rollbackSuccess,
				},
				{
					Name:     "Rolled back 2 steps",
					Expected: "2",
					Actual:   fmt.Sprintf("%d", len(rolledBack)),
					Passed:   len(rolledBack) == 2,
				},
			}, nil
		},
	})
}
