package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/signals"
)

// testEnv bundles the tempdir-isolated state a runTriggerPipeline test needs.
// HOME is redirected so interspect.DefaultPath() resolves into the test dir.
type testEnv struct {
	t           *testing.T
	homeDir     string
	db          *signals.DB
	weightsPath string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	dbPath := filepath.Join(t.TempDir(), "signals.db")
	db, err := signals.NewDB(dbPath)
	if err != nil {
		t.Fatalf("signals.NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return &testEnv{
		t:           t,
		homeDir:     home,
		db:          db,
		weightsPath: filepath.Join(t.TempDir(), "weights.json"),
	}
}

// writeInterspectEvidence plants a delegation-calibration.json that the
// interspect Checker will pick up via its DefaultPath (HOME-rooted).
// highRetry lists themes that should return VerdictUnhealthy; pass nil plus
// a healthy theme in categories to drive a Healthy verdict.
func (e *testEnv) writeInterspectEvidence(highRetry []string, categories map[string]map[string]any, generatedAt time.Time) {
	e.t.Helper()
	dir := filepath.Join(e.homeDir, ".clavain", "interspect")
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("mkdir interspect: %v", err)
	}
	if categories == nil {
		categories = map[string]map[string]any{}
	}
	evidence := map[string]any{
		"schema_version":        1,
		"generated_at":          generatedAt.UTC().Format(time.RFC3339),
		"overall_pass_rate":     0.9,
		"total_delegations":     100,
		"retry_rate":            0.05,
		"categories":            categories,
		"high_retry_categories": highRetry,
	}
	data, _ := json.Marshal(evidence)
	if err := os.WriteFile(filepath.Join(dir, "delegation-calibration.json"), data, 0644); err != nil {
		e.t.Fatalf("write evidence: %v", err)
	}
}

// setPrevDrift plants the prior-window drift so F4 fast-path can compute delta.
func (e *testEnv) setPrevDrift(theme string, prev float64) {
	e.t.Helper()
	if err := e.db.SetSignalState("prev_drift:"+theme,
		fmt.Sprintf("%f", prev), time.Now().Unix()); err != nil {
		e.t.Fatalf("seed prev_drift: %v", err)
	}
}

// newRunner builds a CheckRunner wired for tests: F6 inflight override → nil
// so the pipeline skips bd-shell calls; weights path points at the temp file.
func (e *testEnv) newRunner() *CheckRunner {
	return &CheckRunner{
		db:               e.db,
		weightsPath:      e.weightsPath,
		inflightOverride: true,
		inflightCtl:      nil,
	}
}

// TestRunTriggerPipeline_FirePath: 3 tripped drift observations below the
// fast-path threshold fire CONSTRAIN via F3 confirm streak (RequiredWindows=3).
func TestRunTriggerPipeline_FirePath(t *testing.T) {
	env := newTestEnv(t)
	env.writeInterspectEvidence([]string{"auth"}, nil, time.Now())
	env.setPrevDrift("auth", 0.10)

	runner := env.newRunner()
	state := anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			// Sub-threshold jump (0.10 → 0.15 = 0.05 delta < 0.40 fast-path).
			"auth": {Theme: "auth", Status: anomaly.StatusFired, DriftPct: 0.15},
		},
	}

	for i := 0; i < 3; i++ {
		if err := runner.runTriggerPipeline(state); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}

	active, _, err := constrain.New(env.db).IsConstrained("auth")
	if err != nil {
		t.Fatalf("IsConstrained: %v", err)
	}
	if !active {
		t.Fatal("want CONSTRAIN active after 3 confirm cycles, got none")
	}

	raw, err := os.ReadFile(env.weightsPath)
	if err != nil {
		t.Fatalf("read weights: %v", err)
	}
	var weights struct {
		Themes map[string]int `json:"themes"`
	}
	if err := json.Unmarshal(raw, &weights); err != nil {
		t.Fatalf("parse weights: %v", err)
	}
	if got := weights.Themes["auth"]; got >= 0 {
		t.Fatalf("auth offset = %d, want negative (constrained)", got)
	}
}

