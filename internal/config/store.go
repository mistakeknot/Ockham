package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Store reads and writes ockham.yaml with atomic replacement.
type Store struct {
	path string
}

// NewStore creates a Store for the given YAML file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultStorePath returns ~/.config/ockham/ockham.yaml.
func DefaultStorePath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "ockham.yaml")
}

// Path returns the store's file path.
func (s *Store) Path() string { return s.path }

// Load reads the config file. Returns default if missing. Returns error
// (with default as fallback) for I/O errors or corrupt YAML, so callers
// can log/surface the problem.
func (s *Store) Load() (ConfigFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("reading config file: %w", err)
	}

	var f ConfigFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return DefaultConfig(), fmt.Errorf("parsing config YAML: %w", err)
	}

	if f.Version == 0 {
		return DefaultConfig(), nil
	}

	return f, nil
}

// Save writes the config file atomically (write temp, rename).
func (s *Store) Save(f ConfigFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), "ockham-*.yaml")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// Validate checks a ConfigFile for correctness.
// Returns nil if valid, or a combined error via errors.Join.
func Validate(f ConfigFile) error {
	var errs []error

	if f.Version != 1 {
		errs = append(errs, fmt.Errorf("unsupported config version %d (expected 1)", f.Version))
	}

	if len(f.Orgs) == 0 {
		errs = append(errs, fmt.Errorf("orgs must not be empty"))
	}
	for i, org := range f.Orgs {
		if org.Name == "" {
			errs = append(errs, fmt.Errorf("org[%d]: name must not be empty", i))
		}
	}

	for name, agent := range f.Agents {
		if agent.Runtime == "" {
			errs = append(errs, fmt.Errorf("agent %q: runtime must not be empty", name))
		}
		if agent.CostTier < 0 || agent.CostTier > 3 {
			errs = append(errs, fmt.Errorf("agent %q: cost_tier %d out of range [0, 3]", name, agent.CostTier))
		}
	}

	// Tiers 0-3 must be defined
	for tier := 0; tier <= 3; tier++ {
		if _, ok := f.Cost.Tiers[tier]; !ok {
			errs = append(errs, fmt.Errorf("cost tier %d not defined", tier))
		}
	}
	for tier, def := range f.Cost.Tiers {
		if tier < 0 || tier > 3 {
			errs = append(errs, fmt.Errorf("cost tier %d out of range [0, 3]", tier))
		}
		if def.Name == "" {
			errs = append(errs, fmt.Errorf("cost tier %d: name must not be empty", tier))
		}
	}

	return errors.Join(errs...)
}
