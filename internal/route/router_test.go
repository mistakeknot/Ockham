package route_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/discover"
	"github.com/mistakeknot/Ockham/internal/route"
)

// allowAllGate always approves.
type allowAllGate struct{}

func (allowAllGate) Allowed(int) bool { return true }

// blockAllGate always blocks.
type blockAllGate struct{}

func (blockAllGate) Allowed(int) bool { return false }

// tierGate approves only tiers <= max.
type tierGate struct{ max int }

func (g tierGate) Allowed(tier int) bool { return tier <= g.max }

// testStore creates a config.Store backed by a temp file with default config.
func testStore(t *testing.T) *config.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ockham.yaml")
	s := config.NewStore(path)
	if err := s.Save(config.DefaultConfig()); err != nil {
		t.Fatalf("saving test config: %v", err)
	}
	return s
}

// testStoreWithAgents creates a config.Store with custom agents.
func testStoreWithAgents(t *testing.T, agents config.Agents) *config.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ockham.yaml")
	s := config.NewStore(path)
	cfg := config.DefaultConfig()
	cfg.Agents = agents
	if err := s.Save(cfg); err != nil {
		t.Fatalf("saving test config: %v", err)
	}
	return s
}

// emptyStore creates a config.Store with no agents.
func emptyStore(t *testing.T) *config.Store {
	t.Helper()
	return testStoreWithAgents(t, config.Agents{})
}

