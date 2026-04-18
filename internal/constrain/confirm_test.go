package constrain_test

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/signals"
)

func newTestDB(t *testing.T) *signals.DB {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTrigger_NoFireOnSingleWindow(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 3}
	trigger := constrain.NewTrigger(db, policy)

	fired, count, err := trigger.Observe("theme1", "error_rate", true)
	if err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Error("expected no fire on single tripped window")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestTrigger_NoFireOnTwoWindows(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 3}
	trigger := constrain.NewTrigger(db, policy)

	for i := 1; i <= 2; i++ {
		fired, count, err := trigger.Observe("theme1", "error_rate", true)
		if err != nil {
			t.Fatal(err)
		}
		if fired {
			t.Errorf("expected no fire at window %d", i)
		}
		if count != i {
			t.Errorf("window %d: count = %d, want %d", i, count, i)
		}
	}
}

func TestTrigger_FireOnThreeWindows(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 3}
	trigger := constrain.NewTrigger(db, policy)

	for i := 1; i <= 3; i++ {
		fired, count, err := trigger.Observe("theme1", "error_rate", true)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if fired {
				t.Errorf("expected no fire at window %d", i)
			}
		} else {
			if !fired {
				t.Error("expected fire at window 3")
			}
			if count != 3 {
				t.Errorf("count = %d, want 3", count)
			}
		}
	}

	// After fire, streak should be reset.
	rec, found, err := db.GetStreak("theme1", signals.StreakConfirm, "error_rate")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected streak to be reset after fire, got %d", rec.Count)
	}
}

func TestTrigger_CleanResets(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 3}
	trigger := constrain.NewTrigger(db, policy)

	// Two tripped windows
	_, _, _ = trigger.Observe("theme1", "error_rate", true)
	_, _, _ = trigger.Observe("theme1", "error_rate", true)

	// Clean window resets
	fired, count, err := trigger.Observe("theme1", "error_rate", false)
	if err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Error("expected no fire on clean window")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 after clean", count)
	}

	// Streak should be reset
	rec, found, err := db.GetStreak("theme1", signals.StreakConfirm, "error_rate")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected streak to be reset, got %d", rec.Count)
	}

	// Next tripped window starts fresh
	_, count, _ = trigger.Observe("theme1", "error_rate", true)
	if count != 1 {
		t.Errorf("count after reset = %d, want 1", count)
	}
}

func TestTrigger_TrippedCleanTripped(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 3}
	trigger := constrain.NewTrigger(db, policy)

	_, c1, _ := trigger.Observe("theme1", "error_rate", true)
	_, c2, _ := trigger.Observe("theme1", "error_rate", true)
	_, c3, _ := trigger.Observe("theme1", "error_rate", false) // Reset
	_, c4, _ := trigger.Observe("theme1", "error_rate", true)

	if c1 != 1 || c2 != 2 || c3 != 0 || c4 != 1 {
		t.Errorf("counts: %d, %d, %d, %d; want 1, 2, 0, 1", c1, c2, c3, c4)
	}
}

func TestTrigger_DifferentSignals(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 2}
	trigger := constrain.NewTrigger(db, policy)

	_, c1, _ := trigger.Observe("theme1", "error_rate", true)
	_, c2, _ := trigger.Observe("theme1", "latency", true)

	if c1 != 1 || c2 != 1 {
		t.Errorf("counts for different signals: error_rate=%d, latency=%d; want 1, 1", c1, c2)
	}

	// Incrementing one signal doesn't affect the other
	fired1, c3, _ := trigger.Observe("theme1", "error_rate", true)
	if !fired1 || c3 != 2 {
		t.Errorf("error_rate second increment: fired=%v, count=%d; want true, 2", fired1, c3)
	}

	fired2, c4, _ := trigger.Observe("theme1", "latency", true)
	if !fired2 || c4 != 2 {
		t.Errorf("latency fire: fired=%v, count=%d; want true, 2", fired2, c4)
	}
}

func TestTrigger_DifferentThemes(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 2}
	trigger := constrain.NewTrigger(db, policy)

	_, c1, _ := trigger.Observe("theme1", "default", true)
	_, c2, _ := trigger.Observe("theme2", "default", true)

	if c1 != 1 || c2 != 1 {
		t.Errorf("counts for different themes: theme1=%d, theme2=%d; want 1, 1", c1, c2)
	}

	fired1, c3, _ := trigger.Observe("theme1", "default", true)
	fired2, c4, _ := trigger.Observe("theme2", "default", true)

	if !fired1 || !fired2 {
		t.Errorf("expected both to fire: theme1=%v, theme2=%v", fired1, fired2)
	}
	if c3 != 2 || c4 != 2 {
		t.Errorf("counts: theme1=%d, theme2=%d; want 2, 2", c3, c4)
	}
}

func TestTrigger_CustomPolicy(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 5}
	trigger := constrain.NewTrigger(db, policy)

	var fired bool
	for i := 1; i < 5; i++ {
		fired, _, _ = trigger.Observe("theme1", "default", true)
		if fired {
			t.Errorf("expected no fire at window %d", i)
		}
	}

	fired, _, _ = trigger.Observe("theme1", "default", true)
	if !fired {
		t.Error("expected fire at window 5")
	}
}

func TestTrigger_StreakRecordTracking(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.ConfirmPolicy{RequiredWindows: 2}
	trigger := constrain.NewTrigger(db, policy)

	// Verify streak records are created with proper timestamps
	_, _, _ = trigger.Observe("theme1", "default", true)
	rec1, found, _ := db.GetStreak("theme1", signals.StreakConfirm, "default")
	if !found {
		t.Fatal("expected streak record after first observation")
	}
	if rec1.Count != 1 {
		t.Errorf("count = %d, want 1", rec1.Count)
	}
	if rec1.UpdatedAt == 0 {
		t.Error("updated_at should be set")
	}
	if rec1.Theme != "theme1" {
		t.Errorf("theme = %q, want %q", rec1.Theme, "theme1")
	}
}
