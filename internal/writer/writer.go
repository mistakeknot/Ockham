// Package writer emits the Ockham weight-offset file consumed by Clavain's
// lib-dispatch.sh. It closes the governance flywheel: authority state (F1)
// and CONSTRAIN state (F2) are translated to integer offsets that bias
// dispatch scoring.
//
// File format v1 (JSON):
//
//	{
//	  "version": 1,
//	  "computed_at": <unix seconds>,
//	  "themes":  { "<theme>":  <offset int> },
//	  "agents":  { "<agent>/<domain>": <offset int> }
//	}
//
// Writes are atomic (tempfile + rename). Consumers that miss the file default
// to offset 0 — fail-open so dispatch keeps running when Ockham is silent.
package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// FileVersion is the current schema version of the weight-offsets file.
const FileVersion = 1

// Offset bounds match scoring.OffsetMin / OffsetMax. Duplicated here to avoid
// a circular import with internal/scoring.
const (
	offsetMin = -6
	offsetMax = 6
)

// TierOffsets maps authority tiers to weight deltas. Shadow is penalized to
// keep unproven agents on a shorter leash; autonomous earns a small uplift.
var TierOffsets = map[signals.Tier]int{
	signals.TierShadow:     -3,
	signals.TierSupervised: 0,
	signals.TierAutonomous: +2,
}

// ConstrainOffset is the weight applied to a theme under active CONSTRAIN.
// Floored hard — a constrained theme should lose every score tie.
const ConstrainOffset = -6

// Weights is the marshalled file payload.
type Weights struct {
	Version    int            `json:"version"`
	ComputedAt int64          `json:"computed_at"`
	Themes     map[string]int `json:"themes"`
	Agents     map[string]int `json:"agents"`
}

// DefaultPath returns ~/.config/ockham/weight-offsets.json.
func DefaultPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "weight-offsets.json")
}

// Writer composes DB reads into a Weights payload and persists it atomically.
type Writer struct {
	db  *signals.DB
	now func() time.Time
}

// New constructs a Writer against the given DB.
func New(db *signals.DB) *Writer {
	return &Writer{db: db, now: time.Now}
}

// Compute reads current authority + constrain state from the DB and returns
// the Weights payload without writing it.
func (w *Writer) Compute() (Weights, error) {
	now := w.now()

	active, err := w.db.ListConstrainsActive(now.Unix())
	if err != nil {
		return Weights{}, fmt.Errorf("writer: list constrains: %w", err)
	}
	themes := make(map[string]int, len(active))
	for _, r := range active {
		themes[r.Theme] = ConstrainOffset
	}

	ratchets, err := w.db.ListRatchetStates()
	if err != nil {
		return Weights{}, fmt.Errorf("writer: list ratchets: %w", err)
	}
	agents := make(map[string]int, len(ratchets))
	for _, r := range ratchets {
		off, ok := TierOffsets[r.Tier]
		if !ok {
			continue
		}
		agents[r.Agent+"/"+r.Domain] = clamp(off)
	}

	return Weights{
		Version:    FileVersion,
		ComputedAt: now.Unix(),
		Themes:     themes,
		Agents:     agents,
	}, nil
}

// Write persists w to path atomically (tempfile + rename). The file is
// written 0644 so the dispatcher user can read it.
func Write(path string, w Weights) error {
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("writer: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("writer: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("writer: tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// Only cleans up if rename didn't happen.
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writer: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writer: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writer: close: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("writer: chmod: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("writer: rename: %w", err)
	}
	return nil
}

// ComputeAndWrite is the one-shot entry point invoked by the CLI and tests.
func (w *Writer) ComputeAndWrite(path string) (Weights, error) {
	payload, err := w.Compute()
	if err != nil {
		return Weights{}, err
	}
	if err := Write(path, payload); err != nil {
		return Weights{}, err
	}
	return payload, nil
}

func clamp(v int) int {
	if v < offsetMin {
		return offsetMin
	}
	if v > offsetMax {
		return offsetMax
	}
	return v
}
