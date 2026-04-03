# Ockham — Vision Document

**Status:** Draft
**Date:** 2026-04-03
**Bead:** sylveste-8em
**Sources:** [Brainstorm](../../../docs/brainstorms/2026-04-03-ockham-vision-brainstorm.md) (rev 3, 16-agent reviewed), [Flux-review synthesis](../../../docs/research/flux-drive/ockham-vision/synthesis.md), [Authority tiers research](../../../docs/research/flux-research/authority-tiers/synthesis.md), [Algedonic signal research](../../../docs/research/flux-research/ockham-algedonic-signal-design/synthesis.md)

---

## What Ockham Is

Ockham is the factory governor — the Cyberstride in Sylveste's Cybersyn-inspired architecture. Named after Ockham Saneer (Ada Palmer, *Terra Ignota*), who ran the Humanist transportation network by routing resources through competing hive interests without commanding any vehicle directly.

Ockham sits between the principal's strategic intent and the factory's execution. It computes; Clavain, Zaka, and Alwe act.

```
Principal (human)
  |
  v
Meadowsyn (L3 — ops room UI)
  |
  v
Ockham (L2 — factory governor)       <-- this
  |
  |-- Intent weights ----> Clavain (L2 — sprint dispatch)
  |-- Authority grants ---> Zaka (L2 — agent steering)
  |-- Observation queries -> Alwe (L2 — observation)
  |-- Events/gates -------> Intercore (L1 — kernel)
  |
  v
Interspect (L1 — evidence pipeline, read-only by Ockham)
```

### Four Subsystems

| Subsystem | Input | Output | Wave | Allowed Dependencies |
|-----------|-------|--------|------|---------------------|
| **Intent** | Principal theme budgets, constraints | Per-bead weight offsets | 1 | Beads (read) |
| **Anomaly** | Beads state, interspect, interstat, Alwe | Tiered algedonic signals | 2 | All (read), Alwe (read) |
| **Authority** | Interspect evidence, principal overrides | Domain grants, tier promotions/demotions | 3 | Interspect (read), Intent (read) |
| **Scoring** | Intent offsets + authority + anomaly state | Unified weight vector for dispatch | 1 | Receives typed structs from the other three; imports nothing |

Dependency direction: Intent depends on nothing. Anomaly reads external state. Authority reads interspect. Scoring receives data structs from the other three subsystems but does not import their packages — preventing it from becoming a god-module. (Implementation note: the Scoring subsystem lives at `internal/dispatch/` in the package layout.)

```
Intent --------.
                |
Anomaly --------+---> Scoring ---> lib-dispatch.sh
                |
Authority ------'
     ^
     |
Interspect (external, read-only)
```

### Degradation Contracts

Each subsystem specifies behavior when its upstream dependency is unavailable:

| Subsystem | Dependency | Degradation |
|-----------|-----------|-------------|
| Intent | Beads | Use last-known-good intent.yaml; log degraded-mode event |
| Anomaly | Alwe | Skip Alwe-sourced observation inputs; proceed on remaining channels (interspect, interstat, git) |
| Authority | Interspect | Use last-known snapshot (max 5-minute staleness window); beyond that, fail-closed (deny all authority actions) |
| Scoring | Any input struct | Emit raw score unchanged; log which input was missing |

### Phased Constraint

Ockham is a **policy engine, not an orchestrator** through Wave 3. It never dispatches agents, never claims beads, never touches tmux sessions. It shapes what others do by producing three outputs: dispatch weight offsets, authority grants/revocations, and algedonic signals.

This is a phased constraint, not a permanent identity. At Wave 4, re-evaluate whether Ockham should gain direct dispatch authority for mid-sprint corrections. The constraint forces clean interfaces now; relaxing it later is additive, not disruptive.

### What Ockham Is Not

- **Not a scheduler.** Ockham produces weights; Clavain decides dispatch timing and agent selection.
- **Not an audit log.** Interspect owns the evidence trail. Ockham writes to interspect, never maintains its own audit store.
- **Not a UI.** Meadowsyn renders Ockham's health output. Ockham is headless.
- **Not a Clavain replacement.** Clavain owns sprint execution, quality gates, and the agent dispatch loop. Ockham shapes the inputs to that loop.
- **Not a quality arbiter.** Quality gates are Clavain's domain. Ockham never evaluates code quality or review correctness.
- **Not a Skaffen governor.** Skaffen-dispatched work (direct tmux agent sessions) is outside Ockham's scope until Wave 4 re-evaluation.

