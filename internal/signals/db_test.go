package signals_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/signals"
)

func TestNewDB_CreatesSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.db")

	db, err := signals.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if db.WasRecovered() {
		t.Error("expected WasRecovered=false for fresh DB")
	}

	// Verify schema_meta exists with version 1
	var version int
	err = db.Conn().QueryRow("SELECT version FROM schema_meta").Scan(&version)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Errorf("schema version = %d, want 2", version)
	}
}

func TestNewDB_RecoveryFromCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signals.db")

	// Write garbage to simulate corruption
	if err := os.WriteFile(path, []byte("not a database"), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := signals.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if !db.WasRecovered() {
		t.Error("expected WasRecovered=true after corruption recovery")
	}

	// DB should still be usable
	if err := db.SetSignalState("test", "value", 1000); err != nil {
		t.Errorf("set after recovery: %v", err)
	}
}

func TestNewDB_RecoveryFromMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "signals.db")

	db, err := signals.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Fresh creation is not recovery
	if db.WasRecovered() {
		t.Error("expected WasRecovered=false for fresh creation in new dir")
	}
}

func TestSignalState_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	db, err := signals.NewDB(filepath.Join(dir, "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SetSignalState("foo", "bar", 100); err != nil {
		t.Fatal(err)
	}

	val, found, err := db.GetSignalState("foo")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected to find key 'foo'")
	}
	if val != "bar" {
		t.Errorf("value = %q, want %q", val, "bar")
	}
}

func TestSignalState_NotFound(t *testing.T) {
	dir := t.TempDir()
	db, err := signals.NewDB(filepath.Join(dir, "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, found, err := db.GetSignalState("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found for nonexistent key")
	}
}

func TestSignalState_Upsert(t *testing.T) {
	dir := t.TempDir()
	db, err := signals.NewDB(filepath.Join(dir, "signals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SetSignalState("key", "v1", 100); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSignalState("key", "v2", 200); err != nil {
		t.Fatal(err)
	}

	val, _, err := db.GetSignalState("key")
	if err != nil {
		t.Fatal(err)
	}
	if val != "v2" {
		t.Errorf("value = %q, want %q after upsert", val, "v2")
	}
}

func TestDefaultDBPath(t *testing.T) {
	path := signals.DefaultDBPath()
	if filepath.Base(path) != "signals.db" {
		t.Errorf("expected signals.db, got %s", filepath.Base(path))
	}
}
