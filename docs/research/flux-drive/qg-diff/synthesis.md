# Flux-Drive Quality Gates Review: Ockham F5

**Date:** 2026-04-05
**Scope:** Tier 1 INFORM signals and pleasure signals
**Diff:** 18 files (8 modified, 10 new), ~1980 lines
**Tests:** 53 passing, build clean, vet clean

## Verdict: PASS with 1 P1 and 3 P2 findings

No P0 blockers. One P1 worth fixing before next feature wave. Three P2 items for follow-up.

---

## P1 Findings

### P1-1: `closedBeadsFromBD()` ingests ALL closed beads on every check cycle

**File:** `cmd/ockham/check.go:175-221`
**Issue:** `bd list --status=closed --json` returns the entire closed bead history every time `ockham check` runs. Each bead is then fed to `InsertBeadMetric` which does `INSERT ... ON CONFLICT DO NOTHING`. This works correctly (no duplicates) but the bd subprocess cost scales linearly with total closed beads across the project lifetime. At 785+ sessions (current baseline), this is already hundreds of beads per invocation.

**Impact:** Performance degradation over time. Since `ockham check` runs from SessionStart hook, this adds latency to every agent session start.

**Fix:** Add `--since` or `--after` filtering to the bd command, or track the last-ingested timestamp in signal_state and pass it to bd. The `LastBeadCompletedAt` already exists and could serve as the cutoff. Alternatively, a simple `bd list --status=closed --json --limit=50` would cap the cost.

**Severity rationale:** Not a correctness bug (ON CONFLICT DO NOTHING is correct), but a performance issue that worsens with each closed bead. P1 because it affects session start latency for every agent.

---

## P2 Findings

### P2-1: `time.Parse` errors silently discarded in bead metric ingestion

**File:** `cmd/ockham/check.go:206-207`
```go
created, _ := time.Parse(time.RFC3339, b.CreatedAt)
updated, _ := time.Parse(time.RFC3339, b.UpdatedAt)
```

**Issue:** If bd returns a timestamp in a format other than RFC3339 (or empty string), both parse errors are silently discarded. The metric will be inserted with `CycleTimeMs: 0` and `CompletedAt` as the unix epoch (for the zero-value `updated`). A zero `CompletedAt` would cause the bead to sort to the bottom of the window and potentially be pruned immediately, or worse, poison the baseline half of the drift calculation with unrealistic data.

**Fix:** Skip the metric if either timestamp fails to parse, or log a warning. The guard `!created.IsZero() && !updated.IsZero()` catches the cycleMs calculation but `CompletedAt` is still set to `updated.Unix()` which is `-62135596800` for zero time.

### P2-2: Advisory offset only ratchets to `-MaxAdvisoryPerCycle`, never deepens

**File:** `internal/anomaly/drift.go:64-66`
```go
if result.AdvisoryOffset > -cfg.MaxAdvisoryPerCycle {
    result.AdvisoryOffset = -cfg.MaxAdvisoryPerCycle
}
```

**Issue:** This is rate-limiting correctly on the first fire, but on subsequent fires (when drift persists), the advisory never goes below `-1`. If drift stays at 40% across 5 consecutive evaluations, the advisory stays at `-1` forever. The comment says "rate limit: at most -MaxAdvisoryPerCycle per cycle" but the implementation is a ceiling, not a per-cycle decrement.

**Impact:** Design question rather than bug. The current behavior means INFORM signals can only ever apply `-1` advisory offset total, regardless of drift severity or persistence. This may be intentionally conservative (F5 ships with minimal intervention power) but it means the FactoryGuard ceiling of 12 can never be reached with `MaxAdvisoryPerCycle=1`. Document the intent if this is deliberate; otherwise change to `result.AdvisoryOffset -= cfg.MaxAdvisoryPerCycle` for progressive deepening.

### P2-3: `discoverThemes` uses raw SQL query on `signal_state` table

