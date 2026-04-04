package intent

import (
	"fmt"
	"strings"
	"time"
)

// Priority represents a theme's dispatch priority.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

// Offset returns the additive score offset for this priority.
// Asymmetric: high=+6, normal=0, low=-3.
func (p Priority) Offset() int {
	switch p {
	case PriorityHigh:
		return 6
	case PriorityLow:
		return -3
	default:
		return 0
	}
}

func (p Priority) String() string { return string(p) }

// ParsePriority converts a string to Priority, case-insensitive.
func ParsePriority(s string) (Priority, error) {
	switch Priority(strings.ToLower(s)) {
	case PriorityHigh:
		return PriorityHigh, nil
	case PriorityNormal:
		return PriorityNormal, nil
	case PriorityLow:
		return PriorityLow, nil
	default:
		return "", fmt.Errorf("unknown priority %q (valid: high, normal, low)", s)
	}
}

// ThemeBudget is one entry in the intent file.
type ThemeBudget struct {
	Budget   float64  `yaml:"budget"`
	Priority Priority `yaml:"priority"`
}

// Constraints express freeze/focus directives.
type Constraints struct {
	Freeze []string `yaml:"freeze,omitempty"`
	Focus  []string `yaml:"focus,omitempty"`
}

// IntentFile is the on-disk YAML representation.
type IntentFile struct {
	Version     int                    `yaml:"version"`
	Themes      map[string]ThemeBudget `yaml:"themes"`
	Constraints Constraints            `yaml:"constraints"`
	ValidUntil  *time.Time             `yaml:"valid_until,omitempty"`
	UntilBeads  *int                   `yaml:"until_bead_count,omitempty"`
}

// IntentVector is the computed form consumed by the scoring package.
type IntentVector struct {
	Offsets     map[string]int
	Budgets     map[string]float64
	FrozenLanes map[string]bool
}

// DefaultFile returns the hardcoded fallback: single "open" theme, budget 1.0, normal priority.
func DefaultFile() IntentFile {
	return IntentFile{
		Version: 1,
		Themes: map[string]ThemeBudget{
			"open": {Budget: 1.0, Priority: PriorityNormal},
		},
		Constraints: Constraints{},
	}
}

// ToVector computes the IntentVector from a validated IntentFile.
func (f *IntentFile) ToVector() IntentVector {
	v := IntentVector{
		Offsets:     make(map[string]int, len(f.Themes)),
		Budgets:     make(map[string]float64, len(f.Themes)),
		FrozenLanes: make(map[string]bool, len(f.Constraints.Freeze)),
	}
	for name, tb := range f.Themes {
		v.Offsets[name] = tb.Priority.Offset()
		v.Budgets[name] = tb.Budget
	}
	for _, lane := range f.Constraints.Freeze {
		v.FrozenLanes[lane] = true
	}
	return v
}
