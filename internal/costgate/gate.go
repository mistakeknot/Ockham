package costgate

import (
	"fmt"
	"time"

	"github.com/mistakeknot/Ockham/internal/config"
)

// Gate evaluates cost tier rules and manages approvals.
type Gate struct {
	configStore *config.Store
	approvalDB  *ApprovalDB
	now         func() time.Time
	lease       time.Duration
}

// NewGate creates a Gate.
func NewGate(configStore *config.Store, approvalDB *ApprovalDB) *Gate {
	return &Gate{
		configStore: configStore,
		approvalDB:  approvalDB,
		now:         time.Now,
		lease:       DefaultLease,
	}
}

// SetLease overrides the default lease duration, mainly for tests.
func (g *Gate) SetLease(d time.Duration) { g.lease = d }

// Allowed returns whether a generic tier is currently auto-approved.
// This satisfies route.CostGate for v1's static-config-only behavior.
func (g *Gate) Allowed(costTier int) bool {
	cfg, err := g.configStore.Load()
	if err != nil {
		return false
	}
	def, ok := cfg.Cost.Tiers[costTier]
	return ok && def.AutoApprove
}

// Evaluate checks whether a bead can be dispatched to an agent.
func (g *Gate) Evaluate(beadID string, agentName string) (*Decision, error) {
	cfg, err := g.configStore.Load()
	if err != nil {
		return nil, err
	}
	agent, ok := cfg.Agents[agentName]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", agentName)
	}
	def, ok := cfg.Cost.Tiers[agent.CostTier]
	if !ok {
		return nil, fmt.Errorf("undefined cost tier %d for agent %q", agent.CostTier, agentName)
	}

	now := g.now().Unix()
	d := &Decision{BeadID: beadID, Agent: agentName, Tier: agent.CostTier}

	if def.AutoApprove {
		d.Status = Allowed
		return d, nil
	}

	a, err := g.approvalDB.GetApproval(beadID)
	if err != nil {
		return nil, err
	}
	if a != nil {
		if a.ExpiresAt < now {
			_ = g.approvalDB.DeleteApproval(beadID)
			d.Status = Expired
			d.Reason = "approval expired"
			d.ApprovedBy = a.ApprovedBy
			d.ApprovedAt = a.ApprovedAt
			d.ExpiresAt = a.ExpiresAt
			_ = g.approvalDB.SaveRequest(ApprovalRequest{
				BeadID:      beadID,
				Agent:       agentName,
				Tier:        agent.CostTier,
				Reason:      d.Reason,
				RequestedAt: now,
			})
			return d, nil
		}
		if a.Tier >= agent.CostTier {
			d.Status = Allowed
			d.ApprovedBy = a.ApprovedBy
			d.ApprovedAt = a.ApprovedAt
			d.ExpiresAt = a.ExpiresAt
			return d, nil
		}
	}

	d.Status = Blocked
	d.Reason = fmt.Sprintf("tier %d requires approval (%s)", agent.CostTier, def.Requires)
	d.RequestedAt = now
	if err := g.approvalDB.SaveRequest(ApprovalRequest{
		BeadID:      beadID,
		Agent:       agentName,
		Tier:        agent.CostTier,
		Reason:      d.Reason,
		RequestedAt: now,
	}); err != nil {
		return nil, err
	}
	return d, nil
}

// Approve records explicit approval for a bead at a given tier ceiling.
func (g *Gate) Approve(beadID string, tier int, approvedBy string) error {
	now := g.now().Unix()
	expires := g.now().Add(g.lease).Unix()
	if err := g.approvalDB.SaveApproval(Approval{
		BeadID:      beadID,
		Tier:        tier,
		ApprovedBy:  approvedBy,
		ApprovedAt:  now,
		ExpiresAt:   expires,
		LastTouchAt: now,
	}); err != nil {
		return err
	}
	return g.approvalDB.DeleteRequest(beadID)
}

// Touch renews an existing approval lease due to real activity.
func (g *Gate) Touch(beadID string) error {
	now := g.now()
	return g.approvalDB.TouchApproval(beadID, now.Unix(), now.Add(g.lease).Unix())
}

// Pending returns all currently blocked approval requests.
func (g *Gate) Pending() ([]Decision, error) {
	reqs, err := g.approvalDB.ListRequests()
	if err != nil {
		return nil, err
	}
	out := make([]Decision, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, Decision{
			BeadID:      r.BeadID,
			Agent:       r.Agent,
			Tier:        r.Tier,
			Status:      Blocked,
			Reason:      r.Reason,
			RequestedAt: r.RequestedAt,
		})
	}
	return out, nil
}
