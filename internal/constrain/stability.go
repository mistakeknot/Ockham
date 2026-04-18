package constrain

import (
	"errors"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// StabilityPolicy configures the de-escalation stability period gate for F7.
type StabilityPolicy struct {
	RequiredWindows int // number of consecutive clean windows needed to release (default 5)
}

// DefaultStabilityPolicy returns a conservative policy requiring 5 consecutive
// clean windows before auto-releasing an active CONSTRAIN.
func DefaultStabilityPolicy() StabilityPolicy {
	return StabilityPolicy{RequiredWindows: 5}
}

// ReleaseController implements the F7 de-escalation stability gate. After a
// CONSTRAIN is installed, clean observations must accumulate across RequiredWindows
// consecutive windows before auto-release is allowed. Any anomaly during this
// period resets the streak.
type ReleaseController struct {
	db     *signals.DB
	policy StabilityPolicy
	now    func() time.Time
}

// NewReleaseController constructs a ReleaseController with the given DB and policy.
func NewReleaseController(db *signals.DB, policy StabilityPolicy) *ReleaseController {
	return &ReleaseController{db: db, policy: policy, now: time.Now}
}

// ErrStabilityNotConstrained is returned when a stability operation expects
// an active CONSTRAIN but the theme is not currently constrained.
var ErrStabilityNotConstrained = errors.New("stability: theme is not currently constrained")

// ObserveClean records a clean observation for theme. If the theme is currently
// constrained, increments the stability streak. Returns (shouldRelease, count, err):
// - shouldRelease=true when streak reaches RequiredWindows; also resets the streak
// - count is the streak before reset
// - If theme is not constrained, returns (false, 0, nil) — no error
func (rc *ReleaseController) ObserveClean(theme string) (shouldRelease bool, count int, err error) {
	// Check if theme is currently constrained
	constrained, _, err := rc.db.GetConstrain(theme)
	if err != nil {
		return false, 0, err
	}

	// Convert to active state check (accounting for expiry)
	if constrained.UntilAt != 0 && constrained.UntilAt <= rc.now().Unix() {
		// Row exists but is expired; treat as not constrained
		constrained = signals.ConstrainRecord{}
	}

	// If not in a constrain row, no-op
	if constrained.Theme == "" {
		return false, 0, nil
	}

	// Constrained and clean: increment stability streak
	now := rc.now().Unix()
	newCount, err := rc.db.IncrementStreak(theme, signals.StreakStability, "default", now)
	if err != nil {
		return false, 0, err
	}

	// Check if we've reached the stability threshold for release
	if newCount >= rc.policy.RequiredWindows {
		// Reset the streak before returning
		if err := rc.db.ResetStreak(theme, signals.StreakStability, "default"); err != nil {
			return false, newCount, err
		}
		return true, newCount, nil
	}

	return false, newCount, nil
}

// ObserveAnomalyDuringConstrain resets the stability streak when an anomaly
// is detected while a theme is constrained. Stability restarts from zero.
// If the theme is not constrained, this is a no-op.
func (rc *ReleaseController) ObserveAnomalyDuringConstrain(theme string) error {
	return rc.db.ResetStreak(theme, signals.StreakStability, "default")
}

// IsStabilizing reports whether a theme is currently in a stability observation
// period (streak > 0). Returns (active, count, err):
// - active=true when a stability streak exists for the theme
// - count is the current streak value
// - err indicates database errors only
func (rc *ReleaseController) IsStabilizing(theme string) (active bool, count int, err error) {
	rec, found, err := rc.db.GetStreak(theme, signals.StreakStability, "default")
	if err != nil {
		return false, 0, err
	}
	if !found {
		return false, 0, nil
	}
	return rec.Count > 0, rec.Count, nil
}
