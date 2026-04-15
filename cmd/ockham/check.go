package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/Ockham/internal/anomaly"
	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/mistakeknot/Ockham/internal/observation"
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

	// Step 1: Reconstruct halt sentinel from interspect if needed (MUST be first)
	if err := runner.reconstructHalt(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: halt reconstruction degraded: %v\n", err)
	}

	// Step 2: Check halt state — if halted, only snapshot authority then return
	halted := halt.New(runner.haltPath).IsHalted()

	// Step 3: Snapshot authority (always — read-only capture of external state)
	if err := runner.snapshotAuthority(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: authority snapshot degraded: %v\n", err)
	}

	if halted {
		if !checkDryRun {
			fmt.Fprintln(os.Stderr, "ockham check: factory halted — skipping signal evaluation and reconfirmation")
		}
		return nil
	}

	// Step 4: Evaluate INFORM signals and pleasure signals (only when not halted)
	if err := runner.evaluateSignals(); err != nil {
		if errors.Is(err, anomaly.ErrBypassFailed) {
			return err // safety-critical: exit non-zero
		}
		fmt.Fprintf(os.Stderr, "ockham: signal evaluation degraded: %v\n", err)
	}

	// Step 5: Reconfirmation timers (only when not halted)
	if err := runner.checkReconfirmation(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: reconfirmation check degraded: %v\n", err)
	}

	if checkDryRun {
		fmt.Println("ockham check: dry-run complete (no changes written)")
	}

	return nil
}

// evaluateSignals runs INFORM signal evaluation and pleasure signal computation.
func (r *CheckRunner) evaluateSignals() error {
	if r.dryRun {
		fmt.Println("ockham check: would evaluate signals (dry-run)")
		return nil
	}

	// Ingest new bead metrics from bd
	if err := r.ingestBeadMetrics(); err != nil {
		fmt.Fprintf(os.Stderr, "ockham: bead ingest degraded: %v\n", err)
		// Continue — evaluation will short-circuit on themes with no new data
	}

	// Discover themes: union of bead_metrics themes and existing signal_state keys
	themes, err := r.discoverThemes()
	if err != nil {
		return fmt.Errorf("discover themes: %w", err)
	}
	if len(themes) == 0 {
		return nil // no themes to evaluate
	}

	cfg := anomaly.DefaultConfig()
	obs := observation.NewCassObserver()
	eval := anomaly.NewEvaluator(r.db, cfg, anomaly.WithObserver(obs))
	now := time.Now().Unix()

	state, err := eval.Evaluate(themes, now)
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}

	// Report signal states
	for theme, sig := range state.Signals {
		if sig.Status == anomaly.StatusFired {
			fmt.Printf("  INFORM fired: theme=%s drift=%.1f%% advisory=%+d\n",
				theme, sig.DriftPct*100, sig.AdvisoryOffset)
		}
	}

	_ = state // pleasure signals are persisted by the evaluator
	return nil
}

// ingestBeadMetrics shells out to bd to get recently closed beads and inserts metrics.
func (r *CheckRunner) ingestBeadMetrics() error {
	beads, err := closedBeadsFromBD()
	if err != nil {
		return err
	}
	for _, b := range beads {
		if err := r.db.InsertBeadMetric(b); err != nil {
			return fmt.Errorf("insert %s: %w", b.BeadID, err)
		}
	}
	return nil
}

// discoverThemes returns the union of: bead_metrics themes + signal_state inform:* themes.
func (r *CheckRunner) discoverThemes() ([]string, error) {
	themeSet := make(map[string]bool)

	// From bead_metrics
	dbThemes, err := r.db.DistinctThemes()
	if err != nil {
		return nil, err
	}
	for _, t := range dbThemes {
		themeSet[t] = true
	}

	// From signal_state (keys matching "inform:*")
	rows, err := r.db.Conn().Query("SELECT key FROM signal_state WHERE key LIKE 'inform:%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		theme := strings.TrimPrefix(key, "inform:")
		themeSet[theme] = true
	}

	themes := make([]string, 0, len(themeSet))
	for t := range themeSet {
		themes = append(themes, t)
	}
	return themes, nil
}

// closedBeadsFromBD shells out to bd to get recently closed beads with metrics.
// Limits to 100 most recent to avoid growing ingestion latency (P1 fix).
//
// INVARIANT: Each bead maps to exactly one lane (first lane: label found).
// This ensures bead populations are disjoint per-theme, which is required
// for BYPASS root-cause deduplication (distinct_root_causes >= 2 means
// distinct theme names, which is a valid proxy for causal independence
// ONLY when bead populations are disjoint). If multi-lane beads are
// introduced, BYPASS deduplication must be re-evaluated.
func closedBeadsFromBD() ([]signals.BeadMetric, error) {
	cmd := newBDCommand("list", "--status=closed", "--json", "--limit=100")
	out, err := cmd.Output()
	if err != nil {
		// bd may not support --limit; fall back without it
		cmd = newBDCommand("list", "--status=closed", "--json")
		out, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("bd list --status=closed: %w", err)
		}
	}

	var beads []struct {
		ID        string   `json:"id"`
		Labels    []string `json:"labels"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	var metrics []signals.BeadMetric
	for _, b := range beads {
		lane := ""
		for _, label := range b.Labels {
			if strings.HasPrefix(label, "lane:") {
				lane = strings.TrimPrefix(label, "lane:")
				break
			}
		}
		if lane == "" {
			lane = "open"
		}

		created, errC := time.Parse(time.RFC3339, b.CreatedAt)
		updated, errU := time.Parse(time.RFC3339, b.UpdatedAt)
		if errC != nil || errU != nil || created.IsZero() || updated.IsZero() {
			continue // skip beads with unparseable timestamps rather than poisoning baseline
		}

		cycleMs := updated.Sub(created).Milliseconds()

		metrics = append(metrics, signals.BeadMetric{
			BeadID:      b.ID,
			Theme:       lane,
			CycleTimeMs: cycleMs,
			CompletedAt: updated.Unix(),
		})
	}
	return metrics, nil
}

// newBDCommand creates an exec.Command for bd. Extracted for testability.
func newBDCommand(args ...string) *exec.Cmd {
	return exec.Command("bd", args...)
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
		if record.Status == "resolved" {
			if _, err := os.Stat(r.haltPath); err == nil {
				fmt.Fprintf(os.Stderr, "ockham: halt-record.json resolved but %s still present — resume may have been interrupted; run 'ockham resume --confirm'\n", r.haltPath)
			}
		}
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
	f, err := os.OpenFile(r.haltPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if os.IsExist(err) {
		return nil // sentinel already exists (concurrent reconstruction or operator action)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(sentinel); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
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
