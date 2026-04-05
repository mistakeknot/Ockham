package signals

import (
	"database/sql"
	"errors"
)

// BeadMetric represents one bead's completion metrics.
type BeadMetric struct {
	BeadID           string
	Theme            string
	CycleTimeMs      int64
	PassFirstAttempt bool
	CostUSD          *float64
	CompletedAt      int64
}

// InsertBeadMetric upserts a bead metric row. Duplicate bead_id is silently ignored.
func (db *DB) InsertBeadMetric(m BeadMetric) error {
	passInt := 0
	if m.PassFirstAttempt {
		passInt = 1
	}
	_, err := db.conn.Exec(
		`INSERT INTO bead_metrics (bead_id, theme, cycle_time_ms, pass_first_attempt, cost_usd, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bead_id) DO NOTHING`,
		m.BeadID, m.Theme, m.CycleTimeMs, passInt, m.CostUSD, m.CompletedAt,
	)
	return err
}

// LatestBeadMetrics returns the most recent N beads for a theme, ordered by completed_at DESC.
func (db *DB) LatestBeadMetrics(theme string, limit int) ([]BeadMetric, error) {
	rows, err := db.conn.Query(
		`SELECT bead_id, theme, cycle_time_ms, pass_first_attempt, cost_usd, completed_at
		 FROM bead_metrics WHERE theme = ? ORDER BY completed_at DESC LIMIT ?`,
		theme, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []BeadMetric
	for rows.Next() {
		var m BeadMetric
		var passInt int
		var costUSD sql.NullFloat64
		if err := rows.Scan(&m.BeadID, &m.Theme, &m.CycleTimeMs, &passInt, &costUSD, &m.CompletedAt); err != nil {
			return nil, err
		}
		m.PassFirstAttempt = passInt != 0
		if costUSD.Valid {
			v := costUSD.Float64
			m.CostUSD = &v
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

// LastBeadCompletedAt returns the most recent completed_at timestamp for a theme.
// Returns (0, false, nil) if no beads exist for the theme.
func (db *DB) LastBeadCompletedAt(theme string) (int64, bool, error) {
	var ts int64
	err := db.conn.QueryRow(
		"SELECT completed_at FROM bead_metrics WHERE theme = ? ORDER BY completed_at DESC LIMIT 1",
		theme,
	).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return ts, true, nil
}

// PruneBeadMetrics deletes rows older than the Nth most recent per theme.
// Returns the number of rows deleted.
func (db *DB) PruneBeadMetrics(theme string, keepCount int) (int64, error) {
	result, err := db.conn.Exec(
		`DELETE FROM bead_metrics WHERE theme = ? AND completed_at < (
			SELECT completed_at FROM bead_metrics WHERE theme = ?
			ORDER BY completed_at DESC LIMIT 1 OFFSET ?
		)`,
		theme, theme, keepCount-1,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DistinctThemes returns all distinct theme values from bead_metrics.
func (db *DB) DistinctThemes() ([]string, error) {
	rows, err := db.conn.Query("SELECT DISTINCT theme FROM bead_metrics")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var themes []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		themes = append(themes, t)
	}
	return themes, rows.Err()
}
