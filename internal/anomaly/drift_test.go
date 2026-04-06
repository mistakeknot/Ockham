package anomaly

import (
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func makeMetrics(cycleTimes []int64) []signals.BeadMetric {
	metrics := make([]signals.BeadMetric, len(cycleTimes))
	for i, ct := range cycleTimes {
		metrics[i] = signals.BeadMetric{
			BeadID:      string(rune('A' + i)),
			Theme:       "test",
			CycleTimeMs: ct,
			CompletedAt: int64(len(cycleTimes)-i) * 100, // DESC order
		}
	}
	return metrics
}

func TestEvaluateDrift_InsufficientData(t *testing.T) {
	cfg := DefaultConfig()
	prior := ThemeSignal{Theme: "test", Status: StatusCleared}
	metrics := makeMetrics([]int64{100, 200, 300}) // < MinWindow

	result := EvaluateDrift(metrics, prior, cfg)
	if result.Status != StatusCleared {
		t.Errorf("status = %q, want cleared (insufficient data)", result.Status)
	}
}

func TestEvaluateDrift_FiresOn20PctDrift(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinWindow = 10

	// Recent half (first 5): 1200ms each
	// Baseline half (last 5): 1000ms each
	// Drift = (1200-1000)/1000 = 0.20 = 20%
	times := make([]int64, 10)
	for i := 0; i < 5; i++ {
		times[i] = 1200 // recent (newer, DESC order means first)
	}
	for i := 5; i < 10; i++ {
		times[i] = 1000 // baseline (older)
	}

	prior := ThemeSignal{Theme: "test", Status: StatusCleared}
	result := EvaluateDrift(makeMetrics(times), prior, cfg)

	if result.Status != StatusFired {
		t.Errorf("status = %q, want fired", result.Status)
	}
	if result.AdvisoryOffset != -1 {
		t.Errorf("advisory = %d, want -1", result.AdvisoryOffset)
	}
}

func TestEvaluateDrift_Hysteresis_NoFireAt15Pct(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinWindow = 10

	// 15% drift — between fire (20%) and clear (10%) thresholds
	times := make([]int64, 10)
	for i := 0; i < 5; i++ {
		times[i] = 1150 // recent
	}
	for i := 5; i < 10; i++ {
		times[i] = 1000 // baseline
	}

	prior := ThemeSignal{Theme: "test", Status: StatusCleared}
	result := EvaluateDrift(makeMetrics(times), prior, cfg)

	if result.Status != StatusCleared {
		t.Errorf("status = %q, want cleared (15%% drift below fire threshold)", result.Status)
	}
}

func TestEvaluateDrift_Hysteresis_MaintainsFiredAt15Pct(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinWindow = 10

	// Already fired, drift drops to 15% — still above clear (10%), should stay fired
	times := make([]int64, 10)
	for i := 0; i < 5; i++ {
		times[i] = 1150
	}
	for i := 5; i < 10; i++ {
		times[i] = 1000
	}

	prior := ThemeSignal{Theme: "test", Status: StatusFired, AdvisoryOffset: -1}
	result := EvaluateDrift(makeMetrics(times), prior, cfg)

	if result.Status != StatusFired {
		t.Errorf("status = %q, want fired (hysteresis: 15%% above clear threshold)", result.Status)
	}
}

func TestEvaluateDrift_ClearsAfterConsecutiveEvals(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinWindow = 10
	cfg.ClearCount = 3

	// 5% drift — below clear threshold (10%)
	times := make([]int64, 10)
	for i := 0; i < 5; i++ {
		times[i] = 1050
	}
	for i := 5; i < 10; i++ {
		times[i] = 1000
	}

	// Simulate 3 consecutive evaluations below clear threshold
	prior := ThemeSignal{Theme: "test", Status: StatusFired, AdvisoryOffset: -1, ConsecutiveClears: 0}
	metrics := makeMetrics(times)

	for i := 0; i < 3; i++ {
		prior = EvaluateDrift(metrics, prior, cfg)
	}

	if prior.Status != StatusCleared {
		t.Errorf("status = %q after 3 consecutive clears, want cleared", prior.Status)
	}
	if prior.AdvisoryOffset != 0 {
		t.Errorf("advisory = %d, want 0 after clear", prior.AdvisoryOffset)
	}
}

func TestEvaluateDrift_AdaptiveWindow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinWindow = 10
	cfg.MaxWindow = 30

	// 30 beads — uses full window
	times := make([]int64, 30)
	for i := 0; i < 15; i++ {
		times[i] = 1000 // recent: no drift
	}
	for i := 15; i < 30; i++ {
		times[i] = 1000 // baseline: same
	}

	prior := ThemeSignal{Theme: "test", Status: StatusCleared}
	result := EvaluateDrift(makeMetrics(times), prior, cfg)

	if result.Status != StatusCleared {
		t.Errorf("status = %q, want cleared (no drift)", result.Status)
	}
}

func TestApplyFactoryGuard(t *testing.T) {
	sigs := map[string]ThemeSignal{
		"a": {Theme: "a", AdvisoryOffset: -5},
		"b": {Theme: "b", AdvisoryOffset: -5},
		"c": {Theme: "c", AdvisoryOffset: -5},
	}
	// Total reduction = 15, guard = 12 → need to reduce by 3

	result := ApplyFactoryGuard(sigs, 12)

	totalReduction := 0
	for _, s := range result {
		if s.AdvisoryOffset < 0 {
			totalReduction += -s.AdvisoryOffset
		}
	}
	if totalReduction > 12 {
		t.Errorf("total reduction = %d, want <= 12", totalReduction)
	}
}

func TestApplyFactoryGuard_WithinLimit(t *testing.T) {
	sigs := map[string]ThemeSignal{
		"a": {Theme: "a", AdvisoryOffset: -3},
		"b": {Theme: "b", AdvisoryOffset: -2},
	}
	// Total = 5, guard = 12 → no change needed

	result := ApplyFactoryGuard(sigs, 12)
	if result["a"].AdvisoryOffset != -3 || result["b"].AdvisoryOffset != -2 {
		t.Error("offsets should not change when within guard")
	}
}

func TestMedianCycleTime(t *testing.T) {
	tests := []struct {
		name   string
		times  []int64
		expect float64
	}{
		{"empty", nil, 0},
		{"single", []int64{100}, 100},
		{"odd", []int64{100, 200, 300}, 200},
		{"even", []int64{100, 200, 300, 400}, 250}, // (200+300)/2
		{"unsorted", []int64{300, 100, 200}, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := make([]signals.BeadMetric, len(tt.times))
			for i, ct := range tt.times {
				metrics[i] = signals.BeadMetric{CycleTimeMs: ct}
			}
			got := medianCycleTime(metrics)
			if got != tt.expect {
				t.Errorf("medianCycleTime = %f, want %f", got, tt.expect)
			}
		})
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid default config, got %v", err)
	}
}

func TestConfig_Validate_BypassThresholdTooLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BypassThreshold = 1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for bypass_threshold=1")
	}
	cfg.BypassThreshold = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for bypass_threshold=0")
	}
}
