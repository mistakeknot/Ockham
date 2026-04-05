package anomaly

import (
	"sort"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// Config holds drift detection parameters.
type Config struct {
	MinWindow           int     // minimum beads for evaluation (default 10)
	MaxWindow           int     // adaptive: use up to this many beads (default 30)
	FireThreshold       float64 // drift pct to fire INFORM (default 0.20)
	ClearThreshold      float64 // drift pct to clear INFORM (default 0.10)
	ClearCount          int     // consecutive clears needed (default 3)
	MaxAdvisoryPerCycle int     // rate limit: max offset reduction per theme per cycle (default 1)
	FactoryGuard        int     // max sum of advisory reductions across all themes (default 12)
	StaleDays           int     // days without new beads → stale (default 14)
}

// DefaultConfig returns production defaults.
func DefaultConfig() Config {
	return Config{
		MinWindow:           10,
		MaxWindow:           30,
		FireThreshold:       0.20,
		ClearThreshold:      0.10,
		ClearCount:          3,
		MaxAdvisoryPerCycle: 1,
		FactoryGuard:        12,
		StaleDays:           14,
	}
}

// EvaluateDrift computes the INFORM signal state for a theme from its bead metrics.
// Metrics must be ordered by completed_at DESC (newest first).
// Returns the updated signal; caller is responsible for persistence.
func EvaluateDrift(metrics []signals.BeadMetric, prior ThemeSignal, cfg Config) ThemeSignal {
	if len(metrics) < cfg.MinWindow {
		return prior // insufficient data
	}

	// Split into baseline (older half) and recent (newer half).
	// Metrics are DESC order, so recent = first half, baseline = second half.
	mid := len(metrics) / 2
	recent := metrics[:mid]
	baseline := metrics[mid:]

	recentP50 := medianCycleTime(recent)
	baselineP50 := medianCycleTime(baseline)

	if baselineP50 == 0 {
		return prior // avoid division by zero
	}

	drift := (recentP50 - baselineP50) / baselineP50
	result := prior
	result.DriftPct = drift

	switch {
	case drift >= cfg.FireThreshold:
		result.Status = StatusFired
		result.ConsecutiveClears = 0
		// Rate-limited advisory: at most -MaxAdvisoryPerCycle per cycle
		if result.AdvisoryOffset > -cfg.MaxAdvisoryPerCycle {
			result.AdvisoryOffset = -cfg.MaxAdvisoryPerCycle
		}
	case drift < cfg.ClearThreshold:
		result.ConsecutiveClears++
		if result.ConsecutiveClears >= cfg.ClearCount {
			result.Status = StatusCleared
			result.AdvisoryOffset = 0
			result.ConsecutiveClears = 0
		}
	default:
		// Between clear and fire thresholds: maintain current state (hysteresis).
		// Reset consecutive clears since we're above clear threshold.
		result.ConsecutiveClears = 0
	}

	return result
}

// ApplyFactoryGuard ensures total advisory reductions across all themes
// do not exceed the guard ceiling. Reduces the heaviest offsets first.
func ApplyFactoryGuard(sigs map[string]ThemeSignal, guard int) map[string]ThemeSignal {
	totalReduction := 0
	for _, s := range sigs {
		if s.AdvisoryOffset < 0 {
			totalReduction += -s.AdvisoryOffset
		}
	}

	if totalReduction <= guard {
		return sigs
	}

	// Sort themes by advisory offset (most negative first) for proportional reduction
	type entry struct {
		theme  string
		offset int
	}
	var entries []entry
	for theme, s := range sigs {
		if s.AdvisoryOffset < 0 {
			entries = append(entries, entry{theme, s.AdvisoryOffset})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].offset < entries[j].offset // most negative first
	})

	// Reduce offsets from heaviest until within guard
	remaining := totalReduction - guard
	for _, e := range entries {
		if remaining <= 0 {
			break
		}
		s := sigs[e.theme]
		reduction := -s.AdvisoryOffset
		if reduction > remaining {
			reduction = remaining
		}
		s.AdvisoryOffset += reduction // make less negative
		sigs[e.theme] = s
		remaining -= reduction
	}

	return sigs
}

// medianCycleTime computes the median cycle time from a slice of metrics.
// For even-length slices, averages the two middle values.
func medianCycleTime(metrics []signals.BeadMetric) float64 {
	if len(metrics) == 0 {
		return 0
	}

	times := make([]int64, len(metrics))
	for i, m := range metrics {
		times[i] = m.CycleTimeMs
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	n := len(times)
	if n%2 == 1 {
		return float64(times[n/2])
	}
	return float64(times[n/2-1]+times[n/2]) / 2.0
}