---

## Intent

The principal expresses strategic intent at the theme level, not the bead level:

- *"Spend 40% on auth, 30% on performance, 30% on whatever's ready"*
- *"Freeze all non-critical work until the release"*
- *"Prioritize anything blocking the API launch"*

Ockham translates these directives to scoring weight offsets that Clavain's dispatch function consumes.

### Theme = Lane

Beads already carry lane assignments (`bd set-state lane=<name>`). Lanes ARE themes. No new data model is needed:

- **Lane** — the intercore kernel entity (the data model)
- **Theme** — Ockham's governance concept (the policy label)
- **Mapping:** `theme = bead.lane`. Beads with no lane fall into the `open` theme (default).

### Intent Schema

```yaml
# ~/.config/ockham/intent.yaml
version: 1
themes:
  auth:
    budget: 0.40          # fraction of dispatch capacity
    priority: high         # high | normal | low
  performance:
    budget: 0.30
    priority: normal
  open:
    budget: 0.30
    priority: normal

constraints:
  freeze: []               # themes blocked from dispatch
  focus: []                # all other themes deprioritized

# Expiry
valid_until: "2026-04-10T00:00:00Z"   # optional timestamp
until_bead_count: 50                    # optional: expires after N beads complete
```

Priority maps to offset magnitude: `high` produces a positive offset, `normal` produces zero, `low` produces a negative offset. The exact magnitudes are calibrated during design-phase implementation.

**Validation:** `ockham intent validate` checks: budgets sum to 1.0, no unknown themes, no budget < 0 or > 1.0, expiry dates are in the future.

**Atomic replacement:** Intent is only written after successful validation. Invalid YAML is rejected; the factory continues with the last-known-good file. If intent.yaml is missing or corrupt at startup, Ockham uses a hardcoded default: all themes budget 1/N, priority normal.

**Expiry:** When `valid_until` passes or `until_bead_count` is reached, the directive reverts to neutral weights. `ockham status` warns when intent is stale or expired.

---

## Scoring & Dispatch Integration

Ockham influences dispatch through additive weight offsets, not by replacing or modifying the dispatch function.

### Additive Offsets

```
final_score = raw_score + ockham_offset
```

The offset is bounded such that it cannot invert adjacent priority tiers. Intent can nudge ties and close races, but a low-priority bead boosted by intent cannot outrank a high-priority bead. This preserves the priority ordering that beads carry while allowing intent to express preference within a tier.

Additive offsets (not multiplicative) because multiplicative weights cause priority inversion: a P3 bead with a 1.4x intent multiplier can outscore a P1 bead with a 0.6x penalty. Additive offsets are transparent and bounded.

### Gate-Before-Arithmetic

CONSTRAIN and BYPASS states are **dispatch eligibility gates**, not extreme negative weights. When a theme is frozen (CONSTRAIN) or the factory is halted (BYPASS), beads in those themes are ineligible for dispatch — removed from the candidate set before scoring begins. This prevents the composition problem where a high intent offset partially cancels a negative anomaly weight.

Evaluation order: eligibility gates (anomaly state) → weight arithmetic (intent offsets) → perturbation (tie-breaking) → floor guard.

### Integration Boundary

Ockham writes offsets to intercore state. lib-dispatch.sh reads them. Ockham does not own or modify dispatch functions — it is a data supplier, not a code dependency.

- **Write path:** Ockham writes `ockham_offset` per bead via intercore state
- **Read path:** lib-dispatch.sh bulk-fetches all offsets once per dispatch cycle (not per-bead), adds them during rescoring
- **Dual logging:** Both pre-offset (`raw_score`) and post-offset (`final_score`) are recorded, enabling counterfactual calibration ("would this bead have been dispatched without Ockham's influence?")

### Starvation Prevention

A weight floor prevents low-budget themes from being permanently starved. Below the floor, starvation detection triggers: if a theme receives zero dispatches over a configurable cadence, Ockham emits a Tier 1 INFORM signal. When a high-budget theme exhausts its queue, idle capacity is released to the `open` pool rather than sitting unused.

---

## Algedonic Signals

From Stafford Beer's Viable System Model: pain/pleasure signals that bypass hierarchy to reach the decision-maker when the managed system cannot self-correct.

### Three Tiers

