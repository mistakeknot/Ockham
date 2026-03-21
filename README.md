# Ockham

Factory governor for autonomous AI agent dispatch. Translates strategic intent into dispatch routing, manages authority tiers, and detects factory anomalies.

Named after Ockham Saneer from Ada Palmer's *Terra Ignota* — who ran the world's Humanist transportation network, routing resources through competing hive interests.

Part of the Demarch AI factory architecture:

```
Meadowsyn (UI)  →  Ockham (governor)  →  Clavain/Zaka/Alwe (execution)
```

## Install

```bash
go install github.com/mistakeknot/Ockham/cmd/ockham@latest
```

## Usage

```bash
# Set strategic intent
ockham intent --theme auth --budget 40%
ockham intent --freeze non-critical

# Manage agent authority
ockham authority grant agent-3 --domain "core/*"
ockham authority tier agent-3 --level supervised

# Monitor factory health
ockham health
ockham anomaly --since 1h

# Preview dispatch decisions
ockham dispatch advise
```

## Concepts

- **Intent directives** — theme-level budgets, not per-task commands
- **Authority tiers** — trust ratchet from shadow → supervised → autonomous
- **Algedonic signals** — pain/pleasure indicators that surface immediately
- **Dispatch routing** — synthesizes intent + authority + state into scoring weights

## Part of Demarch

Ockham is an L2 OS component of [Demarch](https://github.com/mistakeknot/Demarch), the autonomous software development agency platform.

## License

MIT
