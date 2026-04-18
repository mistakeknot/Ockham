package signals_test

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestGetStreak_Missing(t *testing.T) {
	db := newTestDB(t)
	_, found, err := db.GetStreak("theme1", signals.StreakConfirm, "default")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found for fresh DB")
	}
}

func TestIncrementStreak_FirstTime(t *testing.T) {
	db := newTestDB(t)
	count, err := db.IncrementStreak("theme1", signals.StreakConfirm, "default", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	got, found, err := db.GetStreak("theme1", signals.StreakConfirm, "default")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected row after increment")
	}
	if got.Count != 1 {
		t.Errorf("got count %d, want 1", got.Count)
	}
	if got.UpdatedAt != 1000 {
		t.Errorf("got updated_at %d, want 1000", got.UpdatedAt)
	}
}

func TestIncrementStreak_Multiple(t *testing.T) {
	db := newTestDB(t)
	for i := 1; i <= 5; i++ {
		count, err := db.IncrementStreak("theme1", signals.StreakConfirm, "default", int64(i*100))
		if err != nil {
			t.Fatalf("increment %d: %v", i, err)
		}
		if count != i {
			t.Errorf("count = %d, want %d", count, i)
		}
	}

	got, _, err := db.GetStreak("theme1", signals.StreakConfirm, "default")
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 5 {
		t.Errorf("final count = %d, want 5", got.Count)
	}
	if got.UpdatedAt != 500 {
		t.Errorf("final updated_at = %d, want 500", got.UpdatedAt)
	}
}

func TestIncrementStreak_DifferentSignals(t *testing.T) {
	db := newTestDB(t)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "error_rate", 100)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "latency", 100)
	_, _ = db.IncrementStreak("theme1", signals.StreakStability, "error_rate", 100)

	sig1, _, _ := db.GetStreak("theme1", signals.StreakConfirm, "error_rate")
	sig2, _, _ := db.GetStreak("theme1", signals.StreakConfirm, "latency")
	sig3, _, _ := db.GetStreak("theme1", signals.StreakStability, "error_rate")

	if sig1.Count != 1 || sig2.Count != 1 || sig3.Count != 1 {
		t.Errorf("counts: confirm/error_rate=%d, confirm/latency=%d, stability/error_rate=%d",
			sig1.Count, sig2.Count, sig3.Count)
	}
}

func TestResetStreak(t *testing.T) {
	db := newTestDB(t)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "default", 100)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "default", 200)

	if err := db.ResetStreak("theme1", signals.StreakConfirm, "default"); err != nil {
		t.Fatal(err)
	}

	_, found, err := db.GetStreak("theme1", signals.StreakConfirm, "default")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected streak to be deleted after reset")
	}
}

func TestResetStreak_Missing(t *testing.T) {
	db := newTestDB(t)
	// Resetting a missing streak should be a no-op, not an error.
	if err := db.ResetStreak("theme1", signals.StreakConfirm, "default"); err != nil {
		t.Errorf("reset missing streak: %v", err)
	}
}

func TestDeleteStreaksForTheme(t *testing.T) {
	db := newTestDB(t)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "error_rate", 100)
	_, _ = db.IncrementStreak("theme1", signals.StreakConfirm, "latency", 100)
	_, _ = db.IncrementStreak("theme1", signals.StreakStability, "error_rate", 100)
	_, _ = db.IncrementStreak("theme2", signals.StreakConfirm, "error_rate", 100)

	if err := db.DeleteStreaksForTheme("theme1"); err != nil {
		t.Fatal(err)
	}

	// All theme1 streaks should be gone
	_, found1, _ := db.GetStreak("theme1", signals.StreakConfirm, "error_rate")
	_, found2, _ := db.GetStreak("theme1", signals.StreakConfirm, "latency")
	_, found3, _ := db.GetStreak("theme1", signals.StreakStability, "error_rate")

	if found1 || found2 || found3 {
		t.Error("expected all theme1 streaks to be deleted")
	}

	// theme2 streak should remain
	_, found4, _ := db.GetStreak("theme2", signals.StreakConfirm, "error_rate")
	if !found4 {
		t.Error("expected theme2 streak to remain")
	}
}

func TestDeleteStreaksForTheme_Missing(t *testing.T) {
	db := newTestDB(t)
	// Deleting for a missing theme should be a no-op, not an error.
	if err := db.DeleteStreaksForTheme("nonexistent"); err != nil {
		t.Errorf("delete streaks for missing theme: %v", err)
	}
}
