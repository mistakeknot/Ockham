// Package trigger composes the Wave 2 signal layers (F3 confirm, F4 fast path,
// F5 interspect pairing, F6 in-flight handling, F7 de-escalation, F8 writer)
// into a single per-theme decision pipeline. Callers feed in one observation
// at a time; Pipeline decides whether to fire CONSTRAIN, release it, or leave
// state unchanged.
package trigger

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/inflight"
	"github.com/mistakeknot/Ockham/internal/interspect"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/mistakeknot/Ockham/internal/writer"
)

// Pipeline wires the Wave 2 signal layers together. All fields except db,
// constr, confirm, fastPath, release, writer are optional — pass nil for
// F5/F6 to skip those gates.
type Pipeline struct {
	db            *signals.DB
	constr        *constrain.Controller
	confirm       *constrain.Trigger
	fastPath      constrain.FastPathPolicy
	release       *constrain.ReleaseController
	interspect    *interspect.Checker
	inflight      *inflight.Controller
	writer        *writer.Writer
	weightsPath   string
	freezeFor     time.Duration
	unknownBlocks bool
	now           func() time.Time
}

// Config bundles the dependencies a Pipeline needs to run.
type Config struct {
	DB         *signals.DB
	Constrain  *constrain.Controller
	Confirm    *constrain.Trigger
	FastPath   constrain.FastPathPolicy
	Release    *constrain.ReleaseController
	Interspect *interspect.Checker // nil → skip F5 pairing (always permits fire)
	InFlight   *inflight.Controller
	Writer     *writer.Writer
	// WeightsPath is where the writer dumps the offsets file. Defaults to
	// writer.DefaultPath() when empty.
	WeightsPath string
	// FreezeFor is how long a CONSTRAIN lives by default. Zero means open-ended
	// (explicit release required).
	FreezeFor time.Duration
	// UnknownVerdictBlocks, when true, treats interspect.VerdictUnknown as a
	// block (same as VerdictHealthy). Default false preserves the permissive
	// fail-open behavior: only explicit Healthy blocks a fire.
	UnknownVerdictBlocks bool
}

// New constructs a Pipeline. The required dependencies (DB, Constrain, Confirm,
// Release, Writer) must be non-nil.
func New(cfg Config) (*Pipeline, error) {
	if cfg.DB == nil || cfg.Constrain == nil || cfg.Confirm == nil || cfg.Release == nil || cfg.Writer == nil {
		return nil, errors.New("trigger: DB, Constrain, Confirm, Release, and Writer are required")
	}
	wp := cfg.WeightsPath
	if wp == "" {
		wp = writer.DefaultPath()
	}
	return &Pipeline{
		db:            cfg.DB,
		constr:        cfg.Constrain,
		confirm:       cfg.Confirm,
		fastPath:      cfg.FastPath,
		release:       cfg.Release,
		interspect:    cfg.Interspect,
		inflight:      cfg.InFlight,
		writer:        cfg.Writer,
		weightsPath:   wp,
		freezeFor:     cfg.FreezeFor,
		unknownBlocks: cfg.UnknownVerdictBlocks,
		now:           time.Now,
	}, nil
}

// Input is a single observation for a theme. Callers compute Previous/Current
// from two consecutive evaluation windows; Tripped encodes the binary
// anomaly-detected state from the caller's drift or obs-metric logic.
type Input struct {
	Theme    string
	Signal   string // e.g. "drift", "tool_error_rate"
	Tripped  bool
	Previous float64
	Current  float64
	Reason   string
}

// Outcome captures what the pipeline did with an observation. Callers log or
// persist this for observability.
type Outcome struct {
	Theme              string
	Fired              bool               // CONSTRAIN freshly installed this call
	FastPath           bool               // fire came via F4 rate-of-change
	Released           bool               // CONSTRAIN freshly lifted this call
	AlreadyConstrained bool               // theme was already under CONSTRAIN at call time
	ConfirmStreak      int                // count before reset, when fire check ran
	StabilityStreak    int                // count before reset, when release check ran
	InFlightCount      int                // beads caught by F6, when HandleConstrain ran
	InterspectVerdict  interspect.Verdict // set when F5 pairing was consulted
}

// OnSignal runs a single observation through the full pipeline. Returns the
// Outcome so callers can log decisions. Errors from downstream writers and
// emitters are logged to stderr and do not fail the call — fail-open matches
// the rest of Ockham.
func (p *Pipeline) OnSignal(in Input) (Outcome, error) {
	out := Outcome{Theme: in.Theme}

	active, _, err := p.constr.IsConstrained(in.Theme)
	if err != nil {
		return out, fmt.Errorf("trigger: IsConstrained: %w", err)
	}
	out.AlreadyConstrained = active

	if in.Tripped {
		return p.handleTripped(in, active, out)
	}
	return p.handleClean(in, active, out)
}

