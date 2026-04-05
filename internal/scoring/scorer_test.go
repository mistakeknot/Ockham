package scoring_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/authority"
	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/mistakeknot/Ockham/internal/scoring"
)

func TestScore_HighPriorityTheme(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6, "open": 0},
		Budgets:     map[string]float64{"auth": 0.6, "open": 0.4},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{
		{ID: "b1", Lane: "auth"},
		{ID: "b2", Lane: "open"},
	}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != 6 {
		t.Errorf("auth bead offset = %d, want 6", wv.Offsets["b1"])
	}
	if wv.Offsets["b2"] != 0 {
		t.Errorf("open bead offset = %d, want 0", wv.Offsets["b2"])
	}
}

func TestScore_LowPriorityTheme(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"cleanup": -3},
		Budgets:     map[string]float64{"cleanup": 1.0},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "cleanup"}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != -3 {
		t.Errorf("low bead offset = %d, want -3", wv.Offsets["b1"])
	}
}

func TestScore_NoLane_DefaultsToOpen(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"open": 0},
		Budgets:     map[string]float64{"open": 1.0},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: ""}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != 0 {
		t.Errorf("no-lane bead offset = %d, want 0", wv.Offsets["b1"])
	}
}

func TestScore_UnknownTheme_ZeroOffset(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6},
		Budgets:     map[string]float64{"auth": 1.0},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "unknown-lane"}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != 0 {
		t.Errorf("unknown-lane bead offset = %d, want 0", wv.Offsets["b1"])
	}
}

func TestScore_ClampToRange(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"x": 100},
		Budgets:     map[string]float64{"x": 1.0},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "x"}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != 6 {
		t.Errorf("offset %d, want exactly 6 (clamped)", wv.Offsets["b1"])
	}
}

func TestScore_ClampNegative(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"x": -100},
		Budgets:     map[string]float64{"x": 1.0},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "x"}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != -6 {
		t.Errorf("offset %d, want exactly -6 (clamped)", wv.Offsets["b1"])
	}
}

func TestScore_EmptyBeads(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6},
		Budgets:     map[string]float64{"auth": 1.0},
		FrozenLanes: map[string]bool{},
	}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, nil)
	if len(wv.Offsets) != 0 {
		t.Errorf("expected empty offsets for nil beads, got %d", len(wv.Offsets))
	}
}

func TestScore_AdvisoryOffsetApplied(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6},
		Budgets:     map[string]float64{"auth": 1.0},
		FrozenLanes: map[string]bool{},
	}
	an := anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			"auth": {Theme: "auth", AdvisoryOffset: -1},
		},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "auth"}}
	wv := scoring.Score(iv, authority.State{}, an, beads)

	if wv.Offsets["b1"] != 5 {
		t.Errorf("final offset = %d, want 5 (6 intent + -1 advisory)", wv.Offsets["b1"])
	}
	if wv.RawOffsets["b1"] != 6 {
		t.Errorf("raw offset = %d, want 6 (intent only)", wv.RawOffsets["b1"])
	}
}

func TestScore_AdvisoryClamped(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"x": -3},
		Budgets:     map[string]float64{"x": 1.0},
		FrozenLanes: map[string]bool{},
	}
	an := anomaly.State{
		Signals: map[string]anomaly.ThemeSignal{
			"x": {Theme: "x", AdvisoryOffset: -5},
		},
	}
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "x"}}
	wv := scoring.Score(iv, authority.State{}, an, beads)

	if wv.Offsets["b1"] != -6 {
		t.Errorf("offset = %d, want -6 (clamped: -3 + -5 = -8 → -6)", wv.Offsets["b1"])
	}
}

func TestScore_NilAnomalyState(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6},
		Budgets:     map[string]float64{"auth": 1.0},
		FrozenLanes: map[string]bool{},
	}
	// Zero-value anomaly.State with nil Signals map — must not panic
	beads := []scoring.BeadInfo{{ID: "b1", Lane: "auth"}}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)

	if wv.Offsets["b1"] != 6 {
		t.Errorf("offset = %d, want 6 (no advisory)", wv.Offsets["b1"])
	}
}

func TestScore_MultipleBeadsSameTheme(t *testing.T) {
	iv := intent.IntentVector{
		Offsets:     map[string]int{"auth": 6, "open": 0},
		Budgets:     map[string]float64{"auth": 0.7, "open": 0.3},
		FrozenLanes: map[string]bool{},
	}
	beads := []scoring.BeadInfo{
		{ID: "b1", Lane: "auth"},
		{ID: "b2", Lane: "auth"},
		{ID: "b3", Lane: "open"},
	}
	wv := scoring.Score(iv, authority.State{}, anomaly.State{}, beads)
	if wv.Offsets["b1"] != 6 || wv.Offsets["b2"] != 6 {
		t.Error("all auth beads should get +6")
	}
	if wv.Offsets["b3"] != 0 {
		t.Error("open bead should get 0")
	}
}
