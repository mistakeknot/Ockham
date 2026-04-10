package notify

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS outbox (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    priority INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    deliver_after INTEGER NOT NULL DEFAULT 0,
    sent_at INTEGER NOT NULL DEFAULT 0,
    dedup_key TEXT NOT NULL DEFAULT '',
    coalesce_key TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox(status, deliver_after, created_at);
CREATE INDEX IF NOT EXISTS idx_outbox_dedup ON outbox(dedup_key, status);
CREATE INDEX IF NOT EXISTS idx_outbox_coalesce ON outbox(coalesce_key, status);
`

// Outbox persists notification records for Hermes delivery.
type Outbox struct {
	db *sql.DB
}

// DefaultOutboxPath returns ~/.config/ockham/outbox.db.
func DefaultOutboxPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "outbox.db")
}

// NewOutbox opens or creates the outbox database.
func NewOutbox(path string) (*Outbox, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("outbox mkdir: %w", err)
	}
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("outbox open: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("outbox schema: %w", err)
	}
	return &Outbox{db: conn}, nil
}

// Close closes the DB.
func (o *Outbox) Close() error {
	if o == nil || o.db == nil {
		return nil
	}
	return o.db.Close()
}

// Enqueue inserts or coalesces a notification.
func (o *Outbox) Enqueue(n Notification) error {
	if n.DedupKey != "" {
		var existingID string
		err := o.db.QueryRow(`SELECT id FROM outbox WHERE dedup_key = ? AND status = ? LIMIT 1`, n.DedupKey, StatusPending).Scan(&existingID)
		if err == nil {
			return nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if n.CoalesceKey != "" {
		var existingID string
		err := o.db.QueryRow(`SELECT id FROM outbox WHERE coalesce_key = ? AND status = ? LIMIT 1`, n.CoalesceKey, StatusPending).Scan(&existingID)
		if err == nil {
			_, err = o.db.Exec(`UPDATE outbox SET title = ?, body = ?, priority = ?, created_at = ?, deliver_after = ? WHERE id = ?`,
				n.Title, n.Body, n.Priority, n.CreatedAt, n.DeliverAfter, existingID)
			return err
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	_, err := o.db.Exec(
		`INSERT INTO outbox (id, type, title, body, priority, status, created_at, deliver_after, sent_at, dedup_key, coalesce_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Type, n.Title, n.Body, n.Priority, n.Status, n.CreatedAt, n.DeliverAfter, n.SentAt, n.DedupKey, n.CoalesceKey,
	)
	return err
}

// ListPending returns all pending notifications.
func (o *Outbox) ListPending() ([]Notification, error) {
	rows, err := o.db.Query(
		`SELECT id, type, title, body, priority, status, created_at, deliver_after, sent_at, dedup_key, coalesce_key
		 FROM outbox WHERE status = ?
		 ORDER BY priority DESC, created_at ASC, id ASC`, StatusPending,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// ListReady returns pending notifications ready for delivery.
func (o *Outbox) ListReady(now int64) ([]Notification, error) {
	rows, err := o.db.Query(
		`SELECT id, type, title, body, priority, status, created_at, deliver_after, sent_at, dedup_key, coalesce_key
		 FROM outbox WHERE status = ? AND deliver_after <= ?
		 ORDER BY priority DESC, created_at ASC, id ASC`, StatusPending, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

// MarkSent marks a notification as delivered.
func (o *Outbox) MarkSent(id string, sentAt int64) error {
	_, err := o.db.Exec(`UPDATE outbox SET status = ?, sent_at = ? WHERE id = ?`, StatusSent, sentAt, id)
	return err
}

func scanNotifications(rows *sql.Rows) ([]Notification, error) {
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Body, &n.Priority, &n.Status, &n.CreatedAt, &n.DeliverAfter, &n.SentAt, &n.DedupKey, &n.CoalesceKey); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
