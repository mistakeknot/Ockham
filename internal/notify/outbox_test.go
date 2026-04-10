package notify_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/Ockham/internal/notify"
)

func testOutbox(t *testing.T) *notify.Outbox {
	t.Helper()
	ob, err := notify.NewOutbox(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatalf("NewOutbox: %v", err)
	}
	t.Cleanup(func() { _ = ob.Close() })
	return ob
}

func TestOutbox_EnqueueDedupApproval(t *testing.T) {
	ob := testOutbox(t)
	now := time.Unix(100, 0)
	n1 := notify.NewApproval("b1", "claude-code", 2, now)
	n2 := notify.NewApproval("b1", "claude-code", 2, now.Add(time.Minute))
	if err := ob.Enqueue(n1); err != nil {
		t.Fatal(err)
	}
	if err := ob.Enqueue(n2); err != nil {
		t.Fatal(err)
	}
	pending, err := ob.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending)=%d, want 1", len(pending))
	}
}

func TestOutbox_CoalesceDigest(t *testing.T) {
	ob := testOutbox(t)
	n1 := notify.NewDigest("Digest", "first", time.Unix(100, 0))
	n2 := notify.NewDigest("Digest", "second", time.Unix(200, 0))
	if err := ob.Enqueue(n1); err != nil {
		t.Fatal(err)
	}
	if err := ob.Enqueue(n2); err != nil {
		t.Fatal(err)
	}
	pending, err := ob.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending)=%d, want 1", len(pending))
	}
	if pending[0].Body != "second" {
		t.Fatalf("digest body=%q, want second", pending[0].Body)
	}
}

func TestOutbox_ListReadyAndMarkSent(t *testing.T) {
	ob := testOutbox(t)
	n := notify.NewAlert("Alert", "body", 2, time.Unix(100, 0))
	n.DeliverAfter = 150
	if err := ob.Enqueue(n); err != nil {
		t.Fatal(err)
	}
	ready, err := ob.ListReady(120)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 0 {
		t.Fatalf("ready before deliver_after = %+v", ready)
	}
	ready, err = ob.ListReady(200)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 {
		t.Fatalf("len(ready)=%d, want 1", len(ready))
	}
	if err := ob.MarkSent(ready[0].ID, 210); err != nil {
		t.Fatal(err)
	}
	pending, _ := ob.ListPending()
	if len(pending) != 0 {
		t.Fatalf("pending after sent = %+v", pending)
	}
}
