package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/discover"
	"github.com/spf13/cobra"
)

var (
	discoverOrg  string
	discoverJSON bool
	discoverAuto bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover work across GitHub orgs",
	RunE:  runDiscover,
}

var discoverScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Auto-discover workspace paths by scanning home directory",
	RunE:  runDiscoverScan,
}

func init() {
	discoverCmd.Flags().StringVar(&discoverOrg, "org", "", "Scan single org")
	discoverCmd.Flags().BoolVar(&discoverJSON, "json", false, "Output as JSON")
	discoverCmd.Flags().BoolVar(&discoverAuto, "auto", false, "Auto-discover workspace paths")

	discoverCmd.AddCommand(discoverScanCmd)
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	cs := config.NewStore(config.DefaultStorePath())
	ws := discover.NewWorkspaceStore(discover.DefaultWorkspacePath())
	d := discover.NewDiscoverer(cs, ws)

	// Auto-discover if requested
	if discoverAuto {
		m, err := d.AutoDiscover()
		if err != nil {
			return fmt.Errorf("auto-discover: %w", err)
		}
		if err := ws.Save(*m); err != nil {
			return fmt.Errorf("saving workspaces: %w", err)
		}
		fmt.Fprintf(os.Stderr, "ockham: auto-discovered %d workspace(s)\n", len(*m))
	}

	var wq *discover.WorkQueue
	var err error

	if discoverOrg != "" {
		wq, err = d.DiscoverOrg(discoverOrg)
	} else {
		wq, err = d.DiscoverAll()
	}
	if err != nil {
		return err
	}

	// Print errors to stderr
	for _, e := range wq.Errors {
		fmt.Fprintf(os.Stderr, "ockham: %s: %s\n", e.Org, e.Error)
	}

	if discoverJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wq)
	}

	// Table output
	if len(wq.Beads) == 0 {
		fmt.Println("No beads found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PRIORITY\tID\tTITLE\tORG\tLANE\tSTATUS")
	for _, b := range wq.Beads {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.Priority, b.ID, truncate(b.Title, 50), b.Org, b.Lane, b.Status)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\n%d bead(s) from %d source(s)\n", len(wq.Beads), len(wq.Sources))
	return nil
}

func runDiscoverScan(cmd *cobra.Command, args []string) error {
	cs := config.NewStore(config.DefaultStorePath())
	ws := discover.NewWorkspaceStore(discover.DefaultWorkspacePath())
	d := discover.NewDiscoverer(cs, ws)

	m, err := d.AutoDiscover()
	if err != nil {
		return fmt.Errorf("auto-discover: %w", err)
	}

	if len(*m) == 0 {
		fmt.Println("No workspaces found with .beads/ directories.")
		return nil
	}

	if err := ws.Save(*m); err != nil {
		return fmt.Errorf("saving workspaces: %w", err)
	}

	// Deterministic output
	names := make([]string, 0, len(*m))
	for name := range *m {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("Discovered %d workspace(s):\n", len(*m))
	for _, name := range names {
		fmt.Printf("  %s → %s\n", name, (*m)[name])
	}
	fmt.Printf("\nWritten to %s\n", ws.Path())
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
