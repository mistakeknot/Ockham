package governor

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/authority"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/mistakeknot/Ockham/internal/scoring"
)

// now returns the current unix timestamp. Extracted for testability.
var now = func() int64 { return time.Now().Unix() }

// Governor assembles subsystem stores and evaluates dispatch weights.
// Dependency direction: governor imports intent, scoring, authority, anomaly, halt.
type Governor struct {
	intent    *intent.Store
	halt      *halt.Sentinel
	anomalyEv *anomaly.Evaluator // nil = stub behavior (no advisory offsets)
}

// New creates a Governor with the given stores.
// anomalyEvaluator may be nil for backward compatibility (Wave 1 stub behavior).
func New(intentStore *intent.Store, haltSentinel *halt.Sentinel, anomalyEvaluator ...*anomaly.Evaluator) *Governor {
	g := &Governor{
		intent: intentStore,
		halt:   haltSentinel,
	}
	if len(anomalyEvaluator) > 0 {
		g.anomalyEv = anomalyEvaluator[0]
	}
	return g
}

// Evaluate computes per-bead weight offsets.
// Returns error if the factory is paused (INV-8).
// Context is reserved for future use (cancellation, deadlines).
func (g *Governor) Evaluate(_ context.Context, beads []scoring.BeadInfo) (scoring.WeightVector, error) {
	// INV-8: halt check FIRST — skip signal evaluation entirely when halted
	if g.halt.IsHalted() {
		return scoring.WeightVector{
			Offsets:    map[string]int{},
			RawOffsets: map[string]int{},
		}, fmt.Errorf("factory halted: %s exists", g.halt.Path())
	}

	// Load intent (falls back to default on missing/corrupt)
	intentFile, err := g.intent.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ockham: intent load degraded: %v (using defaults)\n", err)
	}

	iv := intentFile.ToVector()

	// Authority stub (Wave 3)
	as := authority.State{}

	// Anomaly evaluation
	an := anomaly.State{}
	if g.anomalyEv != nil {
		// Collect themes from beads
		themes := make(map[string]bool)
		for _, b := range beads {
			lane := b.Lane
			if lane == "" {
				lane = "open"
			}
			themes[lane] = true
		}
		themeList := make([]string, 0, len(themes))
		for t := range themes {
			themeList = append(themeList, t)
		}

		var evalErr error
		an, evalErr = g.anomalyEv.Evaluate(themeList, now())
		if evalErr != nil {
			fmt.Fprintf(os.Stderr, "ockham: anomaly evaluation degraded: %v\n", evalErr)
			an = anomaly.State{} // fall back to neutral
		}
	}

	wv := scoring.Score(iv, as, an, beads)
	return wv, nil
}
