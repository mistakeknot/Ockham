package costgate_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/costgate"
)

func testConfigStore(t *testing.T) *config.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ockham.yaml")
	s := config.NewStore(path)
	if err := s.Save(config.DefaultConfig()); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	return s
}

func TestGate_EvaluateAutoApprove(t *testing.T) {
	store := testConfigStore(t)
	db := testApprovalDB(t)
	g := costgate.NewGate(store, db)
	d, err := g.Evaluate("b1", "hermes")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != costgate.Allowed || d.Tier != 0 {
		t.Fatalf("decision = %+v", d)
	}
	if !g.Allowed(0) || g.Allowed(2) {
		t.Fatalf("Allowed static check mismatch")
	}
}

func TestGate_EvaluateBlockedThenApprove(t *testing.T) {
	store := testConfigStore(t)
	db := testApprovalDB(t)
	g := costgate.NewGate(store, db)
	d, err := g.Evaluate("b2", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != costgate.Blocked {
		t.Fatalf("status = %s, want blocked", d.Status)
	}
	pending, err := g.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].BeadID != "b2" {
		t.Fatalf("pending = %+v", pending)
	}
	if err := g.Approve("b2", 2, "tester"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	d, err = g.Evaluate("b2", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != costgate.Allowed || d.ApprovedBy != "tester" {
		t.Fatalf("post-approve decision = %+v", d)
	}
	pending, _ = g.Pending()
	if len(pending) != 0 {
		t.Fatalf("pending after approve = %+v", pending)
	}
}

func TestGate_PerBeadCeilingAndExpiryTouch(t *testing.T) {
	store := testConfigStore(t)
	db := testApprovalDB(t)
	g := costgate.NewGate(store, db)
	g.SetLease(time.Hour)
	now := time.Now().Unix()
	oldExpiry := now + 10
	if err := db.SaveApproval(costgate.Approval{BeadID: "b3", Tier: 2, ApprovedBy: "tester", ApprovedAt: now, ExpiresAt: oldExpiry, LastTouchAt: now}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	d, err := g.Evaluate("b3", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != costgate.Allowed {
		t.Fatalf("tier 2 should be allowed under tier-2 ceiling: %+v", d)
	}
	d, err = g.Evaluate("b3", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status == costgate.Allowed {
		t.Fatalf("tier 3 should not be allowed under tier-2 ceiling: %+v", d)
	}
	if err := g.Touch("b3"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, err := db.GetApproval("b3")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ExpiresAt <= oldExpiry {
		t.Fatalf("touch did not extend approval: %+v", got)
	}
}

func TestGate_ExpiredApprovalBecomesExpiredDecision(t *testing.T) {
	store := testConfigStore(t)
	db := testApprovalDB(t)
	g := costgate.NewGate(store, db)
	if err := db.SaveApproval(costgate.Approval{BeadID: "b4", Tier: 2, ApprovedBy: "tester", ApprovedAt: 1, ExpiresAt: 2, LastTouchAt: 1}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	d, err := g.Evaluate("b4", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != costgate.Expired && d.Status != costgate.Blocked {
		t.Fatalf("status = %s, want expired-or-blocked semantics, got %+v", d.Status, d)
	}
}
