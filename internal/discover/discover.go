package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/config"
)

// bdBead is the JSON shape returned by `bd list --status=open --json`.
type bdBead struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Priority string   `json:"priority"`
	Status   string   `json:"status"`
	Type     string   `json:"type"`
	Labels   []string `json:"labels"`
}

// DiscoverAll scans all configured non-SAML orgs.
func (d *Discoverer) DiscoverAll() (*WorkQueue, error) {
	cfg, err := d.configStore.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ockham: config load degraded: %v (using defaults)\n", err)
	}

	wm, err := d.wsStore.Load()
	if err != nil {
		return nil, fmt.Errorf("loading workspaces: %w", err)
	}

	wq := NewWorkQueue()

	for _, org := range cfg.Orgs {
		if org.SAML {
			fmt.Fprintf(os.Stderr, "ockham: skipping SAML org %q\n", org.Name)
			continue
		}

		wsPath, ok := wm[org.Name]
		if !ok {
			wq.Errors = append(wq.Errors, SourceError{
				Org:   org.Name,
				Error: fmt.Sprintf("no workspace path configured for org %q", org.Name),
			})
			continue
		}

		beads, err := d.scanOrg(org, wsPath)
		if err != nil {
			wq.Errors = append(wq.Errors, SourceError{
				Org:   org.Name,
				Error: err.Error(),
			})
			continue
		}

		wq.Sources = append(wq.Sources, Source{
			Org:       org.Name,
			Path:      wsPath,
			BeadCount: len(beads),
		})
		wq.Beads = append(wq.Beads, beads...)
	}

	SortBeads(wq.Beads)
	return wq, nil
}

// DiscoverOrg scans a single org by name.
func (d *Discoverer) DiscoverOrg(orgName string) (*WorkQueue, error) {
	cfg, err := d.configStore.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ockham: config load degraded: %v (using defaults)\n", err)
	}

	var org *config.Org
	for i := range cfg.Orgs {
		if cfg.Orgs[i].Name == orgName {
			org = &cfg.Orgs[i]
			break
		}
	}
	if org == nil {
		return nil, fmt.Errorf("org %q not found in config", orgName)
	}
	if org.SAML {
		return nil, fmt.Errorf("org %q is a SAML org — discovery not supported", orgName)
	}

	wm, err := d.wsStore.Load()
	if err != nil {
		return nil, fmt.Errorf("loading workspaces: %w", err)
	}

	wq := NewWorkQueue()

	wsPath, ok := wm[orgName]
	if !ok {
		wq.Errors = append(wq.Errors, SourceError{
			Org:   orgName,
			Error: fmt.Sprintf("no workspace path configured for org %q", orgName),
		})
		return wq, nil
	}

	beads, err := d.scanOrg(*org, wsPath)
	if err != nil {
		wq.Errors = append(wq.Errors, SourceError{
			Org:   orgName,
			Error: err.Error(),
		})
		return wq, nil
	}

	wq.Sources = append(wq.Sources, Source{
		Org:       orgName,
		Path:      wsPath,
		BeadCount: len(beads),
	})
	wq.Beads = beads

	SortBeads(wq.Beads)
	return wq, nil
}

// AutoDiscover scans $HOME for .beads/ directories and returns a WorkspaceMap.
func (d *Discoverer) AutoDiscover() (*WorkspaceMap, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	m, err := ScanHome(home)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

// scanOrg runs `bd list --status=open --json` in the workspace directory.
func (d *Discoverer) scanOrg(org config.Org, wsPath string) ([]DiscoveredBead, error) {
	info, err := os.Stat(wsPath)
	if err != nil {
		return nil, fmt.Errorf("workspace path %q: %w", wsPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path %q is not a directory", wsPath)
	}

	beadsDir := fmt.Sprintf("%s/.beads", wsPath)
	if _, err := os.Stat(beadsDir); err != nil {
		return nil, fmt.Errorf("no .beads/ directory in %q", wsPath)
	}

	cmd := exec.Command("bd", "list", "--status=open", "--json")
	cmd.Dir = wsPath
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bd list: %w\nstderr: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("bd list: %w", err)
	}

	var raw []bdBead
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	beads := make([]DiscoveredBead, 0, len(raw))
	for _, b := range raw {
		lane := extractLane(b.Labels)
		beads = append(beads, DiscoveredBead{
			ID:       b.ID,
			Title:    b.Title,
			Priority: NormalizePriority(b.Priority),
			Lane:     lane,
			Status:   b.Status,
			Type:     b.Type,
			Org:      org.Name,
			Repo:     repoFromPath(wsPath),
		})
	}

	return beads, nil
}

// extractLane finds the first "lane:<name>" label.
func extractLane(labels []string) string {
	for _, label := range labels {
		if strings.HasPrefix(label, "lane:") {
			return strings.TrimPrefix(label, "lane:")
		}
	}
	return ""
}

// repoFromPath extracts the directory basename as a repo name.
func repoFromPath(wsPath string) string {
	return strings.TrimRight(wsPath, string(os.PathSeparator))
}

// ParseBDOutput parses bd list JSON output into DiscoveredBeads.
// Exported for testing.
func ParseBDOutput(data []byte, org, repo string) ([]DiscoveredBead, error) {
	var raw []bdBead
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	beads := make([]DiscoveredBead, 0, len(raw))
	for _, b := range raw {
		lane := extractLane(b.Labels)
		beads = append(beads, DiscoveredBead{
			ID:       b.ID,
			Title:    b.Title,
			Priority: NormalizePriority(b.Priority),
			Lane:     lane,
			Status:   b.Status,
			Type:     b.Type,
			Org:      org,
			Repo:     repo,
		})
	}
	return beads, nil
}

// NewWorkQueueFromBeads creates a WorkQueue with the given beads, sorted.
func NewWorkQueueFromBeads(beads []DiscoveredBead) *WorkQueue {
	SortBeads(beads)
	return &WorkQueue{
		Beads:     beads,
		FetchedAt: time.Now().Unix(),
	}
}
