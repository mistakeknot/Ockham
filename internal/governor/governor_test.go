package governor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/governor"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/mistakeknot/Ockham/internal/scoring"
)

func newTestGovernor(t *testing.T) (*governor.Governor, string) {
	t.Helper()
	dir := t.TempDir()
	intentPath := filepath.Join(dir, "intent.yaml")
	haltPath := filepath.Join(dir, "factory-paused.json")

	is := intent.NewStore(intentPath)
	hs := halt.New(haltPath)

	g := governor.New(is, hs)
	return g, dir
}

func TestEvaluate_BasicScoring(t *testing.T) {
	g, dir := newTestGovernor(t)

	is := intent.NewStore(filepath.Join(dir, "intent.yaml"))
	if err := is.Save(intent.IntentFile{
		Version: 1,
		Themes: map[string]intent.ThemeBudget{
			"auth": {Budget: 0.6, Priority: intent.PriorityHigh},
			"open": {Budget: 0.4, Priority: intent.PriorityNormal},
		},
	}); err != nil {
		t.Fatal(err)
	}

	beads := []scoring.BeadInfo{
		{ID: "b1", Lane: "auth"},
		{ID: "b2", Lane: "open"},
	}

	wv, err := g.Evaluate(context.Background(), beads)
	if err != nil {
		t.Fatal(err)
	}
	if wv.Offsets["b1"] != 6 {
		t.Errorf("auth offset = %d, want 6", wv.Offsets["b1"])
	}
	if wv.Offsets["b2"] != 0 {
		t.Errorf("open offset = %d, want 0", wv.Offsets["b2"])
	}
}

func TestEvaluate_HaltedReturnsEmpty(t *testing.T) {
	g, dir := newTestGovernor(t)

	haltPath := filepath.Join(dir, "factory-paused.json")
	if err := os.WriteFile(haltPath, []byte(`{"reason":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	beads := []scoring.BeadInfo{{ID: "b1", Lane: "auth"}}

	wv, err := g.Evaluate(context.Background(), beads)
	if err == nil {
		t.Error("expected error when halted")
	}
	if len(wv.Offsets) != 0 {
		t.Errorf("expected empty offsets when halted, got %d", len(wv.Offsets))
	}
}

func TestEvaluate_MissingIntent_UsesDefault(t *testing.T) {
	g, _ := newTestGovernor(t)

	beads := []scoring.BeadInfo{{ID: "b1", Lane: ""}}

	wv, err := g.Evaluate(context.Background(), beads)
	if err != nil {
		t.Fatal(err)
	}
	if wv.Offsets["b1"] != 0 {
		t.Errorf("default offset = %d, want 0", wv.Offsets["b1"])
	}
}
