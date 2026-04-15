// Package observation bridges external agent observation data into Ockham's
// anomaly evaluator. The Observer interface abstracts the data source so tests
// can run without a live CASS installation.
package observation

import (
	"context"
	"time"
)

// ObservationMetric is a single metric collected from an observation source.
type ObservationMetric struct {
	Theme       string
	MetricType  string  // "session_completion_rate", "tool_error_rate"
	Value       float64 // 0.0-1.0 for rates
	CollectedAt int64   // unix epoch
}

// Observer collects agent outcome metrics from an external source.
type Observer interface {
	IsAvailable() bool
	Collect(ctx context.Context, themes []string, since time.Duration) ([]ObservationMetric, error)
}
