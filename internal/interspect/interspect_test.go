package interspect

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReaderMissingFile verifies fail-open behavior when evidence file is absent.
func TestReaderMissingFile(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "nonexistent.json")

	r := NewReader(path)
	evidence, found, err := r.Load()

	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if found {
		t.Fatal("Load() should return found=false for missing file")
	}
	if !evidence.GeneratedAt.IsZero() || evidence.OverallPassRate != 0 || evidence.RetryRate != 0 || evidence.Categories != nil || evidence.HighRetryThemes != nil {
		t.Fatalf("Load() should return zero Evidence, got %+v", evidence)
	}
}

// TestReaderMalformedJSON verifies error handling on invalid JSON.
func TestReaderMalformedJSON(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "bad.json")

	if err := os.WriteFile(path, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	_, found, err := r.Load()

	if err == nil {
		t.Fatal("Load() should return error on malformed JSON")
	}
	if found {
		t.Fatal("Load() should return found=false when error occurs")
	}
}

// TestReaderValidFile verifies successful parsing of well-formed evidence.
func TestReaderValidFile(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "valid.json")

	jsonData := `{
		"schema_version": 1,
		"generated_at": "2026-03-07T15:52:53Z",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0,
		"categories": {
			"exploration": {
				"count": 2,
				"pass_rate": 0.5,
				"avg_duration_s": 19.0
			},
			"implementation": {
				"count": 5,
				"pass_rate": 0.8,
				"avg_duration_s": 45.0
			}
		},
		"high_retry_categories": ["exploration"]
	}`

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	evidence, found, err := r.Load()

	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if !found {
		t.Fatal("Load() should return found=true for valid file")
	}

	if evidence.OverallPassRate != 0.67 {
		t.Errorf("OverallPassRate: got %v, want 0.67", evidence.OverallPassRate)
	}
	if evidence.RetryRate != 0 {
		t.Errorf("RetryRate: got %v, want 0", evidence.RetryRate)
	}

	if exp, ok := evidence.Categories["exploration"]; !ok {
		t.Fatal("exploration category missing")
	} else {
		if exp.Count != 2 {
			t.Errorf("exploration.Count: got %v, want 2", exp.Count)
		}
		if exp.PassRate != 0.5 {
			t.Errorf("exploration.PassRate: got %v, want 0.5", exp.PassRate)
		}
		if exp.AvgDurationS != 19.0 {
			t.Errorf("exploration.AvgDurationS: got %v, want 19.0", exp.AvgDurationS)
		}
	}

	if impl, ok := evidence.Categories["implementation"]; !ok {
		t.Fatal("implementation category missing")
	} else {
		if impl.Count != 5 {
			t.Errorf("implementation.Count: got %v, want 5", impl.Count)
		}
		if impl.PassRate != 0.8 {
			t.Errorf("implementation.PassRate: got %v, want 0.8", impl.PassRate)
		}
	}

	if len(evidence.HighRetryThemes) != 1 || evidence.HighRetryThemes[0] != "exploration" {
		t.Errorf("HighRetryThemes: got %v, want [exploration]", evidence.HighRetryThemes)
	}
}

// TestCheckerMissingFile returns VerdictUnknown when evidence is absent.
func TestCheckerMissingFile(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "nonexistent.json")

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())

	verdict, err := c.AgreesUnhealthy("some-theme")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictUnknown {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictUnknown", verdict)
	}
}

// TestCheckerStaleEvidence returns VerdictUnknown when evidence is too old.
func TestCheckerStaleEvidence(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "stale.json")

	// Generate evidence that's 2 hours old (default MaxStalenessSeconds is 3600).
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour)

	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0,
		"categories": {},
		"high_retry_categories": []
	}`, twoHoursAgo.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("exploration")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictUnknown {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictUnknown (stale evidence)", verdict)
	}
}

// TestCheckerStalenessBoundary verifies the exact staleness threshold.
func TestCheckerStalenessBoundary(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "boundary.json")

	now := time.Now()
	policy := DefaultPolicy()
	maxAge := time.Duration(policy.MaxStalenessSeconds) * time.Second

	// Test evidence exactly at boundary (just inside acceptable).
	justInside := now.Add(-maxAge + time.Second)

	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0,
		"categories": {},
		"high_retry_categories": []
	}`, justInside.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, policy)
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("exploration")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict == VerdictUnknown {
		t.Errorf("AgreesUnhealthy(): should accept evidence just inside staleness window")
	}
}

