package discover

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WorkspaceMap maps org names to filesystem paths containing .beads/.
type WorkspaceMap map[string]string

// WorkspaceStore reads and writes workspaces.yaml with atomic replacement.
type WorkspaceStore struct {
	path string
}

// NewWorkspaceStore creates a WorkspaceStore for the given YAML path.
func NewWorkspaceStore(path string) *WorkspaceStore {
	return &WorkspaceStore{path: path}
}

// DefaultWorkspacePath returns ~/.config/ockham/workspaces.yaml.
func DefaultWorkspacePath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "workspaces.yaml")
}

// Path returns the store's file path.
func (ws *WorkspaceStore) Path() string { return ws.path }

// Load reads the workspace map. Returns empty map if missing.
func (ws *WorkspaceStore) Load() (WorkspaceMap, error) {
	data, err := os.ReadFile(ws.path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceMap{}, nil
		}
		return WorkspaceMap{}, fmt.Errorf("reading workspaces file: %w", err)
	}

	var m WorkspaceMap
	if err := yaml.Unmarshal(data, &m); err != nil {
		return WorkspaceMap{}, fmt.Errorf("parsing workspaces YAML: %w", err)
	}
	if m == nil {
		return WorkspaceMap{}, nil
	}

	return m, nil
}

// Save writes the workspace map atomically (write temp, rename).
func (ws *WorkspaceStore) Save(m WorkspaceMap) error {
	if err := os.MkdirAll(filepath.Dir(ws.path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling workspaces: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(ws.path), "workspaces-*.yaml")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, ws.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// ScanHome searches for directories under root containing .beads/ subdirectories.
// Returns a WorkspaceMap keyed by directory basename.
func ScanHome(root string) (WorkspaceMap, error) {
	m := WorkspaceMap{}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading home directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Skip hidden directories
		if entry.Name()[0] == '.' {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		beadsDir := filepath.Join(candidate, ".beads")
		if info, err := os.Stat(beadsDir); err == nil && info.IsDir() {
			m[entry.Name()] = candidate
		}
	}

	return m, nil
}
