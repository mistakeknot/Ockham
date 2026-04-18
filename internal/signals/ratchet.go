package signals

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Tier is the ratchet position for an (agent, domain) pair.
type Tier string

const (
	TierShadow     Tier = "shadow"     // propose only; human approves
	TierSupervised Tier = "supervised" // act; human reviews
	TierAutonomous Tier = "autonomous" // act; human audits
)

// ValidTier reports whether s is a known tier.
func ValidTier(s Tier) bool {
	switch s {
	case TierShadow, TierSupervised, TierAutonomous:
		return true
	}
	return false
}

// RatchetState is the persisted tier position for an (agent, domain) pair.
type RatchetState struct {
	Agent      string
	Domain     string
	Tier       Tier
	PromotedAt int64
	DemotedAt  int64
}

// GetRatchetState reads the ratchet row for (agent, domain).
// When no row exists the zero value with Tier=shadow is returned (found=false).
func (db *DB) GetRatchetState(agent, domain string) (RatchetState, bool, error) {
	var r RatchetState
	err := db.conn.QueryRow(
		`SELECT agent, domain, tier, promoted_at, demoted_at
		 FROM ratchet_state WHERE agent = ? AND domain = ?`,
		agent, domain,
	).Scan(&r.Agent, &r.Domain, &r.Tier, &r.PromotedAt, &r.DemotedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RatchetState{Agent: agent, Domain: domain, Tier: TierShadow}, false, nil
	}
	if err != nil {
		return RatchetState{}, false, err
	}
	return r, true, nil
}

// SaveRatchetState upserts a ratchet row. The caller must provide a valid tier.
func (db *DB) SaveRatchetState(r RatchetState) error {
	if !ValidTier(r.Tier) {
		return fmt.Errorf("signals: invalid tier %q", r.Tier)
	}
	now := time.Now().Unix()
	if r.PromotedAt == 0 && r.DemotedAt == 0 {
		r.PromotedAt = now
	}
	_, err := db.conn.Exec(
		`INSERT INTO ratchet_state (agent, domain, tier, promoted_at, demoted_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(agent, domain) DO UPDATE SET
		   tier = excluded.tier,
		   promoted_at = excluded.promoted_at,
		   demoted_at = excluded.demoted_at`,
		r.Agent, r.Domain, r.Tier, r.PromotedAt, r.DemotedAt,
	)
	return err
}

// ListRatchetStates returns every ratchet row.
func (db *DB) ListRatchetStates() ([]RatchetState, error) {
	rows, err := db.conn.Query(
		`SELECT agent, domain, tier, promoted_at, demoted_at FROM ratchet_state`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RatchetState
	for rows.Next() {
		var r RatchetState
		if err := rows.Scan(&r.Agent, &r.Domain, &r.Tier, &r.PromotedAt, &r.DemotedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