// TestCheckerHighRetryThemesMatch returns VerdictUnhealthy when theme is in HighRetryThemes.
func TestCheckerHighRetryThemesMatch(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "retry.json")

	now := time.Now()
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0.1,
		"categories": {},
		"high_retry_categories": ["exploration", "refactor"]
	}`, now.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("refactor")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictUnhealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictUnhealthy (high-retry match)", verdict)
	}
}

// TestCheckerUnhealthyPassRate returns VerdictUnhealthy when category pass rate is below threshold.
func TestCheckerUnhealthyPassRate(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "unhealthy.json")

	now := time.Now()
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.3,
		"total_delegations": 10,
		"retry_rate": 0.2,
		"categories": {
			"auth": {
				"count": 10,
				"pass_rate": 0.3,
				"avg_duration_s": 30.0
			}
		},
		"high_retry_categories": []
	}`, now.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("auth")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictUnhealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictUnhealthy (low pass rate)", verdict)
	}
}

// TestCheckerInsufficientSample returns VerdictHealthy when category count is below minimum.
func TestCheckerInsufficientSample(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "insufficient.json")

	now := time.Now()
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0,
		"categories": {
			"performance": {
				"count": 2,
				"pass_rate": 0.2,
				"avg_duration_s": 50.0
			}
		},
		"high_retry_categories": []
	}`, now.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	policy := DefaultPolicy()
	c := NewChecker(r, policy)
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("performance")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictHealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictHealthy (insufficient sample)", verdict)
	}
}

// TestCheckerMinCategoryCountBoundary verifies the exact sample size threshold.
func TestCheckerMinCategoryCountBoundary(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "count_boundary.json")

	now := time.Now()
	policy := DefaultPolicy()
	minCount := policy.MinCategoryCount

	// Test evidence with exactly minCount observations.
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.3,
		"total_delegations": %d,
		"retry_rate": 0.2,
		"categories": {
			"testing": {
				"count": %d,
				"pass_rate": 0.1,
				"avg_duration_s": 20.0
			}
		},
		"high_retry_categories": []
	}`, now.Format(time.RFC3339), minCount, minCount)

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, policy)
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("testing")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictUnhealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictUnhealthy (at minimum count)", verdict)
	}
}

// TestCheckerHealthyPassRate returns VerdictHealthy when category pass rate meets threshold.
func TestCheckerHealthyPassRate(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "healthy.json")

	now := time.Now()
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.9,
		"total_delegations": 10,
		"retry_rate": 0.05,
		"categories": {
			"build": {
				"count": 10,
				"pass_rate": 0.9,
				"avg_duration_s": 5.0
			}
		},
		"high_retry_categories": []
	}`, now.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("build")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictHealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictHealthy (good pass rate)", verdict)
	}
}

// TestCheckerThemeNotInEvidence returns VerdictHealthy when theme has no category entry.
func TestCheckerThemeNotInEvidence(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "no_theme.json")

	now := time.Now()
	jsonData := fmt.Sprintf(`{
		"schema_version": 1,
		"generated_at": "%s",
		"overall_pass_rate": 0.67,
		"total_delegations": 3,
		"retry_rate": 0,
		"categories": {
			"exploration": {
				"count": 2,
				"pass_rate": 0.5,
				"avg_duration_s": 19.0
			}
		},
		"high_retry_categories": []
	}`, now.Format(time.RFC3339))

	if err := os.WriteFile(path, []byte(jsonData), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReader(path)
	c := NewChecker(r, DefaultPolicy())
	c.now = func() time.Time { return now }

	verdict, err := c.AgreesUnhealthy("unknown-theme")

	if err != nil {
		t.Fatalf("AgreesUnhealthy() returned unexpected error: %v", err)
	}
	if verdict != VerdictHealthy {
		t.Errorf("AgreesUnhealthy(): got %v, want VerdictHealthy (theme not in evidence)", verdict)
	}
}

// TestDefaultPath verifies the default evidence file location.
func TestDefaultPath(t *testing.T) {
	path := DefaultPath()

	if path == "" {
		t.Fatal("DefaultPath() returned empty string")
	}

	// Should contain .clavain/interspect/delegation-calibration.json
	if !contains(path, ".clavain") || !contains(path, "interspect") || !contains(path, "delegation-calibration.json") {
		t.Errorf("DefaultPath() returned unexpected path: %s", path)
	}
}

// TestNewReaderEmptyPath uses DefaultPath when path is empty.
func TestNewReaderEmptyPath(t *testing.T) {
	r := NewReader("")

	if r.path == "" {
		t.Fatal("NewReader(\"\") did not use DefaultPath()")
	}

	defaultPath := DefaultPath()
	if r.path != defaultPath {
		t.Errorf("NewReader(\"\") path: got %s, want %s", r.path, defaultPath)
	}
}

// TestVerdictString tests the String() method on Verdict.
func TestVerdictString(t *testing.T) {
	tests := []struct {
		v    Verdict
		want string
	}{
		{VerdictUnknown, "Unknown"},
		{VerdictHealthy, "Healthy"},
		{VerdictUnhealthy, "Unhealthy"},
	}

	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("Verdict.String(): got %s, want %s", got, tt.want)
		}
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
