package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/spf13/cobra"
)

var (
	resumeConfirm     bool
	resumeConstrained bool
	resumeJSON        bool
)

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume factory after BYPASS halt",
	RunE:  runResume,
}

func init() {
	resumeCmd.Flags().BoolVar(&resumeConfirm, "confirm", false, "Confirm resume (required)")
	resumeCmd.Flags().BoolVar(&resumeConstrained, "constrained", false, "Resume with frozen themes still frozen (Tier 2 stub)")
	resumeCmd.Flags().BoolVar(&resumeJSON, "json", false, "JSON output")
	rootCmd.AddCommand(resumeCmd)
}

// constrainChecker checks for active Tier 2 CONSTRAIN signals.
// Stub for F7 — replaced when Tier 2 ships (F6).
type constrainChecker interface {
	ActiveConstraints() ([]string, error)
}

type nilConstrainChecker struct{}

func (nilConstrainChecker) ActiveConstraints() ([]string, error) { return nil, nil }

func runResume(cmd *cobra.Command, args []string) error {
	h := halt.New(halt.DefaultSentinelPath())

	// 1. Precondition: factory must be halted
	if !h.IsHalted() {
		return fmt.Errorf("factory is not halted — nothing to resume")
	}

	// 2. Read halt context for display
	haltData, _ := os.ReadFile(h.Path())
	var haltRecord struct {
		Reason          string   `json:"reason"`
		Code            string   `json:"code"`
		TriggeredThemes []string `json:"triggered_themes"`
		Timestamp       int64    `json:"timestamp"`
	}
	_ = json.Unmarshal(haltData, &haltRecord)

	// 3. Without --confirm: show preview and exit
	if !resumeConfirm {
		if haltRecord.Timestamp > 0 {
			fmt.Fprintf(os.Stderr, "Factory halted since %s\n", time.Unix(haltRecord.Timestamp, 0).Format(time.RFC3339))
		}
		if haltRecord.Reason != "" {
			fmt.Fprintf(os.Stderr, "Reason: %s\n", haltRecord.Reason)
		}
		if len(haltRecord.TriggeredThemes) > 0 {
			fmt.Fprintf(os.Stderr, "Fired themes: %s\n", strings.Join(haltRecord.TriggeredThemes, ", "))
		}
		fmt.Fprintln(os.Stderr, "All autonomous domains will be reset to supervised tier.")
		fmt.Fprintln(os.Stderr, "\nRun: ockham resume --confirm")
		return fmt.Errorf("resume requires --confirm")
	}

	// 4. Open signals.db
	db, err := signals.NewDB(signals.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("signals.db: %w", err)
	}
	defer db.Close()

	// 5. Dual-sentinel consistency check
	interspectPath := filepath.Join(os.Getenv("HOME"), ".clavain", "interspect", "halt-record.json")
	irData, irErr := os.ReadFile(interspectPath)
	if irErr != nil {
		fmt.Fprintf(os.Stderr, "ockham: warning — factory-paused.json exists but no interspect halt record (possible partial-write during BYPASS trigger)\n")
	} else {
		var ir struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(irData, &ir) == nil && ir.Status != "active" {
			fmt.Fprintf(os.Stderr, "ockham: warning — interspect halt record status=%q (expected 'active') — possible interrupted resume\n", ir.Status)
		}
	}

	// 6. Snapshot pre-halt ratchet_state for forensics
	preHaltSnapshot := snapshotRatchetState(db)

	// 7. Clear interspect halt record (with pre-halt snapshot)
	resolvedRecord := map[string]any{
		"status":               "resolved",
		"resolved_at":          time.Now().Unix(),
		"pre_halt_authority":   preHaltSnapshot,
	}
	if rd, err := json.Marshal(resolvedRecord); err == nil {
		_ = os.WriteFile(interspectPath, rd, 0644)
	}

	// 8. BEGIN IMMEDIATE ratchet_state reset
	conn := db.Conn()
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	_, err = conn.ExecContext(context.Background(),
		"UPDATE ratchet_state SET tier='supervised', demoted_at=? WHERE tier='autonomous'",
		time.Now().Unix())
	if err != nil {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return fmt.Errorf("ratchet reset: %w", err)
	}
	if _, err := conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// 9. WAL checkpoint before sentinel deletion
	conn.ExecContext(context.Background(), "PRAGMA wal_checkpoint(FULL)")

	// 10. Delete sentinel — NEVER deferred, explicit after commit+checkpoint
	// Sentinel was created with 0400 permissions; chmod to allow deletion
	if err := os.Chmod(h.Path(), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: chmod sentinel: %v\n", err)
	}
	if err := os.Remove(h.Path()); err != nil {
		return fmt.Errorf("remove sentinel: %w (factory may still appear halted)", err)
	}

	// 11. Constrain checker stub (Tier 2 wiring point for F6)
	var checker constrainChecker = nilConstrainChecker{}
	constraints, _ := checker.ActiveConstraints()
	if len(constraints) > 0 && !resumeConstrained {
		fmt.Fprintf(os.Stderr, "ockham: active constraints: %s — consider --constrained\n",
			strings.Join(constraints, ", "))
	}

	// 12. Output summary
	resetCount := len(preHaltSnapshot)
	if resumeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"resumed":        true,
			"domains_reset":  resetCount,
			"prior_snapshot": preHaltSnapshot,
		})
	}

	fmt.Printf("Factory resumed.\n")
	fmt.Printf("  Sentinels cleared: factory-paused.json, halt-record.json\n")
	fmt.Printf("  Domains reset: %d autonomous → supervised\n", resetCount)
	fmt.Printf("  Next: ockham check will re-evaluate signal state\n")
	return nil
}

func snapshotRatchetState(db *signals.DB) []map[string]string {
	rows, err := db.Conn().Query("SELECT agent, domain, tier FROM ratchet_state WHERE tier='autonomous'")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var snap []map[string]string
	for rows.Next() {
		var a, d, t string
		if err := rows.Scan(&a, &d, &t); err != nil {
			continue
		}
		snap = append(snap, map[string]string{"agent": a, "domain": d, "tier": t})
	}
	return snap
}