// TestRunTriggerPipeline_FastPath: a single drift jump above the 0.40
// threshold fires CONSTRAIN via F4 fast path — no streak required.
func TestRunTriggerPipeline_FastPath(t *testing.T) {
	env := newTestEnv(t)
	env.writeInterspectEvidence([]string{"auth"}, nil, time.Now())
	env.setPrevDrift("auth", 0.05)

	runner := env.newRunner()
	state := anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			// 0.05 → 0.50 = 0.45 delta > 0.40 fast-path threshold.
			"auth": {Theme: "auth", Status: anomaly.StatusFired, DriftPct: 0.50},
		},
	}
	if err := runner.runTriggerPipeline(state); err != nil {
		t.Fatalf("runTriggerPipeline: %v", err)
	}

	active, rec, err := constrain.New(env.db).IsConstrained("auth")
	if err != nil {
		t.Fatalf("IsConstrained: %v", err)
	}
	if !active {
		t.Fatal("want CONSTRAIN via fast path, got none")
	}
	if !rec.FastPath {
		t.Fatalf("want FastPath=true, got %+v", rec)
	}
}

// TestRunTriggerPipeline_ReleasePath: after a fire, 5 clean cycles meet the
// default StabilityPolicy.RequiredWindows and F7 releases.
func TestRunTriggerPipeline_ReleasePath(t *testing.T) {
	env := newTestEnv(t)
	env.writeInterspectEvidence([]string{"auth"}, nil, time.Now())
	env.setPrevDrift("auth", 0.05)

	runner := env.newRunner()

	// Phase 1: fire via fast path.
	if err := runner.runTriggerPipeline(anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			"auth": {Theme: "auth", Status: anomaly.StatusFired, DriftPct: 0.50},
		},
	}); err != nil {
		t.Fatalf("fire: %v", err)
	}

	// Phase 2: five clean cycles to meet DefaultStabilityPolicy (RequiredWindows=5).
	for i := 0; i < 5; i++ {
		if err := runner.runTriggerPipeline(anomaly.State{
			Signals: map[string]anomaly.ThemeSignal{
				"auth": {Theme: "auth", Status: anomaly.StatusCleared, DriftPct: 0.02},
			},
		}); err != nil {
			t.Fatalf("clean cycle %d: %v", i, err)
		}
	}

	active, _, err := constrain.New(env.db).IsConstrained("auth")
	if err != nil {
		t.Fatalf("IsConstrained: %v", err)
	}
	if active {
		t.Fatal("want CONSTRAIN released after 5 stability cycles, still active")
	}
}

// TestRunTriggerPipeline_HealthyBlocksFire: evidence says theme is healthy
// (not in high_retry, category passes MinPassRate), interspect returns
// VerdictHealthy, pipeline blocks fire even when delta crosses fast-path.
func TestRunTriggerPipeline_HealthyBlocksFire(t *testing.T) {
	env := newTestEnv(t)
	// auth is NOT in high_retry, AND has a category entry passing thresholds.
	env.writeInterspectEvidence(
		[]string{"other"},
		map[string]map[string]any{
			"auth": {
				"count":          50,    // >= MinCategoryCount (default 3)
				"pass_rate":      0.95,  // >= MinPassRate (default 0.5)
				"avg_duration_s": 10.0,
			},
		},
		time.Now(),
	)
	env.setPrevDrift("auth", 0.05)

	runner := env.newRunner()
	// Fast-path-sized jump — would normally fire.
	if err := runner.runTriggerPipeline(anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			"auth": {Theme: "auth", Status: anomaly.StatusFired, DriftPct: 0.50},
		},
	}); err != nil {
		t.Fatalf("runTriggerPipeline: %v", err)
	}

	active, _, err := constrain.New(env.db).IsConstrained("auth")
	if err != nil {
		t.Fatalf("IsConstrained: %v", err)
	}
	if active {
		t.Fatal("want fire blocked by Healthy verdict, got CONSTRAIN")
	}
}
