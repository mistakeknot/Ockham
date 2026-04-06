package halt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sentinel checks factory halt state via a filesystem sentinel file.
type Sentinel struct {
	path string
}

// New creates a Sentinel checking the given file path.
func New(path string) *Sentinel {
	return &Sentinel{path: path}
}

// DefaultSentinelPath returns ~/.config/ockham/factory-paused.json.
func DefaultSentinelPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "factory-paused.json")
}

// IsHalted returns true if the sentinel file exists.
func (s *Sentinel) IsHalted() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// Path returns the sentinel file path.
func (s *Sentinel) Path() string {
	return s.path
}

// RequireRunning returns an error if the factory is halted.
// Reads factory-paused.json for context (reason, timestamp) when available.
func (s *Sentinel) RequireRunning() error {
	if !s.IsHalted() {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("factory halted: %s exists — run 'ockham resume --confirm' first", s.path)
	}
	var record struct {
		Reason    string `json:"reason"`
		Timestamp int64  `json:"timestamp"`
	}
	if json.Unmarshal(data, &record) != nil || record.Reason == "" {
		return fmt.Errorf("factory halted: %s exists — run 'ockham resume --confirm' first", s.path)
	}
	t := time.Unix(record.Timestamp, 0).Format(time.RFC3339)
	return fmt.Errorf("factory halted since %s (reason: %s) — run 'ockham resume --confirm' first", t, record.Reason)
}
