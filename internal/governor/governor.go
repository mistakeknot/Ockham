package governor

import (
	"context"
	"fmt"
	"os"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/authority"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/mistakeknot/Ockham/internal/scoring"
)

// Governor assembles subsystem stores and evaluates dispatch weights.
// Dependency direction: governor imports intent, scoring, authority, anomaly, halt.
type Governor struct {
	intent *intent.Store
	halt   *halt.Sentinel
}

// New creates a Governor with the given stores.
func New(intentStore *intent.Store, haltSentinel *halt.Sentinel) *Governor {
	return &Governor{
		intent: intentStore,
		halt:   haltSentinel,
	}
}

// Evaluate computes per-bead weight offsets.
// Returns error if the factory is paused (INV-8).
// Context is reserved for future use (cancellation, deadlines).
func (g *Governor) Evaluate(_ context.Context, beads []scoring.BeadInfo) (scoring.WeightVector, error) {
	// INV-8: halt check FIRST
	if g.halt.IsHalted() {
		return scoring.WeightVector{Offsets: map[string]int{}}, fmt.Errorf("factory halted: %s exists", g.halt.Path())
	}

	// Load intent (falls back to default on missing/corrupt)
	intentFile, err := g.intent.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ockham: intent load degraded: %v (using defaults)\n", err)
	}

	iv := intentFile.ToVector()

	// Wave 1 stubs: neutral authority and anomaly
	as := authority.State{}
	an := anomaly.State{}

	wv := scoring.Score(iv, as, an, beads)
	return wv, nil
}
