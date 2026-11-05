// Package rules evaluates routing rules and determines network interface assignment,
// bandwidth caps, and fallback behavior for flows and applications.
package rules

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"netpilot/internal/db"
)

// MatchDecision represents the outcome of evaluating a request against the rule set.
type MatchDecision struct {
	MatchedRuleID      string        `json:"matchedRuleId,omitempty"`
	AppName            string        `json:"appName"`
	Destination        string        `json:"destination"`
	SelectedInterface  string        `json:"selectedInterface"`
	IsFallback         bool          `json:"isFallback"`
	FallbackReason     string        `json:"fallbackReason,omitempty"`
	BandwidthLimitMbps float64       `json:"bandwidthLimitMbps"`
	DataQuotaMB        float64       `json:"dataQuotaMb"`
	DataUsedMB         float64       `json:"dataUsedMb"`
	QuotaExceeded      bool          `json:"quotaExceeded"`
	CustomDNS          string        `json:"customDns,omitempty"`
	IsFastestRouting   bool          `json:"isFastestRouting,omitempty"`
	ProbeRTT           time.Duration `json:"probeRtt,omitempty"`
	Reason             string        `json:"reason"`
	EvaluatedAt        time.Time     `json:"evaluatedAt"`
}

// FastestNICProvider resolves the current lowest-latency UP network adapter.
type FastestNICProvider func() (string, time.Duration)

// Engine maintains active routing policies and resolves destination interfaces.
type Engine struct {
	mu          sync.RWMutex
	database    *db.Database
	rules       []db.RuleRecord
	healthyNICs map[string]bool // map of interface name -> health status
	fastestNIC  FastestNICProvider
}

// NewEngine initializes the rule engine with a database handle.
func NewEngine(database *db.Database) (*Engine, error) {
	e := &Engine{
		database:    database,
		healthyNICs: make(map[string]bool),
	}
	if err := e.Reload(); err != nil {
		return nil, err
	}
	return e, nil
}

// SetFastestNICProvider registers a callback to query the lowest-latency active link.
func (e *Engine) SetFastestNICProvider(fn FastestNICProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fastestNIC = fn
}

// Reload loads all rules from database into memory.
func (e *Engine) Reload() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	list, err := e.database.ListRules()
	if err != nil {
		return err
	}
	e.rules = list
	return nil
}

// UpdateNICHealth informs the rule engine of which interfaces are healthy.
func (e *Engine) UpdateNICHealth(health map[string]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.healthyNICs = make(map[string]bool, len(health))
	for k, v := range health {
		e.healthyNICs[k] = v
	}
}

// ValidateRule checks a rule for semantic and configuration errors.
func (e *Engine) ValidateRule(r db.RuleRecord) error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("rule id must not be empty")
	}
	if strings.TrimSpace(r.AppName) == "" && strings.TrimSpace(r.AppPath) == "" && strings.TrimSpace(r.DomainPattern) == "" {
		return errors.New("rule must specify at least an app name, app path, or domain pattern")
	}
	if strings.TrimSpace(r.PreferredInterface) == "" {
		return errors.New("preferred interface is required")
	}
	if !strings.EqualFold(r.PreferredInterface, "FASTEST") && r.PreferredInterface == r.FallbackInterface && r.FallbackInterface != "" {
		return errors.New("fallback interface cannot be identical to preferred interface (causes failover loop)")
	}
	if r.BandwidthLimitMbps < 0 {
		return errors.New("bandwidth limit cannot be negative")
	}
	if r.DataQuotaMB < 0 {
		return errors.New("data quota cannot be negative")
	}
	return nil
}