**File:** `cmd/ockham/check.go:153`
```go
rows, err := r.db.Conn().Query("SELECT key FROM signal_state WHERE key LIKE 'inform:%'")
```

**Issue:** This bypasses the `signals.DB` API and queries `Conn()` directly. While there's no SQL injection risk (the LIKE pattern is a string literal, not interpolated), this creates a coupling between the check command and the signal_state table schema. If signal_state changes shape, this query won't be caught by the compiler.

**Fix:** Add a `signals.DB.InformThemes()` method to encapsulate this query alongside the other metrics CRUD methods.

---

## SQL Injection Assessment: CLEAN

All SQL queries in `metrics.go`, `db.go`, `check.go`, and `signals.go` use parameterized queries (`?` placeholders). No string interpolation in any SQL statement. The `LIKE 'inform:%'` pattern in `discoverThemes` is a literal, not user-supplied. The `PruneBeadMetrics` subquery uses parameters correctly.

## Error Handling Assessment: GOOD

- External subprocess (`bd`) errors are caught and surfaced as degraded (non-fatal)
- `newBDCommand` is extracted for testability (though no test currently mocks it)
- Evaluator errors are wrapped with theme context
- Persist failures in `persistSignal`/`persistPleasure` are silently dropped -- acceptable for non-critical state, but worth noting

## Nil Safety Assessment: CLEAN

- `anomaly.State.Signals` nil check at `scorer.go:40` is correct
- `anomaly.State{}` zero value documented as safe (line 240 of anomaly.go)
- `Governor.anomalyEv` nil check at governor.go:65 is correct
- `PleasureSignal.CostUSD` nil pointer handled via `sql.NullFloat64` scan

## Thread Safety Assessment: ACCEPTABLE

- SQLite WAL mode + busy_timeout(5000ms) handles concurrent readers
- `ockham check` (SessionStart hook) and `ockham dispatch advise` (sprint runner) both open separate DB connections -- WAL mode allows concurrent reads and serializes writes via SQLite's internal locking
- No in-memory shared state between processes (all coordination via SQLite)
- The `var now = func()` in governor.go is a package-level var but only mutated in tests -- acceptable

## Schema Migration Assessment: CLEAN

- Migration from v1 to v2 uses `CREATE TABLE IF NOT EXISTS` -- idempotent
- `CREATE INDEX IF NOT EXISTS` -- idempotent
- Version bump happens after table creation -- correct ordering
- Fresh installs get both tables via the `schema` const
- Recovery path also uses the `schema` const -- consistent

## Test Coverage Assessment: GOOD

- 53 tests covering: drift detection (fire, clear, hysteresis, adaptive window), factory guard, median calculation, pleasure signals (pass rate, cycle time, cost trends), evaluator (no beads, short-circuit, staleness, drift fires, multi-theme, pleasure), metrics CRUD (insert, duplicate, order, prune, last completed, distinct themes, null cost), scoring (advisory applied, clamped, nil anomaly state)
- **Gap:** No test for `closedBeadsFromBD()` or `ingestBeadMetrics()` (subprocess-dependent, understandable but noted)
- **Gap:** No test for the `signals.go` CLI command (display-only, low risk)
- **Gap:** `EvaluateDrift` with exactly `MinWindow` beads (boundary: mid = MinWindow/2, meaning each half has exactly MinWindow/2 beads -- this is tested indirectly but not explicitly at the boundary)

---

## Summary

The implementation is solid. The signal evaluation pipeline (ingest -> discover themes -> evaluate drift -> compute pleasure -> persist -> factory guard) is well-structured with proper short-circuits, staleness detection, and degraded-mode fallbacks. The scoring integration correctly separates raw (intent-only) from final (intent+advisory) offsets.

The one P1 (unbounded bd ingestion) is a performance issue that won't cause incorrect behavior but will slow down session starts as the project accumulates closed beads. Worth fixing before F6 or the next wave.
