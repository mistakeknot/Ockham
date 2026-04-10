package discover_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/discover"
)

func TestSortBeadsByPriority(t *testing.T) {
	beads := []discover.DiscoveredBead{
		{ID: "aaa", Priority: "P3"},
		{ID: "bbb", Priority: "P0"},
		{ID: "ccc", Priority: "P1"},
		{ID: "ddd", Priority: "P2"},
	}

	discover.SortBeads(beads)

	want := []string{"P0", "P1", "P2", "P3"}
	for i, b := range beads {
		if b.Priority != want[i] {
			t.Errorf("beads[%d].Priority = %s, want %s", i, b.Priority, want[i])
		}
	}
}

func TestSortBeadsStableByID(t *testing.T) {
	beads := []discover.DiscoveredBead{
		{ID: "zzz", Priority: "P1"},
		{ID: "aaa", Priority: "P1"},
		{ID: "mmm", Priority: "P1"},
	}

	discover.SortBeads(beads)

	want := []string{"aaa", "mmm", "zzz"}
	for i, b := range beads {
		if b.ID != want[i] {
			t.Errorf("beads[%d].ID = %s, want %s", i, b.ID, want[i])
		}
	}
}

func TestSortBeadsMixedPriorityAndID(t *testing.T) {
	beads := []discover.DiscoveredBead{
		{ID: "b-task", Priority: "P2"},
		{ID: "a-task", Priority: "P2"},
		{ID: "c-task", Priority: "P0"},
		{ID: "d-task", Priority: "P4"},
	}

	discover.SortBeads(beads)

	want := []struct {
		id       string
		priority string
	}{
		{"c-task", "P0"},
		{"a-task", "P2"},
		{"b-task", "P2"},
		{"d-task", "P4"},
	}
	for i, b := range beads {
		if b.ID != want[i].id || b.Priority != want[i].priority {
			t.Errorf("beads[%d] = {%s, %s}, want {%s, %s}",
				i, b.ID, b.Priority, want[i].id, want[i].priority)
		}
	}
}

func TestNormalizePriority(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "P2"},
		{"P0", "P0"},
		{"P4", "P4"},
	}
	for _, tt := range tests {
		if got := discover.NormalizePriority(tt.in); got != tt.want {
			t.Errorf("NormalizePriority(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortBeadsDefaultPriority(t *testing.T) {
	// Beads with empty priority should sort as P2
	beads := []discover.DiscoveredBead{
		{ID: "no-pri", Priority: "P2"}, // normalized empty
		{ID: "high", Priority: "P0"},
		{ID: "low", Priority: "P4"},
	}

	discover.SortBeads(beads)

	want := []string{"high", "no-pri", "low"}
	for i, b := range beads {
		if b.ID != want[i] {
			t.Errorf("beads[%d].ID = %s, want %s", i, b.ID, want[i])
		}
	}
}

func TestSortBeadsUnknownPriority(t *testing.T) {
	// Unknown priority strings should default to P2
	beads := []discover.DiscoveredBead{
		{ID: "unknown", Priority: "critical"},
		{ID: "p1", Priority: "P1"},
		{ID: "p3", Priority: "P3"},
	}

	discover.SortBeads(beads)

	// "critical" → index 2 (P2), so order: P1, P2(critical), P3
	want := []string{"p1", "unknown", "p3"}
	for i, b := range beads {
		if b.ID != want[i] {
			t.Errorf("beads[%d].ID = %s, want %s", i, b.ID, want[i])
		}
	}
}