**Tier 1 — INFORM** (continuous weight adjustment). Signal fires, dispatch offsets adjust automatically. Examples: theme drift, cycle time degradation, cost overrun. Recovery is automatic when the signal clears. Zero human involvement.

**Tier 2 — CONSTRAIN** (freeze + notify). Signal persists past multi-window confirmation: both a short window AND a long window must breach simultaneously. This prevents transient spikes from triggering freezes. On confirmation, the theme's lane is frozen (beads become ineligible for dispatch), affected domains drop to supervised autonomy, an invalidation event is fired to revoke in-flight authority tokens for the frozen domain, and the signal surfaces in Meadowsyn.

CONSTRAIN requires **paired confirmation**: Ockham's anomaly detection and interspect's evidence must independently corroborate the signal before the freeze takes effect. No single-source CONSTRAIN — this prevents the system from halting on its own false positives.

**In-flight beads:** Agents mid-sprint in a frozen theme continue their current work at supervised autonomy (complete, but no new claims). This prevents the freeze-failure-more-pain reinforcing loop where halting mid-work creates the very failures the freeze was meant to prevent.

**De-escalation:** Both windows must drop below threshold simultaneously, then a stability period must pass with no re-fire. The stability period prevents rapid oscillation between constrained and normal states.

**Rate-of-change fast path:** If events exceed a threshold within a fraction of the short window (e.g., 3 events in 30 minutes against a 1-hour window), the system escalates immediately without waiting for confirmation. This closes the blind spot for rapid-onset incidents.

**Tier 3 — BYPASS** (algedonic, reaches principal directly). Triggered when `distinct_root_causes >= 2` fire simultaneously while the factory is operating at reduced oversight. The trigger counts distinct root causes, not raw signal count — preventing cascade false triggers where a single failure produces many correlated signals.

Tier 3 writes `factory-paused.json` FIRST, then notifies. Write-before-notify ordering ensures the halt is durable even if the notification path fails. At least one Tier 3 notification path is independent of Clavain — if Clavain is hung, it never reads the sentinel file, but the independent channel still reaches the principal.

### Six Signal Qualifications

1. **Qualification gate:** Signals must meet minimum evidence thresholds before evaluation
2. **Root-cause deduplication:** Count distinct root causes, not signal volume
3. **Multi-window confirmation:** Short AND long windows must breach simultaneously
4. **Pleasure signals:** Positive signals (clean completions, improving cycle time) enable ratchet promotions and weight validation
5. **Observation separation:** Alwe observes, Ockham acts — the observer and policy-maker are distinct components
6. **Zero cost on healthy path:** When the factory is healthy, the anomaly subsystem imposes no overhead on dispatch

### Weight-Outcome Feedback

Ockham must detect when its own weights are wrong. Without a feedback loop, misconfigured intent degrades the factory indefinitely.

After each bead completion, the weight-outcome feedback loop compares actual cycle time and quality gate pass rate against the baseline for that theme. If a theme's actual-vs-predicted ratio degrades past a configurable threshold over a rolling window, Ockham emits a Tier 1 INFORM signal and logs a `weight_drift` event to interspect. This is the weight-level equivalent of interspect's agent-level canary monitoring.

The feedback loop ships in Wave 1 alongside Tier 1 INFORM. Without it, Ockham is a governor that cannot tell when its own governance is harmful.

### Independent Observation

At least one anomaly input channel must be agent-unwritable: no agent can influence the data that triggers its own demotion or theme freeze. Git revert rate serves as the initial independent channel — it reflects code quality outcomes that agents cannot manipulate through bead selection or evidence reporting.

### Anomaly Degradation

When Alwe is unavailable, the anomaly subsystem skips Alwe-sourced inputs and proceeds on remaining observation channels (interspect evidence, interstat metrics, git history). A degraded-mode event is logged. The subsystem never silently drops all monitoring — it degrades gracefully, not completely.

---

## Authority & Autonomy

The autonomy ratchet governs how much oversight each domain requires. It is **per-domain** (CODEOWNERS-style globs like `interverse/**`, `core/**`), not per-agent — because trust is earned in a context, not globally.

### State Machine

```
              pleasure signals +           pleasure signals +
              evidence guards              evidence guards
shadow ──────────────────────> supervised ──────────────────> autonomous
  ^               |                ^              |               |
  |    CONSTRAIN  |                |   CONSTRAIN  |               |
  |    (already   |                |              |               |
  |    supervised) v                '--------------'               |
  |                                                               |
  '-------- Tier 3 BYPASS (emergency, from any state) -----------'
```

