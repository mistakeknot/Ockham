package constrain

import "math"

// FastPathPolicy configures the F4 rate-of-change fast path. A signal whose
// absolute delta between two windows exceeds the configured threshold fires
// CONSTRAIN immediately, bypassing F3's multi-window confirmation streak.
//
// Thresholds are keyed by signal name (e.g. "tool_error_rate", "drift_pct").
// DefaultThreshold applies to any signal not listed in Thresholds; set it to
// zero to disable the fast path for unlisted signals.
type FastPathPolicy struct {
	Thresholds       map[string]float64
	DefaultThreshold float64
}

// DefaultFastPathPolicy returns thresholds tuned for the two signals Ockham
// currently collects. DefaultThreshold=0 means unlisted signals never fast-path
// — callers extend Thresholds to opt new metrics in.
func DefaultFastPathPolicy() FastPathPolicy {
	return FastPathPolicy{
		Thresholds: map[string]float64{
			"tool_error_rate":         0.30, // absolute — 30 percentage-point jump
			"session_completion_rate": 0.30, // absolute — 30pp drop
			"drift_pct":               0.40, // absolute — 40pp drift in one window
		},
		DefaultThreshold: 0,
	}
}

// FastPathDecision captures the outcome of evaluating a single observation
// against a FastPathPolicy. It carries enough context for observability so
// callers can persist the decision alongside the CONSTRAIN record.
type FastPathDecision struct {
	Theme     string
	Signal    string
	Previous  float64
	Current   float64
	Delta     float64 // absolute
	Threshold float64
	Fired     bool
}

// Threshold returns the configured threshold for signal — either a per-signal
// entry or DefaultThreshold when unset.
func (p FastPathPolicy) Threshold(signal string) float64 {
	if t, ok := p.Thresholds[signal]; ok {
		return t
	}
	return p.DefaultThreshold
}

// Evaluate compares the absolute change between previous and current against
// the configured threshold for signal. When the delta meets or exceeds a
// positive threshold, Fired=true and the caller should invoke
// constrain.ConstrainTheme with fastPath=true — skipping the F3 streak.
//
// A threshold of zero (either unlisted signal with DefaultThreshold=0, or an
// explicit 0 entry) never fires. This is the safe default: fast path only
// activates for metrics the operator has opted in.
func (p FastPathPolicy) Evaluate(theme, signal string, previous, current float64) FastPathDecision {
	threshold := p.Threshold(signal)
	delta := math.Abs(current - previous)
	fired := threshold > 0 && delta >= threshold
	return FastPathDecision{
		Theme:     theme,
		Signal:    signal,
		Previous:  previous,
		Current:   current,
		Delta:     delta,
		Threshold: threshold,
		Fired:     fired,
	}
}
