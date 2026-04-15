package signals

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 3

const schema = `
CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS signal_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS authority_snapshot (
    agent TEXT NOT NULL,
    domain TEXT NOT NULL,
    hit_rate REAL NOT NULL,
    sessions INTEGER NOT NULL,
    confidence REAL NOT NULL,
    captured_at INTEGER NOT NULL,
    PRIMARY KEY (agent, domain)
);
CREATE TABLE IF NOT EXISTS ratchet_state (
    agent TEXT NOT NULL,
    domain TEXT NOT NULL,
    tier TEXT NOT NULL DEFAULT 'shadow',
    promoted_at INTEGER NOT NULL DEFAULT 0,
    demoted_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent, domain)
);
CREATE TABLE IF NOT EXISTS bead_metrics (
    bead_id TEXT PRIMARY KEY,
    theme TEXT NOT NULL,
    cycle_time_ms INTEGER NOT NULL,
    pass_first_attempt INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL,
    completed_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_bead_metrics_theme_completed
    ON bead_metrics(theme, completed_at DESC);
CREATE TABLE IF NOT EXISTS observation_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    theme TEXT NOT NULL,
    metric_type TEXT NOT NULL,
    value REAL NOT NULL,
    collected_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_obs_theme_type
    ON observation_metrics(theme, metric_type, collected_at DESC);
`

// DB wraps a SQLite connection to signals.db.
type DB struct {
	conn         *sql.DB
	path         string
	wasRecovered bool
}

// DefaultDBPath returns ~/.config/ockham/signals.db.
func DefaultDBPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "signals.db")
}

// NewDB opens or creates signals.db at path. On confirmed corruption,
// deletes and recreates with defaults (fail-safe to shadow).
// Transient errors (SQLITE_BUSY) are returned directly — no deletion.
func NewDB(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("signals: mkdir: %w", err)
	}

	db := &DB{path: path}

	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		// File doesn't exist or can't be opened — try recovery
		if recovered, recErr := db.recover(); recovered {
			return db, recErr
		}
		return nil, fmt.Errorf("signals: open: %w", err)
	}
	db.conn = conn

	// Check integrity
	if !db.integrityOK() {
		db.conn.Close()
		if recovered, recErr := db.recover(); recovered {
			return db, recErr
		}
		return nil, errors.New("signals: integrity check failed and recovery failed")
	}

	// Ensure schema exists and version matches
	if err := db.ensureSchema(); err != nil {
		// Only recover on confirmed corruption, not transient errors like SQLITE_BUSY.
		// Integrity already passed above, so schema errors are likely transient.
		if isCorruptionError(err) {
			db.conn.Close()
			if recovered, recErr := db.recover(); recovered {
				return db, recErr
			}
		}
		return nil, fmt.Errorf("signals: schema: %w", err)
	}

	return db, nil
}

// WasRecovered returns true if the DB was recreated from scratch due to corruption.
func (db *DB) WasRecovered() bool {
	return db.wasRecovered
}

// Close closes the database connection.
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Conn returns the underlying sql.DB for direct queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// isCorruptionError checks if an error indicates structural database corruption
// vs a transient error like SQLITE_BUSY or lock contention.
func isCorruptionError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "malformed") ||
		strings.Contains(s, "not a database") ||
		strings.Contains(s, "disk image is malformed") ||
		strings.Contains(s, "file is not a database")
}

func (db *DB) integrityOK() bool {
	var result string
	err := db.conn.QueryRow("PRAGMA integrity_check").Scan(&result)
	return err == nil && result == "ok"
}

func (db *DB) ensureSchema() error {
	// Check if schema_meta exists
	var count int
	err := db.conn.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_meta'",
	).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Fresh DB — create schema
		if _, err := db.conn.Exec(schema); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		_, err := db.conn.Exec("INSERT INTO schema_meta (version) VALUES (?)", currentSchemaVersion)
		return err
	}

	// Schema exists — check version
	var version int
	if err := db.conn.QueryRow("SELECT version FROM schema_meta LIMIT 1").Scan(&version); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if version < currentSchemaVersion {
		return db.migrateSchema(version)
	}

	return nil
}

func (db *DB) migrateSchema(fromVersion int) error {
	if fromVersion < 2 {
		_, err := db.conn.Exec(`
			CREATE TABLE IF NOT EXISTS bead_metrics (
				bead_id TEXT PRIMARY KEY,
				theme TEXT NOT NULL,
				cycle_time_ms INTEGER NOT NULL,
				pass_first_attempt INTEGER NOT NULL DEFAULT 0,
				cost_usd REAL,
				completed_at INTEGER NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_bead_metrics_theme_completed
				ON bead_metrics(theme, completed_at DESC);
		`)
		if err != nil {
			return fmt.Errorf("migrate v1→v2: %w", err)
		}
	}
	if fromVersion < 3 {
		_, err := db.conn.Exec(`
			CREATE TABLE IF NOT EXISTS observation_metrics (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				theme TEXT NOT NULL,
				metric_type TEXT NOT NULL,
				value REAL NOT NULL,
				collected_at INTEGER NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_obs_theme_type
				ON observation_metrics(theme, metric_type, collected_at DESC);
		`)
		if err != nil {
			return fmt.Errorf("migrate v2→v3: %w", err)
		}
	}
	_, err := db.conn.Exec("UPDATE schema_meta SET version = ?", currentSchemaVersion)
	return err
}

// recover deletes the corrupt DB and recreates from scratch.
// Returns (true, nil) on success, (true, err) on partial recovery, (false, nil) if not attempted.
func (db *DB) recover() (bool, error) {
	fmt.Fprintf(os.Stderr, "ockham: signals.db corrupt or missing — recreating with defaults (all domains shadow)\n")

	// Remove the corrupt file and any WAL/SHM
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(db.path + suffix)
	}

	conn, err := sql.Open("sqlite", db.path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return true, fmt.Errorf("signals: recovery open: %w", err)
	}
	db.conn = conn
	db.wasRecovered = true

	if _, err := db.conn.Exec(schema); err != nil {
		return true, fmt.Errorf("signals: recovery schema: %w", err)
	}
	if _, err := db.conn.Exec("INSERT INTO schema_meta (version) VALUES (?)", currentSchemaVersion); err != nil {
		return true, fmt.Errorf("signals: recovery version: %w", err)
	}

	return true, nil
}

// SetSignalState writes a key-value pair with timestamp.
func (db *DB) SetSignalState(key, value string, updatedAt int64) error {
	_, err := db.conn.Exec(
		`INSERT INTO signal_state (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, updatedAt,
	)
	return err
}

// GetSignalState reads a value by key. Returns ("", false, nil) if not found.
func (db *DB) GetSignalState(key string) (string, bool, error) {
	var value string
	err := db.conn.QueryRow("SELECT value FROM signal_state WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}