### Transition Table

| From | To | Trigger | Guard |
|------|----|---------|-------|
| shadow | supervised | Pleasure signals persist past confirmation | Evidence-quantity guard: minimum beads at minimum complexity, minimum confidence |
| supervised | autonomous | Pleasure signals persist past confirmation | Stricter evidence-quantity guard (more beads, higher confidence) |
| autonomous | supervised | Tier 2 CONSTRAIN fires | Automatic, immediate |
| supervised | shadow | Tier 2 CONSTRAIN fires while already supervised | Automatic, immediate |
| any | shadow | Tier 3 BYPASS fires | Automatic, immediate |

**Key design choice:** Promotion guards are evidence-quantity based (minimum beads at minimum complexity), not wall-clock windows. A 24-hour window measures absence-of-failure, not competence against the full difficulty distribution. Evidence-quantity guards require demonstrated competence across representative work.

**Asymmetric thresholds:** The promotion threshold is strictly higher than the demotion threshold. This prevents oscillation at the boundary where an agent hovers between two tiers, alternately promoting and demoting on noise.

### Cold Start

When Ockham activates for the first time, it infers initial domain positions from existing interspect evidence. For each known agent-domain pair, it queries `agent_reliability(agent, domain)` and maps evidence levels to tiers — conservatively. Even if evidence qualifies for autonomous, cold start places the domain at supervised. Promotion to autonomous happens in the normal ratchet cycle if evidence holds. This prevents the activation regression where turning on Ockham increases principal load by resetting everything to shadow.

### Cross-Domain Resolution

When a bead spans multiple domains (e.g., files in both `interverse/**` and `core/**`), authority resolves to the most restrictive:

```
effective_tier = min(tier for each matched domain)
```

If any touched domain is frozen (CONSTRAIN), the bead is ineligible for dispatch regardless of other domains' status. Ockham computes this during weight synthesis; the dispatch consumer receives the final weight without needing to understand domain resolution.

### Ratchet Runaway Prevention

Autonomous domains require periodic re-confirmation. On a configurable cadence, autonomous domains are re-evaluated against the promotion guard. If evidence has degraded below threshold, the domain demotes to supervised. This balancing loop prevents the all-autonomous steady state where the ratchet silently loses sensitivity.

### Post-Promotion Audit

After promotion, the system verifies the promoted agent performs as predicted within a confirmation period. If performance degrades, the domain demotes one level. This closes the open feedback loop identified in the flux-review.

### Pleasure Signals (Wave 1)

Pleasure signals ship alongside Wave 1 Tier 1 INFORM, not deferred to Wave 3:

- `first_attempt_pass_rate` — fraction of beads passing quality gates on first attempt
- `cycle_time_p50_trend` — improving (decreasing) over a rolling window
- `cost_per_landed_change_trend` — stable or improving over a rolling window

### Interspect Interface

Ockham reads from interspect for authority decisions. The contract: `agent_reliability(agent, domain)` returns hit rate, session count, confidence band, and last-active timestamp. Ockham consumes this to make promotion/demotion decisions.

**During interspect degradation:** In-progress promotions are paused (not committed) until interspect recovers. In-progress demotions proceed immediately (fail-safe: when uncertain, restrict rather than permit).

### Known Gaming Surface

**Pass-rate gaming:** Agents influence their own `first_attempt_pass_rate` through bead granularity choices — smaller beads are easier to pass on first attempt. **Interim mitigation (pre-Wave 3):** Pleasure signals are treated as advisory for promotion decisions. The ratchet can promote, but the promotion guard's evidence-quantity requirement (minimum beads at minimum complexity) reduces the impact of cherry-picking. The canonical evidence source is resolved during Authority package design (Wave 3).

**Lane-relabeling:** Agents could relabel beads to attract high-weight themes. **Accepted residual risk for Wave 1:** Lane assignment is a low-frequency principal action (not agent-initiated in normal workflow). The flux-review recommended file-path-based theme resolution as a structural mitigation; this is evaluated during Wave 3 Authority design alongside other gaming vectors.

---

## Safety Invariants

Eight invariants, each with structural enforcement — meaning the invariant is upheld by the consuming system's rejection logic, not by Ockham's behavioral compliance alone.

