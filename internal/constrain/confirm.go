// Package constrain implements multi-window confirmation (F3) and stability
// period (F7) gates for CONSTRAIN trigger decisions.
package constrain

import (
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// ConfirmPolicy configures the multi-window confirmation gate for F3.
type ConfirmPolicy struct {
	RequiredWindows int // number of consecutive windows needed to fire (default 3)
}

// DefaultConfirmPolicy returns a conservative policy requiring 3 consecutive
// anomalous windows before firing a CONSTRAIN trigger.
func DefaultConfirmPolicy() ConfirmPolicy {
	return ConfirmPolicy{RequiredWindows: 3}
}

// Trigger implements the F3 multi-window confirmation gate. Tripped signals
// must be observed in RequiredWindows consecutive windows before Trigger returns
// fired=true. A clean window resets the streak.
type Trigger struct {
	db     *signals.DB
	policy ConfirmPolicy
	now    func() time.Time
}

// NewTrigger constructs a Trigger with the given DB and policy.
func NewTrigger(db *signals.DB, policy ConfirmPolicy) *Trigger {
	return &Trigger{db: db, policy: policy, now: time.Now}
}

// Observe records an observation for (theme, signal). When tripped=true the
// confirmation streak increments; when tripped=false or on reaching RequiredWindows,
// the streak resets. Returns (fired, count, err):
// - fired=true when count reaches RequiredWindows (caller must constrain)
// - count is the current streak before reset
// - err indicates database errors only
func (t *Trigger) Observe(theme, signal string, tripped bool) (fired bool, count int, err error) {
	now := t.now().Unix()

	if !tripped {
		// Clean window: reset streak and return early.
		if err := t.db.ResetStreak(theme, signals.StreakConfirm, signal); err != nil {
			return false, 0, err
		}
		return false, 0, nil
	}

	// Anomalous window: increment streak.
	newCount, err := t.db.IncrementStreak(theme, signals.StreakConfirm, signal, now)
	if err != nil {
		return false, 0, err
	}

	// Check if we've reached the confirmation threshold.
	if newCount >= t.policy.RequiredWindows {
		// Fire trigger and reset the streak for next cycle.
		if err := t.db.ResetStreak(theme, signals.StreakConfirm, signal); err != nil {
			return false, newCount, err
		}
		return true, newCount, nil
	}

	return false, newCount, nil
}
