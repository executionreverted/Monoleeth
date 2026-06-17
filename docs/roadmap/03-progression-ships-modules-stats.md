# Phase 03: Progression, Ships, Modules, And Stats

## Status

- State: Not started
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

- [ ] Define player progression state.
- [ ] Define role level state.
- [ ] Define skill point state.
- [ ] Define unlocked skill node state.
- [ ] Implement deterministic main level XP table.
- [ ] Implement deterministic role level XP table.
- [ ] Implement `GrantXP` with source type, source ID, and idempotency key.
- [ ] Implement XP source uniqueness.
- [ ] Implement `TryRankUp` with simple requirement table.
- [ ] Implement rank history.
- [ ] Grant one pilot skill point on rank up.
- [ ] Emit stat invalidation on rank up and role level up.
- [ ] Implement `UnlockPilotSkill`.
- [ ] Implement pilot skill prerequisite validation.
- [ ] Implement pilot skill respec skeleton.

## TODO: Ships And Hangar

- [ ] Define ship catalog.
- [ ] Define player ship state.
- [ ] Define active ship state.
- [ ] Implement starter ship guarantee.
- [ ] Implement idempotent ship unlock.
- [ ] Implement active ship selection.
- [ ] Implement safe-area ship swap validation.
- [ ] Block ship swap in combat.
- [ ] Block ship swap outside hangar/safe area.
- [ ] Block active use of disabled ships.
- [ ] Block swap if current cargo exceeds target cargo capacity.
- [ ] Emit stat invalidation when active ship changes.

## TODO: Loadouts And Modules

- [ ] Define module catalog.
- [ ] Define equipped module state.
- [ ] Define loadout model.
- [ ] Implement `SaveLoadout`.
- [ ] Implement `ApplyLoadout`.
- [ ] Validate module ownership.
- [ ] Validate item location is equip-eligible.
- [ ] Validate slot type compatibility.
- [ ] Validate rank and role requirements.
- [ ] Validate module durability.
- [ ] Prevent same module instance in two slots.
- [ ] Prevent escrow/reserved modules from being equipped.
- [ ] Emit stat invalidation on equip/unequip/break.

## TODO: Stat Aggregation

- [ ] Define effective stat model.
- [ ] Implement aggregation order: base ship, flat modules, flat passives, role bonuses, percent modules, percent passives, buffs/debuffs, clamp.
- [ ] Implement stat snapshot version.
- [ ] Implement stat invalidation.
- [ ] Implement `GetEffectiveStats`.
- [ ] Cache active session stats behind explicit versioning.
- [ ] Ensure broken equipped modules provide no stat.

## Tests

- [ ] XP grant idempotent.
- [ ] Main level threshold works.
- [ ] Role level threshold works.
- [ ] Rank up double-click grants one rank and one skill point.
- [ ] Locked skill node cannot be unlocked.
- [ ] Skill unlock consumes points once.
- [ ] Respec invalidates stat snapshot.
- [ ] Starter ship always exists.
- [ ] Duplicate ship unlock is no-op.
- [ ] Swap in combat fails.
- [ ] Swap outside safe area fails.
- [ ] Swap to lower cargo ship fails when cargo does not fit.
- [ ] Destroyed ship cannot become active.
- [ ] Wrong module slot type fails.
- [ ] Rank-too-low module equip fails.
- [ ] Broken module equip fails.
- [ ] Duplicate module equip fails.
- [ ] Stat aggregation is deterministic.
- [ ] Broken equipped module is removed from effective stats.

## Abuse And Safety Checks

- [ ] Client cannot spoof XP source completion.
- [ ] Client cannot spoof rank milestone.
- [ ] Client cannot unlock hidden or locked skill node.
- [ ] Client cannot activate locked ship.
- [ ] Client cannot bypass cargo capacity via ship swap.
- [ ] Client cannot inject module stats or tier metadata.

## Done Criteria

- [ ] Player progression snapshot exists.
- [ ] Starter ship flow exists.
- [ ] Loadout and equip validation exists.
- [ ] Effective stats are server-calculated.
- [ ] Cargo capacity can be read from effective stats.
- [ ] Combat and scanner can use stat snapshots next phase.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, search for any code using client-provided stat values. Replace with `StatAggregationService` output before moving to combat or world worker phases.
