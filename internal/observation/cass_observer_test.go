package observation

import (
	"context"
	"testing"
	"time"
)

// mockObserver implements Observer for testing.
type mockObserver struct {
	available bool
	metrics   []ObservationMetric
	err       error
}

func (m *mockObserver) IsAvailable() bool { return m.available }
func (m *mockObserver) Collect(_ context.Context, _ []string, _ time.Duration) ([]ObservationMetric, error) {
	return m.metrics, m.err
}

func TestMockObserver_Available(t *testing.T) {
	obs := &mockObserver{available: true, metrics: []ObservationMetric{
		{Theme: "auth", MetricType: "session_completion_rate", Value: 0.85, CollectedAt: 100},
	}}

	if !obs.IsAvailable() {
		t.Fatal("expected available")
	}
	metrics, err := obs.Collect(context.Background(), []string{"auth"}, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 {
		t.Fatalf("got %d metrics, want 1", len(metrics))
	}
	if metrics[0].Value != 0.85 {
		t.Errorf("value = %f, want 0.85", metrics[0].Value)
	}
}

func TestMockObserver_Unavailable(t *testing.T) {
	obs := &mockObserver{available: false}
	if obs.IsAvailable() {
		t.Fatal("expected unavailable")
	}
	metrics, err := obs.Collect(context.Background(), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if metrics != nil {
		t.Errorf("expected nil metrics, got %v", metrics)
	}
}

func TestParseTimeline_Empty(t *testing.T) {
	for _, input := range []string{"", "null", "[]"} {
		entries, err := parseTimeline(input)
		if err != nil {
			t.Errorf("parseTimeline(%q) error: %v", input, err)
		}
		if len(entries) != 0 {
			t.Errorf("parseTimeline(%q) = %d entries, want 0", input, len(entries))
		}
	}
}

func TestParseTimeline_Valid(t *testing.T) {
	raw := `[{"session_id":"s1","provider":"claude","timestamp":"2026-04-14T12:00:00Z","status":"done"},{"session_id":"s2","provider":"codex","timestamp":"2026-04-14T13:00:00Z","status":"error"}]`
	entries, err := parseTimeline(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Status != "done" {
		t.Errorf("entries[0].Status = %q, want done", entries[0].Status)
	}
	if entries[1].Status != "error" {
		t.Errorf("entries[1].Status = %q, want error", entries[1].Status)
	}
}

func TestParseTimeline_InvalidJSON(t *testing.T) {
	_, err := parseTimeline("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDurationToString(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{24 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{7 * 24 * time.Hour, "7d"},
		{12 * time.Hour, "12h"},
		{36 * time.Hour, "36h"}, // not evenly divisible by 24
	}
	for _, tt := range tests {
		got := durationToString(tt.d)
		if got != tt.want {
			t.Errorf("durationToString(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestCassObserver_UnavailableReturnsNil(t *testing.T) {
	// CassObserver with no CASS binary should return nil, nil from Collect
	obs := NewCassObserver()
	// Force unavailable state
	avail := false
	obs.availableCache = &avail
	obs.cacheExpiry = time.Now().Add(time.Hour)

	metrics, err := obs.Collect(context.Background(), []string{"auth"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metrics != nil {
		t.Errorf("expected nil metrics, got %v", metrics)
	}
}
