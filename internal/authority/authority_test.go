package authority_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/authority"
	"github.com/mistakeknot/Ockham/internal/signals"
)

func newDB(t *testing.T) *signals.DB {
	t.Helper()
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// strongSnapshot writes an authority_snapshot that satisfies DefaultPolicy.
func strongSnapshot(t *testing.T, db *signals.DB, agent, domain string) {
	t.Helper()
	err := db.SaveAuthoritySnapshot(signals.AuthoritySnapshot{
		Agent:      agent,
		Domain:     domain,
		HitRate:    0.90,
		Sessions:   50,
		Confidence: 0.85,
		CapturedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("SaveAuthoritySnapshot: %v", err)
	}
}

func TestTier_DefaultShadow(t *testing.T) {
	db := newDB(t)
	s := authority.New(db, authority.DefaultPolicy())

	got, err := s.Tier("codex", "refactor")
	if err != nil {
		t.Fatal(err)
	}
	if got != authority.TierShadow {
		t.Errorf("Tier = %q, want %q", got, authority.TierShadow)
	}
}

func TestPromote_Succeeds_WithStrongEvidence(t *testing.T) {
	db := newDB(t)
	strongSnapshot(t, db, "codex", "refactor")

	s := authority.New(db, authority.DefaultPolicy())

	got, err := s.Promote("codex", "refactor", "unit test")
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if got != authority.TierSupervised {
		t.Errorf("promoted to %q, want %q", got, authority.TierSupervised)
	}

	tier, _ := s.Tier("codex", "refactor")
	if tier != authority.TierSupervised {
		t.Errorf("Tier after promote = %q, want %q", tier, authority.TierSupervised)
	}
}

func TestPromote_FailsOnNoSnapshot(t *testing.T) {
	db := newDB(t)
	s := authority.New(db, authority.DefaultPolicy())

	_, err := s.Promote("codex", "refactor", "nope")
	if !errors.Is(err, authority.ErrInsufficientEvidence) {
		t.Fatalf("err = %v, want ErrInsufficientEvidence", err)
	}
}

func TestPromote_FailsBelowThresholds(t *testing.T) {
	db := newDB(t)
	// Weak snapshot: sessions below floor.
	db.SaveAuthoritySnapshot(signals.AuthoritySnapshot{
		Agent: "codex", Domain: "refactor",
		HitRate: 0.95, Sessions: 5, Confidence: 0.95,
		CapturedAt: time.Now().Unix(),
	})
	s := authority.New(db, authority.DefaultPolicy())

	_, err := s.Promote("codex", "refactor", "")
	if !errors.Is(err, authority.ErrInsufficientEvidence) {
		t.Fatalf("err = %v, want ErrInsufficientEvidence", err)
	}
}

func TestPromote_AtCeiling(t *testing.T) {
	db := newDB(t)
	strongSnapshot(t, db, "codex", "refactor")

	// Pre-seed at autonomous with ancient timestamp so dwell elapses.
	db.SaveRatchetState(signals.RatchetState{
		Agent: "codex", Domain: "refactor",
		Tier: authority.TierAutonomous, PromotedAt: 1,
	})

	s := authority.New(db, authority.DefaultPolicy())
	_, err := s.Promote("codex", "refactor", "")
	if !errors.Is(err, authority.ErrAtCeiling) {
		t.Fatalf("err = %v, want ErrAtCeiling", err)
	}
}

func TestPromote_DwellGated(t *testing.T) {
	db := newDB(t)
	strongSnapshot(t, db, "codex", "refactor")

	// Fresh promotion timestamp — dwell has not elapsed.
	db.SaveRatchetState(signals.RatchetState{
		Agent: "codex", Domain: "refactor",
		Tier: authority.TierShadow, PromotedAt: time.Now().Unix(),
	})

	s := authority.New(db, authority.DefaultPolicy())
	_, err := s.Promote("codex", "refactor", "")
	if !errors.Is(err, authority.ErrInsufficientEvidence) {
		t.Fatalf("err = %v, want ErrInsufficientEvidence (dwell)", err)
	}
}

func TestDemote_Succeeds(t *testing.T) {
	db := newDB(t)
	db.SaveRatchetState(signals.RatchetState{
		Agent: "codex", Domain: "refactor",
		Tier: authority.TierAutonomous, PromotedAt: 1,
	})
	s := authority.New(db, authority.DefaultPolicy())

	got, err := s.Demote("codex", "refactor", "anomaly")
	if err != nil {
		t.Fatal(err)
	}
	if got != authority.TierSupervised {
		t.Errorf("demoted to %q, want %q", got, authority.TierSupervised)
	}

	got2, err := s.Demote("codex", "refactor", "anomaly")
	if err != nil {
		t.Fatal(err)
	}
	if got2 != authority.TierShadow {
		t.Errorf("demoted to %q, want %q", got2, authority.TierShadow)
	}
}

func TestDemote_AtFloor(t *testing.T) {
	db := newDB(t)
	s := authority.New(db, authority.DefaultPolicy())

	_, err := s.Demote("codex", "refactor", "anomaly")
	if !errors.Is(err, authority.ErrAtFloor) {
		t.Fatalf("err = %v, want ErrAtFloor", err)
	}
}
