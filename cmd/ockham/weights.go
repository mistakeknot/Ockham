package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/mistakeknot/Ockham/internal/writer"
	"github.com/spf13/cobra"
)

var (
	weightsPath  string
	weightsJSON  bool
	weightsForce bool
)

var weightsCmd = &cobra.Command{
	Use:   "weights",
	Short: "Manage the Ockham weight-offset file consumed by Clavain dispatch",
}

var weightsWriteCmd = &cobra.Command{
	Use:   "write",
	Short: "Compute current weight offsets from authority + CONSTRAIN state and write the file atomically",
	RunE:  runWeightsWrite,
}

var weightsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current weight offsets — from disk by default, --force recomputes",
	RunE:  runWeightsShow,
}

func init() {
	weightsCmd.PersistentFlags().StringVar(&weightsPath, "path", "", "Override weight-offsets file path (default: ~/.config/ockham/weight-offsets.json)")
	weightsShowCmd.Flags().BoolVar(&weightsJSON, "json", false, "Output as JSON")
	weightsShowCmd.Flags().BoolVar(&weightsForce, "force", false, "Recompute from DB instead of reading disk")

	weightsCmd.AddCommand(weightsWriteCmd, weightsShowCmd)
	rootCmd.AddCommand(weightsCmd)
}

func resolveWeightsPath() string {
	if weightsPath != "" {
		return weightsPath
	}
	return writer.DefaultPath()
}

func runWeightsWrite(cmd *cobra.Command, args []string) error {
	db, err := signals.NewDB(signals.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("signals.db: %w", err)
	}
	defer db.Close()

	path := resolveWeightsPath()
	w := writer.New(db)
	payload, err := w.ComputeAndWrite(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "ockham: wrote %d themes, %d agents to %s\n",
		len(payload.Themes), len(payload.Agents), path)
	return nil
}

func runWeightsShow(cmd *cobra.Command, args []string) error {
	var payload writer.Weights

	if weightsForce {
		db, err := signals.NewDB(signals.DefaultDBPath())
		if err != nil {
			return fmt.Errorf("signals.db: %w", err)
		}
		defer db.Close()
		payload, err = writer.New(db).Compute()
		if err != nil {
			return err
		}
	} else {
		data, err := os.ReadFile(resolveWeightsPath())
		if err != nil {
			return fmt.Errorf("read weights: %w", err)
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("parse weights: %w", err)
		}
	}

	if weightsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "computed_at\t%s\n", time.Unix(payload.ComputedAt, 0).Format(time.RFC3339))
	fmt.Fprintln(tw, "SCOPE\tKEY\tOFFSET")

	themeKeys := sortedKeys(payload.Themes)
	for _, k := range themeKeys {
		fmt.Fprintf(tw, "theme\t%s\t%+d\n", k, payload.Themes[k])
	}
	agentKeys := sortedKeys(payload.Agents)
	for _, k := range agentKeys {
		fmt.Fprintf(tw, "agent\t%s\t%+d\n", k, payload.Agents[k])
	}
	return tw.Flush()
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
