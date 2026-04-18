package signals_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestGetRatchetState_DefaultShadow(t *testing.T) {
	db := newTestDB(t)

	r, found, err := db.GetRatchetState("codex", "refactor")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("expected no row for fresh DB, got %+v", r)
	}
	if r.Tier != signals.TierShadow {
		t.Errorf("default tier = %q, want %q", r.Tier, signals.TierShadow)
	}
}

func TestSaveAndGetRatchetState(t *testing.T) {
	db := newTestDB(t)

	want := signals.RatchetState{
		Agent:      "codex",
		Domain:     "refactor",
		Tier:       signals.TierSupervised,
		PromotedAt: 1_700_000_000,
	}
	if err := db.SaveRatchetState(want); err != nil {
		t.Fatalf("SaveRatchetState: %v", err)
	}

	got, found, err := db.GetRatchetState("codex", "refactor")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected row to be found")
	}
	if got.Tier != want.Tier || got.PromotedAt != want.PromotedAt {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaveRatchetState_RejectsInvalidTier(t *testing.T) {
	db := newTestDB(t)
	err := db.SaveRatchetState(signals.RatchetState{
		Agent:  "codex",
		Domain: "refactor",
		Tier:   "bogus",
	})
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestSaveRatchetState_Upsert(t *testing.T) {
	db := newTestDB(t)

	first := signals.RatchetState{Agent: "codex", Domain: "refactor", Tier: signals.TierShadow, PromotedAt: 100}
	if err := db.SaveRatchetState(first); err != nil {
		t.Fatal(err)
	}
	second := signals.RatchetState{Agent: "codex", Domain: "refactor", Tier: signals.TierSupervised, PromotedAt: 200}
	if err := db.SaveRatchetState(second); err != nil {
		t.Fatal(err)
	}

	got, _, err := db.GetRatchetState("codex", "refactor")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tier != signals.TierSupervised || got.PromotedAt != 200 {
		t.Errorf("upsert failed: got %+v", got)
	}
}

func TestListRatchetStates(t *testing.T) {
	db := newTestDB(t)
	rows := []signals.RatchetState{
		{Agent: "a", Domain: "d1", Tier: signals.TierShadow, PromotedAt: 1},
		{Agent: "a", Domain: "d2", Tier: signals.TierSupervised, PromotedAt: 2},
		{Agent: "b", Domain: "d1", Tier: signals.TierAutonomous, PromotedAt: 3},
	}
	for _, r := range rows {
		if err := db.SaveRatchetState(r); err != nil {
			t.Fatalf("save %+v: %v", r, err)
		}
	}

	got, err := db.ListRatchetStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(rows) {
		t.Errorf("len = %d, want %d", len(got), len(rows))
	}
}
