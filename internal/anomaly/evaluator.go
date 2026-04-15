package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/observation"
	"github.com/mistakeknot/Ockham/internal/signals"
)

// Evaluator orchestrates signal evaluation across all themes.
type Evaluator struct {
	db           *signals.DB
	cfg          Config
	sentinelPath string // path to factory-paused.json; testable via NewEvaluator
	observer     observation.Observer
}

// EvaluatorOption configures an Evaluator.
type EvaluatorOption func(*Evaluator)

// WithSentinelPath overrides the default halt sentinel path.
func WithSentinelPath(path string) EvaluatorOption {
	return func(e *Evaluator) {
		if path != "" {
			e.sentinelPath = path
		}
	}
}

// WithObserver attaches an observation source to the evaluator.
func WithObserver(obs observation.Observer) EvaluatorOption {
	return func(e *Evaluator) {
		e.observer = obs
	}
}

// NewEvaluator creates an Evaluator with the given DB and config.
func NewEvaluator(db *signals.DB, cfg Config, opts ...EvaluatorOption) *Evaluator {
	e := &Evaluator{
		db:           db,
		cfg:          cfg,
		sentinelPath: halt.DefaultSentinelPath(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Evaluate runs drift detection and pleasure signals for all themes.
// Returns the full State. Handles short-circuit, staleness, and degradation.
// When >=BypassThreshold themes fire simultaneously, triggers BYPASS (factory halt).
func (e *Evaluator) Evaluate(themes []string, now int64) (State, error) {
	if err := e.cfg.Validate(); err != nil {
		return State{}, err
	}

	state := State{
		Signals:  make(map[string]ThemeSignal, len(themes)),
		Pleasure: make([]PleasureSignal, 0, len(themes)*3),
	}

	// Observation metrics: collect and persist
	var obsMetrics []observation.ObservationMetric
	loggedDegradation := false
	if e.observer != nil {
		if e.observer.IsAvailable() {
			var err error
			obsMetrics, err = e.observer.Collect(context.Background(), themes, 24*time.Hour)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ockham: observation collect degraded: %v\n", err)
			}
			for _, m := range obsMetrics {
				e.db.InsertObservation(m.Theme, m.MetricType, m.Value, m.CollectedAt)
			}
		} else if !loggedDegradation {
			fmt.Fprintf(os.Stderr, "ockham: Alwe observation degraded: CASS unavailable\n")
			loggedDegradation = true
		}
	}

	for _, theme := range themes {
		lastCompleted, found, err := e.db.LastBeadCompletedAt(theme)
		if err != nil {
			return state, fmt.Errorf("last completed for %q: %w", theme, err)
		}

		prior := e.loadPriorSignal(theme)

		// Short-circuit: no beads at all for this theme
		if !found {
			state.Signals[theme] = prior
			continue
		}

		// Short-circuit: no new beads since last evaluation
		if lastCompleted <= prior.LastEvalAt {
			state.Signals[theme] = prior
			continue
		}

		// Staleness check
		staleThreshold := now - int64(e.cfg.StaleDays*86400)
		if lastCompleted < staleThreshold {
			stale := ThemeSignal{
				Theme:      theme,
				Status:     StatusStale,
				LastEvalAt: now,
			}
			state.Signals[theme] = stale
			e.persistSignal(theme, stale)
			continue
		}

		// Fetch rolling window
		metrics, err := e.db.LatestBeadMetrics(theme, e.cfg.MaxWindow)
		if err != nil {
			return state, fmt.Errorf("metrics for %q: %w", theme, err)
		}

		// Drift detection
		signal := EvaluateDrift(metrics, prior, e.cfg)
		signal.Theme = theme
		signal.LastEvalAt = now

		// Advisory: observation metrics can increase INFORM severity (not BYPASS)
		for _, m := range obsMetrics {
			if m.Theme != theme {
				continue
			}
			elevate := false
			if m.MetricType == "tool_error_rate" && m.Value > 0.3 {
				elevate = true
			}
			if m.MetricType == "session_completion_rate" && m.Value < 0.5 {
				elevate = true
			}
			if elevate && signal.Status != StatusFired {
				signal.Status = StatusFired
				if signal.AdvisoryOffset > -e.cfg.MaxAdvisoryPerCycle {
					signal.AdvisoryOffset = -e.cfg.MaxAdvisoryPerCycle
				}
			}
		}

		// Log transitions
		if signal.Status != prior.Status {
			fmt.Fprintf(os.Stderr, "ockham: INFORM %s → %s for theme %q (drift=%.1f%%)\n",
				prior.Status, signal.Status, theme, signal.DriftPct*100)
		}

		state.Signals[theme] = signal

		// Pleasure signals
		state.Pleasure = append(state.Pleasure,
			EvaluatePassRate(metrics, e.cfg.MinWindow, theme),
			EvaluateCycleTimeTrend(metrics, e.cfg.MinWindow, theme),
			EvaluateCostTrend(metrics, e.cfg.MinWindow, theme),
		)

		// Persist signal and pleasure state
		e.persistSignal(theme, signal)
		e.persistPleasure(theme, state.Pleasure[len(state.Pleasure)-3:])

		// Prune old data
		if _, err := e.db.PruneBeadMetrics(theme, e.cfg.MaxWindow*2); err != nil {
			fmt.Fprintf(os.Stderr, "ockham: prune degraded for %q: %v\n", theme, err)
		}
		if _, err := e.db.PruneObservations(theme, e.cfg.MaxWindow*2); err != nil {
			fmt.Fprintf(os.Stderr, "ockham: observation prune degraded for %q: %v\n", theme, err)
		}
	}

	// Factory guard
	state.Signals = ApplyFactoryGuard(state.Signals, e.cfg.FactoryGuard)

	// BYPASS trigger: count distinct fired themes
	var firedThemes []string
	for theme, sig := range state.Signals {
		if sig.Status == StatusFired {
			firedThemes = append(firedThemes, theme)
		}
	}
	if len(firedThemes) >= e.cfg.BypassThreshold {
		if err := e.triggerBypass(firedThemes, state, now); err != nil {
			return state, fmt.Errorf("%w: %v", ErrBypassFailed, err)
		}
	}

	return state, nil
}

// triggerBypass writes the factory halt sentinel and interspect record.
// Sentinel write is atomic + durable (temp + fsync + rename).
// Interspect write is soft failure — never rolls back sentinel.
func (e *Evaluator) triggerBypass(firedThemes []string, state State, now int64) error {
	record := map[string]any{
		"reason":           "BYPASS",
		"code":             "bypass_multi_root_cause",
		"triggered_themes": firedThemes,
		"signal_values":    state.Signals,
		"timestamp":        now,
		"schema_version":   1,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	// Step 1: Atomic durable sentinel write (temp + fsync + rename)
	dir := filepath.Dir(e.sentinelPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".factory-paused-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // cleanup temp on any failure path

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0400); err != nil {
		return err
	}
	// Atomic rename — if sentinel already exists (concurrent BYPASS), overwrites
	// which is acceptable (halt is already active with same or similar reason).
	if err := os.Rename(tmpPath, e.sentinelPath); err != nil {
		return err
	}

	// Step 2: Write interspect halt record (soft failure — NEVER roll back sentinel)
	interspectPath := filepath.Join(os.Getenv("HOME"), ".clavain", "interspect", "halt-record.json")
	interspectRecord := map[string]any{
		"event_id":         fmt.Sprintf("bypass-%d", now),
		"timestamp":        now,
		"reason":           fmt.Sprintf("BYPASS: %d distinct root causes fired: %s", len(firedThemes), strings.Join(firedThemes, ", ")),
		"status":           "active",
		"triggered_themes": firedThemes,
	}
	irData, _ := json.Marshal(interspectRecord)
	if err := os.MkdirAll(filepath.Dir(interspectPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: interspect halt record degraded (mkdir): %v\n", err)
	} else if err := os.WriteFile(interspectPath, irData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: interspect halt record degraded (write): %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "ockham: BYPASS triggered — %d distinct root causes: %s\n",
		len(firedThemes), strings.Join(firedThemes, ", "))
	return nil
}

func (e *Evaluator) loadPriorSignal(theme string) ThemeSignal {
	val, found, err := e.db.GetSignalState("inform:" + theme)
	if err != nil || !found {
		return ThemeSignal{Theme: theme, Status: StatusCleared}
	}

	var sig ThemeSignal
	if err := json.Unmarshal([]byte(val), &sig); err != nil {
		return ThemeSignal{Theme: theme, Status: StatusCleared}
	}
	return sig
}

func (e *Evaluator) persistSignal(theme string, sig ThemeSignal) {
	data, err := json.Marshal(sig)
	if err != nil {
		return
	}
	e.db.SetSignalState("inform:"+theme, string(data), sig.LastEvalAt)
}

func (e *Evaluator) persistPleasure(theme string, sigs []PleasureSignal) {
	for _, sig := range sigs {
		data, err := json.Marshal(sig)
		if err != nil {
			continue
		}
		e.db.SetSignalState(
			fmt.Sprintf("pleasure:%s:%s", sig.Name, theme),
			string(data),
			0, // pleasure signals don't need timestamps
		)
	}
}
