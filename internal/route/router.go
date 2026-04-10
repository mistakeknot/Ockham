package route

import (
	"sort"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/discover"
)

// maxAlternatives caps the number of alternatives per decision.
const maxAlternatives = 3

// Router produces routing decisions from a work queue.
type Router struct {
	configStore  *config.Store
	gate         CostGate
	availability AvailabilityFunc
}

// NewRouter creates a Router with the given config store and cost gate.
func NewRouter(configStore *config.Store, gate CostGate) *Router {
	return &Router{
		configStore:  configStore,
		gate:         gate,
		availability: DefaultAvailability,
	}
}

// SetAvailability overrides the default availability function.
func (r *Router) SetAvailability(fn AvailabilityFunc) {
	r.availability = fn
}

// Route takes a WorkQueue and produces a RoutingTable.
func (r *Router) Route(queue *discover.WorkQueue) (*RoutingTable, error) {
	return r.route(queue, false)
}

// DryRun produces routing decisions without checking cost gates (preview only).
func (r *Router) DryRun(queue *discover.WorkQueue) (*RoutingTable, error) {
	return r.route(queue, true)
}

func (r *Router) route(queue *discover.WorkQueue, dryRun bool) (*RoutingTable, error) {
	cfg, err := r.configStore.Load()
	if err != nil {
		return nil, err
	}

	rt := NewRoutingTable()
	for _, bead := range queue.Beads {
		d := r.routeOne(bead, cfg.Agents, dryRun)
		rt.Decisions = append(rt.Decisions, d)
	}
	rt.ComputeSummary()
	return rt, nil
}

// RouteOne routes a single bead.
func (r *Router) RouteOne(bead discover.DiscoveredBead) (*RoutingDecision, error) {
	cfg, err := r.configStore.Load()
	if err != nil {
		return nil, err
	}
	d := r.routeOne(bead, cfg.Agents, false)
	return &d, nil
}

// agentScore holds a scored agent candidate.
type agentScore struct {
	name     string
	model    string
	costTier int
	score    float64
	reason   string
}

func (r *Router) routeOne(bead discover.DiscoveredBead, agents config.Agents, dryRun bool) RoutingDecision {
	d := RoutingDecision{
		BeadID:    bead.ID,
		BeadTitle: bead.Title,
		BeadType:  bead.Type,
		BeadOrg:   bead.Org,
	}

	if len(agents) == 0 {
		d.Status = Deferred
		d.Reason = "no agents configured"
		return d
	}

	required := typeCapabilities[bead.Type]
	if len(required) == 0 {
		// Unknown bead type — treat as generic, any agent can try
		required = []string{}
	}

	// Score all agents deterministically: sort by name first.
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)

	var candidates []agentScore
	var blockedByGate []agentScore

	for _, name := range names {
		agent := agents[name]

		// Check availability
		if !r.availability(name) {
			continue
		}

		score := capabilityScore(agent.Capabilities, required)

		as := agentScore{
			name:     name,
			model:    agent.Model,
			costTier: agent.CostTier,
			score:    score,
		}

		// Check cost gate (skip in dry-run mode)
		if !dryRun && !r.gate.Allowed(agent.CostTier) {
			as.reason = "cost tier not approved"
			blockedByGate = append(blockedByGate, as)
			continue
		}

		candidates = append(candidates, as)
	}

	// Sort candidates: highest score first, then lowest cost tier, then name.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].costTier != candidates[j].costTier {
			return candidates[i].costTier < candidates[j].costTier
		}
		return candidates[i].name < candidates[j].name
	})

	if len(candidates) == 0 {
		if len(blockedByGate) > 0 {
			d.Status = Blocked
			d.Reason = "all capable agents blocked by cost gate"
		} else {
			d.Status = Deferred
			d.Reason = "no suitable agent available"
		}
		return d
	}

	// Pick the winner.
	winner := candidates[0]
	d.Agent = winner.name
	d.AgentModel = winner.model
	d.CostTier = winner.costTier
	d.Score = winner.score
	d.Status = Routed

	// Build alternatives from remaining candidates (up to maxAlternatives).
	rest := candidates[1:]
	if len(rest) > maxAlternatives {
		rest = rest[:maxAlternatives]
	}
	for _, alt := range rest {
		d.Alternatives = append(d.Alternatives, Alternative{
			Agent:    alt.name,
			Score:    alt.score,
			CostTier: alt.costTier,
			Reason:   "lower score or higher cost",
		})
	}

	return d
}

// capabilityScore returns the fraction of required capabilities matched.
// If no capabilities are required (unknown type), returns 0 for all agents.
func capabilityScore(agentCaps []string, required []string) float64 {
	if len(required) == 0 {
		return 0
	}
	have := make(map[string]bool, len(agentCaps))
	for _, c := range agentCaps {
		have[c] = true
	}
	matched := 0
	for _, req := range required {
		if have[req] {
			matched++
		}
	}
	return float64(matched) / float64(len(required))
}
