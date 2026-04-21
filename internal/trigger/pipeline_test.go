package trigger_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/inflight"
	"github.com/mistakeknot/Ockham/internal/interspect"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/mistakeknot/Ockham/internal/trigger"
	"github.com/mistakeknot/Ockham/internal/writer"
)

type harness struct {
	dir         string
	db          *signals.DB
	pipeline    *trigger.Pipeline
	weightsPath string
}

// newHarness builds a Pipeline with short streak thresholds so tests run fast:
// F3 RequiredWindows=2, F7 RequiredWindows=2. Interspect is not wired by
// default — individual tests opt in with withInterspect().
func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	db, err := signals.NewDB(filepath.Join(dir, "signals.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	confirm := constrain.NewTrigger(db, constrain.ConfirmPolicy{RequiredWindows: 2})
	release := constrain.NewReleaseController(db, constrain.StabilityPolicy{RequiredWindows: 2})
	fp := constrain.FastPathPolicy{
		Thresholds: map[string]float64{"drift": 0.30},
	}

	weightsPath := filepath.Join(dir, "weights.json")
	pl, err := trigger.New(trigger.Config{
		DB:          db,
		Constrain:   constrain.New(db),
		Confirm:     confirm,
		FastPath:    fp,
		Release:     release,
		Writer:      writer.New(db),
		WeightsPath: weightsPath,
		FreezeFor:   0, // open-ended
	})
	if err != nil {
		t.Fatalf("trigger.New: %v", err)
	}

	return &harness{dir: dir, db: db, pipeline: pl, weightsPath: weightsPath}
}

// writeInterspectEvidence writes a delegation-calibration.json into dir and
// returns a *interspect.Checker rooted at that file with a 5s staleness window.
func writeInterspectEvidence(t *testing.T, dir string, payload string) *interspect.Checker {
	t.Helper()
	path := filepath.Join(dir, "interspect.json")
	if err := os.WriteFile(path, []byte(payload), 0644); err != nil {
		t.Fatalf("write interspect: %v", err)
	}
	return interspect.NewChecker(interspect.NewReader(path), interspect.Policy{
		MaxStalenessSeconds: 3600,
		MinPassRate:         0.5,
		MinCategoryCount:    3,
	})
}

func TestPipeline_MultiWindowConfirmation(t *testing.T) {
	h := newHarness(t)

	// First tripped observation: streak=1, not yet fired.
	out, err := h.pipeline.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0.10, Current: 0.15, // delta 0.05 < 0.30 threshold
		Reason: "drift spike",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Fired || out.Released {
		t.Fatalf("first observation should not fire: %+v", out)
	}
	if out.ConfirmStreak != 1 {
		t.Errorf("ConfirmStreak = %d, want 1", out.ConfirmStreak)
	}

	// Second tripped observation: streak=2, fires.
	out, err = h.pipeline.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0.15, Current: 0.20,
		Reason: "drift spike",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Fired {
		t.Errorf("second observation should fire (streak=2 met threshold): %+v", out)
	}
	if out.FastPath {
		t.Error("expected slow path, got FastPath=true")
	}

	active, rec, _ := constrain.New(h.db).IsConstrained("refactor")
	if !active {
		t.Fatal("expected theme to be constrained after fire")
	}
	if rec.FastPath {
		t.Error("record should not be marked fast_path")
	}
}

func TestPipeline_FastPathSkipsConfirmation(t *testing.T) {
	h := newHarness(t)

	// Single observation with large delta → F4 fires immediately.
	out, err := h.pipeline.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0.02, Current: 0.40, // delta 0.38 > 0.30
		Reason: "drift jump",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Fired {
		t.Fatal("expected fast-path fire")
	}
	if !out.FastPath {
		t.Error("expected FastPath=true")
	}

	_, rec, _ := constrain.New(h.db).IsConstrained("refactor")
	if !rec.FastPath {
		t.Error("record should be marked fast_path")
	}
}

func TestPipeline_CleanWindowResetsConfirm(t *testing.T) {
	h := newHarness(t)

	// Trip once, then clean, then trip again — streak should not cross threshold.
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true, Reason: "r",
	})
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: false,
	})
	out, err := h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true, Reason: "r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Fired {
		t.Errorf("clean window should have reset streak: %+v", out)
	}
}

