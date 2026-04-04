package halt

import (
	"os"
	"path/filepath"
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
