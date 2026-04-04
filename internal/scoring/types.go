package scoring

// BeadInfo carries the data needed to score a single bead.
// Populated by the caller (CLI layer), not by the scoring package.
type BeadInfo struct {
	ID   string
	Lane string // empty → "open" theme
}

// WeightVector maps bead ID → additive offset for dispatch scoring.
type WeightVector struct {
	Offsets map[string]int
}
