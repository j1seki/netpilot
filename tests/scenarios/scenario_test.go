package scenarios_test

import (
	"testing"

	"netpilot/internal/db"
	"netpilot/internal/logger"
	"netpilot/internal/scenarios"
)

// Classification: System
func TestDeterministicSimulationScenarios(t *testing.T) {
	ring := logger.NewRingLogger(500)
	memDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer memDB.Close()

	engine := scenarios.NewEngine(ring, memDB)
	results := engine.RunAll()

	if len(results) == 0 {
		t.Fatal("expected registered scenarios, got 0")
	}

	for _, res := range results {
		t.Run(res.ID, func(t *testing.T) {
			t.Logf("[%s] %s (%.2fms)", res.Category, res.Name, res.DurationMs)
			if res.Status != scenarios.StatusPass {
				t.Fatalf("scenario failed: %s (error: %s)", res.Name, res.Error)
			}
			for _, a := range res.Assertions {
				if !a.Passed {
					t.Errorf("Assertion %q failed: expected %s, got %s", a.Name, a.Expected, a.Actual)
				}
			}
		})
	}
}