### Invariant 1: No Self-Promotion

An agent cannot promote its own authority tier. Enforced structurally at the write path: consumers of authority writes (bd set-state, intercore events) reject self-referential authority changes where the granting actor matches the benefiting agent. The `--actor` flag is validated against the session's agent ID.

### Invariant 2: Delegation Ceiling

An agent cannot grant authority exceeding its own level. If agent A holds supervised authority in a domain, it cannot promote agent B to autonomous in that domain.

### Invariant 3: Action-Time Validation

Authority is checked at execution time, not just at claim time. **Degradation contract:** If interspect is unavailable, action-time validation uses the last-known authority snapshot with a maximum staleness window. Beyond that window, fail-closed: deny all authority-dependent actions until interspect recovers.

### Invariant 4: Audit Completeness

Every authority decision — grant, revocation, promotion, demotion — produces a durable receipt in interspect. No authority change is fire-and-forget.

### Invariant 5: Human Halt Supremacy

The principal can halt the factory at any time by creating `factory-paused.json`. **Write-before-notify ordering** ensures durability: the sentinel file is written before any notification is sent. **Crash recovery:** If the process is killed between the write and the notification, `factory-paused.json` is still present and honored on restart. The notification is best-effort; the file is the authority.

### Invariant 6: Weight Neutrality Floor

No bead can be silently blackholed by organic weight accumulation. Reducing a bead's effective score below a defined floor requires an explicit freeze constraint — the principal's conscious decision, not an arithmetic accident.

### Invariant 7: Signal Independence

At least one Tier 3 trigger path is agent-unwritable and Clavain-independent. The `factory-paused.json` sentinel check is filesystem-only — no agent can suppress it through evidence manipulation, and it does not depend on Clavain being responsive.

### Invariant 8: Policy Immutability During Halt

When `factory-paused.json` exists, all Ockham subsystems are read-only. No weight updates, no authority changes, no signal evaluation. Only `ockham resume` (a principal action) re-enables writes.

### Authority Write Tokens

Authority writes are signed by intercore using HMAC with a per-session key derived from the agent's identity. Tokens are single-use (nonce-bound) and expire with the session. Revocation is event-based: an interspect event invalidates all tokens for a given agent-domain pair. Design-phase specs define the exact key derivation, HMAC algorithm, and nonce management.

### Halt Protocol

**Tier 3 restart sequence:**
1. Principal runs `ockham resume` (or removes `factory-paused.json`)
2. Ockham checks all Tier 2 signals — if any are still active, factory resumes in constrained mode (frozen themes stay frozen)
3. If all clear, factory resumes in normal mode
4. All domains reset to supervised atomically (single transaction — if `ockham resume` crashes mid-reset, all domains remain at their pre-halt level, and the next resume attempt retries the full reset). The supervised state persists for one confirmation period before the ratchet can restore prior autonomy levels

At least one Tier 3 notification path is independent of Clavain. If Clavain is hung, the filesystem sentinel still blocks dispatch, and the independent channel (e.g., direct file watch, Meadowsyn poll) reaches the principal.

---

## Phased Rollout

| Wave | Subsystems | Key Capabilities |
|------|-----------|-----------------|
| **1** | Intent, Scoring | Theme budgets, additive offsets, weight floor, starvation detection, dual logging, weight-outcome feedback, pleasure signals |
| **2** | Anomaly | Tier 1 INFORM, Tier 2 CONSTRAIN, multi-window confirmation, rate-of-change fast path, paired confirmation, de-escalation |
| **3** | Authority | Autonomy ratchet, domain grants, cold start, cross-domain resolution, evidence-quantity promotion, gaming mitigations |
| **4** | Re-evaluation | Direct dispatch authority for Ockham? Skaffen governance? Multi-factory? |

Each wave builds on the prior. Wave 1 delivers value immediately (intent-driven dispatch priority) while establishing the logging and feedback infrastructure that Waves 2-3 depend on.

---

## Open Questions

1. **Evidence gaming vectors (S-02):** Agents influence their own pass rates through bead granularity choices. Interim mitigation (advisory-only pleasure signals) is active from Wave 1; the canonical evidence source is resolved during Authority package design (Wave 3).

2. **Multi-factory:** If multiple Sylveste instances share a beads tracker, does each get its own Ockham instance? Deferred to post-Wave 1 — single-factory assumption for now.
