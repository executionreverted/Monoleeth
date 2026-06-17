# Phase 03 Progression Ships Stats Symphony Wave Plan

Date: 2026-06-17

This plan is for the main Codex project-manager session. Do not paste this full
file into worker prompts. Worker prompts must point workers to
`docs/symphony-worker-rules.md` and must explicitly say "do not commit".

## Goal

Build Phase 03 in conflict-aware Symphony waves so progression, ships, modules,
loadouts, and stat aggregation are ready before combat/world work starts.

## Current Phase

- Roadmap: `docs/roadmap/03-progression-ships-modules-stats.md`
- Source specs:
  - `docs/plans/modules/01-player-progression-rank-role-skills.md`
  - `docs/plans/modules/03-ship-hangar-loadout.md`
  - `docs/plans/modules/04-module-stat-aggregation.md`
  - `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- Dependencies complete: Phase 01 and Phase 02.

## Manager Rules

- Keep `AGENTS.md` and `docs/symphony-operating-model.md` out of worker prompts.
- Every worker prompt must include `docs/symphony-worker-rules.md`.
- Every worker prompt must include "Do not spawn subagents" and "Do not commit".
- Prefer 3-4 parallel tasks when write sets are disjoint.
- Fetch `/api/v1/tasks/{id}/workspace-diff` after completion.
- Review each patch before applying.
- Apply, verify, review, and commit one task at a time.
- After every commit, run verification and a security/performance/code-quality review pass.

## Wave 1: Foundation Models And Catalogs

### W1-A: Progression Models And XP Tables

Files:

- Create: `internal/game/progression/`
- Modify: `docs/roadmap/03-progression-ships-modules-stats.md`

Scope:

- Define player progression state, role level state, skill point state, and unlocked skill node state.
- Define main and role XP tables for rank 1 through 5 coverage.
- Add deterministic level lookup helpers.
- Add snapshot helpers that return defensive copies where needed.
- Do not implement `GrantXP`, `TryRankUp`, or skill unlock service methods in this task.

Validation:

```bash
go test ./internal/game/progression -count=1
git diff --check
```

### W1-B: Ship Catalog And Hangar Models

Files:

- Create: `internal/game/ships/`
- Modify: `docs/roadmap/03-progression-ships-modules-stats.md`

Scope:

- Define ship catalog rows for `starter`, `fighter_t1`, `scout_t1`, and `hauler_t1`.
- Define player ship state and active ship state.
- Validate ship definition rank requirements, slot counts, base stats, and cargo capacity.
- Do not implement ship unlock or active ship service methods in this task.

Validation:

```bash
go test ./internal/game/ships -count=1
git diff --check
```

### W1-C: Module Catalog And Equip Models

Files:

- Create: `internal/game/modules/`
- Modify: `docs/roadmap/03-progression-ships-modules-stats.md`

Scope:

- Define module slot types, module definitions, module stat modifiers, and equipped module state.
- Add MVP module catalog rows for one laser, shield, scanner, radar, and cargo expander.
- Validate slot compatibility data, rank/role requirements, durability fields, and trade flags.
- Do not implement loadout apply or equip service methods in this task.

Validation:

```bash
go test ./internal/game/modules -count=1
git diff --check
```

### W1-D: Effective Stats Model And Aggregation Skeleton

Files:

- Create: `internal/game/stats/`
- Modify: `docs/roadmap/03-progression-ships-modules-stats.md`

Scope:

- Define effective stat model with core, combat, exploration, and economy stats needed by Phase 03.
- Implement deterministic aggregation order: base ship, flat modules, flat passives, role bonuses, percent modules, percent passives, buffs/debuffs, clamp.
- Define stat snapshot version and invalidation state.
- Do not wire to ships/modules/progression packages yet unless imports are purely type-level and stable.

Validation:

```bash
go test ./internal/game/stats -count=1
git diff --check
```

## Wave 1 Merge Order

1. W1-A progression
2. W1-B ships
3. W1-C modules
4. W1-D stats

Reasoning: the first three write separate packages and define inputs; stats can
remain package-local in W1 and wire to concrete packages later.

## Wave 2: Service Slices

Open Wave 2 only after Wave 1 is merged and verified.

- Progression service: `GrantXP`, source uniqueness, role XP, `TryRankUp`, rank history, skill point grant.
- Ship service: starter guarantee, idempotent unlock, active ship selection, safe-area/combat/disabled validation.
- Module/loadout service: save/apply loadout, ownership/location/policy checks, duplicate instance prevention, broken module rejection.
- Stat service: `GetEffectiveStats`, explicit invalidation/versioning, cargo capacity derived from effective stats.

## Wave 3: Audit And Closure

- Run a Phase 03 audit task after Wave 2.
- Verify abuse checks and done criteria.
- Mark only tested roadmap lines complete.
- Keep Phase 03 open if cargo capacity is not genuinely read from effective stats.
