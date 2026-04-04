package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/mistakeknot/Ockham/internal/governor"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/mistakeknot/Ockham/internal/scoring"
	"github.com/spf13/cobra"
)

var dispatchJSON bool

var dispatchCmd = &cobra.Command{
	Use:   "dispatch",
	Short: "Dispatch weight operations",
}

var dispatchAdviseCmd = &cobra.Command{
	Use:   "advise",
	Short: "Show current weight offsets for all open beads",
	RunE:  runDispatchAdvise,
}

func init() {
	dispatchAdviseCmd.Flags().BoolVar(&dispatchJSON, "json", false, "Output as JSON")
	dispatchCmd.AddCommand(dispatchAdviseCmd)
	rootCmd.AddCommand(dispatchCmd)
}

// beadsFromBD shells out to bd to get open beads with lane labels.
// NOTE: bd list --json label format ("lane:<name>") is undocumented — if
// bd changes this, lane falls through to "open" default silently.
func beadsFromBD() ([]scoring.BeadInfo, error) {
	cmd := exec.Command("bd", "list", "--status=open", "--json")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("bd list: %w\nstderr: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("bd list: %w", err)
	}

	var beads []struct {
		ID     string   `json:"id"`
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	infos := make([]scoring.BeadInfo, 0, len(beads))
	for _, b := range beads {
		lane := ""
		for _, label := range b.Labels {
			if strings.HasPrefix(label, "lane:") {
				lane = strings.TrimPrefix(label, "lane:")
				break
			}
		}
		infos = append(infos, scoring.BeadInfo{ID: b.ID, Lane: lane})
	}
	return infos, nil
}

func runDispatchAdvise(cmd *cobra.Command, args []string) error {
	is := intent.NewStore(intent.DefaultStorePath())
	hs := halt.New(halt.DefaultSentinelPath())
	g := governor.New(is, hs)

	beads, err := beadsFromBD()
	if err != nil {
		return err
	}

	wv, err := g.Evaluate(cmd.Context(), beads)
	if err != nil {
		return err
	}

	if dispatchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(wv.Offsets)
	}

	// Deterministic table output (sorted by bead ID)
	ids := make([]string, 0, len(wv.Offsets))
	for id := range wv.Offsets {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("%-30s  %s\n", "BEAD", "OFFSET")
	for _, id := range ids {
		fmt.Printf("%-30s  %+d\n", id, wv.Offsets[id])
	}

	return nil
}
