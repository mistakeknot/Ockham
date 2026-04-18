// Package constrain implements Ockham Tier 2 CONSTRAIN: per-theme dispatch
// freezes while the rest of the factory continues. Writes are advisory —
// Clavain consumes the state via the F8 scoring write path.
package constrain

import (
	"errors"
	"fmt"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// Record is the public view of a CONSTRAIN entry.
type Record struct {
	Theme     string
	Reason    string
	FastPath  bool  // true when triggered via F4 rate-of-change fast path
	CreatedAt int64 // unix seconds
	UntilAt   int64 // unix seconds; 0 means open-ended
}

// Controller is the CONSTRAIN API backed by signals.DB.
type Controller struct {
	db  *signals.DB
	now func() time.Time
}

// New constructs a Controller with the given DB.
func New(db *signals.DB) *Controller {
	return &Controller{db: db, now: time.Now}
}

// ErrNotConstrained is returned by Release when the theme has no active freeze.
var ErrNotConstrained = errors.New("constrain: theme is not currently constrained")

// ConstrainTheme installs or refreshes a freeze on theme.
//
// reason is stored for observability. When until is zero the freeze is
// open-ended and must be released explicitly by Release. When until is in the
// past the call is rejected — callers pass a future time or zero for open.
// fastPath labels entries that triggered via the F4 rate-of-change bypass.
func (c *Controller) ConstrainTheme(theme, reason string, until time.Time, fastPath bool) error {
	if theme == "" {
		return errors.New("constrain: theme is required")
	}
	now := c.now()
	var untilAt int64
	if !until.IsZero() {
		if !until.After(now) {
			return fmt.Errorf("constrain: until %s is not in the future", until.Format(time.RFC3339))
		}
		untilAt = until.Unix()
	}
	return c.db.UpsertConstrain(signals.ConstrainRecord{
		Theme:     theme,
		Reason:    reason,
		FastPath:  fastPath,
		CreatedAt: now.Unix(),
		UntilAt:   untilAt,
	})
}

// ReleaseTheme lifts an active freeze. Returns ErrNotConstrained when the
// theme has no active row (expired rows count as inactive).
func (c *Controller) ReleaseTheme(theme string) error {
	active, _, err := c.lookup(theme)
	if err != nil {
		return err
	}
	if !active {
		return ErrNotConstrained
	}
	return c.db.DeleteConstrain(theme)
}

// IsConstrained reports whether theme currently has an active freeze. The
// second return value carries the record when active and the zero value
// otherwise.
func (c *Controller) IsConstrained(theme string) (bool, Record, error) {
	active, rec, err := c.lookup(theme)
	if err != nil {
		return false, Record{}, err
	}
	if !active {
		return false, Record{}, nil
	}
	return true, fromDB(rec), nil
}

// ListActive returns every theme under active constraint.
func (c *Controller) ListActive() ([]Record, error) {
	rows, err := c.db.ListConstrainsActive(c.now().Unix())
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromDB(r))
	}
	return out, nil
}

// PurgeExpired deletes rows whose until_at has already passed. Safe to call
// periodically from a tick loop; returns the count removed.
func (c *Controller) PurgeExpired() (int64, error) {
	return c.db.PurgeExpiredConstrains(c.now().Unix())
}

// lookup reads the row for theme and interprets expiry at the current clock.
func (c *Controller) lookup(theme string) (bool, signals.ConstrainRecord, error) {
	rec, found, err := c.db.GetConstrain(theme)
	if err != nil {
		return false, signals.ConstrainRecord{}, err
	}
	if !found {
		return false, signals.ConstrainRecord{}, nil
	}
	if rec.UntilAt != 0 && rec.UntilAt <= c.now().Unix() {
		return false, rec, nil
	}
	return true, rec, nil
}

func fromDB(r signals.ConstrainRecord) Record {
	return Record{
		Theme:     r.Theme,
		Reason:    r.Reason,
		FastPath:  r.FastPath,
		CreatedAt: r.CreatedAt,
		UntilAt:   r.UntilAt,
	}
}
