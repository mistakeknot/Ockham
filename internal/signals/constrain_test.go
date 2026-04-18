package signals_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestUpsertAndGetConstrain(t *testing.T) {
	db := newTestDB(t)

	rec := signals.ConstrainRecord{
		Theme:     "refactor",
		Reason:    "error_rate spike",
		FastPath:  true,
		CreatedAt: 1_700_000_000,
		UntilAt:   1_700_003_600,
	}
	if err := db.UpsertConstrain(rec); err != nil {
		t.Fatalf("UpsertConstrain: %v", err)
	}

	got, found, err := db.GetConstrain("refactor")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected row to be present")
	}
	if got != rec {
		t.Errorf("got %+v, want %+v", got, rec)
	}
}

func TestGetConstrain_Missing(t *testing.T) {
	db := newTestDB(t)
	_, found, err := db.GetConstrain("nope")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found for fresh DB")
	}
}

func TestUpsertConstrain_Overwrite(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "x", Reason: "a", CreatedAt: 1, UntilAt: 10})
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "x", Reason: "b", CreatedAt: 2, UntilAt: 20})

	got, _, _ := db.GetConstrain("x")
	if got.Reason != "b" || got.UntilAt != 20 {
		t.Errorf("upsert did not overwrite: %+v", got)
	}
}

func TestDeleteConstrain(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "x", CreatedAt: 1})

	if err := db.DeleteConstrain("x"); err != nil {
		t.Fatal(err)
	}
	_, found, _ := db.GetConstrain("x")
	if found {
		t.Error("expected delete to remove row")
	}

	// Deleting a missing row is a no-op, not an error.
	if err := db.DeleteConstrain("y"); err != nil {
		t.Errorf("delete missing: %v", err)
	}
}

func TestListConstrainsActive_FiltersExpired(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "open", CreatedAt: 1, UntilAt: 0})
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "future", CreatedAt: 1, UntilAt: 100})
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "past", CreatedAt: 1, UntilAt: 50})

	got, err := db.ListConstrainsActive(75)
	if err != nil {
		t.Fatal(err)
	}
	gotThemes := map[string]bool{}
	for _, r := range got {
		gotThemes[r.Theme] = true
	}
	if !gotThemes["open"] || !gotThemes["future"] || gotThemes["past"] {
		t.Errorf("unexpected active set: %v", gotThemes)
	}
}

func TestPurgeExpiredConstrains(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "open", CreatedAt: 1, UntilAt: 0})
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "future", CreatedAt: 1, UntilAt: 100})
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "past", CreatedAt: 1, UntilAt: 50})

	n, err := db.PurgeExpiredConstrains(75)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1", n)
	}
	if _, found, _ := db.GetConstrain("past"); found {
		t.Error("expected past row to be removed")
	}
	if _, found, _ := db.GetConstrain("open"); !found {
		t.Error("expected open-ended row to remain")
	}
}
