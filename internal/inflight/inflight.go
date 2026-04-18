// Package inflight provides query and event-emission for in-flight beads
// when CONSTRAIN fires on a theme. It enables callers to react appropriately
// (e.g., block new dispatch while allowing in-flight beads to finish).
package inflight

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Policy determines the advisory reaction to constrain firing on a theme.
type Policy int

const (
	// PolicyFinishThenBlock: in-flight beads finish; new dispatch blocked (default).
	PolicyFinishThenBlock Policy = iota
	// PolicyGracePeriod: wait N seconds then abort (advisory — caller acts).
	PolicyGracePeriod
	// PolicyPreempt: caller should immediately abort (advisory).
	PolicyPreempt
)

// InFlightBead represents a bead currently claimed and mid-execution on a theme.
type InFlightBead struct {
	BeadID    string // Bead ID; validated to ^[A-Za-z0-9._-]+$
	Theme     string // Theme extracted from lane:<name> label
	Assignee  string // Agent or session ID that claimed it
	ClaimedAt int64  // Unix seconds; 0 when unknown
}

// Event represents a constrain event emitted for in-flight beads.
type Event struct {
	BeadID    string
	Theme     string
	Policy    Policy
	EmittedAt int64  // Unix seconds
	Notes     string // Optional caller notes
}

// MarshalJSON encodes Event as JSON with string policy name.
func (e Event) MarshalJSON() ([]byte, error) {
	type Alias Event
	return json.Marshal(&struct {
		*Alias
		Policy string `json:"policy"`
	}{
		Alias:  (*Alias)(&e),
		Policy: policyString(e.Policy),
	})
}

// UnmarshalJSON decodes Event from JSON, converting string policy to Policy value.
func (e *Event) UnmarshalJSON(data []byte) error {
	type Alias Event
	aux := &struct {
		*Alias
		Policy string `json:"policy"`
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.Policy = policyFromString(aux.Policy)
	return nil
}

func policyFromString(s string) Policy {
	switch s {
	case "finish_then_block":
		return PolicyFinishThenBlock
	case "grace_period":
		return PolicyGracePeriod
	case "preempt":
		return PolicyPreempt
	default:
		return PolicyFinishThenBlock
	}
}

func policyString(p Policy) string {
	switch p {
	case PolicyFinishThenBlock:
		return "finish_then_block"
	case PolicyGracePeriod:
		return "grace_period"
	case PolicyPreempt:
		return "preempt"
	default:
		return "unknown"
	}
}

// Lister provides in-flight bead queries. Implementations may shell out to
// external tools or query internal state.
type Lister interface {
	ListByTheme(theme string) ([]InFlightBead, error)
}

// Controller manages inflight bead reactions to constrain events.
type Controller struct {
	policy Policy
	emit   func(Event)
	lister Lister
	now    func() time.Time
}

// Option configures a Controller.
type Option func(*Controller)

// WithEmitter sets a custom event sink. Default writes JSONL to
// ~/.config/ockham/inflight-events.jsonl.
func WithEmitter(emit func(Event)) Option {
	return func(c *Controller) {
		if emit != nil {
			c.emit = emit
		}
	}
}

// WithLister sets a custom bead lister. Default uses the real bd CLI.
func WithLister(l Lister) Option {
	return func(c *Controller) {
		if l != nil {
			c.lister = l
		}
	}
}

// WithNow sets a custom time provider (for testing).
func WithNow(fn func() time.Time) Option {
	return func(c *Controller) {
		if fn != nil {
			c.now = fn
		}
	}
}

// New constructs a Controller with the given policy and options.
func New(policy Policy, opts ...Option) *Controller {
	c := &Controller{
		policy: policy,
		emit:   defaultEmitter,
		lister: &realLister{},
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// HandleConstrain lists in-flight beads for the theme, emits one Event per
// bead with the configured policy, and returns the slice. Errors in querying
// beads are returned; errors in emitting events are logged to stderr but do
// not fail the call (fail-open).
func (c *Controller) HandleConstrain(theme, reason string) ([]InFlightBead, error) {
	if theme == "" {
		return nil, fmt.Errorf("inflight: theme is required")
	}

	beads, err := c.lister.ListByTheme(theme)
	if err != nil {
		return nil, err
	}

	now := c.now()
	for _, bead := range beads {
		evt := Event{
			BeadID:    bead.BeadID,
			Theme:     bead.Theme,
			Policy:    c.policy,
			EmittedAt: now.Unix(),
			Notes:     reason,
		}
		c.emit(evt)
	}

	return beads, nil
}

// defaultEmitter writes events to ~/.config/ockham/inflight-events.jsonl.
func defaultEmitter(evt Event) {
	path := defaultEventPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "inflight: mkdir failed: %v\n", err)
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inflight: open failed: %v\n", err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(evt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "inflight: marshal failed: %v\n", err)
		return
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "inflight: write failed: %v\n", err)
	}
}

// defaultEventPath returns ~/.config/ockham/inflight-events.jsonl.
func defaultEventPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(cfg, "ockham", "inflight-events.jsonl")
}

// realLister shells out to `bd list --status=in_progress --json` and filters
// beads by theme (extracted from lane:<name> label). Tolerates bd being
// absent (returns empty slice, nil error).
type realLister struct{}

func (rl *realLister) ListByTheme(theme string) ([]InFlightBead, error) {
	data, err := bdListInProgress()
	if err != nil {
		// Tolerate bd absence: return empty slice, nil error.
		if isNotFoundError(err) {
			return []InFlightBead{}, nil
		}
		return nil, err
	}

	var beads []struct {
		ID        string   `json:"id"`
		Assignee  string   `json:"assignee"`
		Labels    []string `json:"labels"`
		ClaimedAt int64    `json:"claimed_at"`
	}
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, fmt.Errorf("inflight: parse bd output: %w", err)
	}

	var result []InFlightBead
	for _, b := range beads {
		beadTheme := extractThemeFromLabels(b.Labels)
		if beadTheme == "" {
			// Bead has no lane label; skip it.
			continue
		}
		if beadTheme != theme {
			// Bead is on a different theme.
			continue
		}

		// Validate bead ID for defense-in-depth.
		if !isValidBeadID(b.ID) {
			fmt.Fprintf(os.Stderr, "inflight: rejecting invalid bead ID %q\n", b.ID)
			continue
		}

		result = append(result, InFlightBead{
			BeadID:    b.ID,
			Theme:     beadTheme,
			Assignee:  b.Assignee,
			ClaimedAt: b.ClaimedAt,
		})
	}

	return result, nil
}

// bdListInProgress shells out to `bd list --status=in_progress --json`.
func bdListInProgress() ([]byte, error) {
	cmd := exec.Command("bash", "-c", "bd list --status=in_progress --json")
	return cmd.Output()
}

// extractThemeFromLabels finds and returns the value of the first lane:<name>
// label, or "" if none found.
func extractThemeFromLabels(labels []string) string {
	for _, label := range labels {
		if strings.HasPrefix(label, "lane:") {
			return strings.TrimPrefix(label, "lane:")
		}
	}
	return ""
}

// isValidBeadID checks that beadID matches ^[A-Za-z0-9._-]+$.
func isValidBeadID(beadID string) bool {
	if beadID == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[A-Za-z0-9._-]+$`, beadID)
	return matched
}

// isNotFoundError checks if an error indicates bd was not found.
func isNotFoundError(err error) bool {
	// Accept various "not found" errors: exec.ErrNotFound (Go 1.19+) or
	// "command not found" in stderr/message.
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "no such file") ||
		strings.Contains(s, "executable file not found")
}