func TestPipeline_DeEscalationReleases(t *testing.T) {
	h := newHarness(t)

	// Fire via fast path.
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true,
		Previous: 0.02, Current: 0.50, Reason: "r",
	})

	// Two clean windows → release (StabilityPolicy.RequiredWindows=2).
	out1, _ := h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})
	if out1.Released {
		t.Errorf("should not release after 1 clean window")
	}
	if out1.StabilityStreak != 1 {
		t.Errorf("StabilityStreak = %d, want 1", out1.StabilityStreak)
	}

	out2, _ := h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})
	if !out2.Released {
		t.Fatalf("should release on 2nd clean window: %+v", out2)
	}

	active, _, _ := constrain.New(h.db).IsConstrained("x")
	if active {
		t.Error("expected theme to be released")
	}
}

func TestPipeline_AnomalyResetsStability(t *testing.T) {
	h := newHarness(t)

	// Fire, then one clean window, then an anomaly, then one clean window.
	// Stability should NOT release — the anomaly reset the streak.
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true, Previous: 0, Current: 0.50, Reason: "r",
	})
	_, _ = h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})

	// Anomaly during constrain — should reset stability streak.
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true, Previous: 0, Current: 0.10, Reason: "r",
	})

	// One more clean window — should not release yet.
	out, _ := h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})
	if out.Released {
		t.Errorf("should not release — anomaly reset streak: %+v", out)
	}
}

func TestPipeline_InterspectHealthyBlocksFire(t *testing.T) {
	h := newHarness(t)

	// Write evidence that says the theme is healthy.
	recentTS := time.Now().UTC().Format(time.RFC3339)
	evidence := `{
		"schema_version": 1,
		"generated_at": "` + recentTS + `",
		"overall_pass_rate": 0.95,
		"total_delegations": 100,
		"retry_rate": 0.05,
		"categories": {"refactor": {"count": 20, "pass_rate": 0.95, "avg_duration_s": 10}},
		"high_retry_categories": []
	}`
	checker := writeInterspectEvidence(t, h.dir, evidence)

	// Rebuild pipeline with interspect wired.
	pl, err := trigger.New(trigger.Config{
		DB:          h.db,
		Constrain:   constrain.New(h.db),
		Confirm:     constrain.NewTrigger(h.db, constrain.ConfirmPolicy{RequiredWindows: 2}),
		FastPath:    constrain.FastPathPolicy{Thresholds: map[string]float64{"drift": 0.30}},
		Release:     constrain.NewReleaseController(h.db, constrain.StabilityPolicy{RequiredWindows: 2}),
		Interspect:  checker,
		Writer:      writer.New(h.db),
		WeightsPath: h.weightsPath,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fast path fire attempt — interspect should veto.
	out, err := pl.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0.02, Current: 0.50, Reason: "r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Fired {
		t.Errorf("interspect Healthy should block fire: %+v", out)
	}
	if out.InterspectVerdict != interspect.VerdictHealthy {
		t.Errorf("verdict = %v, want Healthy", out.InterspectVerdict)
	}
}

func TestPipeline_InterspectUnknownAllowsFire(t *testing.T) {
	h := newHarness(t)

	// Stale evidence → Unknown verdict → fail-open (fire permitted).
	staleTS := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	evidence := `{
		"schema_version": 1,
		"generated_at": "` + staleTS + `",
		"overall_pass_rate": 0.95,
		"total_delegations": 10,
		"retry_rate": 0,
		"categories": {},
		"high_retry_categories": []
	}`
	checker := writeInterspectEvidence(t, h.dir, evidence)

	pl, _ := trigger.New(trigger.Config{
		DB:          h.db,
		Constrain:   constrain.New(h.db),
		Confirm:     constrain.NewTrigger(h.db, constrain.ConfirmPolicy{RequiredWindows: 2}),
		FastPath:    constrain.FastPathPolicy{Thresholds: map[string]float64{"drift": 0.30}},
		Release:     constrain.NewReleaseController(h.db, constrain.StabilityPolicy{RequiredWindows: 2}),
		Interspect:  checker,
		Writer:      writer.New(h.db),
		WeightsPath: h.weightsPath,
	})

	out, _ := pl.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0, Current: 0.50, Reason: "r",
	})
	if !out.Fired {
		t.Errorf("Unknown verdict should fail-open and allow fire: %+v", out)
	}
	if out.InterspectVerdict != interspect.VerdictUnknown {
		t.Errorf("verdict = %v, want Unknown", out.InterspectVerdict)
	}
}

