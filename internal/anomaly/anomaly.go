package anomaly

// SignalStatus represents the state of an INFORM signal for a theme.
type SignalStatus string

const (
	StatusCleared SignalStatus = "cleared"
	StatusFired   SignalStatus = "fired"
	StatusStale   SignalStatus = "stale"
)

// ThemeSignal holds the INFORM signal state for one theme.
type ThemeSignal struct {
	Theme             string
	Status            SignalStatus
	DriftPct          float64 // current drift percentage (0 = no drift)
	AdvisoryOffset    int     // recommended offset adjustment (0 or negative)
	ConsecutiveClears int     // consecutive evaluations below clear threshold
	LastEvalAt        int64   // unix timestamp of last evaluation
}

// PleasureTrend represents the direction of a pleasure signal.
type PleasureTrend string

const (
	TrendImproving    PleasureTrend = "improving"
	TrendStable       PleasureTrend = "stable"
	TrendDegrading    PleasureTrend = "degrading"
	TrendInsufficient PleasureTrend = "insufficient_data"
)

// PleasureSignal holds one pleasure signal for one theme.
type PleasureSignal struct {
	Name  string        // e.g., "first_attempt_pass_rate"
	Theme string
	Trend PleasureTrend
	Value float64 // current value (rate or p50)
}

// State carries the full anomaly evaluation result.
// Consumed by governor.Evaluate() and scoring.Score().
// Zero value is safe: nil Signals map means no advisory offsets.
type State struct {
	Signals  map[string]ThemeSignal // key: theme name
	Pleasure []PleasureSignal
}
