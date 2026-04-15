package observation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mistakeknot/Alwe/pkg/observer"
)

// CassObserver wraps Alwe's CassObserver to implement the Observer interface.
type CassObserver struct {
	mu             sync.Mutex
	cass           *observer.CassObserver
	availableCache *bool
	cacheExpiry    time.Time
}

// NewCassObserver creates a CassObserver. Returns (observer, nil) even if CASS
// is unavailable — availability is checked lazily via IsAvailable().
func NewCassObserver() *CassObserver {
	return &CassObserver{}
}

// IsAvailable reports whether CASS is installed. Caches the result for 60 seconds
// to avoid repeated PATH lookups.
func (o *CassObserver) IsAvailable() bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.availableCache != nil && time.Now().Before(o.cacheExpiry) {
		return *o.availableCache
	}

	cass, err := observer.New()
	available := err == nil
	o.availableCache = &available
	o.cacheExpiry = time.Now().Add(60 * time.Second)
	if available {
		o.cass = cass
	}
	return available
}

// Collect gathers session_completion_rate and tool_error_rate metrics.
// Returns nil, nil if CASS is unavailable (degradation contract).
func (o *CassObserver) Collect(ctx context.Context, themes []string, since time.Duration) ([]ObservationMetric, error) {
	if !o.IsAvailable() {
		return nil, nil
	}

	now := time.Now().Unix()
	sinceStr := durationToString(since)

	// Get timeline data
	rawTimeline, err := o.cass.Timeline(ctx, sinceStr)
	if err != nil {
		return nil, fmt.Errorf("timeline: %w", err)
	}

	entries, err := parseTimeline(rawTimeline)
	if err != nil {
		return nil, fmt.Errorf("parse timeline: %w", err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	// Session completion rate: done / total
	var done, total int
	for _, e := range entries {
		total++
		if e.Status == "done" {
			done++
		}
	}

	var metrics []ObservationMetric
	completionRate := 0.0
	if total > 0 {
		completionRate = float64(done) / float64(total)
	}
	// Use "open" as default theme for F1 (matches bead default lane)
	theme := "open"
	if len(themes) > 0 {
		theme = themes[0]
	}

	metrics = append(metrics, ObservationMetric{
		Theme:       theme,
		MetricType:  "session_completion_rate",
		Value:       completionRate,
		CollectedAt: now,
	})

	// Tool error rate: use session-count proxy (error-matching sessions / total)
	errorSessions, err := o.cass.SearchSessions(ctx, "error", "", 100)
	if err != nil {
		return metrics, nil // degrade gracefully — return what we have
	}

	errorRate := 0.0
	if total > 0 {
		errorRate = float64(len(errorSessions)) / float64(total)
		if errorRate > 1.0 {
			errorRate = 1.0
		}
	}
	metrics = append(metrics, ObservationMetric{
		Theme:       theme,
		MetricType:  "tool_error_rate",
		Value:       errorRate,
		CollectedAt: now,
	})

	return metrics, nil
}

// timelineEntry matches the CASS timeline JSON schema.
type timelineEntry struct {
	SessionID string `json:"session_id"`
	Provider  string `json:"provider"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
}

func parseTimeline(raw string) ([]timelineEntry, error) {
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}
	var entries []timelineEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// durationToString converts a time.Duration to the format CASS expects (e.g., "24h", "7d").
func durationToString(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
