package anomaly

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func makePassMetrics(recent, baseline []bool) []signals.BeadMetric {
	var metrics []signals.BeadMetric
	ts := int64(1000)
	for _, pass := range recent {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(len(metrics) + 65)), Theme: "test",
			CycleTimeMs: 1000, PassFirstAttempt: pass, CompletedAt: ts,
		})
		ts -= 10
	}
	for _, pass := range baseline {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(len(metrics) + 65)), Theme: "test",
			CycleTimeMs: 1000, PassFirstAttempt: pass, CompletedAt: ts,
		})
		ts -= 10
	}
	return metrics
}

func TestEvaluatePassRate_Improving(t *testing.T) {
	// Recent: 4/5 pass (80%), Baseline: 2/5 pass (40%)
	metrics := makePassMetrics(
		[]bool{true, true, true, true, false},
		[]bool{true, true, false, false, false},
	)
	sig := EvaluatePassRate(metrics, 10, "test")
	if sig.Trend != TrendImproving {
		t.Errorf("trend = %q, want improving", sig.Trend)
	}
}

func TestEvaluatePassRate_Degrading(t *testing.T) {
	// Recent: 1/5 pass (20%), Baseline: 4/5 pass (80%)
	metrics := makePassMetrics(
		[]bool{true, false, false, false, false},
		[]bool{true, true, true, true, false},
	)
	sig := EvaluatePassRate(metrics, 10, "test")
	if sig.Trend != TrendDegrading {
		t.Errorf("trend = %q, want degrading", sig.Trend)
	}
}

func TestEvaluatePassRate_Stable(t *testing.T) {
	// Recent: 3/5 (60%), Baseline: 3/5 (60%)
	metrics := makePassMetrics(
		[]bool{true, true, true, false, false},
		[]bool{true, true, true, false, false},
	)
	sig := EvaluatePassRate(metrics, 10, "test")
	if sig.Trend != TrendStable {
		t.Errorf("trend = %q, want stable", sig.Trend)
	}
}

func TestEvaluatePassRate_InsufficientData(t *testing.T) {
	metrics := makePassMetrics([]bool{true, true}, []bool{true})
	sig := EvaluatePassRate(metrics, 10, "test")
	if sig.Trend != TrendInsufficient {
		t.Errorf("trend = %q, want insufficient_data", sig.Trend)
	}
}

func TestEvaluateCycleTimeTrend_Improving(t *testing.T) {
	// Recent: faster (lower p50), Baseline: slower
	var metrics []signals.BeadMetric
	for i := 0; i < 5; i++ {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 65)), Theme: "test",
			CycleTimeMs: 800, CompletedAt: int64(1000 - i*10),
		})
	}
	for i := 0; i < 5; i++ {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 70)), Theme: "test",
			CycleTimeMs: 1200, CompletedAt: int64(500 - i*10),
		})
	}

	sig := EvaluateCycleTimeTrend(metrics, 10, "test")
	if sig.Trend != TrendImproving {
		t.Errorf("trend = %q, want improving (faster cycle time)", sig.Trend)
	}
}

func TestEvaluateCycleTimeTrend_Degrading(t *testing.T) {
	var metrics []signals.BeadMetric
	for i := 0; i < 5; i++ {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 65)), Theme: "test",
			CycleTimeMs: 1500, CompletedAt: int64(1000 - i*10),
		})
	}
	for i := 0; i < 5; i++ {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 70)), Theme: "test",
			CycleTimeMs: 1000, CompletedAt: int64(500 - i*10),
		})
	}

	sig := EvaluateCycleTimeTrend(metrics, 10, "test")
	if sig.Trend != TrendDegrading {
		t.Errorf("trend = %q, want degrading (slower cycle time)", sig.Trend)
	}
}

func TestEvaluateCostTrend_InsufficientData_AllNil(t *testing.T) {
	var metrics []signals.BeadMetric
	for i := 0; i < 10; i++ {
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 65)), Theme: "test",
			CycleTimeMs: 1000, CostUSD: nil, CompletedAt: int64(1000 - i*10),
		})
	}

	sig := EvaluateCostTrend(metrics, 10, "test")
	if sig.Trend != TrendInsufficient {
		t.Errorf("trend = %q, want insufficient_data (all nil cost)", sig.Trend)
	}
}

func TestEvaluateCostTrend_Improving(t *testing.T) {
	var metrics []signals.BeadMetric
	for i := 0; i < 5; i++ {
		cost := 1.50 // recent: cheaper
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 65)), Theme: "test",
			CycleTimeMs: 1000, CostUSD: &cost, CompletedAt: int64(1000 - i*10),
		})
	}
	for i := 0; i < 5; i++ {
		cost := 3.00 // baseline: more expensive
		metrics = append(metrics, signals.BeadMetric{
			BeadID: string(rune(i + 70)), Theme: "test",
			CycleTimeMs: 1000, CostUSD: &cost, CompletedAt: int64(500 - i*10),
		})
	}

	sig := EvaluateCostTrend(metrics, 10, "test")
	if sig.Trend != TrendImproving {
		t.Errorf("trend = %q, want improving (lower cost)", sig.Trend)
	}
}
