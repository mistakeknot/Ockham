// Package interspect implements F5 signal layer pairing with the interspect
// delegation-calibration evidence. Before an Ockham anomaly trigger escalates
// to CONSTRAIN, both Ockham's evaluator AND interspect's evidence must agree
// the theme is unhealthy.
package interspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Evidence captures the subset of interspect delegation-calibration we care about.
type Evidence struct {
	GeneratedAt     time.Time
	OverallPassRate float64
	RetryRate       float64
	Categories      map[string]CategoryEvidence
	HighRetryThemes []string
}

// CategoryEvidence represents observation data for a single theme category.
type CategoryEvidence struct {
	Count        int
	PassRate     float64
	AvgDurationS float64
}

// Reader loads and parses interspect delegation-calibration.json.
type Reader struct {
	path string
	now  func() time.Time
}

// NewReader constructs a Reader pointing at the given file path.
// When path is empty, DefaultPath() is used.
func NewReader(path string) *Reader {
	if path == "" {
		path = DefaultPath()
	}
	return &Reader{
		path: path,
		now:  time.Now,
	}
}

// DefaultPath returns the default location of interspect evidence.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".clavain", "interspect", "delegation-calibration.json")
}

// Load reads and parses the evidence file. Returns (zero, false, nil) when the
// file does not exist (fail-open). Returns an error only on malformed JSON or
// read errors other than file-not-found.
func (r *Reader) Load() (Evidence, bool, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Evidence{}, false, nil
		}
		return Evidence{}, false, fmt.Errorf("interspect: read failed: %w", err)
	}

	var raw struct {
		SchemaVersion    int     `json:"schema_version"`
		GeneratedAt      string  `json:"generated_at"`
		OverallPassRate  float64 `json:"overall_pass_rate"`
		TotalDelegations int     `json:"total_delegations"`
		RetryRate        float64 `json:"retry_rate"`
		Categories       map[string]struct {
			Count        int     `json:"count"`
			PassRate     float64 `json:"pass_rate"`
			AvgDurationS float64 `json:"avg_duration_s"`
		} `json:"categories"`
		HighRetryCategories []string `json:"high_retry_categories"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return Evidence{}, false, fmt.Errorf("interspect: JSON parse failed: %w", err)
	}

	generated, err := time.Parse(time.RFC3339, raw.GeneratedAt)
	if err != nil {
		return Evidence{}, false, fmt.Errorf("interspect: invalid generated_at timestamp: %w", err)
	}

	evidence := Evidence{
		GeneratedAt:     generated,
		OverallPassRate: raw.OverallPassRate,
		RetryRate:       raw.RetryRate,
		Categories:      make(map[string]CategoryEvidence),
		HighRetryThemes: raw.HighRetryCategories,
	}

	for name, cat := range raw.Categories {
		evidence.Categories[name] = CategoryEvidence{
			Count:        cat.Count,
			PassRate:     cat.PassRate,
			AvgDurationS: cat.AvgDurationS,
		}
	}

	return evidence, true, nil
}

// Verdict indicates whether interspect evidence agrees the theme is unhealthy.
type Verdict int

const (
	VerdictUnknown   Verdict = iota // no evidence / stale / insufficient sample
	VerdictHealthy                  // interspect disagrees, pairing should block fire
	VerdictUnhealthy                // interspect agrees, pairing green-lights fire
)

// String provides a human-readable label for Verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictUnknown:
		return "Unknown"
	case VerdictHealthy:
		return "Healthy"
	case VerdictUnhealthy:
		return "Unhealthy"
	default:
		return fmt.Sprintf("Verdict(%d)", v)
	}
}

// Policy configures thresholds for evidence evaluation.
type Policy struct {
	MaxStalenessSeconds int64   // evidence older than this is "unknown", default 3600
	MinPassRate         float64 // below → theme unhealthy, default 0.5
	MinCategoryCount    int     // minimum observations to trust verdict, default 3
}

// DefaultPolicy returns standard thresholds.
func DefaultPolicy() Policy {
	return Policy{
		MaxStalenessSeconds: 3600,
		MinPassRate:         0.5,
		MinCategoryCount:    3,
	}
}

// Checker combines Reader and Policy to evaluate theme health.
type Checker struct {
	reader *Reader
	policy Policy
	now    func() time.Time
}

// NewChecker constructs a Checker with the given Reader and Policy.
func NewChecker(r *Reader, p Policy) *Checker {
	return &Checker{
		reader: r,
		policy: p,
		now:    time.Now,
	}
}

// AgreesUnhealthy evaluates whether interspect evidence agrees that theme is
// unhealthy. Returns VerdictUnknown when evidence is absent or stale, or when
// the sample is too small to trust. Returns VerdictUnhealthy when either:
//   - theme is listed in HighRetryThemes, OR
//   - theme has a Categories entry with Count >= MinCategoryCount and PassRate < MinPassRate
//
// Otherwise returns VerdictHealthy.
func (c *Checker) AgreesUnhealthy(theme string) (Verdict, error) {
	evidence, found, err := c.reader.Load()
	if err != nil {
		return VerdictUnknown, err
	}

	if !found {
		return VerdictUnknown, nil
	}

	// Check staleness.
	age := c.now().Sub(evidence.GeneratedAt).Seconds()
	if age > float64(c.policy.MaxStalenessSeconds) {
		return VerdictUnknown, nil
	}

	// Check HighRetryThemes.
	for _, t := range evidence.HighRetryThemes {
		if t == theme {
			return VerdictUnhealthy, nil
		}
	}

	// Check Categories entry.
	cat, ok := evidence.Categories[theme]
	if ok && cat.Count >= c.policy.MinCategoryCount && cat.PassRate < c.policy.MinPassRate {
		return VerdictUnhealthy, nil
	}

	return VerdictHealthy, nil
}
