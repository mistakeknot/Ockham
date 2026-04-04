package signals

import (
	"database/sql"
	"errors"
	"time"
)

// AuthoritySnapshot represents a point-in-time capture of agent authority metrics.
type AuthoritySnapshot struct {
	Agent      string
	Domain     string
	HitRate    float64
	Sessions   int
	Confidence float64
	CapturedAt int64
}

// SaveAuthoritySnapshot upserts an authority snapshot for an agent+domain pair.
func (db *DB) SaveAuthoritySnapshot(snap AuthoritySnapshot) error {
	if snap.CapturedAt == 0 {
		snap.CapturedAt = time.Now().Unix()
	}
	_, err := db.conn.Exec(
		`INSERT INTO authority_snapshot (agent, domain, hit_rate, sessions, confidence, captured_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent, domain) DO UPDATE SET
		   hit_rate = excluded.hit_rate,
		   sessions = excluded.sessions,
		   confidence = excluded.confidence,
		   captured_at = excluded.captured_at`,
		snap.Agent, snap.Domain, snap.HitRate, snap.Sessions, snap.Confidence, snap.CapturedAt,
	)
	return err
}

// GetAuthoritySnapshot reads the snapshot for an agent+domain pair.
// Returns (zero, false, nil) if not found.
func (db *DB) GetAuthoritySnapshot(agent, domain string) (AuthoritySnapshot, bool, error) {
	var s AuthoritySnapshot
	err := db.conn.QueryRow(
		`SELECT agent, domain, hit_rate, sessions, confidence, captured_at
		 FROM authority_snapshot WHERE agent = ? AND domain = ?`,
		agent, domain,
	).Scan(&s.Agent, &s.Domain, &s.HitRate, &s.Sessions, &s.Confidence, &s.CapturedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthoritySnapshot{}, false, nil
	}
	if err != nil {
		return AuthoritySnapshot{}, false, err
	}
	return s, true, nil
}

// ListAuthoritySnapshots returns all stored snapshots.
func (db *DB) ListAuthoritySnapshots() ([]AuthoritySnapshot, error) {
	rows, err := db.conn.Query(
		"SELECT agent, domain, hit_rate, sessions, confidence, captured_at FROM authority_snapshot",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []AuthoritySnapshot
	for rows.Next() {
		var s AuthoritySnapshot
		if err := rows.Scan(&s.Agent, &s.Domain, &s.HitRate, &s.Sessions, &s.Confidence, &s.CapturedAt); err != nil {
			return nil, err
		}
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}
