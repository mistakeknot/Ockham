package constrain_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/signals"
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

func TestConstrainTheme_Persists(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)

	until := time.Now().Add(1 * time.Hour)
	if err := c.ConstrainTheme("refactor", "error_rate spike", until, false); err != nil {
		t.Fatalf("ConstrainTheme: %v", err)
	}

	active, rec, err := c.IsConstrained("refactor")
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("expected theme to be constrained")
	}
	if rec.Reason != "error_rate spike" {
		t.Errorf("Reason = %q", rec.Reason)
	}
	if rec.UntilAt != until.Unix() {
		t.Errorf("UntilAt = %d, want %d", rec.UntilAt, until.Unix())
	}
	if rec.FastPath {
		t.Error("FastPath should default to false")
	}
}

func TestConstrainTheme_OpenEnded(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)

	if err := c.ConstrainTheme("refactor", "manual", time.Time{}, false); err != nil {
		t.Fatal(err)
	}
	active, rec, _ := c.IsConstrained("refactor")
	if !active {
		t.Fatal("expected active")
	}
	if rec.UntilAt != 0 {
		t.Errorf("UntilAt = %d, want 0 (open-ended)", rec.UntilAt)
	}
}

func TestConstrainTheme_RejectsPastUntil(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)

	past := time.Now().Add(-1 * time.Minute)
	if err := c.ConstrainTheme("refactor", "oops", past, false); err == nil {
		t.Fatal("expected error for past until")
	}
}

func TestConstrainTheme_RejectsEmptyTheme(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	if err := c.ConstrainTheme("", "reason", time.Time{}, false); err == nil {
		t.Fatal("expected error for empty theme")
	}
}

func TestConstrainTheme_FastPathRecorded(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	if err := c.ConstrainTheme("refactor", "delta", time.Now().Add(time.Hour), true); err != nil {
		t.Fatal(err)
	}
	_, rec, _ := c.IsConstrained("refactor")
	if !rec.FastPath {
		t.Error("FastPath not persisted")
	}
}

func TestIsConstrained_DefaultFalse(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	active, _, err := c.IsConstrained("refactor")
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Error("expected fresh theme to be unconstrained")
	}
}

func TestIsConstrained_ExpiredReadsAsInactive(t *testing.T) {
	db := newDB(t)
	// Write directly so we can fabricate an expired row.
	_ = db.UpsertConstrain(signals.ConstrainRecord{
		Theme:     "stale",
		Reason:    "expired",
		CreatedAt: 1,
		UntilAt:   2,
	})
	c := constrain.New(db)
	active, _, err := c.IsConstrained("stale")
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Error("expected expired row to read as inactive")
	}
}

func TestReleaseTheme_RemovesRow(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	_ = c.ConstrainTheme("refactor", "r", time.Now().Add(time.Hour), false)

	if err := c.ReleaseTheme("refactor"); err != nil {
		t.Fatalf("ReleaseTheme: %v", err)
	}
	active, _, _ := c.IsConstrained("refactor")
	if active {
		t.Error("expected theme to be released")
	}
}

func TestReleaseTheme_MissingReturnsError(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	err := c.ReleaseTheme("nope")
	if !errors.Is(err, constrain.ErrNotConstrained) {
		t.Fatalf("err = %v, want ErrNotConstrained", err)
	}
}

func TestListActive(t *testing.T) {
	db := newDB(t)
	c := constrain.New(db)
	_ = c.ConstrainTheme("a", "", time.Time{}, false)
	_ = c.ConstrainTheme("b", "", time.Now().Add(time.Hour), true)

	got, err := c.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestPurgeExpired(t *testing.T) {
	db := newDB(t)
	// Fabricate expired + future rows directly.
	_ = db.UpsertConstrain(signals.ConstrainRecord{Theme: "gone", CreatedAt: 1, UntilAt: 2})
	_ = db.UpsertConstrain(signals.ConstrainRecord{
		Theme: "keep", CreatedAt: 1, UntilAt: time.Now().Add(time.Hour).Unix(),
	})
	c := constrain.New(db)

	n, err := c.PurgeExpired()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d, want 1", n)
	}
}
