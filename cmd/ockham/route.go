package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/discover"
	"github.com/mistakeknot/Ockham/internal/route"
	"github.com/spf13/cobra"
)

var (
	routeDryRun bool
	routeJSON   bool
	routeAgent  string
	routeOrg    string
)

var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Route beads to agents based on capability and cost",
	RunE:  runRoute,
}

func init() {
	routeCmd.Flags().BoolVar(&routeDryRun, "dry-run", false, "Preview without cost gates")
	routeCmd.Flags().BoolVar(&routeJSON, "json", false, "Output as JSON")
	routeCmd.Flags().StringVar(&routeAgent, "agent", "", "Route only to specific agent")
	routeCmd.Flags().StringVar(&routeOrg, "org", "", "Route only beads from one org")
	rootCmd.AddCommand(routeCmd)
}

// configCostGate uses the config's auto_approve field to gate costs.
type configCostGate struct {
	tiers map[int]config.CostTierDef
}

func (g *configCostGate) Allowed(tier int) bool {
	def, ok := g.tiers[tier]
	if !ok {
		return false
	}
	return def.AutoApprove
}

func runRoute(cmd *cobra.Command, args []string) error {
	cs := config.NewStore(config.DefaultStorePath())
	ws := discover.NewWorkspaceStore(discover.DefaultWorkspacePath())
	d := discover.NewDiscoverer(cs, ws)

	cfg, err := cs.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	gate := &configCostGate{tiers: cfg.Cost.Tiers}
	router := route.NewRouter(cs, gate)

	// Discover beads.
	var wq *discover.WorkQueue
	if routeOrg != "" {
		wq, err = d.DiscoverOrg(routeOrg)
	} else {
		wq, err = d.DiscoverAll()
	}
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}

	for _, e := range wq.Errors {
		fmt.Fprintf(os.Stderr, "ockham: %s: %s\n", e.Org, e.Error)
	}

	// Route.
	var rt *route.RoutingTable
	if routeDryRun {
		rt, err = router.DryRun(wq)
	} else {
		rt, err = router.Route(wq)
	}
	if err != nil {
		return fmt.Errorf("routing: %w", err)
	}

	// Filter by agent if requested.
	if routeAgent != "" {
		var filtered []route.RoutingDecision
		for _, d := range rt.Decisions {
			if d.Agent == routeAgent {
				filtered = append(filtered, d)
			}
		}
		rt.Decisions = filtered
		rt.ComputeSummary()
	}

	if routeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rt)
	}

	if len(rt.Decisions) == 0 {
		fmt.Println("No routing decisions.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tBEAD\tTYPE\tAGENT\tMODEL\tTIER\tSCORE")
	for _, d := range rt.Decisions {
		agent := d.Agent
		model := d.AgentModel
		if d.Status != route.Routed {
			agent = "-"
			model = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%.2f\n",
			d.Status, truncateRoute(d.BeadID, 12), d.BeadType, agent, model, d.CostTier, d.Score)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d routed, %d blocked, %d deferred (%d total)\n",
		rt.Summary.Routed, rt.Summary.Blocked, rt.Summary.Deferred, rt.Summary.Total)
	return nil
}

func truncateRoute(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
