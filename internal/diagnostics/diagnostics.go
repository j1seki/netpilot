// Package diagnostics generates redacted diagnostic bundles and system reports
// for troubleshooting, support, and platform verification without leaking sensitive data.
package diagnostics

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"time"

	"netpilot/internal/branding"
	"netpilot/internal/capability"
	"netpilot/internal/db"
	"netpilot/internal/logger"
	"netpilot/internal/mode"
)

// ReportBundle represents the complete one-click diagnostic output.
type ReportBundle struct {
	GeneratedAt       time.Time                  `json:"generatedAt"`
	Application       ApplicationInfo            `json:"application"`
	Environment       capability.EnvironmentDetails `json:"environment"`
	ExecutionMode     mode.State                 `json:"executionMode"`
	Capabilities      []capability.CapabilityItem `json:"capabilities"`
	RecentOperations  []logger.LogEntry          `json:"recentOperations"`
	RecentTestRuns    []db.TestRunRecord         `json:"recentTestRuns"`
	KnownLimitations  []string                   `json:"knownLimitations"`
	ActiveRules       []db.RuleRecord            `json:"activeRules"`
}

// ApplicationInfo summarizes application build and version facts.
type ApplicationInfo struct {
	Name        string `json:"name"`
	AppID       string `json:"appId"`
	Version     string `json:"version"`
	GoVersion   string `json:"goVersion"`
	Compiler    string `json:"compiler"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	Vendor      string `json:"vendor"`
}

// Generator builds diagnostic reports.
type Generator struct {
	capDetector *capability.Detector
	modeCtrl    *mode.Controller
	ringLog     *logger.RingLogger
	database    *db.Database
}

// NewGenerator creates a new generator.
func NewGenerator(capDetector *capability.Detector, modeCtrl *mode.Controller, ringLog *logger.RingLogger, database *db.Database) *Generator {
	return &Generator{
		capDetector: capDetector,
		modeCtrl:    modeCtrl,
		ringLog:     ringLog,
		database:    database,
	}
}

// Generate creates a comprehensive diagnostic bundle with strict redactions.
func (g *Generator) Generate(strictRedact bool) ReportBundle {
	capReport := g.capDetector.Detect()
	modeState := g.modeCtrl.GetState()

	recentLogs := g.ringLog.Query(logger.FilterOptions{Limit: 200})
	testRuns, _ := g.database.ListTestRuns(10)
	rulesList, _ := g.database.ListRules()

	limitations := []string{
		"OrbStack VM paravirtualizes eth0; host Wi-Fi/LTE interfaces are not visible directly to guest kernel.",
		"Real hardware route aggregation and WFP/utun drivers require native Windows or macOS host runners.",
		"Simulation Mode is active to prevent disruption to host development environment.",
	}

	bundle := ReportBundle{
		GeneratedAt: time.Now(),
		Application: ApplicationInfo{
			Name:      branding.AppName,
			AppID:     branding.AppID,
			Version:   branding.AppVersion,
			GoVersion: runtime.Version(),
			Compiler:  runtime.Compiler,
			Platform:  runtime.GOOS,
			Arch:      runtime.GOARCH,
			Vendor:    branding.VendorName,
		},
		Environment:      capReport.Environment,
		ExecutionMode:    modeState,
		Capabilities:     capReport.Capabilities,
		RecentOperations: recentLogs,
		RecentTestRuns:   testRuns,
		KnownLimitations: limitations,
		ActiveRules:      rulesList,
	}

	if strictRedact {
		bundle = sanitizeBundle(bundle)
	}

	return bundle
}

// ToJSON serializes the bundle to formatted JSON bytes.
func (g *Generator) ToJSON(strictRedact bool) ([]byte, error) {
	bundle := g.Generate(strictRedact)
	return json.MarshalIndent(bundle, "", "  ")
}

func sanitizeBundle(b ReportBundle) ReportBundle {
	// Sanitize paths in environment
	user := os.Getenv("USER")
	if user == "" {
		user = "x"
	}

	for i := range b.RecentOperations {
		b.RecentOperations[i].Message = logger.RedactStrict(b.RecentOperations[i].Message)
		b.RecentOperations[i].ErrorMessage = logger.RedactStrict(b.RecentOperations[i].ErrorMessage)
		b.RecentOperations[i].Message = strings.ReplaceAll(b.RecentOperations[i].Message, user, "[USER]")
	}

	for i := range b.ActiveRules {
		b.ActiveRules[i].AppPath = strings.ReplaceAll(b.ActiveRules[i].AppPath, user, "[USER]")
	}

	return b
}
