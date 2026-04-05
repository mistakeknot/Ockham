package anomaly

import "github.com/mistakeknot/Ockham/internal/signals"

const trendTolerance = 0.05 // 5% change threshold for stable vs improving/degrading

// EvaluatePassRate computes the first_attempt_pass_rate trend.
// Compares pass rate of recent half vs baseline half.
func EvaluatePassRate(metrics []signals.BeadMetric, minWindow int, theme string) PleasureSignal {
	sig := PleasureSignal{Name: "first_attempt_pass_rate", Theme: theme}

	if len(metrics) < minWindow {
		sig.Trend = TrendInsufficient
		return sig
	}

	mid := len(metrics) / 2
	recentRate := passRate(metrics[:mid])
	baselineRate := passRate(metrics[mid:])
	sig.Value = recentRate

	sig.Trend = compareTrend(recentRate, baselineRate, true) // higher is better
	return sig
}

// EvaluateCycleTimeTrend computes the cycle_time_p50 trend.
// Improving means lower (faster). Degrading means higher (slower).
func EvaluateCycleTimeTrend(metrics []signals.BeadMetric, minWindow int, theme string) PleasureSignal {
	sig := PleasureSignal{Name: "cycle_time_p50_trend", Theme: theme}

	if len(metrics) < minWindow {
		sig.Trend = TrendInsufficient
		return sig
	}

	mid := len(metrics) / 2
	recentP50 := medianCycleTime(metrics[:mid])
	baselineP50 := medianCycleTime(metrics[mid:])
	sig.Value = recentP50

	sig.Trend = compareTrend(recentP50, baselineP50, false) // lower is better
	return sig
}

// EvaluateCostTrend computes the cost_per_landed_change trend.
// Only considers metrics with non-nil CostUSD. Improving means lower cost.
func EvaluateCostTrend(metrics []signals.BeadMetric, minWindow int, theme string) PleasureSignal {
	sig := PleasureSignal{Name: "cost_per_landed_change_trend", Theme: theme}

	// Filter to metrics with cost data
	var withCost []signals.BeadMetric
	for _, m := range metrics {
		if m.CostUSD != nil {
			withCost = append(withCost, m)
		}
	}

	if len(withCost) < minWindow {
		sig.Trend = TrendInsufficient
		return sig
	}

	mid := len(withCost) / 2
	recentAvg := avgCost(withCost[:mid])
	baselineAvg := avgCost(withCost[mid:])
	sig.Value = recentAvg

	sig.Trend = compareTrend(recentAvg, baselineAvg, false) // lower is better
	return sig
}

// compareTrend determines the trend direction.
// higherIsBetter: true for pass rate, false for cycle time and cost.
func compareTrend(recent, baseline float64, higherIsBetter bool) PleasureTrend {
	if baseline == 0 {
		return TrendStable
	}
	change := (recent - baseline) / baseline

	if higherIsBetter {
		if change > trendTolerance {
			return TrendImproving
		}
		if change < -trendTolerance {
			return TrendDegrading
		}
	} else {
		if change < -trendTolerance {
			return TrendImproving // lower is better
		}
		if change > trendTolerance {
			return TrendDegrading
		}
	}
	return TrendStable
}

func passRate(metrics []signals.BeadMetric) float64 {
	if len(metrics) == 0 {
		return 0
	}
	passed := 0
	for _, m := range metrics {
		if m.PassFirstAttempt {
			passed++
		}
	}
	return float64(passed) / float64(len(metrics))
}

func avgCost(metrics []signals.BeadMetric) float64 {
	if len(metrics) == 0 {
		return 0
	}
	var total float64
	var count int
	for _, m := range metrics {
		if m.CostUSD != nil {
			total += *m.CostUSD
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}
