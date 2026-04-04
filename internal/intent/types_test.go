package intent_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/intent"
)

func TestPriorityOffset(t *testing.T) {
	tests := []struct {
		p    intent.Priority
		want int
	}{
		{intent.PriorityHigh, 6},
		{intent.PriorityNormal, 0},
		{intent.PriorityLow, -3},
	}
	for _, tt := range tests {
		if got := tt.p.Offset(); got != tt.want {
			t.Errorf("Priority(%s).Offset() = %d, want %d", tt.p, got, tt.want)
		}
	}
}

func TestPriorityFromString(t *testing.T) {
	tests := []struct {
		s       string
		want    intent.Priority
		wantErr bool
	}{
		{"high", intent.PriorityHigh, false},
		{"normal", intent.PriorityNormal, false},
		{"low", intent.PriorityLow, false},
		{"HIGH", intent.PriorityHigh, false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		got, err := intent.ParsePriority(tt.s)
		if tt.wantErr && err == nil {
			t.Errorf("ParsePriority(%q) expected error", tt.s)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParsePriority(%q) = %s, want %s", tt.s, got, tt.want)
		}
	}
}

func TestIntentFileDefault(t *testing.T) {
	f := intent.DefaultFile()
	if len(f.Themes) == 0 {
		t.Error("default should have at least one theme")
	}
	var total float64
	for _, tb := range f.Themes {
		total += tb.Budget
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("default budgets sum to %f, want 1.0", total)
	}
}

func TestToVector(t *testing.T) {
	f := intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 0.6, Priority: intent.PriorityHigh},
			"open": {Budget: 0.4, Priority: intent.PriorityNormal},
		},
		Constraints: intent.Constraints{Freeze: []string{"auth"}},
	}
	v := f.ToVector()
	if v.Offsets["auth"] != 6 {
		t.Errorf("auth offset = %d, want 6", v.Offsets["auth"])
	}
	if v.Offsets["open"] != 0 {
		t.Errorf("open offset = %d, want 0", v.Offsets["open"])
	}
	if !v.FrozenLanes["auth"] {
		t.Error("auth should be frozen")
	}
}
