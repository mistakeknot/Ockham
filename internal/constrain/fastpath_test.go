package constrain_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/constrain"
)

func TestDefaultFastPathPolicy_HasKnownSignals(t *testing.T) {
	p := constrain.DefaultFastPathPolicy()
	for _, sig := range []string{"tool_error_rate", "session_completion_rate", "drift_pct"} {
		if p.Threshold(sig) <= 0 {
			t.Errorf("default policy missing threshold for %q", sig)
		}
	}
	if p.DefaultThreshold != 0 {
		t.Errorf("DefaultThreshold = %v, want 0 (opt-in only)", p.DefaultThreshold)
	}
}

func TestThreshold_FallbackToDefault(t *testing.T) {
	p := constrain.FastPathPolicy{
		Thresholds:       map[string]float64{"known": 0.5},
		DefaultThreshold: 0.25,
	}
	if got := p.Threshold("known"); got != 0.5 {
		t.Errorf("known threshold = %v, want 0.5", got)
	}
	if got := p.Threshold("unknown"); got != 0.25 {
		t.Errorf("unknown threshold = %v, want 0.25 (default)", got)
	}
}

func TestEvaluate_FiresOnLargeDelta(t *testing.T) {
	p := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"tool_error_rate": 0.30},
	}
	// 2% → 40% = 38pp jump, exceeds 30pp threshold.
	d := p.Evaluate("refactor", "tool_error_rate", 0.02, 0.40)
	if !d.Fired {
		t.Errorf("expected Fired=true for delta=%.2f threshold=%.2f", d.Delta, d.Threshold)
	}
	if d.Theme != "refactor" || d.Signal != "tool_error_rate" {
		t.Errorf("decision missing context: %+v", d)
	}
}

func TestEvaluate_DoesNotFireBelowThreshold(t *testing.T) {
	p := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"tool_error_rate": 0.30},
	}
	d := p.Evaluate("refactor", "tool_error_rate", 0.02, 0.10) // 8pp
	if d.Fired {
		t.Errorf("unexpected fire: %+v", d)
	}
}

func TestEvaluate_SymmetricOnDecrease(t *testing.T) {
	// session_completion_rate dropping counts the same as error_rate rising —
	// delta is absolute.
	p := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"completion": 0.30},
	}
	d := p.Evaluate("refactor", "completion", 0.95, 0.60) // drop of 35pp
	if !d.Fired {
		t.Errorf("expected fire on drop: %+v", d)
	}
}

func TestEvaluate_ZeroThresholdNeverFires(t *testing.T) {
	p := constrain.FastPathPolicy{
		Thresholds:       map[string]float64{"explicit_zero": 0},
		DefaultThreshold: 0,
	}
	cases := []struct{ signal string }{
		{"explicit_zero"},
		{"unlisted"},
	}
	for _, c := range cases {
		d := p.Evaluate("refactor", c.signal, 0, 1) // huge delta
		if d.Fired {
			t.Errorf("%s fired with zero threshold: %+v", c.signal, d)
		}
	}
}

func TestEvaluate_EqualToThresholdFires(t *testing.T) {
	// >= threshold, not strictly greater.
	p := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"x": 0.30},
	}
	d := p.Evaluate("t", "x", 0, 0.30)
	if !d.Fired {
		t.Errorf("expected fire at exact threshold: %+v", d)
	}
}

func TestEvaluate_CapturesFullContext(t *testing.T) {
	p := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"x": 0.10},
	}
	d := p.Evaluate("auth", "x", 0.05, 0.22)
	if d.Previous != 0.05 || d.Current != 0.22 {
		t.Errorf("prev/curr lost: %+v", d)
	}
	// Delta is absolute; allow tiny FP slop.
	if got := d.Delta; got < 0.1699 || got > 0.1701 {
		t.Errorf("delta = %v, want ~0.17", got)
	}
	if d.Threshold != 0.10 {
		t.Errorf("threshold = %v, want 0.10", d.Threshold)
	}
}
