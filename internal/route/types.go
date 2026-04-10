package route

import "time"

// RoutingDecision maps a bead to an agent with rationale.
type RoutingDecision struct {
	BeadID       string        `json:"bead_id"`
	BeadTitle    string        `json:"bead_title"`
	BeadType     string        `json:"bead_type"`
	BeadOrg      string        `json:"bead_org"`
	Agent        string        `json:"agent"`
	AgentModel   string        `json:"agent_model"`
	CostTier     int           `json:"cost_tier"`
	Score        float64       `json:"score"`
	Status       RoutingStatus `json:"status"`
	Reason       string        `json:"reason,omitempty"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
}

// RoutingStatus is the outcome of routing a bead.
type RoutingStatus string

const (
	Routed   RoutingStatus = "routed"
	Blocked  RoutingStatus = "blocked"
	Deferred RoutingStatus = "deferred"
)

// Alternative is an agent that could do the job but wasn't picked.
type Alternative struct {
	Agent    string  `json:"agent"`
	Score    float64 `json:"score"`
	CostTier int     `json:"cost_tier"`
	Reason   string  `json:"reason"`
}

// RoutingTable is the full set of routing decisions.
type RoutingTable struct {
	Decisions   []RoutingDecision `json:"decisions"`
	Summary     RoutingSummary    `json:"summary"`
	GeneratedAt int64             `json:"generated_at"`
}

// RoutingSummary counts by status.
type RoutingSummary struct {
	Total    int `json:"total"`
	Routed   int `json:"routed"`
	Blocked  int `json:"blocked"`
	Deferred int `json:"deferred"`
}

// CostGate decides whether a cost tier is allowed.
type CostGate interface {
	Allowed(costTier int) bool
}

// AvailabilityFunc checks whether an agent is available for dispatch.
type AvailabilityFunc func(agentName string) bool

// DefaultAvailability always returns true.
func DefaultAvailability(agentName string) bool { return true }

// typeCapabilities maps bead types to required agent capabilities.
var typeCapabilities = map[string][]string{
	"task":     {"coding", "execution"},
	"bug":      {"coding", "debugging"},
	"feature":  {"coding", "refactoring"},
	"epic":     {"planning", "research"},
	"research": {"research", "planning"},
}

// ExportedTypeCapabilities returns the typeCapabilities map for testing.
func ExportedTypeCapabilities() map[string][]string { return typeCapabilities }

// NewRoutingTable creates a RoutingTable with the current timestamp.
func NewRoutingTable() *RoutingTable {
	return &RoutingTable{
		GeneratedAt: time.Now().Unix(),
	}
}

// ComputeSummary fills the Summary from Decisions.
func (rt *RoutingTable) ComputeSummary() {
	rt.Summary = RoutingSummary{Total: len(rt.Decisions)}
	for _, d := range rt.Decisions {
		switch d.Status {
		case Routed:
			rt.Summary.Routed++
		case Blocked:
			rt.Summary.Blocked++
		case Deferred:
			rt.Summary.Deferred++
		}
	}
}
