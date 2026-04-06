package anomaly_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/signals"
)

func newTestEvaluator(t *testing.T) (*anomaly.Evaluator, *signals.DB) {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := anomaly.DefaultConfig()
	cfg.MinWindow = 4 // smaller for testing
	cfg.MaxWindow = 10
	return anomaly.NewEvaluator(db, cfg), db
}

func insertMetrics(t *testing.T, db *signals.DB, theme string, cycleTimes []int64, pass []bool) {
	t.Helper()
	for i, ct := range cycleTimes {
		p := false
		if i < len(pass) {
			p = pass[i]
		}
		cost := 2.50
		err := db.InsertBeadMetric(signals.BeadMetric{
			BeadID:           theme + "-" + string(rune(i+65)),
			Theme:            theme,
			CycleTimeMs:      ct,
			PassFirstAttempt: p,
			CostUSD:          &cost,
			CompletedAt:      int64((len(cycleTimes) - i) * 100),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvaluator_NoBeads(t *testing.T) {
	eval, _ := newTestEvaluator(t)
	state, err := eval.Evaluate([]string{"auth"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if state.Signals["auth"].Status != anomaly.StatusCleared {
		t.Errorf("status = %q, want cleared (no beads)", state.Signals["auth"].Status)
	}
}

func TestEvaluator_ShortCircuit_NoNewBeads(t *testing.T) {
	eval, db := newTestEvaluator(t)

	insertMetrics(t, db, "auth", []int64{1000, 1000, 1000, 1000}, nil)

	// First evaluation
	state1, err := eval.Evaluate([]string{"auth"}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	// Second evaluation with same data — should short-circuit
	state2, err := eval.Evaluate([]string{"auth"}, 2000)
	if err != nil {
		t.Fatal(err)
	}

	if state1.Signals["auth"].LastEvalAt != state2.Signals["auth"].LastEvalAt {
		t.Error("expected short-circuit: no new beads should reuse prior eval")
	}
}

func TestEvaluator_Staleness(t *testing.T) {
	eval, db := newTestEvaluator(t)

	// Insert beads with old timestamps
	insertMetrics(t, db, "auth", []int64{1000, 1000, 1000, 1000}, nil)

	// Now is far in the future (beads are stale)
	now := int64(100000000) // way past 14-day window
	state, err := eval.Evaluate([]string{"auth"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if state.Signals["auth"].Status != anomaly.StatusStale {
		t.Errorf("status = %q, want stale", state.Signals["auth"].Status)
	}
}

func TestEvaluator_DriftFires(t *testing.T) {
	eval, db := newTestEvaluator(t)

	// Recent: 1200ms, Baseline: 1000ms → 20% drift
	times := []int64{1200, 1200, 1000, 1000}
	insertMetrics(t, db, "auth", times, nil)

	state, err := eval.Evaluate([]string{"auth"}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if state.Signals["auth"].Status != anomaly.StatusFired {
		t.Errorf("status = %q, want fired", state.Signals["auth"].Status)
	}
	if state.Signals["auth"].AdvisoryOffset >= 0 {
		t.Errorf("advisory = %d, want negative", state.Signals["auth"].AdvisoryOffset)
	}
}

func TestEvaluator_MultipleThemes(t *testing.T) {
	eval, db := newTestEvaluator(t)

	insertMetrics(t, db, "auth", []int64{1200, 1200, 1000, 1000}, nil)
	insertMetrics(t, db, "perf", []int64{1000, 1000, 1000, 1000}, nil)

	state, err := eval.Evaluate([]string{"auth", "perf"}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	if state.Signals["auth"].Status != anomaly.StatusFired {
		t.Error("auth should be fired (20% drift)")
	}
	if state.Signals["perf"].Status != anomaly.StatusCleared {
		t.Error("perf should be cleared (no drift)")
	}
}

func TestEvaluator_PleasureSignals(t *testing.T) {
	eval, db := newTestEvaluator(t)

	pass := []bool{true, true, false, false}
	insertMetrics(t, db, "auth", []int64{1000, 1000, 1000, 1000}, pass)

	state, err := eval.Evaluate([]string{"auth"}, 1000)
	if err != nil {
		t.Fatal(err)
	}

	if len(state.Pleasure) != 3 {
		t.Fatalf("got %d pleasure signals, want 3", len(state.Pleasure))
	}

	names := map[string]bool{}
	for _, p := range state.Pleasure {
		names[p.Name] = true
	}
	for _, expected := range []string{"first_attempt_pass_rate", "cycle_time_p50_trend", "cost_per_landed_change_trend"} {
		if !names[expected] {
			t.Errorf("missing pleasure signal: %s", expected)
		}
	}
}

func makeBypassDB(t *testing.T, dir string) *signals.DB {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertDriftMetrics(t *testing.T, db *signals.DB, theme string, count int, driftHalf bool) {
	t.Helper()
	for i := 0; i < count; i++ {
		cycle := int64(1000)
		if driftHalf && i >= count/2 {
			cycle = 1300 // 30% drift
		}
		db.InsertBeadMetric(signals.BeadMetric{
			BeadID:      fmt.Sprintf("%s-%d", theme, i),
			Theme:       theme,
			CycleTimeMs: cycle,
			CompletedAt: int64(i + 1),
		})
	}
}

func TestEvaluator_BypassNotTriggered_OneFired(t *testing.T) {
	dir := t.TempDir()
	db := makeBypassDB(t, dir)
	sentinelPath := filepath.Join(dir, "factory-paused.json")

	insertDriftMetrics(t, db, "auth", 20, true)  // 30% drift — will fire
	insertDriftMetrics(t, db, "perf", 20, false)  // no drift — won't fire

	cfg := anomaly.DefaultConfig()
	eval := anomaly.NewEvaluator(db, cfg, sentinelPath)

	_, err := eval.Evaluate([]string{"auth", "perf"}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinelPath); err == nil {
		t.Error("sentinel should not exist with only 1 fired theme")
	}
}

func TestEvaluator_BypassTriggered_TwoFired(t *testing.T) {
	dir := t.TempDir()
	db := makeBypassDB(t, dir)
	sentinelPath := filepath.Join(dir, "factory-paused.json")

	insertDriftMetrics(t, db, "auth", 20, true)
	insertDriftMetrics(t, db, "perf", 20, true)

	cfg := anomaly.DefaultConfig()
	eval := anomaly.NewEvaluator(db, cfg, sentinelPath)

	_, err := eval.Evaluate([]string{"auth", "perf"}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinelPath); os.IsNotExist(err) {
		t.Error("sentinel should exist after BYPASS trigger")
	}
	data, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("sentinel JSON invalid: %v", err)
	}
	if record["code"] != "bypass_multi_root_cause" {
		t.Errorf("expected bypass_multi_root_cause, got %v", record["code"])
	}
}

func TestEvaluator_BypassSentinelAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	db := makeBypassDB(t, dir)
	sentinelPath := filepath.Join(dir, "factory-paused.json")

	os.WriteFile(sentinelPath, []byte(`{"reason":"prior"}`), 0644)

	insertDriftMetrics(t, db, "t1", 20, true)
	insertDriftMetrics(t, db, "t2", 20, true)

	cfg := anomaly.DefaultConfig()
	eval := anomaly.NewEvaluator(db, cfg, sentinelPath)

	_, err := eval.Evaluate([]string{"t1", "t2"}, 500)
	if err != nil {
		t.Fatalf("expected no error with existing sentinel, got %v", err)
	}
}

func TestEvaluator_ErrBypassFailed_PropagatesTyped(t *testing.T) {
	dir := t.TempDir()
	db := makeBypassDB(t, dir)
	sentinelPath := filepath.Join(dir, "nonexistent-readonly", "sub", "factory-paused.json")
	os.MkdirAll(filepath.Join(dir, "nonexistent-readonly"), 0555)

	insertDriftMetrics(t, db, "t1", 20, true)
	insertDriftMetrics(t, db, "t2", 20, true)

	cfg := anomaly.DefaultConfig()
	eval := anomaly.NewEvaluator(db, cfg, sentinelPath)

	_, err := eval.Evaluate([]string{"t1", "t2"}, 500)
	if err == nil {
		t.Fatal("expected ErrBypassFailed")
	}
	if !errors.Is(err, anomaly.ErrBypassFailed) {
		t.Errorf("expected ErrBypassFailed, got %v", err)
	}
}
