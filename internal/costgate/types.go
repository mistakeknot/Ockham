package costgate

import "time"

// DefaultLease is the default approval lease duration.
const DefaultLease = time.Hour

// DecisionStatus is the result of evaluating a dispatch against cost rules.
type DecisionStatus string

const (
	Allowed DecisionStatus = "allowed"
	Blocked DecisionStatus = "blocked"
	Expired DecisionStatus = "expired"
)

// Decision is the result of evaluating a dispatch against cost rules.
type Decision struct {
	BeadID      string         `json:"bead_id"`
	Agent       string         `json:"agent"`
	Tier        int            `json:"tier"`
	Status      DecisionStatus `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	ApprovedBy  string         `json:"approved_by,omitempty"`
	ApprovedAt  int64          `json:"approved_at,omitempty"`
	ExpiresAt   int64          `json:"expires_at,omitempty"`
	RequestedAt int64          `json:"requested_at,omitempty"`
}

// Approval is a durable grant to spend up to Tier on a bead.
// It is per-bead-ceiling, not per-agent.
type Approval struct {
	BeadID      string `json:"bead_id"`
	Tier        int    `json:"tier"`
	ApprovedBy  string `json:"approved_by"`
	ApprovedAt  int64  `json:"approved_at"`
	ExpiresAt   int64  `json:"expires_at"`
	LastTouchAt int64  `json:"last_touch_at"`
}

// ApprovalRequest records that a bead was blocked and needs approval.
type ApprovalRequest struct {
	BeadID      string `json:"bead_id"`
	Agent       string `json:"agent"`
	Tier        int    `json:"tier"`
	Reason      string `json:"reason"`
	RequestedAt int64  `json:"requested_at"`
}
