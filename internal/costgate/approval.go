package costgate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const approvalSchema = `
CREATE TABLE IF NOT EXISTS approvals (
    bead_id TEXT PRIMARY KEY,
    tier INTEGER NOT NULL,
    approved_by TEXT NOT NULL,
    approved_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    last_touch_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS approval_requests (
    bead_id TEXT PRIMARY KEY,
    agent TEXT NOT NULL,
    tier INTEGER NOT NULL,
    reason TEXT NOT NULL,
    requested_at INTEGER NOT NULL
);
`

// ApprovalDB stores approval state in SQLite.
type ApprovalDB struct {
	db *sql.DB
}

// DefaultDBPath returns ~/.config/ockham/approvals.db.
func DefaultDBPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "approvals.db")
}

// NewApprovalDB opens or creates approvals.db.
func NewApprovalDB(path string) (*ApprovalDB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("approval db mkdir: %w", err)
	}
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("approval db open: %w", err)
	}
	if _, err := conn.Exec(approvalSchema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("approval db schema: %w", err)
	}
	return &ApprovalDB{db: conn}, nil
}

// Close closes the DB.
func (db *ApprovalDB) Close() error {
	if db == nil || db.db == nil {
		return nil
	}
	return db.db.Close()
}

// GetApproval fetches an approval by bead ID.
func (db *ApprovalDB) GetApproval(beadID string) (*Approval, error) {
	var a Approval
	err := db.db.QueryRow(
		`SELECT bead_id, tier, approved_by, approved_at, expires_at, last_touch_at
		 FROM approvals WHERE bead_id = ?`, beadID,
	).Scan(&a.BeadID, &a.Tier, &a.ApprovedBy, &a.ApprovedAt, &a.ExpiresAt, &a.LastTouchAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// SaveApproval upserts an approval.
func (db *ApprovalDB) SaveApproval(a Approval) error {
	_, err := db.db.Exec(
		`INSERT INTO approvals (bead_id, tier, approved_by, approved_at, expires_at, last_touch_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bead_id) DO UPDATE SET
		   tier = excluded.tier,
		   approved_by = excluded.approved_by,
		   approved_at = excluded.approved_at,
		   expires_at = excluded.expires_at,
		   last_touch_at = excluded.last_touch_at`,
		a.BeadID, a.Tier, a.ApprovedBy, a.ApprovedAt, a.ExpiresAt, a.LastTouchAt,
	)
	return err
}

// DeleteApproval removes an approval by bead ID.
func (db *ApprovalDB) DeleteApproval(beadID string) error {
	_, err := db.db.Exec(`DELETE FROM approvals WHERE bead_id = ?`, beadID)
	return err
}

// TouchApproval extends an active approval lease.
func (db *ApprovalDB) TouchApproval(beadID string, touchedAt, expiresAt int64) error {
	_, err := db.db.Exec(
		`UPDATE approvals SET last_touch_at = ?, expires_at = ? WHERE bead_id = ?`,
		touchedAt, expiresAt, beadID,
	)
	return err
}

// ExpireOlderThan deletes approvals whose expires_at is older than ts.
func (db *ApprovalDB) ExpireOlderThan(ts int64) (int, error) {
	res, err := db.db.Exec(`DELETE FROM approvals WHERE expires_at < ?`, ts)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// SaveRequest upserts a pending approval request.
func (db *ApprovalDB) SaveRequest(r ApprovalRequest) error {
	_, err := db.db.Exec(
		`INSERT INTO approval_requests (bead_id, agent, tier, reason, requested_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(bead_id) DO UPDATE SET
		   agent = excluded.agent,
		   tier = excluded.tier,
		   reason = excluded.reason,
		   requested_at = excluded.requested_at`,
		r.BeadID, r.Agent, r.Tier, r.Reason, r.RequestedAt,
	)
	return err
}

// DeleteRequest removes a pending approval request.
func (db *ApprovalDB) DeleteRequest(beadID string) error {
	_, err := db.db.Exec(`DELETE FROM approval_requests WHERE bead_id = ?`, beadID)
	return err
}

// ListRequests returns pending approval requests in requested order.
func (db *ApprovalDB) ListRequests() ([]ApprovalRequest, error) {
	rows, err := db.db.Query(
		`SELECT bead_id, agent, tier, reason, requested_at
		 FROM approval_requests ORDER BY requested_at ASC, bead_id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ApprovalRequest
	for rows.Next() {
		var r ApprovalRequest
		if err := rows.Scan(&r.BeadID, &r.Agent, &r.Tier, &r.Reason, &r.RequestedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
