package signals_test

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func newTestDB(t *testing.T) *signals.DB {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndQuery(t *testing.T) {
	db := newTestDB(t)

	cost := 2.50
	m := signals.BeadMetric{
		BeadID: "b1", Theme: "auth", CycleTimeMs: 5000,
		PassFirstAttempt: true, CostUSD: &cost, CompletedAt: 100,
	}
	if err := db.InsertBeadMetric(m); err != nil {
		t.Fatal(err)
	}

	metrics, err := db.LatestBeadMetrics("auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if metrics[0].BeadID != "b1" {
		t.Errorf("bead_id = %q, want b1", metrics[0].BeadID)
	}
	if !metrics[0].PassFirstAttempt {
		t.Error("pass_first_attempt should be true")
	}
	if metrics[0].CostUSD == nil || *metrics[0].CostUSD != 2.50 {
		t.Errorf("cost_usd = %v, want 2.50", metrics[0].CostUSD)
	}
}

func TestInsertDuplicate_Ignored(t *testing.T) {
	db := newTestDB(t)

	m := signals.BeadMetric{
		BeadID: "b1", Theme: "auth", CycleTimeMs: 5000, CompletedAt: 100,
	}
	if err := db.InsertBeadMetric(m); err != nil {
		t.Fatal(err)
	}
	// Insert same bead_id again — should be silently ignored
	m.CycleTimeMs = 9999
	if err := db.InsertBeadMetric(m); err != nil {
		t.Fatal(err)
	}

	metrics, err := db.LatestBeadMetrics("auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1 (duplicate should be ignored)", len(metrics))
	}
	if metrics[0].CycleTimeMs != 5000 {
		t.Errorf("cycle_time_ms = %d, want 5000 (original, not duplicate)", metrics[0].CycleTimeMs)
	}
}

func TestLatestBeadMetrics_Order(t *testing.T) {
	db := newTestDB(t)

	for i, ts := range []int64{100, 300, 200} {
		m := signals.BeadMetric{
			BeadID: "b" + string(rune('1'+i)), Theme: "auth",
			CycleTimeMs: 1000, CompletedAt: ts,
		}
		if err := db.InsertBeadMetric(m); err != nil {
			t.Fatal(err)
		}
	}

	metrics, err := db.LatestBeadMetrics("auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 3 {
		t.Fatalf("got %d metrics, want 3", len(metrics))
	}
	// Should be sorted by completed_at DESC: 300, 200, 100
	if metrics[0].CompletedAt != 300 || metrics[1].CompletedAt != 200 || metrics[2].CompletedAt != 100 {
		t.Errorf("order wrong: %d, %d, %d", metrics[0].CompletedAt, metrics[1].CompletedAt, metrics[2].CompletedAt)
	}
}

func TestPruneBeadMetrics(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 20; i++ {
		m := signals.BeadMetric{
			BeadID: "b" + string(rune(i+65)), Theme: "auth",
			CycleTimeMs: 1000, CompletedAt: int64(i * 100),
		}
		if err := db.InsertBeadMetric(m); err != nil {
			t.Fatal(err)
		}
	}

	deleted, err := db.PruneBeadMetrics("auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 10 {
		t.Errorf("deleted %d rows, want 10", deleted)
	}

	remaining, err := db.LatestBeadMetrics("auth", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 10 {
		t.Errorf("got %d remaining, want 10", len(remaining))
	}
}

func TestLastBeadCompletedAt_Empty(t *testing.T) {
	db := newTestDB(t)

	ts, found, err := db.LastBeadCompletedAt("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found for empty table")
	}
	if ts != 0 {
		t.Errorf("ts = %d, want 0", ts)
	}
}

func TestLastBeadCompletedAt_ReturnsLatest(t *testing.T) {
	db := newTestDB(t)

	for i, ts := range []int64{100, 300, 200} {
		m := signals.BeadMetric{
			BeadID: "b" + string(rune('1'+i)), Theme: "auth",
			CycleTimeMs: 1000, CompletedAt: ts,
		}
		if err := db.InsertBeadMetric(m); err != nil {
			t.Fatal(err)
		}
	}

	ts, found, err := db.LastBeadCompletedAt("auth")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected found")
	}
	if ts != 300 {
		t.Errorf("ts = %d, want 300", ts)
	}
}

func TestDistinctThemes(t *testing.T) {
	db := newTestDB(t)

	for _, theme := range []string{"auth", "perf", "auth"} {
		m := signals.BeadMetric{
			BeadID: "b-" + theme + "-" + string(rune(len(theme)+65)), Theme: theme,
			CycleTimeMs: 1000, CompletedAt: 100,
		}
		db.InsertBeadMetric(m)
	}

	themes, err := db.DistinctThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 2 {
		t.Errorf("got %d themes, want 2", len(themes))
	}
}

func TestNullCostUSD(t *testing.T) {
	db := newTestDB(t)

	m := signals.BeadMetric{
		BeadID: "b1", Theme: "auth", CycleTimeMs: 5000,
		CostUSD: nil, CompletedAt: 100,
	}
	if err := db.InsertBeadMetric(m); err != nil {
		t.Fatal(err)
	}

	metrics, err := db.LatestBeadMetrics("auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if metrics[0].CostUSD != nil {
		t.Errorf("cost_usd should be nil, got %v", *metrics[0].CostUSD)
	}
}
