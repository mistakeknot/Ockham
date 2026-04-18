package signals

import (
	"database/sql"
	"errors"
)

// StreakKind identifies the streak type: confirmation or stability.
type StreakKind string

const (
	StreakConfirm   StreakKind = "confirm"
	StreakStability StreakKind = "stability"
)

// StreakRecord is a persisted per-(theme, kind, signal) streak entry.
type StreakRecord struct {
	Theme     string
	Kind      StreakKind
	Signal    string // anomaly type or 'default'
	Count     int
	UpdatedAt int64
}

// GetStreak reads a streak record by (theme, kind, signal).
// Returns (zero, false, nil) if absent.
func (db *DB) GetStreak(theme string, kind StreakKind, signal string) (StreakRecord, bool, error) {
	var r StreakRecord
	err := db.conn.QueryRow(
		`SELECT theme, kind, signal, count, updated_at
		 FROM streak_state WHERE theme = ? AND kind = ? AND signal = ?`,
		theme, string(kind), signal,
	).Scan(&r.Theme, &r.Kind, &r.Signal, &r.Count, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StreakRecord{}, false, nil
	}
	if err != nil {
		return StreakRecord{}, false, err
	}
	r.Kind = StreakKind(r.Kind)
	return r, true, nil
}

// IncrementStreak increments the streak count for (theme, kind, signal).
// Returns the new count and any error. Upserts the row with updated_at.
func (db *DB) IncrementStreak(theme string, kind StreakKind, signal string, updatedAt int64) (int, error) {
	rec, found, err := db.GetStreak(theme, kind, signal)
	if err != nil {
		return 0, err
	}

	newCount := 1
	if found {
		newCount = rec.Count + 1
	}

	_, err = db.conn.Exec(
		`INSERT INTO streak_state (theme, kind, signal, count, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(theme, kind, signal) DO UPDATE SET
		   count = excluded.count,
		   updated_at = excluded.updated_at`,
		theme, string(kind), signal, newCount, updatedAt,
	)
	if err != nil {
		return 0, err
	}
	return newCount, nil
}

// ResetStreak sets the streak count to 0 for (theme, kind, signal).
func (db *DB) ResetStreak(theme string, kind StreakKind, signal string) error {
	_, err := db.conn.Exec(
		`DELETE FROM streak_state WHERE theme = ? AND kind = ? AND signal = ?`,
		theme, string(kind), signal,
	)
	return err
}

// DeleteStreaksForTheme removes all streak records for a given theme.
// Called when a theme is released from constrain.
func (db *DB) DeleteStreaksForTheme(theme string) error {
	_, err := db.conn.Exec(
		`DELETE FROM streak_state WHERE theme = ?`,
		theme,
	)
	return err
}
