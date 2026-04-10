package scoring

// BeadInfo carries the data needed to score a single bead.
// Populated by the caller (CLI layer), not by the scoring package.
type BeadInfo struct {
	ID   string
	Lane string // empty → "open" theme
	Org  string // github org
	Repo string // repo name
}

// WeightVector maps bead ID → additive offset for dispatch scoring.
type WeightVector struct {
	Offsets    map[string]int // final offsets (intent + advisory, clamped)
	RawOffsets map[string]int // intent-only offsets (for dual logging)
}