// Evaluate matches an application name, path, and destination against active rules.
func (e *Engine) Evaluate(appName, appPath, destination string, defaultInterface string) MatchDecision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	decision := MatchDecision{
		AppName:           appName,
		Destination:       destination,
		SelectedInterface: defaultInterface,
		EvaluatedAt:       time.Now(),
	}

	normApp := strings.ToLower(strings.TrimSpace(appName))
	normPath := strings.ToLower(strings.TrimSpace(appPath))
	normDest := strings.ToLower(strings.TrimSpace(destination))

	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}

		matched := false

		// Check app name match
		if r.AppName != "" {
			ruleName := strings.ToLower(r.AppName)
			if normApp == ruleName || strings.Contains(normApp, ruleName) {
				matched = true
			}
		}

		// Check app path match (supports glob)
		if !matched && r.AppPath != "" {
			rulePath := strings.ToLower(r.AppPath)
			if ok, _ := filepath.Match(rulePath, normPath); ok || normPath == rulePath {
				matched = true
			}
		}

		// Check domain pattern match
		if !matched && r.DomainPattern != "" {
			pattern := strings.ToLower(r.DomainPattern)
			if pattern == "*" || pattern == normDest || strings.HasSuffix(normDest, strings.TrimPrefix(pattern, "*")) {
				matched = true
			}
		}

		if matched {
			decision.MatchedRuleID = r.ID
			decision.BandwidthLimitMbps = r.BandwidthLimitMbps
			decision.DataQuotaMB = r.DataQuotaMB
			decision.DataUsedMB = r.DataUsedMB
			decision.CustomDNS = r.CustomDNS

			// Check quota
			if r.DataQuotaMB > 0 && r.DataUsedMB >= r.DataQuotaMB {
				decision.QuotaExceeded = true
				decision.Reason = fmt.Sprintf("Data quota exceeded (%.1f MB / %.1f MB). Throttling/blocking.", r.DataUsedMB, r.DataQuotaMB)
			}

			// Check if rule requests dynamic Fastest-Adapter routing
			if strings.EqualFold(r.PreferredInterface, "FASTEST") {
				if e.fastestNIC != nil {
					fastest, rtt := e.fastestNIC()
					if fastest != "" {
						decision.SelectedInterface = fastest
						decision.IsFastestRouting = true
						decision.ProbeRTT = rtt
						decision.Reason = fmt.Sprintf("Matched rule %q (Priority %d, Fastest-Adapter mode): dynamically selected %s (probe RTT: %v)", r.ID, r.Priority, fastest, rtt)
						return decision
					}
				}
			}

			// Check preferred interface health
			prefHealthy := true
			if len(e.healthyNICs) > 0 {
				h, ok := e.healthyNICs[r.PreferredInterface]
				prefHealthy = ok && h
			}

			if prefHealthy {
				decision.SelectedInterface = r.PreferredInterface
				decision.Reason = fmt.Sprintf("Matched rule %q (Priority %d): routed to preferred adapter %s", r.ID, r.Priority, r.PreferredInterface)
				return decision
			}

			// Preferred is unhealthy; check fallback
			if r.FallbackInterface != "" {
				fbHealthy := true
				if len(e.healthyNICs) > 0 {
					h, ok := e.healthyNICs[r.FallbackInterface]
					fbHealthy = ok && h
				}
				if fbHealthy {
					decision.SelectedInterface = r.FallbackInterface
					decision.IsFallback = true
					decision.FallbackReason = fmt.Sprintf("Preferred interface %s is down/unhealthy", r.PreferredInterface)
					decision.Reason = fmt.Sprintf("Matched rule %q: failover from %s to fallback adapter %s", r.ID, r.PreferredInterface, r.FallbackInterface)
					return decision
				}
			}

			// Both preferred and fallback unhealthy; fall through to default interface
			decision.SelectedInterface = defaultInterface
			decision.IsFallback = true
			decision.FallbackReason = fmt.Sprintf("Both preferred (%s) and fallback (%s) are unavailable", r.PreferredInterface, r.FallbackInterface)
			decision.Reason = fmt.Sprintf("Default interface %s used because target interfaces are down", defaultInterface)
			return decision
		}
	}

	decision.Reason = fmt.Sprintf("No specific rule matched: routed to default system interface %s", defaultInterface)
	return decision
}
