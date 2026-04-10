package costgate_test

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/costgate"
)

func testApprovalDB(t *testing.T) *costgate.ApprovalDB {
	t.Helper()
	db, err := costgate.NewApprovalDB(filepath.Join(t.TempDir(), "approvals.db"))
	if err != nil {
		t.Fatalf("NewApprovalDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestApprovalDB_SaveGetDeleteApproval(t *testing.T) {
	db := testApprovalDB(t)
	a := costgate.Approval{BeadID: "b1", Tier: 2, ApprovedBy: "tester", ApprovedAt: 10, ExpiresAt: 70, LastTouchAt: 10}
	if err := db.SaveApproval(a); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	got, err := db.GetApproval("b1")
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if got == nil || got.Tier != 2 || got.ApprovedBy != "tester" {
		t.Fatalf("GetApproval = %+v", got)
	}
	if err := db.TouchApproval("b1", 20, 80); err != nil {
		t.Fatalf("TouchApproval: %v", err)
	}
	got, _ = db.GetApproval("b1")
	if got.LastTouchAt != 20 || got.ExpiresAt != 80 {
		t.Fatalf("touch not applied: %+v", got)
	}
	if err := db.DeleteApproval("b1"); err != nil {
		t.Fatalf("DeleteApproval: %v", err)
	}
	got, err = db.GetApproval("b1")
	if err != nil {
		t.Fatalf("GetApproval after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("GetApproval after delete = %+v, want nil", got)
	}
}

func TestApprovalDB_RequestsAndExpiry(t *testing.T) {
	db := testApprovalDB(t)
	if err := db.SaveRequest(costgate.ApprovalRequest{BeadID: "b2", Agent: "claude-code", Tier: 2, Reason: "need approval", RequestedAt: 5}); err != nil {
		t.Fatalf("SaveRequest: %v", err)
	}
	if err := db.SaveApproval(costgate.Approval{BeadID: "old", Tier: 2, ApprovedBy: "x", ApprovedAt: 1, ExpiresAt: 2, LastTouchAt: 1}); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	reqs, err := db.ListRequests()
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(reqs) != 1 || reqs[0].BeadID != "b2" {
		t.Fatalf("ListRequests = %+v", reqs)
	}
	n, err := db.ExpireOlderThan(3)
	if err != nil {
		t.Fatalf("ExpireOlderThan: %v", err)
	}
	if n != 1 {
		t.Fatalf("ExpireOlderThan deleted %d, want 1", n)
	}
	if err := db.DeleteRequest("b2"); err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	reqs, _ = db.ListRequests()
	if len(reqs) != 0 {
		t.Fatalf("ListRequests after delete = %+v, want empty", reqs)
	}
}