func TestPipeline_WriterSyncsOnFireAndRelease(t *testing.T) {
	h := newHarness(t)

	// Pre-fire: weights file shouldn't exist.
	if _, err := os.Stat(h.weightsPath); !os.IsNotExist(err) {
		t.Fatalf("expected weights file absent before fire, got err=%v", err)
	}

	// Fast-path fire.
	_, _ = h.pipeline.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true,
		Previous: 0, Current: 0.50, Reason: "r",
	})

	// File should exist now with x: -6.
	data, err := os.ReadFile(h.weightsPath)
	if err != nil {
		t.Fatalf("weights file missing after fire: %v", err)
	}
	if !contains(data, `"x": -6`) {
		t.Errorf("expected x theme at -6 in %s, got: %s", h.weightsPath, string(data))
	}

	// Two clean windows → release → file should update.
	_, _ = h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})
	_, _ = h.pipeline.OnSignal(trigger.Input{Theme: "x", Signal: "drift", Tripped: false})

	data, err = os.ReadFile(h.weightsPath)
	if err != nil {
		t.Fatal(err)
	}
	if contains(data, `"x": -6`) {
		t.Errorf("expected x to be removed from themes after release, got: %s", string(data))
	}
}

func TestPipeline_InFlightEventsOnFire(t *testing.T) {
	h := newHarness(t)

	// Inject a fake in-flight lister that always returns 2 beads on the target theme.
	fakeLister := &fakeLister{
		byTheme: map[string][]inflight.InFlightBead{
			"x": {
				{BeadID: "b1", Theme: "x", Assignee: "agent-a"},
				{BeadID: "b2", Theme: "x", Assignee: "agent-b"},
			},
		},
	}
	events := []inflight.Event{}
	inflightCtl := inflight.New(inflight.PolicyFinishThenBlock,
		inflight.WithLister(fakeLister),
		inflight.WithEmitter(func(e inflight.Event) { events = append(events, e) }),
	)

	pl, _ := trigger.New(trigger.Config{
		DB:          h.db,
		Constrain:   constrain.New(h.db),
		Confirm:     constrain.NewTrigger(h.db, constrain.ConfirmPolicy{RequiredWindows: 2}),
		FastPath:    constrain.FastPathPolicy{Thresholds: map[string]float64{"drift": 0.30}},
		Release:     constrain.NewReleaseController(h.db, constrain.StabilityPolicy{RequiredWindows: 2}),
		InFlight:    inflightCtl,
		Writer:      writer.New(h.db),
		WeightsPath: h.weightsPath,
	})

	out, _ := pl.OnSignal(trigger.Input{
		Theme: "x", Signal: "drift", Tripped: true,
		Previous: 0, Current: 0.50, Reason: "r",
	})
	if !out.Fired {
		t.Fatal("expected fire")
	}
	if out.InFlightCount != 2 {
		t.Errorf("InFlightCount = %d, want 2", out.InFlightCount)
	}
	if len(events) != 2 {
		t.Errorf("events emitted = %d, want 2", len(events))
	}
}

