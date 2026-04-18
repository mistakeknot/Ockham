package signals

import (
	"database/sql"
	"errors"
)

// ConstrainRecord is a persisted per-theme CONSTRAIN entry.
// UntilAt == 0 means open-ended (release must be explicit).
type ConstrainRecord struct {
	Theme     string
	Reason    string
	FastPath  bool
	CreatedAt int64
	UntilAt   int64
}

// UpsertConstrain writes or updates the CONSTRAIN row for a theme.
func (db *DB) UpsertConstrain(r ConstrainRecord) error {
	fp := 0
	if r.FastPath {
		fp = 1
	}
	_, err := db.conn.Exec(
		`INSERT INTO constrain_state (theme, reason, fast_path, created_at, until_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(theme) DO UPDATE SET
		   reason = excluded.reason,
		   fast_path = excluded.fast_path,
		   created_at = excluded.created_at,
		   until_at = excluded.until_at`,
		r.Theme, r.Reason, fp, r.CreatedAt, r.UntilAt,
	)
	return err
}

// GetConstrain reads the row for a theme. Returns (zero, false, nil) if absent.
func (db *DB) GetConstrain(theme string) (ConstrainRecord, bool, error) {
	var r ConstrainRecord
	var fp int
	err := db.conn.QueryRow(
		`SELECT theme, reason, fast_path, created_at, until_at
		 FROM constrain_state WHERE theme = ?`,
		theme,
	).Scan(&r.Theme, &r.Reason, &fp, &r.CreatedAt, &r.UntilAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ConstrainRecord{}, false, nil
	}
	if err != nil {
		return ConstrainRecord{}, false, err
	}
	r.FastPath = fp != 0
	return r, true, nil
}

// DeleteConstrain removes the row for a theme. Missing rows are not an error.
func (db *DB) DeleteConstrain(theme string) error {
	_, err := db.conn.Exec(`DELETE FROM constrain_state WHERE theme = ?`, theme)
	return err
}

// ListConstrainsActive returns rows that are still in force at now:
// either UntilAt == 0 (open-ended) or UntilAt > now.
func (db *DB) ListConstrainsActive(now int64) ([]ConstrainRecord, error) {
	rows, err := db.conn.Query(
		`SELECT theme, reason, fast_path, created_at, until_at
		 FROM constrain_state
		 WHERE until_at = 0 OR until_at > ?`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConstrainRecord
	for rows.Next() {
		var r ConstrainRecord
		var fp int
		if err := rows.Scan(&r.Theme, &r.Reason, &fp, &r.CreatedAt, &r.UntilAt); err != nil {
			return nil, err
		}
		r.FastPath = fp != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// PurgeExpiredConstrains removes rows whose UntilAt is set and already past.
// Returns the count removed.
func (db *DB) PurgeExpiredConstrains(now int64) (int64, error) {
	res, err := db.conn.Exec(
		`DELETE FROM constrain_state WHERE until_at > 0 AND until_at <= ?`,
		now,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
