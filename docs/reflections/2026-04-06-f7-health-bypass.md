---
artifact_type: reflection
bead: sylveste-fzt
date: 2026-04-06
---
# F7 Reflection: Health JSON + Tier 3 BYPASS

## What Shipped
- `ockham health` — machine-readable factory health JSON (signals, pleasure, halt state, themes)
- Tier 3 BYPASS trigger — atomic durable sentinel write (temp+fsync+rename) when >=2 themes fire
- `ockham resume` — controlled restart with --confirm, dual-sentinel consistency, atomic domain reset
- INV-8 enforcement — PersistentPreRunE allowlist, runCheck() reorder, halt-guarded signal evaluation
- 12 files changed, ~770 lines added, 62 tests passing

## Review Findings That Shaped Implementation
- **fsync gap (P0, 3/4 convergence):** Brainstorm review's highest-confidence finding. A Bronze Age Mycenaean scribe, a Chinese hydraulic engineer, and a Go crash-recovery specialist all independently found that write-before-notify was code-order only, not syscall-durable. Added f.Sync() to all sentinel write paths.
- **runCheck() step ordering (P0, 3/4 convergence):** reconstructHalt() must run before evaluateSignals() to ensure crash-recovered halt state is consistent before evaluation. Nuclear SCRAM sequencing and Polynesian storm protocols both demanded this independently.
- **INV-8 allowlist vs blocklist (3/4 convergence):** Igbo ofo-binding and Benedictine interdict governance both insisted that enumerated blocklists degrade over time — future commands forget the guard. Switched to PersistentPreRunE allowlist where all commands default to blocked when halted.
- **Plan review P0: haltAllowed missing "check":** Would have blocked ockham check itself when halted, defeating the entire runCheck redesign. Caught before implementation.
- **Plan review P0: ApplyFactoryGuard wrong signature:** Would not have compiled. Plan specified `Config` arg but function takes `int`.

## Key Design Decisions
- **Evaluator gains sentinelPath field:** Enables test isolation without touching ~/.config/ockham/. Variadic parameter maintains backward compatibility with existing callers.
- **Interspect write is soft failure:** BYPASS sentinel write succeeds → interspect record fails → halt is effective but audit trail is incomplete. Never roll back the sentinel for an interspect failure.
- **Resume uses database/sql BeginTx, not raw SQL:** Quality gates caught that raw BEGIN IMMEDIATE bypasses connection pool guarantees. modernc.org/sqlite issues IMMEDIATE via BeginTx anyway.
- **Health always JSON, no --json flag:** Machine-readable by design. Avoids the dead-code problem of a non-JSON output path that doesn't exist.

## Lessons Learned
- 4-track brainstorm review (16 agents) found 5 findings at 3/4 convergence — genuinely high-confidence signals that shaped the implementation. The cost (~$7) was justified for a safety-critical feature.
- Plan review caught 2 P0s that would have blocked compilation or broken the core check-when-halted flow. Type-level review (will it compile? does the allowlist include the right commands?) is the highest-ROI review for implementation plans.
- The anomaly→halt dependency edge (new in F7) is architecturally clean but should be documented in AGENTS.md.
