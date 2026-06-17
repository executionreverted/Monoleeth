# Phase 03: Progression, Ships, Modules, And Stats

## Status

- State: In progress
- Owner: Player progression and ship build systems
- Depends on: Phase 01, Phase 02
- Unlocks: combat, scanning, cargo validation, planet rank checks, crafting outputs

## Goal

Build player progression, rank, role XP, pilot skills, starter ships, hangar, loadouts, module equip, and server-side effective stat aggregation.

## Why This Comes Before Combat

Combat, movement, scanner, cargo, route, and production rules must read server-calculated effective stats. The client must never submit speed, damage, radar, cargo, cooldown, or energy values.

## Source Specs

Read before implementation:

- `docs/plans/modules/01-player-progression-rank-role-skills.md`
- `docs/plans/modules/03-ship-hangar-loadout.md`
- `docs/plans/modules/04-module-stat-aggregation.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `PlayerProgressionService`
- `RankService`
- `RoleXpService`
- `PilotSkillTreeService`
- `ShipService`
- `HangarService`
- `LoadoutService`
- `ModuleService`
- `StatAggregationService`

Does not own:

- combat damage application
- loot drops
- craft recipe validation
- market trading
- planet ownership

## Package Direction

Suggested packages:

```text
internal/game/progression/
internal/game/ships/
internal/game/modules/
internal/game/stats/
```

## Catalogs To Start With

Keep MVP small:

- ships: `starter`, `fighter_t1`, `scout_t1`, `hauler_t1`
- modules: one laser, one shield, one scanner, one radar, one cargo expander
- passive tree: 3 branches, 5-7 nodes per branch
- rank table: enough to test rank 1 through rank 5
- role levels: combat, scout, crafting, construction

## TODO: Progression

- [x] Define player progression state.
- [x] Define role level state.
- [x] Define skill point state.
- [x] Define unlocked skill node state.
- [x] Implement deterministic main level XP table.
- [x] Implement deterministic role level XP table.
- [x] Implement `GrantXP` with source type, source ID, and idempotency key.
- [x] Implement XP source uniqueness.
- [x] Implement `TryRankUp` with simple requirement table.
- [x] Implement rank history.
- [x] Grant one pilot skill point on rank up.
- [x] Emit stat invalidation on rank up and role level up.
- [x] Implement `UnlockPilotSkill`.
- [x] Implement pilot skill prerequisite validation.
- [x] Implement pilot skill respec skeleton.

## TODO: Ships And Hangar

- [x] Define ship catalog.
- [x] Define player ship state.
- [x] Define active ship state.
- [x] Implement starter ship guarantee.
- [x] Implement idempotent ship unlock.
- [x] Implement active ship selection.
- [x] Implement safe-area ship swap validation.
- [x] Block ship swap in combat.
- [x] Block ship swap outside hangar/safe area.
- [x] Block active use of disabled ships.
- [x] Block swap if current cargo exceeds target cargo capacity.
- [x] Emit stat invalidation when active ship changes.

## TODO: Loadouts And Modules

- [x] Define module catalog.
- [x] Define equipped module state.
- [x] Define loadout model.
- [x] Implement `SaveLoadout`.
- [x] Implement `ApplyLoadout`.
- [x] Validate module ownership.
- [x] Validate item location is equip-eligible.
- [x] Validate slot type compatibility.
- [x] Validate rank and role requirements.
- [x] Validate module durability.
- [x] Prevent same module instance in two slots.
- [x] Prevent escrow/reserved modules from being equipped.
- [x] Emit stat invalidation on equip/unequip/break.

## TODO: Stat Aggregation

- [x] Define effective stat model.
- [x] Implement aggregation order: base ship, flat modules, flat passives, role bonuses, percent modules, percent passives, buffs/debuffs, clamp.
- [x] Implement stat snapshot version.
- [x] Implement stat invalidation.
- [x] Implement `GetEffectiveStats`.
- [x] Cache active session stats behind explicit versioning.
- [x] Ensure broken equipped modules provide no stat.

## Tests

- [x] XP grant idempotent.
- [x] Main level threshold works.
- [x] Role level threshold works.
- [x] Rank up double-click grants one rank and one skill point.
- [x] Locked skill node cannot be unlocked.
- [x] Skill unlock consumes points once.
- [x] Respec invalidates stat snapshot.
- [x] Starter ship always exists.
- [x] Duplicate ship unlock is no-op.
- [x] Swap in combat fails.
- [x] Swap outside safe area fails.
- [x] Swap to lower cargo ship fails when cargo does not fit.
- [x] Destroyed ship cannot become active.
- [x] Wrong module slot type fails.
- [x] Rank-too-low module equip fails.
- [x] Broken module equip fails.
- [x] Duplicate module equip fails.
- [x] Stat aggregation is deterministic.
- [x] Broken equipped module is removed from effective stats.

## Abuse And Safety Checks

- [ ] Client cannot spoof XP source completion.
- [x] Client cannot spoof rank milestone.
- [x] Client cannot unlock hidden or locked skill node.
- [x] Client cannot activate locked ship.
- [x] Client cannot bypass cargo capacity via ship swap.
- [x] Client cannot inject module stats or tier metadata.

## Done Criteria

- [x] Player progression snapshot exists.
- [x] Starter ship flow exists.
- [x] Loadout and equip validation exists.
- [x] Effective stats are server-calculated.
- [x] Cargo capacity can be read from effective stats.
- [x] Combat and scanner can use stat snapshots next phase.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Resume Notes

If resuming here, search for any code using client-provided stat values. Replace with `StatAggregationService` output before moving to combat or world worker phases.

Current Symphony wave plan:

- `docs/plans/2026-06-17-phase-03-progression-ships-stats-wave.md`

Verified slices:

- Progression state, snapshot, main XP table, and role XP table are implemented in `internal/game/progression`.
- Ship catalog, player ship state, and active ship state are implemented in `internal/game/ships`.
- Module catalog validation and equipped module state are implemented in `internal/game/modules`.
- Effective stat model, deterministic aggregation order, snapshot versioning, and invalidation state are implemented in `internal/game/stats`.
- Stat service `GetEffectiveStats`, explicit active-session cache versioning, cargo capacity output, and broken-module exclusion are implemented in `internal/game/stats`.
- Starter guarantee, idempotent ship unlock, active ship selection, safe-area/combat/disabled/cargo swap validation, and active-ship stat invalidation signal are implemented in `internal/game/ships`.
- Loadout model, `SaveLoadout`, `ApplyLoadout`, ownership/location/slot/rank/role/durability/duplicate-instance validation, and equip invalidation signals are implemented in `internal/game/modules`.
- Equipped module break handling emits one stat invalidation, is idempotent after the first durability transition, and rejects non-equipped/wrong-owner/wrong-ship spoof attempts in `internal/game/modules`.
- Progression `GrantXP`, role XP, XP source/idempotency uniqueness, `TryRankUp`, rank history, rank-up skill point grant, and progression stat invalidation signals are implemented in `internal/game/progression`.
- Pilot skill definitions, `UnlockPilotSkill`, prerequisite/rank/role/point validation, duplicate unlock safety, and respec stat invalidation signals are implemented in `internal/game/progression`.
- Final verification passed with `go test ./...`, `git diff --check`, and `go test -race ./internal/game/progression ./internal/game/modules ./internal/game/ships ./internal/game/stats`.
- Review hardening batch 1 added concrete ship slot-layout validation for loadouts, player-scoped loadout ids, mandatory rank-up idempotency keys, no-op stat invalidation suppression for unchanged loadout apply and empty respec, and non-zero active ship timestamp validation.

Remaining follow-up:

- XP source completion spoofing remains open until XP grants are wired behind concrete domain owners such as quest, combat, scanner, production, or crafting completion services.
- Ship unlock/activation should derive rank checks from authoritative progression state, not caller-provided context.
- Module equip should derive rank/role gates from authoritative progression state and move/bind item locations through an inventory ledger transaction.
- Stat snapshots need an authoritative input builder from ship/module/progression records plus atomic invalidation/recalculation semantics before combat or scanner consume them.
