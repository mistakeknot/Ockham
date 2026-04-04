# Ockham — Agent Reference

## Architecture

Ockham is the factory governor — the Cyberstride in Demarch's Cybersyn-inspired architecture. It sits between the principal's strategic intent (expressed through Meadowsyn or CLI) and the factory's execution (Clavain self-dispatch, Zaka agent steering, Alwe observation).

```
Principal
  │ intent directives
  ▼
Ockham (governor)
  │ dispatch weights + authority grants
  ├──→ Clavain (self-dispatch scoring)
  ├──→ Zaka (agent spawning)
  ├──→ Alwe (observation queries)
  └──→ Intercore (events, gates)
```

## Package Map

| Package | Purpose | Key Types | Wave |
|---------|---------|-----------|------|
| `halt` | Factory halt sentinel (INV-8) | `Sentinel` | 1 |
| `intent` | Theme budgets, YAML read/write/validate | `IntentFile`, `ThemeBudget`, `IntentVector`, `Priority`, `Store` | 1 |
| `authority` | Trust tiers, domain grants | `State` (stub) | 3 |
| `anomaly` | Algedonic signals, circuit breakers | `State` (stub) | 2 |
| `scoring` | Weight synthesis, offset clamping | `BeadInfo`, `WeightVector`, `Score()` | 1 |
| `governor` | Subsystem assembly, halt-first evaluation | `Governor`, `Evaluate()` | 1 |

Dependency direction: `halt` imports nothing. `intent` imports nothing (uses yaml.v3). `authority`/`anomaly` import nothing. `scoring` imports `intent`, `authority`, `anomaly`. `governor` imports all five.

## Core Concepts

### Intent Directives
The principal expresses strategic intent at the theme level, not the bead level:
- "Spend 40% on auth, 30% on performance, 30% on whatever's ready"
- "Freeze all non-critical work until the release"
- "Prioritize anything blocking the API launch"

Ockham translates these to scoring weights that Clavain's dispatch function consumes.

### Authority Tiers
From the AI factory brainstorm (Wave 3):
- `authority = min(fleet_tier, domain_grant)`
- CODEOWNERS-style domain globs
- 5 safety invariants: no self-promotion, delegation ceiling, action-time validation, audit completeness, human halt supremacy

### Algedonic Signals
From Stafford Beer's VSM — pain/pleasure signals that bypass hierarchy:
- **Pain:** quarantined beads, circuit breaker trips, gate failures, stale claims
- **Pleasure:** clean first-attempt completions, improving cycle time, shrinking backlog
- These surface first in Meadowsyn, not buried in logs

### Autonomy Ratchet
Three modes, progressing as the factory earns trust:
1. **Shadow** — Ockham proposes dispatch, principal approves/overrides
2. **Supervised** — Ockham dispatches, principal reviews outcomes
3. **Autonomous** — Ockham dispatches, principal audits periodically

## Build & Test

```bash
go build ./cmd/ockham
go test ./... -count=1
go vet ./...
```

## CLI

**Implemented (Wave 1):**
```bash
ockham intent --theme auth --budget 0.4 --priority high   # set theme
ockham intent --freeze auth                                 # freeze a theme
ockham intent show                                          # display table
ockham intent validate                                      # check consistency
ockham dispatch advise --json                               # weight vector output
```

**Planned (Wave 2-3):**
```bash
ockham authority show --json     # ratchet state per domain
ockham anomaly --since 1h        # signal history
ockham health --json              # factory health dashboard
ockham check                      # signal evaluation
ockham resume                     # clear halt sentinel
```

## Dependencies

- Beads (`bd` CLI) — reads backlog state
- Alwe — queries session history for anomaly detection
- Intercore — reads events, gate verdicts
- Clavain — consumes dispatch weights

## Related Research

- `docs/brainstorms/2026-03-19-ai-factory-orchestration-brainstorm.md`
- `docs/plans/2026-03-20-ai-factory-wave1-foundation.md`
- `docs/research/flux-research/authority-tiers/synthesis.md`
- `docs/research/flux-research/phase1-self-dispatch/synthesis.md`
