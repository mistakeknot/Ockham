package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/spf13/cobra"
)

var signalsJSON bool

var signalsCmd = &cobra.Command{
	Use:   "signals",
	Short: "Show current INFORM signal state and pleasure signals per theme",
	RunE:  runSignals,
}

func init() {
	signalsCmd.Flags().BoolVar(&signalsJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(signalsCmd)
}

func runSignals(cmd *cobra.Command, args []string) error {
	db, err := signals.NewDB(signals.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("signals.db: %w", err)
	}
	defer db.Close()

	// Collect all signal_state entries
	rows, err := db.Conn().Query("SELECT key, value, updated_at FROM signal_state ORDER BY key")
	if err != nil {
		return err
	}
	defer rows.Close()

	type signalEntry struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		UpdatedAt int64  `json:"updated_at"`
	}

	var informSignals []anomaly.ThemeSignal
	pleasureByTheme := make(map[string][]anomaly.PleasureSignal)
	themes := make(map[string]bool)

	for rows.Next() {
		var e signalEntry
		if err := rows.Scan(&e.Key, &e.Value, &e.UpdatedAt); err != nil {
			return err
		}

		if strings.HasPrefix(e.Key, "inform:") {
			theme := strings.TrimPrefix(e.Key, "inform:")
			var sig anomaly.ThemeSignal
			if err := json.Unmarshal([]byte(e.Value), &sig); err != nil {
				continue
			}
			sig.Theme = theme
			informSignals = append(informSignals, sig)
			themes[theme] = true
		} else if strings.HasPrefix(e.Key, "pleasure:") {
			var sig anomaly.PleasureSignal
			if err := json.Unmarshal([]byte(e.Value), &sig); err != nil {
				continue
			}
			pleasureByTheme[sig.Theme] = append(pleasureByTheme[sig.Theme], sig)
			themes[sig.Theme] = true
		}
	}

	if signalsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"inform":   informSignals,
			"pleasure": pleasureByTheme,
		})
	}

	if len(themes) == 0 {
		fmt.Println("No signal data. Run 'ockham check' to evaluate.")
		return nil
	}

	// Sort themes
	sortedThemes := make([]string, 0, len(themes))
	for t := range themes {
		sortedThemes = append(sortedThemes, t)
	}
	sort.Strings(sortedThemes)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "THEME\tINFORM\tDRIFT\tADVISORY\tPASS_RATE\tCYCLE_TIME\tCOST\tLAST_EVAL")

	informByTheme := make(map[string]anomaly.ThemeSignal)
	for _, sig := range informSignals {
		informByTheme[sig.Theme] = sig
	}

	for _, theme := range sortedThemes {
		inform := informByTheme[theme]
		status := string(inform.Status)
		if status == "" {
			status = "-"
		}
		drift := "-"
		if inform.DriftPct != 0 {
			drift = fmt.Sprintf("%.1f%%", inform.DriftPct*100)
		}
		advisory := fmt.Sprintf("%+d", inform.AdvisoryOffset)
		lastEval := "-"
		if inform.LastEvalAt > 0 {
			lastEval = time.Unix(inform.LastEvalAt, 0).Format("15:04:05")
		}

		passRate := "-"
		cycleTime := "-"
		cost := "-"
		for _, p := range pleasureByTheme[theme] {
			switch p.Name {
			case "first_attempt_pass_rate":
				passRate = string(p.Trend)
			case "cycle_time_p50_trend":
				cycleTime = string(p.Trend)
			case "cost_per_landed_change_trend":
				cost = string(p.Trend)
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			theme, status, drift, advisory, passRate, cycleTime, cost, lastEval)
	}
	w.Flush()

	return nil
}
