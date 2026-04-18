package constrain_test

import (
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/constrain"
	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestReleaseController_NotConstrainedNoOp(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// ObserveClean on unconstrained theme is a no-op
	shouldRelease, count, err := rc.ObserveClean("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if shouldRelease {
		t.Error("expected no release on unconstrained theme")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestReleaseController_CleanWhileConstrained(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain a theme
	if err := db.UpsertConstrain(signals.ConstrainRecord{
		Theme:     "theme1",
		Reason:    "error_rate",
		CreatedAt: 1000,
		UntilAt:   0, // open-ended
	}); err != nil {
		t.Fatal(err)
	}

	// First clean observation
	shouldRelease, count, err := rc.ObserveClean("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if shouldRelease {
		t.Error("expected no release on first clean")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestReleaseController_NoReleaseBeforeThreshold(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Two clean observations
	for i := 1; i <= 2; i++ {
		shouldRelease, count, err := rc.ObserveClean("theme1")
		if err != nil {
			t.Fatal(err)
		}
		if shouldRelease {
			t.Errorf("expected no release at observation %d", i)
		}
		if count != i {
			t.Errorf("count = %d, want %d", count, i)
		}
	}
}

func TestReleaseController_ReleaseAtThreshold(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Three clean observations
	for i := 1; i <= 3; i++ {
		shouldRelease, count, err := rc.ObserveClean("theme1")
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if shouldRelease {
				t.Errorf("expected no release at observation %d", i)
			}
		} else {
			if !shouldRelease {
				t.Error("expected release at observation 3")
			}
			if count != 3 {
				t.Errorf("count = %d, want 3", count)
			}
		}
	}

	// After release, streak should be reset
	rec, found, err := db.GetStreak("theme1", signals.StreakStability, "default")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected streak to be reset after release, got %d", rec.Count)
	}
}

func TestReleaseController_AnomalyResets(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Two clean observations
	_, c1, _ := rc.ObserveClean("theme1")
	_, c2, _ := rc.ObserveClean("theme1")

	if c1 != 1 || c2 != 2 {
		t.Errorf("counts before anomaly: %d, %d; want 1, 2", c1, c2)
	}

	// Anomaly resets stability
	if err := rc.ObserveAnomalyDuringConstrain("theme1"); err != nil {
		t.Fatal(err)
	}

	// Streak should be deleted
	rec, found, err := db.GetStreak("theme1", signals.StreakStability, "default")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Errorf("expected streak to be reset, got %d", rec.Count)
	}

	// Next clean starts fresh
	_, c3, _ := rc.ObserveClean("theme1")
	if c3 != 1 {
		t.Errorf("count after anomaly = %d, want 1", c3)
	}
}

func TestReleaseController_IsStabilizing_NoStreak(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	active, count, err := rc.IsStabilizing("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Error("expected active=false with no streak")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestReleaseController_IsStabilizing_WithStreak(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 5}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Build up a partial streak
	for i := 1; i <= 3; i++ {
		rc.ObserveClean("theme1")
	}

	// Check IsStabilizing
	active, count, err := rc.IsStabilizing("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Error("expected active=true with streak")
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestReleaseController_IsStabilizing_AfterRelease(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 2}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Reach release threshold
	rc.ObserveClean("theme1")
	rc.ObserveClean("theme1")

	// IsStabilizing should show no active streak (was reset on release)
	active, count, err := rc.IsStabilizing("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if active {
		t.Error("expected active=false after release (streak reset)")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestReleaseController_ExpiredConstrainTreatedAsNotConstrained(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Create an expired constraint (until_at in the past)
	now := time.Now().Unix()
	if err := db.UpsertConstrain(signals.ConstrainRecord{
		Theme:     "theme1",
		Reason:    "error_rate",
		CreatedAt: now - 200,
		UntilAt:   now - 100, // already expired
	}); err != nil {
		t.Fatal(err)
	}

	// ObserveClean should return (false, 0, nil) for expired row
	shouldRelease, count, err := rc.ObserveClean("theme1")
	if err != nil {
		t.Fatal(err)
	}
	if shouldRelease {
		t.Error("expected no release for expired constraint")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for expired constraint", count)
	}

	// Streak should not be created for expired constraint
	active, _, _ := rc.IsStabilizing("theme1")
	if active {
		t.Error("expected no stabilizing streak for expired constraint")
	}
}

func TestReleaseController_AnomalyOnUnconstrained(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Anomaly on unconstrained theme is a no-op
	if err := rc.ObserveAnomalyDuringConstrain("theme1"); err != nil {
		t.Fatal(err)
	}

	// No streak should exist
	active, _, _ := rc.IsStabilizing("theme1")
	if active {
		t.Error("expected no stabilizing streak after anomaly on unconstrained")
	}
}

func TestReleaseController_CustomPolicy(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 5}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})

	// Reach threshold at 5
	var shouldRelease bool
	for i := 1; i < 5; i++ {
		shouldRelease, _, _ = rc.ObserveClean("theme1")
		if shouldRelease {
			t.Errorf("expected no release at observation %d", i)
		}
	}

	shouldRelease, _, _ = rc.ObserveClean("theme1")
	if !shouldRelease {
		t.Error("expected release at observation 5")
	}
}

func TestReleaseController_MultipleThemes(t *testing.T) {
	db := newTestDB(t)
	policy := constrain.StabilityPolicy{RequiredWindows: 3}
	rc := constrain.NewReleaseController(db, policy)

	// Constrain two themes
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme1", CreatedAt: 1000, UntilAt: 0})
	db.UpsertConstrain(signals.ConstrainRecord{Theme: "theme2", CreatedAt: 1000, UntilAt: 0})

	// Accumulate streaks step by step
	// Step 1: both at count 1
	_, c1_1, _ := rc.ObserveClean("theme1")
	_, c2_1, _ := rc.ObserveClean("theme2")
	if c1_1 != 1 || c2_1 != 1 {
		t.Errorf("step 1: theme1=%d, theme2=%d; want 1, 1", c1_1, c2_1)
	}

	// Step 2: both at count 2
	_, c1_2, _ := rc.ObserveClean("theme1")
	_, c2_2, _ := rc.ObserveClean("theme2")
	if c1_2 != 2 || c2_2 != 2 {
		t.Errorf("step 2: theme1=%d, theme2=%d; want 2, 2", c1_2, c2_2)
	}

	// Step 3: theme1 hits threshold and fires, resetting its streak
	r1, c1_3, _ := rc.ObserveClean("theme1")
	if !r1 {
		t.Error("expected theme1 to fire at step 3")
	}
	if c1_3 != 3 {
		t.Errorf("theme1 step 3 count = %d, want 3", c1_3)
	}

	// theme2 continues and is now at count 3 (same threshold)
	r2, c2_3, _ := rc.ObserveClean("theme2")
	if !r2 {
		t.Error("expected theme2 to fire at step 3")
	}
	if c2_3 != 3 {
		t.Errorf("theme2 step 3 count = %d, want 3", c2_3)
	}

	// Both streaks should be reset
	active1, _, _ := rc.IsStabilizing("theme1")
	active2, _, _ := rc.IsStabilizing("theme2")
	if active1 || active2 {
		t.Errorf("expected both streaks reset: theme1=%v, theme2=%v", active1, active2)
	}
}
