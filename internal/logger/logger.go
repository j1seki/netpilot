// Package logger provides structured, bounded, correlated, and redactable logging
// for operations, lifecycle phases, and diagnostics.
package logger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Level represents log severity.
type Level string

const (
	LevelTrace Level = "TRACE"
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Phase represents the lifecycle stage of an operation.
type Phase string

const (
	PhaseRequested            Phase = "REQUESTED"
	PhaseValidationStarted    Phase = "VALIDATION_STARTED"
	PhaseValidationCompleted  Phase = "VALIDATION_COMPLETED"
	PhaseCapabilityStarted    Phase = "CAPABILITY_CHECK_STARTED"
	PhaseCapabilityCompleted  Phase = "CAPABILITY_CHECK_COMPLETED"
	PhasePolicyStarted        Phase = "POLICY_EVAL_STARTED"
	PhasePolicyCompleted      Phase = "POLICY_EVAL_COMPLETED"
	PhaseBackendStarted       Phase = "BACKEND_OP_STARTED"
	PhaseBackendCompleted     Phase = "BACKEND_OP_COMPLETED"
	PhaseVerificationStarted  Phase = "VERIFICATION_STARTED"
	PhaseVerificationCompleted Phase = "VERIFICATION_COMPLETED"
	PhaseFinalized            Phase = "FINALIZED"
	PhaseFailed               Phase = "FAILED"
	PhaseRollbackStarted      Phase = "ROLLBACK_STARTED"
	PhaseRollbackCompleted    Phase = "ROLLBACK_COMPLETED"
)

// LogEntry is the primary structured log item.
type LogEntry struct {
	Timestamp     time.Time              `json:"timestamp"`
	MonotonicNs   int64                  `json:"monotonicNs"`
	Level         Level                  `json:"level"`
	OperationID   string                 `json:"operationId,omitempty"`
	CorrelationID string                 `json:"correlationId,omitempty"`
	ParentOpID    string                 `json:"parentOpId,omitempty"`
	Component     string                 `json:"component"`
	OperationName string                 `json:"operationName,omitempty"`
	Phase         Phase                  `json:"phase,omitempty"`
	Platform      string                 `json:"platform"`
	ExecutionMode string                 `json:"executionMode"`
	Message       string                 `json:"message"`
	Result        string                 `json:"result,omitempty"`
	ErrorCategory string                 `json:"errorCategory,omitempty"`
	ErrorMessage  string                 `json:"errorMessage,omitempty"`
	RecoveryAction string                `json:"recoveryAction,omitempty"`
	RollbackStatus string                `json:"rollbackStatus,omitempty"`
	DurationMs    float64                `json:"durationMs,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// FilterOptions sets criteria for log queries.
type FilterOptions struct {
	Level       string `json:"level"`
	Component   string `json:"component"`
	OperationID string `json:"operationId"`
	SearchText  string `json:"searchText"`
	ErrorsOnly  bool   `json:"errorsOnly"`
	Limit       int    `json:"limit"`
}

var (
	// Redaction patterns
	tokenRegex = regexp.MustCompile(`(?i)(bearer\s+[a-zA-Z0-9_\-\.]+)|(key|token|password|secret|auth)[=:\s]+["']?([a-zA-Z0-9_\-\.\@]{6,})["']?`)
	macRegex   = regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`)
)

// RingLogger stores a bounded circular buffer of structured logs.
type RingLogger struct {
	mu       sync.RWMutex
	capacity int
	entries  []LogEntry
	cursor   int
	full     bool
	subscribers map[chan LogEntry]struct{}
}

// NewRingLogger creates a bounded memory logger.
func NewRingLogger(capacity int) *RingLogger {
	if capacity <= 0 {
		capacity = 5000
	}
	return &RingLogger{
		capacity:    capacity,
		entries:     make([]LogEntry, capacity),
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

func (l *RingLogger) Log(e LogEntry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	e.Message = Redact(e.Message)

	l.mu.Lock()
	l.entries[l.cursor] = e
	l.cursor = (l.cursor + 1) % l.capacity
	if l.cursor == 0 {
		l.full = true
	}
	// Broadcast to active streaming subscribers
	for ch := range l.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
	l.mu.Unlock()
}

// Redact cleans sensitive credentials, tokens, or MAC addresses from messages.
func Redact(s string) string {
	s = tokenRegex.ReplaceAllString(s, "[REDACTED_CREDENTIAL]")
	return s
}

// RedactStrict also masks MAC addresses and private subnets for diagnostic exports.
func RedactStrict(s string) string {
	s = Redact(s)
	s = macRegex.ReplaceAllString(s, "XX:XX:XX:XX:XX:XX")
	return s
}

// Query returns entries matching the given filter.
func (l *RingLogger) Query(opts FilterOptions) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var all []LogEntry
	if !l.full {
		all = make([]LogEntry, l.cursor)
		copy(all, l.entries[:l.cursor])
	} else {
		all = make([]LogEntry, l.capacity)
		copy(all, l.entries[l.cursor:])
		copy(all[l.capacity-l.cursor:], l.entries[:l.cursor])
	}

	var matched []LogEntry
	search := strings.ToLower(opts.SearchText)

	// Iterate in reverse (newest first)
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if opts.ErrorsOnly && e.Level != LevelError && e.Level != LevelWarn {
			continue
		}
		if opts.Level != "" && string(e.Level) != opts.Level {
			continue
		}
		if opts.Component != "" && !strings.EqualFold(e.Component, opts.Component) {
			continue
		}
		if opts.OperationID != "" && e.OperationID != opts.OperationID {
			continue
		}
		if search != "" {
			combined := strings.ToLower(fmt.Sprintf("%s %s %s %s", e.Message, e.OperationName, e.ErrorMessage, e.Component))
			if !strings.Contains(combined, search) {
				continue
			}
		}

		matched = append(matched, e)
		if opts.Limit > 0 && len(matched) >= opts.Limit {
			break
		}
	}

	return matched
}

// ExportJSONL produces JSONL byte output.
func (l *RingLogger) ExportJSONL(strictRedact bool) []byte {
	entries := l.Query(FilterOptions{Limit: 10000})
	var sb strings.Builder
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if strictRedact {
			e.Message = RedactStrict(e.Message)
			e.ErrorMessage = RedactStrict(e.ErrorMessage)
		}
		b, _ := json.Marshal(e)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// RandomID generates a hex identifier for correlation.
func RandomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// OperationLifecycle tracks a single operation from start to finish.
type OperationLifecycle struct {
	logger        *RingLogger
	entry         LogEntry
	startTime     time.Time
	startMono     int64
	phaseTimers   map[Phase]time.Time
}

// OperationID returns the tracked operation ID.
func (op *OperationLifecycle) OperationID() string {
	return op.entry.OperationID
}

// StartOperation initializes a tracked operation.
func (l *RingLogger) StartOperation(name, component, mode, platform, parentOpID string) *OperationLifecycle {
	now := time.Now()
	opID := RandomID()
	corrID := opID
	if parentOpID != "" {
		corrID = parentOpID
	}

	base := LogEntry{
		Timestamp:     now,
		Level:         LevelInfo,
		OperationID:   opID,
		CorrelationID: corrID,
		ParentOpID:    parentOpID,
		Component:     component,
		OperationName: name,
		Phase:         PhaseRequested,
		Platform:      platform,
		ExecutionMode: mode,
		Message:       fmt.Sprintf("Operation %s started", name),
	}

	l.Log(base)

	return &OperationLifecycle{
		logger:    l,
		entry:     base,
		startTime: now,
		phaseTimers: map[Phase]time.Time{
			PhaseRequested: now,
		},
	}
}

// Step logs an intermediate phase transition.
func (op *OperationLifecycle) Step(p Phase, msg string, meta map[string]interface{}) {
	now := time.Now()
	op.phaseTimers[p] = now
	dur := float64(now.Sub(op.startTime).Microseconds()) / 1000.0

	entry := op.entry
	entry.Timestamp = now
	entry.Level = LevelInfo
	entry.Phase = p
	entry.Message = msg
	entry.DurationMs = dur
	entry.Metadata = meta

	op.logger.Log(entry)
}

// Fail records an operation failure.
func (op *OperationLifecycle) Fail(p Phase, category string, err error, recoveryAction, rollbackStatus string, meta map[string]interface{}) {
	now := time.Now()
	dur := float64(now.Sub(op.startTime).Microseconds()) / 1000.0

	entry := op.entry
	entry.Timestamp = now
	entry.Level = LevelError
	entry.Phase = p
	entry.Result = "FAILED"
	entry.ErrorCategory = category
	if err != nil {
		entry.ErrorMessage = err.Error()
	}
	entry.RecoveryAction = recoveryAction
	entry.RollbackStatus = rollbackStatus
	entry.DurationMs = dur
	entry.Message = fmt.Sprintf("Operation %s failed at %s: %v", op.entry.OperationName, p, err)
	entry.Metadata = meta

	op.logger.Log(entry)
}

// Complete finalizes the operation successfully.
func (op *OperationLifecycle) Complete(result string, meta map[string]interface{}) {
	now := time.Now()
	dur := float64(now.Sub(op.startTime).Microseconds()) / 1000.0

	entry := op.entry
	entry.Timestamp = now
	entry.Level = LevelInfo
	entry.Phase = PhaseFinalized
	entry.Result = result
	entry.DurationMs = dur
	entry.Message = fmt.Sprintf("Operation %s completed successfully in %.2fms", op.entry.OperationName, dur)
	entry.Metadata = meta

	op.logger.Log(entry)
}

// Subscribe opens a channel for live streaming logs to frontend.
func (l *RingLogger) Subscribe() (chan LogEntry, func()) {
	l.mu.Lock()
	ch := make(chan LogEntry, 100)
	l.subscribers[ch] = struct{}{}
	l.mu.Unlock()

	unsubscribe := func() {
		l.mu.Lock()
		delete(l.subscribers, ch)
		close(ch)
		l.mu.Unlock()
	}
	return ch, unsubscribe
}
