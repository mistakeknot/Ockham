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
func Score(iv intent.IntentVector, _ authority.State, _ anomaly.State, beads []BeadInfo) WeightVector {
	wv := WeightVector{Offsets: make(map[string]int, len(beads))}

	for _, b := range beads {
		lane := b.Lane
		if lane == "" {
			lane = "open"
		}

		offset, ok := iv.Offsets[lane]
		if !ok {
			offset = 0 // unknown theme → neutral
		}

		wv.Offsets[b.ID] = clamp(offset, OffsetMin, OffsetMax)
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
