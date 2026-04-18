// Package authority implements the Ockham authority ratchet: per-(agent, domain)
// tier state with evidence-gated promotion and direct demotion.
//
// Tiers ladder upward as evidence accumulates:
//
//	shadow → supervised → autonomous
//
// Promotion requires an authority snapshot meeting Policy thresholds. Demotion
// is direct — it is the lever pulled by anomaly detection and, later, by Tier 2
// CONSTRAIN when a subsystem misbehaves.
package authority

import (
	"errors"
	"fmt"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
)

// Tier re-exports signals.Tier so callers do not need both imports.
type Tier = signals.Tier

const (
	TierShadow     = signals.TierShadow
	TierSupervised = signals.TierSupervised
	TierAutonomous = signals.TierAutonomous
)

// Policy configures evidence thresholds and dwell time for promotion.
type Policy struct {
	MinSessions   int           // required sessions in authority_snapshot
	MinHitRate    float64       // required hit_rate in [0,1]
	MinConfidence float64       // required confidence in [0,1]
	MinDwell      time.Duration // time since last tier change before promotion eligible
}

// DefaultPolicy is the conservative starting configuration.
// Tuned so a freshly-observed agent needs real signal before leaving shadow.
func DefaultPolicy() Policy {
	return Policy{
		MinSessions:   20,
		MinHitRate:    0.80,
		MinConfidence: 0.75,
		MinDwell:      24 * time.Hour,
	}
}

// State is the live authority ratchet backed by signals.DB.
type State struct {
	db     *signals.DB
	policy Policy
	now    func() time.Time
}

// New constructs a State with the given DB and policy. The DB must be non-nil.
func New(db *signals.DB, policy Policy) *State {
	return &State{db: db, policy: policy, now: time.Now}
}

// Tier returns the current tier for (agent, domain). Defaults to TierShadow
// when the pair has no ratchet row yet.
func (s *State) Tier(agent, domain string) (Tier, error) {
	r, _, err := s.db.GetRatchetState(agent, domain)
	if err != nil {
		return TierShadow, err
	}
	return r.Tier, nil
}

// ErrInsufficientEvidence is returned when a promote call cannot be satisfied
// by the current authority_snapshot data under the configured policy.
var ErrInsufficientEvidence = errors.New("authority: insufficient evidence to promote")

// ErrAtCeiling is returned when promote is called on an agent already at the
// top tier.
var ErrAtCeiling = errors.New("authority: already at ceiling tier")

// ErrAtFloor is returned when demote is called on an agent already at shadow.
var ErrAtFloor = errors.New("authority: already at floor tier")

// Promote advances (agent, domain) one tier if and only if the authority
// snapshot for the pair meets Policy thresholds and dwell time has elapsed.
// The reason is stored for observability but does not influence the decision.
func (s *State) Promote(agent, domain, reason string) (Tier, error) {
	cur, _, err := s.db.GetRatchetState(agent, domain)
	if err != nil {
		return TierShadow, fmt.Errorf("authority: read ratchet: %w", err)
	}

	next, ok := nextTier(cur.Tier)
	if !ok {
		return cur.Tier, ErrAtCeiling
	}

	// Dwell check — most recent tier change must be older than MinDwell.
	lastChange := cur.PromotedAt
	if cur.DemotedAt > lastChange {
		lastChange = cur.DemotedAt
	}
	if lastChange > 0 && s.now().Unix()-lastChange < int64(s.policy.MinDwell.Seconds()) {
		return cur.Tier, fmt.Errorf("%w: dwell time not elapsed", ErrInsufficientEvidence)
	}

	snap, found, err := s.db.GetAuthoritySnapshot(agent, domain)
	if err != nil {
		return cur.Tier, fmt.Errorf("authority: read snapshot: %w", err)
	}
	if !found {
		return cur.Tier, fmt.Errorf("%w: no snapshot for %s/%s", ErrInsufficientEvidence, agent, domain)
	}
	if snap.Sessions < s.policy.MinSessions ||
		snap.HitRate < s.policy.MinHitRate ||
		snap.Confidence < s.policy.MinConfidence {
		return cur.Tier, fmt.Errorf("%w: snapshot below thresholds", ErrInsufficientEvidence)
	}

	cur.Tier = next
	cur.PromotedAt = s.now().Unix()
	if err := s.db.SaveRatchetState(cur); err != nil {
		return cur.Tier, fmt.Errorf("authority: save ratchet: %w", err)
	}
	return next, nil
}

// Demote lowers (agent, domain) one tier. No evidence gating — demotion is the
// direct lever for anomaly response. Returns ErrAtFloor at shadow.
func (s *State) Demote(agent, domain, reason string) (Tier, error) {
	cur, _, err := s.db.GetRatchetState(agent, domain)
	if err != nil {
		return TierShadow, fmt.Errorf("authority: read ratchet: %w", err)
	}
	prev, ok := prevTier(cur.Tier)
	if !ok {
		return cur.Tier, ErrAtFloor
	}
	cur.Tier = prev
	cur.DemotedAt = s.now().Unix()
	if err := s.db.SaveRatchetState(cur); err != nil {
		return cur.Tier, fmt.Errorf("authority: save ratchet: %w", err)
	}
	return prev, nil
}

func nextTier(t Tier) (Tier, bool) {
	switch t {
	case TierShadow:
		return TierSupervised, true
	case TierSupervised:
		return TierAutonomous, true
	}
	return t, false
}

func prevTier(t Tier) (Tier, bool) {
	switch t {
	case TierAutonomous:
		return TierSupervised, true
	case TierSupervised:
		return TierShadow, true
	}
	return t, false
}
