package signals_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestInsertObservation_Roundtrip(t *testing.T) {
	db := newTestDB(t)

	if err := db.InsertObservation("auth", "session_completion_rate", 0.85, 1000); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertObservation("auth", "tool_error_rate", 0.10, 1000); err != nil {
		t.Fatal(err)
	}

	rows, err := db.RecentObservations("auth", "session_completion_rate", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Value != 0.85 {
		t.Errorf("value = %f, want 0.85", rows[0].Value)
	}
}

func TestRecentObservations_FiltersByTime(t *testing.T) {
	db := newTestDB(t)

	db.InsertObservation("auth", "session_completion_rate", 0.80, 100)
	db.InsertObservation("auth", "session_completion_rate", 0.85, 200)
	db.InsertObservation("auth", "session_completion_rate", 0.90, 300)

	rows, err := db.RecentObservations("auth", "session_completion_rate", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (since=200)", len(rows))
	}
}

func TestPruneObservations(t *testing.T) {
	db := newTestDB(t)

	for i := 1; i <= 10; i++ {
		db.InsertObservation("auth", "session_completion_rate", float64(i)*0.1, int64(i*100))
	}

	deleted, err := db.PruneObservations("auth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 5 {
		t.Errorf("deleted = %d, want 5", deleted)
	}

	remaining, err := db.RecentObservations("auth", "session_completion_rate", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 5 {
		t.Errorf("remaining = %d, want 5", len(remaining))
	}
}

func TestGetObservationStats_Empty(t *testing.T) {
	db := newTestDB(t)
	stats, err := db.GetObservationStats(1000)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Available {
		t.Error("expected unavailable with no data")
	}
	if stats.MetricCount != 0 {
		t.Errorf("metric_count = %d, want 0", stats.MetricCount)
	}
}

func TestGetObservationStats_WithData(t *testing.T) {
	db := newTestDB(t)

	db.InsertObservation("auth", "session_completion_rate", 0.85, 5000)
	db.InsertObservation("auth", "tool_error_rate", 0.10, 5000)

	stats, err := db.GetObservationStats(4000)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Available {
		t.Error("expected available with recent data")
	}
	if stats.LastCollect != 5000 {
		t.Errorf("last_collect = %d, want 5000", stats.LastCollect)
	}
	if stats.MetricCount != 2 {
		t.Errorf("metric_count = %d, want 2", stats.MetricCount)
	}
}

func TestSchemaMigration_V2ToV3(t *testing.T) {
	db := newTestDB(t)

	// Verify observation_metrics table exists by inserting
	if err := db.InsertObservation("test", "rate", 0.5, 100); err != nil {
		t.Fatalf("observation_metrics table should exist after migration: %v", err)
	}

	// Verify existing bead_metrics still works
	err := db.InsertBeadMetric(signals.BeadMetric{
		BeadID:      "test-1",
		Theme:       "test",
		CycleTimeMs: 1000,
		CompletedAt: 100,
	})
	if err != nil {
		t.Fatalf("bead_metrics should still work after migration: %v", err)
	}
}
