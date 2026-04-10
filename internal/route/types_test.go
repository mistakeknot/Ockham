package route_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/route"
)

func TestTypeCapabilitiesCompleteness(t *testing.T) {
	// Every expected bead type must be in the map.
	expectedTypes := []string{"task", "bug", "feature", "epic", "research"}
	caps := route.ExportedTypeCapabilities()

	for _, typ := range expectedTypes {
		reqs, ok := caps[typ]
		if !ok {
			t.Errorf("typeCapabilities missing bead type %q", typ)
			continue
		}
		if len(reqs) == 0 {
			t.Errorf("typeCapabilities[%q] has no required capabilities", typ)
		}
	}
}

func TestTypeCapabilitiesNoDuplicates(t *testing.T) {
	caps := route.ExportedTypeCapabilities()

	for typ, reqs := range caps {
		seen := make(map[string]bool)
		for _, cap := range reqs {
			if seen[cap] {
				t.Errorf("typeCapabilities[%q]: duplicate capability %q", typ, cap)
			}
			seen[cap] = true
		}
	}
}

func TestRoutingStatusValues(t *testing.T) {
	// Ensure the status constants have expected string values.
	tests := []struct {
		status route.RoutingStatus
		want   string
	}{
		{route.Routed, "routed"},
		{route.Blocked, "blocked"},
		{route.Deferred, "deferred"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("RoutingStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestRoutingSummaryComputation(t *testing.T) {
	rt := &route.RoutingTable{
		Decisions: []route.RoutingDecision{
			{Status: route.Routed},
			{Status: route.Routed},
			{Status: route.Blocked},
			{Status: route.Deferred},
		},
	}
	rt.ComputeSummary()

	if rt.Summary.Total != 4 {
		t.Errorf("Total = %d, want 4", rt.Summary.Total)
	}
	if rt.Summary.Routed != 2 {
		t.Errorf("Routed = %d, want 2", rt.Summary.Routed)
	}
	if rt.Summary.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", rt.Summary.Blocked)
	}
	if rt.Summary.Deferred != 1 {
		t.Errorf("Deferred = %d, want 1", rt.Summary.Deferred)
	}
}
