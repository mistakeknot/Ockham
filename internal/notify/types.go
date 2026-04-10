package notify

import (
	"fmt"
	"time"
)

// NotificationType classifies an outbound message.
type NotificationType string

const (
	NotifDigest   NotificationType = "digest"
	NotifApproval NotificationType = "approval"
	NotifAlert    NotificationType = "alert"
)

// Status tracks outbox delivery state.
type Status string

const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
)

// Notification is a structured outbound record for Hermes to deliver.
type Notification struct {
	ID          string           `json:"id"`
	Type        NotificationType `json:"type"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	Priority    int              `json:"priority"`
	Status      Status           `json:"status"`
	CreatedAt   int64            `json:"created_at"`
	DeliverAfter int64           `json:"deliver_after,omitempty"`
	SentAt      int64            `json:"sent_at,omitempty"`
	DedupKey    string           `json:"dedup_key,omitempty"`
	CoalesceKey string           `json:"coalesce_key,omitempty"`
}

// NewAlert creates an immediate alert record.
func NewAlert(title, body string, priority int, now time.Time) Notification {
	return Notification{
		ID:        fmt.Sprintf("alert-%d", now.UnixNano()),
		Type:      NotifAlert,
		Title:     title,
		Body:      body,
		Priority:  priority,
		Status:    StatusPending,
		CreatedAt: now.Unix(),
	}
}

// NewApproval creates a durable approval request notification.
func NewApproval(beadID, agent string, tier int, now time.Time) Notification {
	return Notification{
		ID:        fmt.Sprintf("approval-%s-%d", beadID, tier),
		Type:      NotifApproval,
		Title:     "Approval Needed",
		Body:      fmt.Sprintf("Bead %s needs approval for %s at tier %d.", beadID, agent, tier),
		Priority:  1,
		Status:    StatusPending,
		CreatedAt: now.Unix(),
		DedupKey:  fmt.Sprintf("approval:%s:%d", beadID, tier),
	}
}

// NewDigest creates a coalescible digest notification.
func NewDigest(title, body string, now time.Time) Notification {
	return Notification{
		ID:          fmt.Sprintf("digest-%d", now.UnixNano()),
		Type:        NotifDigest,
		Title:       title,
		Body:        body,
		Priority:    0,
		Status:      StatusPending,
		CreatedAt:   now.Unix(),
		CoalesceKey: "daily-digest",
	}
}
