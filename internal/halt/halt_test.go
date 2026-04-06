package halt_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRequireRunning_NotHalted(t *testing.T) {
	dir := t.TempDir()
	h := halt.New(filepath.Join(dir, "factory-paused.json"))
	if err := h.RequireRunning(); err != nil {
		t.Errorf("expected nil when not halted, got %v", err)
	}
}

func TestRequireRunning_HaltedWithContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "factory-paused.json")
	if err := os.WriteFile(path, []byte(`{"reason":"BYPASS","timestamp":1743955200}`), 0644); err != nil {
		t.Fatal(err)
	}
	h := halt.New(path)
	err := h.RequireRunning()
	if err == nil {
		t.Fatal("expected error when halted")
	}
	s := err.Error()
	if !strings.Contains(s, "BYPASS") {
		t.Errorf("expected reason in error, got %q", s)
	}
	if !strings.Contains(s, "resume --confirm") {
		t.Errorf("expected resume hint in error, got %q", s)
	}
}

func TestRequireRunning_HaltedInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "factory-paused.json")
	if err := os.WriteFile(path, []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}
	h := halt.New(path)
	err := h.RequireRunning()
	if err == nil {
		t.Fatal("expected error when halted with invalid JSON")
	}
	if !strings.Contains(err.Error(), "resume") {
		t.Errorf("expected fallback message, got %q", err.Error())
	}
}

