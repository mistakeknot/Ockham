package anomaly

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// Evaluator orchestrates signal evaluation across all themes.
type Evaluator struct {
	db  *signals.DB
	cfg Config
}

// NewEvaluator creates an Evaluator with the given DB and config.
func NewEvaluator(db *signals.DB, cfg Config) *Evaluator {
	return &Evaluator{db: db, cfg: cfg}
}

// Evaluate runs drift detection and pleasure signals for all themes.
// Returns the full State. Handles short-circuit, staleness, and degradation.
func (e *Evaluator) Evaluate(themes []string, now int64) (State, error) {
	state := State{
		Signals:  make(map[string]ThemeSignal, len(themes)),
		Pleasure: make([]PleasureSignal, 0, len(themes)*3),
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
	}

	// Factory guard
	state.Signals = ApplyFactoryGuard(state.Signals, e.cfg.FactoryGuard)

	return state, nil
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
