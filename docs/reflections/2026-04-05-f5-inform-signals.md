---
artifact_type: reflection
bead: sylveste-usj
stage: reflect
---

# Reflection: Ockham F5 — Tier 1 INFORM Signals + Pleasure Signals

## What shipped

Filled the anomaly.State stub with a complete Tier 1 INFORM signal system: weight-drift detection (split-window p50 comparison, 20%/10% hysteresis, rate-limited advisory offsets with factory guard), three pleasure signals (pass rate, cycle time trend, cost trend), signals.db schema v2 migration with bead_metrics table, evaluator orchestrator with short-circuit and staleness handling, scoring integration with nil-safe dual logging, and `ockham signals` CLI. 53 tests, 18 files changed/added.

## What the reviews caught

The brainstorm flux-review (10 agents, 2 tracks) caught the most impactful issue: **first_attempt_pass_rate sourced from interspect evidence would violate Signal Independence** (agents could game their own authority promotions). This was P0 — would have propagated through PRD, plan, and implementation before discovery. Fixed by specifying agent-unwritable sources only.

The plan review caught **nil-map panic in scorer** — Score() would read anomaly.State.Signals[lane] but existing tests pass zero-value State with nil map. Also caught missing PRIMARY KEY on bead_metrics.bead_id (every check cycle would re-insert all closed beads).

Quality gates caught **unbounded bd ingestion** — closedBeadsFromBD() fetched ALL closed beads, growing with lifetime count. Fixed with --limit cap and fallback.

## Key design decisions

- **Split-window baseline** instead of proper Mann-Whitney: acceptable for advisory signals where false positives are tolerable. The 10-bead minimum + 20% threshold gives ~50% power — fine for "something might be wrong," not for automated action.
- **Rate-limited advisory (-1 per cycle)** prevents integral windup but means persistent severe drift (40%) still only applies -1. This is intentionally conservative — the FactoryGuard ceiling (12) exists for future graduated response.
- **Variadic governor.New()** for backward-compatible API evolution — existing callers compile unchanged.

## What I would do differently

- Write the bead_metrics PRIMARY KEY constraint from the start (plan review caught it, but it should have been obvious from the "skip duplicates" requirement in the plan).
- The `closedBeadsFromBD()` should have been scoped from the start. "Fetch all closed beads" is a classic unbounded-growth pattern that should trigger immediate skepticism.
- The flux-review brainstorm step was disproportionately expensive (10 agents on a 40-line capture doc). Next time for clear-requirements features, skip flux-review on the brainstorm entirely and invest review budget on the plan.
