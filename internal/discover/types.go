package discover

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/config"
)

// DiscoveredBead represents a bead found in an org's backlog.
type DiscoveredBead struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority string `json:"priority"`
	Lane     string `json:"lane"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Org      string `json:"org"`
	Repo     string `json:"repo"`
}

// WorkQueue is a merged, sorted collection of beads across orgs.
type WorkQueue struct {
	Beads     []DiscoveredBead `json:"beads"`
	Sources   []Source         `json:"sources"`
	Errors    []SourceError    `json:"errors,omitempty"`
	FetchedAt int64            `json:"fetched_at"`
}

// Source records which orgs were scanned.
type Source struct {
	Org       string `json:"org"`
	Repo      string `json:"repo,omitempty"`
	Path      string `json:"path"`
	BeadCount int    `json:"bead_count"`
}

// SourceError records a failed scan.
type SourceError struct {
	Org   string `json:"org"`
	Error string `json:"error"`
}

// Discoverer scans orgs and collects beads.
type Discoverer struct {
	configStore *config.Store
	wsStore     *WorkspaceStore
}

// NewDiscoverer creates a Discoverer with the given config and workspace stores.
func NewDiscoverer(configStore *config.Store, wsStore *WorkspaceStore) *Discoverer {
	return &Discoverer{
		configStore: configStore,
		wsStore:     wsStore,
	}
}

// priorityIndex returns a sort key for priority strings.
// P0=0, P1=1, P2=2, P3=3, P4=4. Unknown defaults to 2 (P2).
func priorityIndex(p string) int {
	s := strings.TrimPrefix(strings.ToUpper(p), "P")
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 4 {
		return 2 // default P2
	}
	return n
}

// SortBeads sorts beads by priority (P0 first), then by ID for stability.
func SortBeads(beads []DiscoveredBead) {
	sort.SliceStable(beads, func(i, j int) bool {
		pi, pj := priorityIndex(beads[i].Priority), priorityIndex(beads[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return beads[i].ID < beads[j].ID
	})
}

// NormalizePriority ensures empty priority becomes "P2".
func NormalizePriority(p string) string {
	if p == "" {
		return "P2"
	}
	return p
}

// NewWorkQueue creates an empty WorkQueue with the current timestamp.
func NewWorkQueue() *WorkQueue {
	return &WorkQueue{
		FetchedAt: time.Now().Unix(),
	}
}
