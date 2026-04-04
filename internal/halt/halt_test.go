package halt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/halt"
)

func TestIsHalted_NoFile(t *testing.T) {
	dir := t.TempDir()
	h := halt.New(filepath.Join(dir, "factory-paused.json"))
	if h.IsHalted() {
		t.Error("expected not halted when file missing")
	}
}

func TestIsHalted_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "factory-paused.json")
	if err := os.WriteFile(path, []byte(`{"reason":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := halt.New(path)
	if !h.IsHalted() {
		t.Error("expected halted when file exists")
	}
}

func TestDefaultSentinelPath(t *testing.T) {
	path := halt.DefaultSentinelPath()
	if filepath.Base(path) != "factory-paused.json" {
		t.Errorf("expected factory-paused.json, got %s", filepath.Base(path))
	}
}
