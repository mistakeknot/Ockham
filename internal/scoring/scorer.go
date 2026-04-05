package scoring

import (
	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/authority"
	"github.com/mistakeknot/Ockham/internal/intent"
)

const (
	// OffsetMin is the minimum clamped offset.
	OffsetMin = -6
	// OffsetMax is the maximum clamped offset.
	OffsetMax = 6
)

// Score computes per-bead weight offsets from intent, authority, and anomaly state.
// Dependency direction: scoring imports intent, authority, anomaly. Governor imports all.
func Score(iv intent.IntentVector, _ authority.State, an anomaly.State, beads []BeadInfo) WeightVector {
	wv := WeightVector{
		Offsets:    make(map[string]int, len(beads)),
		RawOffsets: make(map[string]int, len(beads)),
	}

	for _, b := range beads {
		lane := b.Lane
		if lane == "" {
			lane = "open"
		}

		intentOffset, ok := iv.Offsets[lane]
		if !ok {
			intentOffset = 0 // unknown theme → neutral
		}

		rawOffset := clamp(intentOffset, OffsetMin, OffsetMax)
		wv.RawOffsets[b.ID] = rawOffset

		// Apply anomaly advisory offset (nil-safe: zero value if map is nil or key missing)
		advisory := 0
		if an.Signals != nil {
			if sig, ok := an.Signals[lane]; ok {
				advisory = sig.AdvisoryOffset
			}
		}

		wv.Offsets[b.ID] = clamp(intentOffset+advisory, OffsetMin, OffsetMax)
	}

	return wv
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
