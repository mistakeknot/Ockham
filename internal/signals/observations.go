package signals

// ObservationRow represents a stored observation metric.
type ObservationRow struct {
	ID          int64
	Theme       string
	MetricType  string
	Value       float64
	CollectedAt int64
}

// InsertObservation stores a collected observation metric.
func (db *DB) InsertObservation(theme, metricType string, value float64, collectedAt int64) error {
	_, err := db.conn.Exec(
		`INSERT INTO observation_metrics (theme, metric_type, value, collected_at)
		 VALUES (?, ?, ?, ?)`,
		theme, metricType, value, collectedAt,
	)
	return err
}

// RecentObservations returns observation metrics for a theme+type collected since the given timestamp.
func (db *DB) RecentObservations(theme, metricType string, since int64) ([]ObservationRow, error) {
	rows, err := db.conn.Query(
		`SELECT id, theme, metric_type, value, collected_at
		 FROM observation_metrics
		 WHERE theme = ? AND metric_type = ? AND collected_at >= ?
		 ORDER BY collected_at DESC`,
		theme, metricType, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ObservationRow
	for rows.Next() {
		var r ObservationRow
		if err := rows.Scan(&r.ID, &r.Theme, &r.MetricType, &r.Value, &r.CollectedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// PruneObservations deletes rows older than the Nth most recent per theme.
// Matches the PruneBeadMetrics convention.
func (db *DB) PruneObservations(theme string, keepCount int) (int64, error) {
	result, err := db.conn.Exec(
		`DELETE FROM observation_metrics WHERE theme = ? AND collected_at < (
			SELECT collected_at FROM observation_metrics WHERE theme = ?
			ORDER BY collected_at DESC LIMIT 1 OFFSET ?
		)`,
		theme, theme, keepCount-1,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ObservationStats returns aggregate stats for health reporting.
type ObservationStats struct {
	Available   bool  // true if any metrics exist within cutoff
	LastCollect int64 // MAX(collected_at)
	MetricCount int   // COUNT(*) within 24h
}

// GetObservationStats returns observation health stats from the DB.
func (db *DB) GetObservationStats(since24h int64) (ObservationStats, error) {
	var stats ObservationStats

	// Last collect
	var lastCollect *int64
	err := db.conn.QueryRow("SELECT MAX(collected_at) FROM observation_metrics").Scan(&lastCollect)
	if err != nil {
		return stats, err
	}
	if lastCollect != nil {
		stats.LastCollect = *lastCollect
		// Available if last collect within 48h
		stats.Available = *lastCollect >= since24h-(24*3600)
	}

	// Metric count within 24h
	err = db.conn.QueryRow(
		"SELECT COUNT(*) FROM observation_metrics WHERE collected_at >= ?", since24h,
	).Scan(&stats.MetricCount)
	if err != nil {
		return stats, err
	}

	return stats, nil
}