func (p *Pipeline) handleTripped(in Input, active bool, out Outcome) (Outcome, error) {
	// F7 invariant: any anomaly during an active CONSTRAIN zeros the stability
	// streak. Safe to call when not constrained — it's a no-op reset.
	if err := p.release.ObserveAnomalyDuringConstrain(in.Theme); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: stability reset degraded for %q: %v\n", in.Theme, err)
	}

	// F4: rate-of-change fast path.
	decision := p.fastPath.Evaluate(in.Theme, in.Signal, in.Previous, in.Current)
	fastPathFired := decision.Fired

	// F3: multi-window confirmation (unless fast path bypassed it).
	var confirmed bool
	if !fastPathFired {
		fired, count, err := p.confirm.Observe(in.Theme, in.Signal, true)
		if err != nil {
			return out, fmt.Errorf("trigger: confirm observe: %w", err)
		}
		out.ConfirmStreak = count
		confirmed = fired
	} else {
		// Fast-path bypass — reset any pending confirm streak.
		_ = p.db.ResetStreak(in.Theme, signals.StreakConfirm, in.Signal)
	}

	if !confirmed && !fastPathFired {
		return out, nil
	}

	// Already frozen — nothing to install again, but refresh the record so
	// until_at extends and the reason is current.
	if active {
		if err := p.fire(in, fastPathFired); err != nil {
			return out, err
		}
		out.FastPath = fastPathFired
		return out, nil
	}

	// F5: paired confirmation with interspect (only when installed).
	if p.interspect != nil {
		verdict, err := p.interspect.AgreesUnhealthy(in.Theme)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ockham: interspect pairing degraded for %q: %v\n", in.Theme, err)
			verdict = interspect.VerdictUnknown
		}
		out.InterspectVerdict = verdict
		if verdict == interspect.VerdictHealthy ||
			(p.unknownBlocks && verdict == interspect.VerdictUnknown) {
			// Pairing blocks the fire — interspect disagrees, or Unknown is
			// configured to block (strict mode).
			return out, nil
		}
	}

	// Install the freeze.
	if err := p.fire(in, fastPathFired); err != nil {
		return out, err
	}
	out.Fired = true
	out.FastPath = fastPathFired

	// F6: emit events for in-flight beads on the constrained theme.
	if p.inflight != nil {
		handled, err := p.inflight.HandleConstrain(in.Theme, in.Reason)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ockham: inflight handling degraded for %q: %v\n", in.Theme, err)
		}
		out.InFlightCount = len(handled)
	}

	// F8: refresh the weight-offsets file so dispatch picks up the penalty.
	if _, err := p.writer.ComputeAndWrite(p.weightsPath); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: weight file write degraded after fire on %q: %v\n", in.Theme, err)
	}

	return out, nil
}

func (p *Pipeline) handleClean(in Input, active bool, out Outcome) (Outcome, error) {
	// F3: clean window resets the confirm streak.
	if _, _, err := p.confirm.Observe(in.Theme, in.Signal, false); err != nil {
		return out, fmt.Errorf("trigger: confirm observe clean: %w", err)
	}

	// No CONSTRAIN to relax — nothing else to do.
	if !active {
		return out, nil
	}

	// F7: stability streak. Release when it reaches policy threshold.
	shouldRelease, count, err := p.release.ObserveClean(in.Theme)
	if err != nil {
		return out, fmt.Errorf("trigger: stability observe: %w", err)
	}
	out.StabilityStreak = count
	if !shouldRelease {
		return out, nil
	}

	if err := p.constr.ReleaseTheme(in.Theme); err != nil {
		return out, fmt.Errorf("trigger: release theme: %w", err)
	}
	if err := p.db.DeleteStreaksForTheme(in.Theme); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: streak cleanup degraded for %q: %v\n", in.Theme, err)
	}
	out.Released = true

	if _, err := p.writer.ComputeAndWrite(p.weightsPath); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: weight file write degraded after release on %q: %v\n", in.Theme, err)
	}

	return out, nil
}

func (p *Pipeline) fire(in Input, fastPath bool) error {
	var until time.Time
	if p.freezeFor > 0 {
		until = p.now().Add(p.freezeFor)
	}
	return p.constr.ConstrainTheme(in.Theme, in.Reason, until, fastPath)
}
