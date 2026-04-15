package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Display factory health state as JSON (machine-readable)",
	RunE:  runHealth,
}

func init() {
	rootCmd.AddCommand(healthCmd)
}

// HealthOutput is the JSON structure for factory health.
type HealthOutput struct {
	Halted        bool                       `json:"halted"`
	HaltReason    *HaltReasonOutput          `json:"halt_reason"`
	Signals       map[string]SignalOutput     `json:"signals"`
	Pleasure      map[string]PleasureOutput   `json:"pleasure"`
	Observation   *ObservationHealthOutput    `json:"observation"`
	Themes        []string                   `json:"themes"`
	LastCheck     int64                      `json:"last_check"`
	SchemaVersion int                        `json:"schema_version"`
}

// ObservationHealthOutput reports observation data availability.
type ObservationHealthOutput struct {
	Available   bool  `json:"available"`
	LastCollect int64 `json:"last_collect,omitempty"`
	MetricCount int   `json:"metric_count"`
}

// HaltReasonOutput is the structured halt reason.
type HaltReasonOutput struct {
	Code        string   `json:"code"`
	FiredThemes []string `json:"fired_themes"`
	FiredAt     int64    `json:"fired_at"`
}

// SignalOutput is the per-theme signal state.
type SignalOutput struct {
	Status            string  `json:"status"`
	DriftPct          float64 `json:"drift_pct"`
	AdvisoryOffset    int     `json:"advisory_offset"`
	AtAdvisoryFloor   bool    `json:"at_advisory_floor"`
	ConsecutiveClears int     `json:"consecutive_clears"`
}

// PleasureOutput is the per-signal pleasure trend.
type PleasureOutput struct {
	Trend string  `json:"trend"`
	Value float64 `json:"value"`
}

// runHealth reads all data from signals.db persisted state — never calls evaluateSignals().
func runHealth(cmd *cobra.Command, args []string) error {
	db, err := signals.NewDB(signals.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("signals.db: %w", err)
	}
	defer db.Close()

	h := halt.New(halt.DefaultSentinelPath())
	halted := h.IsHalted()

	output := HealthOutput{
		Halted:        halted,
		Signals:       make(map[string]SignalOutput),
		Pleasure:      make(map[string]PleasureOutput),
		SchemaVersion: 3,
	}

	// Read halt context if halted
	if halted {
		data, err := os.ReadFile(h.Path())
		if err == nil {
			var record struct {
				Code        string   `json:"code"`
				FiredThemes []string `json:"triggered_themes"`
				Timestamp   int64    `json:"timestamp"`
			}
			if json.Unmarshal(data, &record) == nil && record.Code != "" {
				output.HaltReason = &HaltReasonOutput{
					Code:        record.Code,
					FiredThemes: record.FiredThemes,
					FiredAt:     record.Timestamp,
				}
			}
		}
	}

	// All reads in a single transaction for snapshot consistency
	// NOTE: modernc.org/sqlite ignores ReadOnly — we rely on query-only access
	tx, err := db.Conn().BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	cfg := anomaly.DefaultConfig()

	// Read INFORM signals
	rows, err := tx.Query("SELECT key, value FROM signal_state WHERE key LIKE 'inform:%'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			rows.Close()
			return err
		}
		theme := strings.TrimPrefix(key, "inform:")
		var sig anomaly.ThemeSignal
		if json.Unmarshal([]byte(val), &sig) != nil {
			continue
		}
		output.Signals[theme] = SignalOutput{
			Status:            string(sig.Status),
			DriftPct:          sig.DriftPct,
			AdvisoryOffset:    sig.AdvisoryOffset,
			AtAdvisoryFloor:   sig.Status == anomaly.StatusFired && sig.AdvisoryOffset <= -cfg.MaxAdvisoryPerCycle,
			ConsecutiveClears: sig.ConsecutiveClears,
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Read pleasure signals
	rows, err = tx.Query("SELECT key, value FROM signal_state WHERE key LIKE 'pleasure:%'")
	if err != nil {
		return err
	}
	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			rows.Close()
			return err
		}
		var sig anomaly.PleasureSignal
		if json.Unmarshal([]byte(val), &sig) != nil {
			continue
		}
		output.Pleasure[sig.Name] = PleasureOutput{
			Trend: string(sig.Trend),
			Value: sig.Value,
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// last_check = MAX(updated_at) from signal_state
	var lastCheck *int64
	tx.QueryRow("SELECT MAX(updated_at) FROM signal_state").Scan(&lastCheck)
	if lastCheck != nil {
		output.LastCheck = *lastCheck
	}

	// Observation stats
	since24h := time.Now().Unix() - 24*3600
	obsStats, err := db.GetObservationStats(since24h)
	if err != nil {
		// Degrade gracefully — observation is advisory
		fmt.Fprintf(os.Stderr, "ockham: observation stats degraded: %v\n", err)
	} else if obsStats.Available {
		output.Observation = &ObservationHealthOutput{
			Available:   true,
			LastCollect: obsStats.LastCollect,
			MetricCount: obsStats.MetricCount,
		}
	} else {
		output.Observation = &ObservationHealthOutput{Available: false}
	}

	// Themes from bead_metrics
	themeRows, err := tx.Query("SELECT DISTINCT theme FROM bead_metrics ORDER BY theme")
	if err != nil {
		return err
	}
	for themeRows.Next() {
		var t string
		themeRows.Scan(&t)
		output.Themes = append(output.Themes, t)
	}
	themeRows.Close()
	if err := themeRows.Err(); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
