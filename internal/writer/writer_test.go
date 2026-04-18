package writer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/mistakeknot/Ockham/internal/writer"
)

func newDB(t *testing.T) *signals.DB {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCompute_EmptyDB(t *testing.T) {
	db := newDB(t)
	w := writer.New(db)

	payload, err := w.Compute()
	if err != nil {
		t.Fatal(err)
	}
	if payload.Version != writer.FileVersion {
		t.Errorf("Version = %d, want %d", payload.Version, writer.FileVersion)
	}
	if len(payload.Themes) != 0 || len(payload.Agents) != 0 {
		t.Errorf("expected empty maps, got themes=%v agents=%v", payload.Themes, payload.Agents)
	}
}

func TestCompute_ConstrainedThemesGetFloor(t *testing.T) {
	db := newDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{
		Theme: "refactor", Reason: "spike",
		CreatedAt: 1, UntilAt: time.Now().Add(time.Hour).Unix(),
	})
	_ = db.UpsertConstrain(signals.ConstrainRecord{
		Theme: "expired", Reason: "old",
		CreatedAt: 1, UntilAt: 10, // already past
	})

	w := writer.New(db)
	payload, err := w.Compute()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := payload.Themes["refactor"]; !ok || got != writer.ConstrainOffset {
		t.Errorf("refactor offset = %d (ok=%v), want %d", got, ok, writer.ConstrainOffset)
	}
	if _, ok := payload.Themes["expired"]; ok {
		t.Error("expired theme should not appear")
	}
}

func TestCompute_AgentTiersMapToOffsets(t *testing.T) {
	db := newDB(t)
	_ = db.SaveRatchetState(signals.RatchetState{
		Agent: "codex", Domain: "refactor", Tier: signals.TierAutonomous, PromotedAt: 1,
	})
	_ = db.SaveRatchetState(signals.RatchetState{
		Agent: "claude", Domain: "auth", Tier: signals.TierShadow, PromotedAt: 1,
	})

	w := writer.New(db)
	payload, _ := w.Compute()

	if payload.Agents["codex/refactor"] != writer.TierOffsets[signals.TierAutonomous] {
		t.Errorf("codex/refactor = %d, want %d",
			payload.Agents["codex/refactor"], writer.TierOffsets[signals.TierAutonomous])
	}
	if payload.Agents["claude/auth"] != writer.TierOffsets[signals.TierShadow] {
		t.Errorf("claude/auth = %d, want %d",
			payload.Agents["claude/auth"], writer.TierOffsets[signals.TierShadow])
	}
}

func TestWrite_Atomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "weight-offsets.json")

	payload := writer.Weights{
		Version:    1,
		ComputedAt: 1_700_000_000,
		Themes:     map[string]int{"refactor": -6},
		Agents:     map[string]int{"codex/refactor": 2},
	}
	if err := writer.Write(path, payload); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got writer.Weights
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Themes["refactor"] != -6 || got.Agents["codex/refactor"] != 2 {
		t.Errorf("roundtrip lost data: %+v", got)
	}

	// No tempfile should remain alongside the real file.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}

func TestComputeAndWrite_EndToEnd(t *testing.T) {
	db := newDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{
		Theme: "refactor", CreatedAt: 1, UntilAt: time.Now().Add(time.Hour).Unix(),
	})
	_ = db.SaveRatchetState(signals.RatchetState{
		Agent: "codex", Domain: "refactor", Tier: signals.TierAutonomous, PromotedAt: 1,
	})

	path := filepath.Join(t.TempDir(), "weight-offsets.json")
	w := writer.New(db)

	payload, err := w.ComputeAndWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Themes["refactor"] != writer.ConstrainOffset {
		t.Errorf("refactor not constrained: %d", payload.Themes["refactor"])
	}

	// Re-read from disk and verify.
	data, _ := os.ReadFile(path)
	var fromDisk writer.Weights
	_ = json.Unmarshal(data, &fromDisk)
	if fromDisk.Agents["codex/refactor"] != writer.TierOffsets[signals.TierAutonomous] {
		t.Errorf("agent offset mismatch on disk: %+v", fromDisk.Agents)
	}
}
