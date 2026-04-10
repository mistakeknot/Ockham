package discover_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/discover"
)

func TestWorkspaceStoreLoadMissing(t *testing.T) {
	ws := discover.NewWorkspaceStore(filepath.Join(t.TempDir(), "missing.yaml"))
	m, err := ws.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(m) != 0 {
		t.Errorf("Load() returned %d entries, want 0", len(m))
	}
}

func TestWorkspaceStoreSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.yaml")
	ws := discover.NewWorkspaceStore(path)

	want := discover.WorkspaceMap{
		"orgA": "/tmp/orgA",
		"orgB": "/tmp/orgB",
	}

	if err := ws.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := ws.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("Load() returned %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Load()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestWorkspaceStoreSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep")
	path := filepath.Join(dir, "workspaces.yaml")
	ws := discover.NewWorkspaceStore(path)

	if err := ws.Save(discover.WorkspaceMap{"org": "/tmp/org"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s: %v", path, err)
	}
}

func TestWorkspaceStorePath(t *testing.T) {
	ws := discover.NewWorkspaceStore("/some/path.yaml")
	if got := ws.Path(); got != "/some/path.yaml" {
		t.Errorf("Path() = %q, want %q", got, "/some/path.yaml")
	}
}

func TestScanHome(t *testing.T) {
	root := t.TempDir()

	// Create directories: one with .beads/, one without, one hidden
	beadsDir := filepath.Join(root, "project-a", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	noBeadsDir := filepath.Join(root, "project-b")
	if err := os.MkdirAll(noBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	hiddenDir := filepath.Join(root, ".hidden", ".beads")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatal(err)
	}

	m, err := discover.ScanHome(root)
	if err != nil {
		t.Fatalf("ScanHome() error = %v", err)
	}

	if len(m) != 1 {
		t.Fatalf("ScanHome() returned %d entries, want 1", len(m))
	}
	if m["project-a"] != filepath.Join(root, "project-a") {
		t.Errorf("ScanHome()[project-a] = %q, want %q", m["project-a"], filepath.Join(root, "project-a"))
	}
}

func TestScanHomeEmpty(t *testing.T) {
	root := t.TempDir()

	m, err := discover.ScanHome(root)
	if err != nil {
		t.Fatalf("ScanHome() error = %v", err)
	}
	if len(m) != 0 {
		t.Errorf("ScanHome() returned %d entries, want 0", len(m))
	}
}
