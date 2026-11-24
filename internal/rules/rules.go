// Package rules evaluates routing rules and determines network interface assignment,
// bandwidth caps, and fallback behavior for flows and applications.
package rules

import (
	"errors"
	"fmt"
	"net/netip"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"netpilot/internal/db"
	"netpilot/internal/engine"
)

// MatchDecision represents the outcome of evaluating a request against the rule set.
type MatchDecision struct {
	MatchedRuleID      string        `json:"matchedRuleId,omitempty"`
	AppName            string        `json:"appName"`
	Destination        string        `json:"destination"`
	PreferredInterface string        `json:"preferredInterface,omitempty"`
	SelectedInterface  string        `json:"selectedInterface"`
	FallbackInterface  string        `json:"fallbackInterface,omitempty"`
	IsFallback         bool          `json:"isFallback"`
	FallbackReason     string        `json:"fallbackReason,omitempty"`
	BandwidthLimitMbps float64       `json:"bandwidthLimitMbps"`
	DataQuotaMB        float64       `json:"dataQuotaMb"`
	DataUsedMB         float64       `json:"dataUsedMb"`
	QuotaExceeded      bool          `json:"quotaExceeded"`
	Blocked            bool          `json:"blocked,omitempty"`
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

// appNameMatches reports whether an observed application identity belongs to a
// rule's AppName. A raw substring match made "chromedriver" and "chrome helper"
// count against a "chrome" rule, inflating its quota and throttling unrelated
// traffic. Compare executable basenames, ignoring case and a Windows .exe suffix.
func appNameMatches(observed, ruleName string) bool {
	observed = strings.ToLower(strings.TrimSpace(observed))
	ruleName = strings.ToLower(strings.TrimSpace(ruleName))
	if ruleName == "" {
		return false
	}
	// Match the executable basename, not an arbitrary substring: "chrome" must
	// not capture chromedriver or chrome helper. Windows process names commonly
	// carry .exe; compare with and without that extension.
	base := strings.ToLower(path.Base(strings.ReplaceAll(observed, `\`, "/")))
	base = strings.TrimSuffix(base, ".exe")
	ruleBase := strings.ToLower(path.Base(strings.ReplaceAll(ruleName, `\`, "/")))
	ruleBase = strings.TrimSuffix(ruleBase, ".exe")
	return base == ruleBase
}

// RecordUsage adds a finished flow's byte counts to every matching rule that has a data
// quota. Rules without a quota are skipped so high-frequency flow accounting never turns
// into a database write per connection.
func (e *Engine) RecordUsage(appName string, bytesIn, bytesOut int64) {
	total := bytesIn + bytesOut
	if total <= 0 || strings.TrimSpace(appName) == "" {
		return
	}
	mb := float64(total) / (1024 * 1024)

	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		r := &e.rules[i]
		if r.Enabled && r.DataQuotaMB > 0 && appNameMatches(appName, r.AppName) {
			e.addUsage(r, mb)
		}
	}
}

// RecordRuleUsage attributes a finished flow to the policy that selected it.
func (e *Engine) RecordRuleUsage(ruleID string, bytesIn, bytesOut int64) {
	total := bytesIn + bytesOut
	if ruleID == "" || total <= 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		r := &e.rules[i]
		if r.ID == ruleID && r.DataQuotaMB > 0 {
			e.addUsage(r, float64(total)/(1024*1024))
			return
		}
	}
}

func (e *Engine) addUsage(rule *db.RuleRecord, mb float64) {
	if err := e.database.AddRuleUsage(rule.ID, mb); err == nil {
		rule.DataUsedMB += mb
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
	if dns := strings.TrimSpace(r.CustomDNS); dns != "" {
		if _, err := engine.ParseDNSAddress(dns); err != nil {
			return err
		}
	}
	return nil
}

// RuleMatch is an inspection result for a policy whose criteria match the proposed flow.
type RuleMatch struct {
	Rule      db.RuleRecord `json:"rule"`
	MatchedBy string        `json:"matchedBy"`
	Result    string        `json:"result"`
}

// MatchRules lists matching policies in precedence order, including disabled rules.
func (e *Engine) MatchRules(appName, appPath, destination string) []RuleMatch {
	e.mu.RLock()
	defer e.mu.RUnlock()
	normPath := strings.ToLower(strings.TrimSpace(appPath))
	normDest := strings.ToLower(strings.TrimSpace(destination))
	matches := []RuleMatch{}
	selected := false
	for _, rule := range e.rules {
		matchedBy := matchingCriterion(rule, appName, normPath, normDest)
		if matchedBy == "" {
			continue
		}
		result := "shadowed"
		if !rule.Enabled {
			result = "disabled"
		} else if !selected {
			result = "selected"
			selected = true
		}
		matches = append(matches, RuleMatch{Rule: rule, MatchedBy: matchedBy, Result: result})
	}
	return matches
}

// matchingCriterion is shared by rule inspection and the forwarding policy resolver.
func matchingCriterion(rule db.RuleRecord, appName, normPath, normDest string) string {
	if appNameMatches(appName, rule.AppName) {
		return "application name"
	}
	if rule.AppPath != "" && normPath != "" {
		pattern := strings.ToLower(strings.TrimSpace(rule.AppPath))
		if ok, _ := filepath.Match(pattern, normPath); ok || normPath == pattern {
			return "executable path"
		}
		if strings.HasSuffix(pattern, ".app") && strings.HasPrefix(normPath, pattern+"/contents/") {
			return "application bundle"
		}
	}
	if rule.DomainPattern != "" && matchesDestination(rule.DomainPattern, normDest) {
		return "destination"
	}
	return ""
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

	normPath := strings.ToLower(strings.TrimSpace(appPath))
	normDest := strings.ToLower(strings.TrimSpace(destination))

	for _, r := range e.rules {
		if !r.Enabled {
			continue
		}

		if matchingCriterion(r, appName, normPath, normDest) != "" {
			decision.MatchedRuleID = r.ID
			decision.BandwidthLimitMbps = r.BandwidthLimitMbps
			decision.DataQuotaMB = r.DataQuotaMB
			decision.DataUsedMB = r.DataUsedMB
			decision.CustomDNS = strings.TrimSpace(r.CustomDNS)
			decision.PreferredInterface = r.PreferredInterface
			decision.FallbackInterface = r.FallbackInterface

			// Check quota
			if r.DataQuotaMB > 0 && r.DataUsedMB >= r.DataQuotaMB {
				decision.QuotaExceeded = true
				decision.Blocked = true
				decision.SelectedInterface = ""
				decision.Reason = fmt.Sprintf("Data quota reached (%.1f MB / %.1f MB)", r.DataUsedMB, r.DataQuotaMB)
				return decision
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
			if e.available(r.PreferredInterface) {
				decision.SelectedInterface = r.PreferredInterface
				decision.Reason = fmt.Sprintf("Matched rule %q (Priority %d): routed to preferred adapter %s", r.ID, r.Priority, r.PreferredInterface)
				return decision
			}

			// Preferred is unhealthy; check fallback
			if r.FallbackInterface != "" && e.available(r.FallbackInterface) {
				decision.SelectedInterface = r.FallbackInterface
				decision.IsFallback = true
				decision.FallbackReason = fmt.Sprintf("Preferred adapter %s is unavailable", r.PreferredInterface)
				decision.Reason = fmt.Sprintf("Matched rule %q, using fallback %s", r.ID, r.FallbackInterface)
				return decision
			}

			decision.SelectedInterface = ""
			decision.Blocked = true
			decision.Reason = fmt.Sprintf("Rule %q blocks new connections while its configured adapters are unavailable", r.ID)
			return decision
		}
	}

	decision.Reason = fmt.Sprintf("No specific rule matched: routed to default system interface %s", defaultInterface)
	return decision
}

func (e *Engine) available(adapter string) bool {
	return adapter == "ALL" || adapter == "net-merge" || len(e.healthyNICs) == 0 || e.healthyNICs[adapter]
}

func matchesDestination(patterns, destination string) bool {
	if destination == "" {
		return false
	}
	host := strings.TrimSuffix(destination, ".")
	ip, _ := netip.ParseAddr(host)
	for item := range strings.SplitSeq(patterns, ",") {
		pattern := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(item)), ".")
		switch {
		case pattern == "*" || pattern == host:
			return true
		case strings.HasPrefix(pattern, "*."):
			suffix := pattern[1:]
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		case strings.HasPrefix(pattern, "*"):
			if strings.HasSuffix(host, pattern[1:]) {
				return true
			}
		case ip.IsValid():
			if prefix, err := netip.ParsePrefix(pattern); err == nil && prefix.Contains(ip.Unmap()) {
				return true
			}
		}
	}
	return false
}
