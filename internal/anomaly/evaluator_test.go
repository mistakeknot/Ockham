package anomaly_test

import (
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
