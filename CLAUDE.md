# Ockham

Factory governor for autonomous agent dispatch. Headless L2 component that mediates between the principal's strategic intent and the factory's execution.

Named after Ockham Saneer from Ada Palmer's *Terra Ignota* — who ran the world's Humanist transportation network, routing resources through competing hive interests.

## Quick Reference

- **Build:** `go build ./cmd/ockham`
- **Test:** `go test ./... -count=1`
- **Vet:** `go vet ./...`

## What Ockham Does

- **Intent directives** — principal sets theme-level budgets ("40% auth, 30% performance"), Ockham translates to dispatch scoring weights
- **Authority management** — agent trust tiers, domain grants, delegation ceiling enforcement
- **Anomaly detection** — algedonic signals (pain: quarantined beads, circuit breakers; pleasure: clean completions, improving cycle time)
- **Dispatch routing** — translates intent + authority + factory state into bead scoring weights for Clavain's self-dispatch
- **Autonomy ratchet** — shadow mode (propose, human approves) → supervised (act, human reviews) → autonomous (act, human audits)

## Structure

```
cmd/ockham/            CLI entry point
internal/
  intent/              Intent directives (theme budgets, priority overrides)
  authority/           Authority tiers, domain grants, delegation ceiling
  anomaly/             Algedonic signals, anomaly detection, circuit breakers
  dispatch/            Dispatch weight synthesis (intent + authority + state → scores)
docs/
  vision.md            Vision and philosophy
```

## Architecture Position

```
Principal (human)
  │
  ▼
Meadowsyn (L3 UI — ops room)
  │
  ▼
Ockham (L2 — factory governor)     ← this
  │
  ├── Intent → Clavain dispatch scoring
  ├── Authority → agent trust tiers
  ├── Anomaly → algedonic signals
  └── Dispatch → weight synthesis
  │
  ▼
Clavain / Zaka / Alwe (L2 — execution + observation)
  │
  ▼
Intercore (L1 — kernel)
```

## Git

Ockham has its own git repo at `os/Ockham/`. Commit from here, not the monorepo root.

## Beads

Uses the Demarch monorepo beads tracker at `/home/mk/projects/Demarch/.beads/` (prefix `Demarch-`). Epic: Demarch-6fdq.