// --- test helpers ----------------------------------------------------------

type fakeLister struct {
	byTheme map[string][]inflight.InFlightBead
}

func (f *fakeLister) ListByTheme(theme string) ([]inflight.InFlightBead, error) {
	return f.byTheme[theme], nil
}

func contains(haystack []byte, needle string) bool {
	return indexBytes(haystack, needle) >= 0
}

func indexBytes(haystack []byte, needle string) int {
	n, h := len(needle), len(haystack)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= h; i++ {
		if string(haystack[i:i+n]) == needle {
			return i
		}
	}
	return -1
}

// unknownVerdictChecker returns a checker whose evidence file does not exist,
// so AgreesUnhealthy always returns VerdictUnknown.
func unknownVerdictChecker(t *testing.T, dir string) *interspect.Checker {
	t.Helper()
	missing := filepath.Join(dir, "does-not-exist.json")
	return interspect.NewChecker(interspect.NewReader(missing), interspect.Policy{
		MaxStalenessSeconds: 3600,
		MinPassRate:         0.5,
		MinCategoryCount:    3,
	})
}

// TestPipeline_UnknownVerdictBlocks_WhenConfigured verifies the strict-mode
// knob: with UnknownVerdictBlocks=true, an Unknown verdict prevents fire.
func TestPipeline_UnknownVerdictBlocks_WhenConfigured(t *testing.T) {
	h := newHarness(t)
	checker := unknownVerdictChecker(t, h.dir)

	pl, err := trigger.New(trigger.Config{
		DB:                   h.db,
		Constrain:            constrain.New(h.db),
		Confirm:              constrain.NewTrigger(h.db, constrain.ConfirmPolicy{RequiredWindows: 2}),
		FastPath:             constrain.FastPathPolicy{Thresholds: map[string]float64{"drift": 0.30}},
		Release:              constrain.NewReleaseController(h.db, constrain.StabilityPolicy{RequiredWindows: 2}),
		Interspect:           checker,
		Writer:               writer.New(h.db),
		WeightsPath:          h.weightsPath,
		UnknownVerdictBlocks: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Fast-path-sized jump would normally fire; Unknown + strict mode blocks.
	out, err := pl.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0, Current: 0.50, Reason: "r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Fired {
		t.Errorf("strict Unknown should block fire, got Fired=true: %+v", out)
	}
	if out.InterspectVerdict != interspect.VerdictUnknown {
		t.Errorf("verdict = %v, want Unknown", out.InterspectVerdict)
	}
}

// TestPipeline_UnknownVerdictAllowsFire_Default verifies the permissive
// default: UnknownVerdictBlocks=false → Unknown fails open, allowing fire.
func TestPipeline_UnknownVerdictAllowsFire_Default(t *testing.T) {
	h := newHarness(t)
	checker := unknownVerdictChecker(t, h.dir)

	pl, err := trigger.New(trigger.Config{
		DB:          h.db,
		Constrain:   constrain.New(h.db),
		Confirm:     constrain.NewTrigger(h.db, constrain.ConfirmPolicy{RequiredWindows: 2}),
		FastPath:    constrain.FastPathPolicy{Thresholds: map[string]float64{"drift": 0.30}},
		Release:     constrain.NewReleaseController(h.db, constrain.StabilityPolicy{RequiredWindows: 2}),
		Interspect:  checker,
		Writer:      writer.New(h.db),
		WeightsPath: h.weightsPath,
		// UnknownVerdictBlocks intentionally omitted — default false.
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := pl.OnSignal(trigger.Input{
		Theme: "refactor", Signal: "drift",
		Tripped: true, Previous: 0, Current: 0.50, Reason: "r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Fired {
		t.Errorf("permissive Unknown should allow fire, got Fired=false: %+v", out)
	}
	if out.InterspectVerdict != interspect.VerdictUnknown {
		t.Errorf("verdict = %v, want Unknown", out.InterspectVerdict)
	}
}
