package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/signals"
	"github.com/spf13/cobra"
)

var checkDryRun bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Evaluate signals, persist authority snapshots, check re-confirmation timers",
	RunE:  runCheck,
}

func init() {
	checkCmd.Flags().BoolVar(&checkDryRun, "dry-run", false, "Report what would change without writing")
	rootCmd.AddCommand(checkCmd)
}

// CheckRunner owns the signals.DB lifecycle and orchestrates check steps.
type CheckRunner struct {
	db       *signals.DB
	haltPath string
	dryRun   bool
}

func runCheck(cmd *cobra.Command, args []string) error {
	db, err := signals.NewDB(signals.DefaultDBPath())
	if err != nil {
		return fmt.Errorf("signals.db: %w", err)
	}
	defer db.Close()

	if db.WasRecovered() {
		fmt.Fprintln(os.Stderr, "ockham: cold start — signals.db was recreated")
	}

	runner := &CheckRunner{
		db:       db,
		haltPath: halt.DefaultSentinelPath(),
		dryRun:   checkDryRun,
	}

	// Step 1: Snapshot authority from interspect confidence.json
	if err := runner.snapshotAuthority(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: authority snapshot degraded: %v\n", err)
	}

	// Step 2: Reconstruct halt sentinel if needed
	if err := runner.reconstructHalt(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: halt reconstruction degraded: %v\n", err)
	}

	// Step 3: Check re-confirmation timers
	if err := runner.checkReconfirmation(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: reconfirmation check degraded: %v\n", err)
	}

	if checkDryRun {
		fmt.Println("ockham check: dry-run complete (no changes written)")
	}

	return nil
}

// snapshotAuthority reads interspect confidence.json and persists snapshots.
func (r *CheckRunner) snapshotAuthority() error {
	confPath := filepath.Join(os.Getenv("HOME"), ".clavain", "interspect", "confidence.json")
	data, err := os.ReadFile(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // interspect not yet running — skip silently
		}
		return fmt.Errorf("read confidence.json: %w", err)
	}

	// confidence.json format: {"agent": {"domain": {"hit_rate": 0.85, "sessions": 42, "confidence": 0.9}}}
	var conf map[string]map[string]struct {
		HitRate    float64 `json:"hit_rate"`
		Sessions   int     `json:"sessions"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal(data, &conf); err != nil {
		return fmt.Errorf("parse confidence.json: %w", err)
	}

	now := time.Now().Unix()
	for agent, domains := range conf {
		for domain, metrics := range domains {
			snap := signals.AuthoritySnapshot{
				Agent:      agent,
				Domain:     domain,
				HitRate:    metrics.HitRate,
				Sessions:   metrics.Sessions,
				Confidence: metrics.Confidence,
				CapturedAt: now,
			}
			if r.dryRun {
				fmt.Printf("  would snapshot: %s/%s hit_rate=%.2f sessions=%d\n", agent, domain, metrics.HitRate, metrics.Sessions)
				continue
			}
			if err := r.db.SaveAuthoritySnapshot(snap); err != nil {
				return fmt.Errorf("save snapshot %s/%s: %w", agent, domain, err)
			}
		}
	}
	return nil
}

// haltRecord is the minimal struct for reading interspect halt records.
// NOTE: interspect halt record format is not yet a stable contract.
type haltRecord struct {
	EventID   string `json:"event_id"`
	Timestamp int64  `json:"timestamp"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
}

// reconstructHalt recreates factory-paused.json from interspect halt record if missing.
// Source is interspect file, NOT signals.db — interspect is the agent-unwritable sentinel.
func (r *CheckRunner) reconstructHalt() error {
	// If sentinel exists, nothing to reconstruct
	if _, err := os.Stat(r.haltPath); err == nil {
		return nil
	}

	// Read interspect halt record directly from file
	haltRecordPath := filepath.Join(os.Getenv("HOME"), ".clavain", "interspect", "halt-record.json")
	data, err := os.ReadFile(haltRecordPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no halt record — nothing to reconstruct
		}
		return fmt.Errorf("read halt-record.json: %w", err)
	}

	var record haltRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("parse halt-record.json: %w", err)
	}

	// Only reconstruct if the record is active
	if record.Status != "active" {
		return nil
	}

	if r.dryRun {
		fmt.Printf("  would reconstruct %s from interspect halt record (reason: %s)\n", r.haltPath, record.Reason)
		return nil
	}

	// Write sentinel — write-before-notify ordering
	sentinel, _ := json.Marshal(map[string]any{
		"reason":       record.Reason,
		"reconstructed": true,
		"source":       "interspect-halt-record",
		"timestamp":    record.Timestamp,
	})

	if err := os.MkdirAll(filepath.Dir(r.haltPath), 0755); err != nil {
		return err
	}
	// Atomic create — O_EXCL prevents TOCTOU race where concurrent check
	// or operator resume could conflict with this write.
	f, err := os.OpenFile(r.haltPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(err) {
		return nil // sentinel already exists (concurrent reconstruction or operator action)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(sentinel)
	return err
}

// checkReconfirmation flags autonomous domains past their 30-day window.
func (r *CheckRunner) checkReconfirmation() error {
	rows, err := r.db.Conn().Query(
		"SELECT agent, domain, promoted_at FROM ratchet_state WHERE tier = 'autonomous'",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now()
	thirtyDays := 30 * 24 * time.Hour

	for rows.Next() {
		var agent, domain string
		var promotedAt int64
		if err := rows.Scan(&agent, &domain, &promotedAt); err != nil {
			return err
		}

		promoted := time.Unix(promotedAt, 0)

		// Sanity guard: future timestamps or > 1 year old → re-confirm immediately
		if promoted.After(now) || now.Sub(promoted) > 365*24*time.Hour {
			fmt.Fprintf(os.Stderr, "ockham: suspicious promoted_at for %s/%s (%v) — forcing re-confirmation\n",
				agent, domain, promoted)
			if err := r.flagReconfirm(agent, domain, now); err != nil {
				return err
			}
			continue
		}

		// Normal 30-day check
		if time.Since(promoted) > thirtyDays {
			if err := r.flagReconfirm(agent, domain, now); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

// flagReconfirm sets a reconfirm signal only if not already pending.
// Preserves the original updated_at so downstream age-based escalation works.
func (r *CheckRunner) flagReconfirm(agent, domain string, now time.Time) error {
	key := fmt.Sprintf("reconfirm:%s:%s", agent, domain)
	if r.dryRun {
		fmt.Printf("  would flag reconfirm: %s/%s\n", agent, domain)
		return nil
	}
	// Conditional write — skip if already pending to preserve original timestamp
	existing, found, err := r.db.GetSignalState(key)
	if err != nil {
		return err
	}
	if found && existing == "pending" {
		return nil // already flagged
	}
	return r.db.SetSignalState(key, "pending", now.Unix())
}