func TestRouteOne_BasicCapabilityMatch(t *testing.T) {
	agents := config.Agents{
		"coder": {
			Model:        "m1",
			CostTier:     1,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
		"planner": {
			Model:        "m2",
			CostTier:     0,
			Capabilities: []string{"planning", "research"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	// Bug bead needs coding + debugging → coder should win.
	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Title: "Fix crash", Type: "bug", Org: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != route.Routed {
		t.Fatalf("Status = %s, want routed", d.Status)
	}
	if d.Agent != "coder" {
		t.Errorf("Agent = %q, want %q", d.Agent, "coder")
	}
	if d.Score != 1.0 {
		t.Errorf("Score = %f, want 1.0", d.Score)
	}
}

func TestRouteOne_TieBreakByLowerCost(t *testing.T) {
	agents := config.Agents{
		"cheap": {
			Model:        "m1",
			CostTier:     0,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
		"expensive": {
			Model:        "m2",
			CostTier:     2,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Type: "bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Agent != "cheap" {
		t.Errorf("Agent = %q, want %q (lower cost tier wins)", d.Agent, "cheap")
	}
}

func TestRouteOne_TieBreakByName(t *testing.T) {
	agents := config.Agents{
		"beta": {
			Model:        "m1",
			CostTier:     0,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
		"alpha": {
			Model:        "m2",
			CostTier:     0,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Type: "bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Agent != "alpha" {
		t.Errorf("Agent = %q, want %q (alphabetical tie-break)", d.Agent, "alpha")
	}
}

func TestRoute_BlockedByCostGate(t *testing.T) {
	store := testStore(t)
	r := route.NewRouter(store, blockAllGate{})

	wq := &discover.WorkQueue{
		Beads: []discover.DiscoveredBead{
			{ID: "b1", Title: "Fix it", Type: "bug"},
		},
	}
	rt, err := r.Route(wq)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.Decisions) != 1 {
		t.Fatalf("len(Decisions) = %d, want 1", len(rt.Decisions))
	}
	if rt.Decisions[0].Status != route.Blocked {
		t.Errorf("Status = %s, want blocked", rt.Decisions[0].Status)
	}
	if rt.Summary.Blocked != 1 {
		t.Errorf("Summary.Blocked = %d, want 1", rt.Summary.Blocked)
	}
}

func TestRoute_DryRunIgnoresCostGate(t *testing.T) {
	store := testStore(t)
	r := route.NewRouter(store, blockAllGate{})

	wq := &discover.WorkQueue{
		Beads: []discover.DiscoveredBead{
			{ID: "b1", Title: "Fix it", Type: "bug"},
		},
	}
	rt, err := r.DryRun(wq)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Decisions[0].Status != route.Routed {
		t.Errorf("DryRun should ignore cost gate, got Status = %s", rt.Decisions[0].Status)
	}
}

func TestRoute_NoAgentsDeferred(t *testing.T) {
	store := emptyStore(t)
	r := route.NewRouter(store, allowAllGate{})

	wq := &discover.WorkQueue{
		Beads: []discover.DiscoveredBead{
			{ID: "b1", Type: "task"},
		},
	}
	rt, err := r.Route(wq)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Decisions[0].Status != route.Deferred {
		t.Errorf("Status = %s, want deferred (no agents)", rt.Decisions[0].Status)
	}
}

func TestRoute_AlternativesCapped(t *testing.T) {
	agents := config.Agents{
		"a1": {Model: "m", CostTier: 0, Capabilities: []string{"coding", "debugging"}, Runtime: "t"},
		"a2": {Model: "m", CostTier: 1, Capabilities: []string{"coding", "debugging"}, Runtime: "t"},
		"a3": {Model: "m", CostTier: 2, Capabilities: []string{"coding", "debugging"}, Runtime: "t"},
		"a4": {Model: "m", CostTier: 3, Capabilities: []string{"coding", "debugging"}, Runtime: "t"},
		"a5": {Model: "m", CostTier: 3, Capabilities: []string{"coding", "debugging"}, Runtime: "t"},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	d, err := r.RouteOne(discover.DiscoveredBead{ID: "b1", Type: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Alternatives) > 3 {
		t.Errorf("Alternatives = %d, want <= 3", len(d.Alternatives))
	}
}

func TestRoute_PartialCapabilityScore(t *testing.T) {
	agents := config.Agents{
		"half": {
			Model:        "m1",
			CostTier:     0,
			Capabilities: []string{"coding"}, // has 1 of 2 for "bug"
			Runtime:      "test",
		},
		"full": {
			Model:        "m2",
			CostTier:     0,
			Capabilities: []string{"coding", "debugging"}, // has 2 of 2
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Type: "bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Agent != "full" {
		t.Errorf("Agent = %q, want %q (higher capability score)", d.Agent, "full")
	}
	if d.Score != 1.0 {
		t.Errorf("Score = %f, want 1.0", d.Score)
	}
}

func TestRoute_AvailabilityFilter(t *testing.T) {
	agents := config.Agents{
		"busy": {
			Model:        "m1",
			CostTier:     0,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
		"idle": {
			Model:        "m2",
			CostTier:     1,
			Capabilities: []string{"coding", "debugging"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})
	r.SetAvailability(func(name string) bool {
		return name != "busy"
	})

	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Type: "bug",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Agent != "idle" {
		t.Errorf("Agent = %q, want %q (busy agent filtered)", d.Agent, "idle")
	}
}

func TestRoute_UnknownBeadType(t *testing.T) {
	agents := config.Agents{
		"coder": {
			Model:        "m1",
			CostTier:     0,
			Capabilities: []string{"coding"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	r := route.NewRouter(store, allowAllGate{})

	d, err := r.RouteOne(discover.DiscoveredBead{
		ID: "b1", Type: "unknown-type",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Unknown type → 0 required capabilities → score 0 for all → still routed
	if d.Status != route.Routed {
		t.Errorf("Status = %s, want routed (unknown type still gets routed)", d.Status)
	}
}

func TestRoute_Deterministic(t *testing.T) {
	store := testStore(t)
	r := route.NewRouter(store, allowAllGate{})
	bead := discover.DiscoveredBead{ID: "b1", Type: "task"}

	var first string
	for i := 0; i < 10; i++ {
		d, err := r.RouteOne(bead)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = d.Agent
		} else if d.Agent != first {
			t.Fatalf("RouteOne not deterministic: run %d got %q, want %q", i, d.Agent, first)
		}
	}
}

func TestRoute_EmptyQueue(t *testing.T) {
	store := testStore(t)
	r := route.NewRouter(store, allowAllGate{})

	wq := &discover.WorkQueue{}
	rt, err := r.Route(wq)
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.Decisions) != 0 {
		t.Errorf("len(Decisions) = %d, want 0", len(rt.Decisions))
	}
	if rt.Summary.Total != 0 {
		t.Errorf("Summary.Total = %d, want 0", rt.Summary.Total)
	}
}

func TestRoute_TierGatePartialBlock(t *testing.T) {
	agents := config.Agents{
		"free-agent": {
			Model:        "local",
			CostTier:     0,
			Capabilities: []string{"coding", "execution"},
			Runtime:      "test",
		},
		"paid-agent": {
			Model:        "cloud",
			CostTier:     2,
			Capabilities: []string{"coding", "execution"},
			Runtime:      "test",
		},
	}
	store := testStoreWithAgents(t, agents)
	// Only approve tier 0.
	r := route.NewRouter(store, tierGate{max: 0})

	d, err := r.RouteOne(discover.DiscoveredBead{ID: "b1", Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Agent != "free-agent" {
		t.Errorf("Agent = %q, want %q (paid agent blocked)", d.Agent, "free-agent")
	}
}

func TestRoute_MultipleBeads(t *testing.T) {
	store := testStore(t)
	r := route.NewRouter(store, allowAllGate{})

	wq := &discover.WorkQueue{
		Beads: []discover.DiscoveredBead{
			{ID: "b1", Type: "bug"},
			{ID: "b2", Type: "epic"},
			{ID: "b3", Type: "research"},
		},
	}
	rt, err := r.Route(wq)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", rt.Summary.Total)
	}
	for _, d := range rt.Decisions {
		if d.Status != route.Routed {
			t.Errorf("bead %s: Status = %s, want routed", d.BeadID, d.Status)
		}
	}
}

func TestRoute_ConfigLoadError(t *testing.T) {
	// Point to a file that exists but has invalid YAML.
	dir := t.TempDir()
	path := filepath.Join(dir, "ockham.yaml")
	os.WriteFile(path, []byte("not: valid: yaml: ["), 0644)

	store := config.NewStore(path)
	r := route.NewRouter(store, allowAllGate{})

	// Config.Load returns (default, error) for invalid YAML — Router propagates the error.
	_, err := r.RouteOne(discover.DiscoveredBead{ID: "b1", Type: "task"})
	if err == nil {
		t.Fatal("expected error for invalid config YAML")
	}
}
