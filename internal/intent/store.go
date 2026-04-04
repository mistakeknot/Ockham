package intent

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Store reads and writes intent files with atomic replacement.
type Store struct {
	path string
}

// NewStore creates a Store for the given YAML file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultStorePath returns ~/.config/ockham/intent.yaml.
func DefaultStorePath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "intent.yaml")
}

// Path returns the store's file path.
func (s *Store) Path() string { return s.path }

// Load reads the intent file. Returns default if missing. Returns error
// (with default as fallback) for I/O errors or corrupt YAML, so callers
// can log/surface the problem.
func (s *Store) Load() (IntentFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultFile(), nil
		}
		return DefaultFile(), fmt.Errorf("reading intent file: %w", err)
	}

	var f IntentFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return DefaultFile(), fmt.Errorf("parsing intent YAML: %w", err)
	}

	if f.Themes == nil {
		return DefaultFile(), nil
	}

	return f, nil
}

// Save writes the intent file atomically (write temp, rename).
func (s *Store) Save(f IntentFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling intent: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), "intent-*.yaml")
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

// Validate checks an IntentFile for correctness.
// Returns nil if valid, or a combined error via errors.Join.
func Validate(f IntentFile) error {
	var errs []error

	// Priority enum check
	for name, tb := range f.Themes {
		if _, err := ParsePriority(string(tb.Priority)); err != nil {
			errs = append(errs, fmt.Errorf("theme %q: %w", name, err))
		}
	}

	// Budget range check
	for name, tb := range f.Themes {
		if tb.Budget < 0 || tb.Budget > 1.0 {
			errs = append(errs, fmt.Errorf("theme %q: budget %f out of range [0, 1]", name, tb.Budget))
		}
	}

	// Budgets must sum to 1.0 (tolerance: 0.001)
	var total float64
	for _, tb := range f.Themes {
		total += tb.Budget
	}
	if math.Abs(total-1.0) > 0.001 {
		errs = append(errs, fmt.Errorf("budgets sum to %f, must equal 1.0", total))
	}

	// Freeze/focus entries must reference declared themes
	for _, lane := range f.Constraints.Freeze {
		if _, ok := f.Themes[lane]; !ok {
			errs = append(errs, fmt.Errorf("freeze references unknown theme %q", lane))
		}
	}
	for _, lane := range f.Constraints.Focus {
		if _, ok := f.Themes[lane]; !ok {
			errs = append(errs, fmt.Errorf("focus references unknown theme %q", lane))
		}
	}

	return errors.Join(errs...)
}
