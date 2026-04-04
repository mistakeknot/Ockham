package signals_test

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestAuthoritySnapshot_Roundtrip(t *testing.T) {
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	snap := signals.AuthoritySnapshot{
		Agent:      "fd-architecture",
		Domain:     "review",
		HitRate:    0.85,
		Sessions:   42,
		Confidence: 0.9,
		CapturedAt: 1000,
	}

	if err := db.SaveAuthoritySnapshot(snap); err != nil {
		t.Fatal(err)
	}

	got, found, err := db.GetAuthoritySnapshot("fd-architecture", "review")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected to find snapshot")
	}
	if got.HitRate != 0.85 {
		t.Errorf("hit_rate = %f, want 0.85", got.HitRate)
	}
	if got.Sessions != 42 {
		t.Errorf("sessions = %d, want 42", got.Sessions)
	}
}

func TestAuthoritySnapshot_Upsert(t *testing.T) {
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	snap1 := signals.AuthoritySnapshot{
		Agent: "agent-a", Domain: "d1",
		HitRate: 0.5, Sessions: 10, Confidence: 0.6, CapturedAt: 100,
	}
	snap2 := signals.AuthoritySnapshot{
		Agent: "agent-a", Domain: "d1",
		HitRate: 0.9, Sessions: 50, Confidence: 0.95, CapturedAt: 200,
	}

	if err := db.SaveAuthoritySnapshot(snap1); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAuthoritySnapshot(snap2); err != nil {
		t.Fatal(err)
	}

	got, _, err := db.GetAuthoritySnapshot("agent-a", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.HitRate != 0.9 {
		t.Errorf("hit_rate after upsert = %f, want 0.9", got.HitRate)
	}
	if got.CapturedAt != 200 {
		t.Errorf("captured_at after upsert = %d, want 200", got.CapturedAt)
	}
}

func TestAuthoritySnapshot_NotFound(t *testing.T) {
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, found, err := db.GetAuthoritySnapshot("nope", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found")
	}
}

func TestAuthoritySnapshot_List(t *testing.T) {
	db, err := signals.NewDB(filepath.Join(t.TempDir(), "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, snap := range []signals.AuthoritySnapshot{
		{Agent: "a1", Domain: "d1", HitRate: 0.8, Sessions: 10, Confidence: 0.7, CapturedAt: 100},
		{Agent: "a2", Domain: "d2", HitRate: 0.9, Sessions: 20, Confidence: 0.8, CapturedAt: 200},
	} {
		if err := db.SaveAuthoritySnapshot(snap); err != nil {
			t.Fatal(err)
		}
	}

	snaps, err := db.ListAuthoritySnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Errorf("got %d snapshots, want 2", len(snaps))
	}
}
