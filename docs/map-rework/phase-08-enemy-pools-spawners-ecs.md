# Phase 08: Enemy Pools, Spawners, And ECS

## Goal

Replace the current training NPC/demo combat slice with map-owned enemy pools,
spawn areas, spawn caps, respawn timing, NPC stat templates, aggro/leash rules,
and map/risk-aware drop table selection.

This phase must keep combat, loot, cargo, XP, and quest mutations
server-authoritative. The ECS/data-oriented step is scoped to live map worker
and spawner state first; it is not a broad rewrite of every gameplay domain
service.

## Current State To Replace/Reuse

Replace:

- `internal/game/server/combat_loot_repair.go:16-19` defines
  `trainingNPCType = "training_drone"` as the only runtime NPC type for the
  current browser loop.
- `internal/game/server/combat_loot_helpers.go:93-108` turns any target world
  NPC entity into a combat actor by calling `trainingNPCActor` when no actor
  already exists.
- `internal/game/server/combat_loot_helpers.go:110-140` hard-codes the training
  NPC stat snapshot: type, level, HP, shield, energy, weapon range, accuracy,
  and radar range.
- `internal/game/server/combat_loot_catalog.go:11-47` creates one global
  `training_drone_salvage` loot table that always drops `raw_ore x3`.
- `internal/game/server/combat_loot_repair.go:111-148` handles NPC kill side
  effects against the single runtime loot table and removes the entity without
  map-owned respawn accounting.

Reuse:

- `internal/game/combat/types.go:32-56` already models server-owned combat
  actor state with `EntityID`, `NPCType`, `WorldID`, `ZoneID`, position,
  signature, stats, HP, shield, energy, cooldowns, and contribution tracking.
- `internal/game/combat/types.go:82-92` already emits `NPCKilledEvent` with
  `NPCType`, `WorldID`, `ZoneID`, position, owner player, and timestamp.
- `internal/game/loot/service.go:155-227` already creates NPC kill drops once
  per source kill event, preserving `WorldID`, `ZoneID`, position, owner lock,
  public lifetime, source type, source id, metrics, and scheduled drop tasks.
- `internal/game/loot/service.go:290-345` already validates pickup ownership,
  visibility, pickup range, cargo capacity, and the
  `loot_pickup:<drop_id>` idempotency key.
- `internal/game/loot/service.go:471-487` already filters visible drops through
  AOI/radar visibility.
- `docs/2026-06-17-progression-economy-systems-design.md:1476-1574` defines
  loot as server-generated, AOI/radar-visible, owner-locked, and cargo-bound.
- `docs/2026-06-17-progression-economy-systems-design.md:2166-2241` defines
  combat as client intent with server-owned range, visibility, cooldown,
  energy, hit/miss, damage, aggro, loot rights, and XP contribution.

## Target Model

Each bounded map owns its hostile population. A map definition declares enemy
pools, spawn areas, max alive counts, respawn delays, NPC stat templates, drop
table selectors, aggro/leash behavior, and risk modifiers. The active map
worker owns live spawned NPC instances and is the only runtime authority that
can create, despawn, respawn, or move those NPCs.

Map-level spawn policy must include both periodic fill and kill-delay respawn:

- `map_max_alive`: hard cap across all hostile NPCs in the map
- `pool_max_alive`: hard cap for one pool
- `initial_alive`: count to seed at map start
- `spawn_interval_ms`: periodic fill cadence
- `kill_respawn_delay_ms`: delay after an NPC death before the pool may replace
  it
- `spawn_jitter_ms`: randomization window to avoid synchronized waves
- spawn selection: server RNG chooses a valid point inside the spawn area,
  outside portal exclusion radii and hostile-forbidden safe zones
- spawn mode: periodic fill, kill-triggered replacement, boss/event scheduled
  spawn, or disabled

The initial data-oriented model should be local to map worker/spawner internals:
arrays/maps indexed by compact entity slot or `EntityID` for position, velocity,
NPC type, spawn area, alive/dead state, next respawn time, aggro target, leash
origin, combat actor id, and public status flags. Existing domain services keep
their current APIs while adapters project map-worker NPC rows into
`combat.ActorState`, world/AOI entity payloads, and loot kill events.

Drop table selection must be based on `npc_type + map_id/risk/rank_band`, not a
single global table. The selected table is passed into the existing loot service
at kill time. Hidden loot rolls, future spawn candidates, spawn seeds, and table
contents are never serialized to clients.

## Phase08A Landed Catalog Slice

The landed Phase08A slice adds server-owned enemy pool catalog primitives and
validation under `internal/game/world/maps`. Starter map `1-1` now declares the
current `training_drone` and `training_drone_salvage` behavior as explicit map
catalog content: one circular spawn area, one enabled periodic pool, one stat
template, one drop profile, one aggro profile, and one leash profile. These
catalog internals remain outside `ClientMapProjection`; browser JSON does not
receive enemy pool, spawn area, profile, loot table, or future spawn data.
Pool validation resolves referenced stat templates and drop profiles and rejects
NPC type mismatches, level bands that do not fully cover the pool band, and drop
profiles whose risk band does not exactly match the owning map risk band.

Deferred Phase 08 work:

- map-worker spawner component
- initial fill from enabled pools
- runtime actor projection from stat templates
- kill/respawn path and cap accounting
- map-aware loot selector
- aggro/leash ticks
- boss/event hooks

### NPC State Ownership

Avoid dual truth between ECS-style worker storage and `CombatService`.

MVP ownership:

- map worker/spawner owns NPC lifecycle, spawn area, position, movement intent,
  aggro target, leash origin, alive/dead membership, and respawn timing
- `CombatService` owns NPC combat HP, shield, energy, cooldowns,
  contributions, death decision, and kill event
- adapters project one map-worker NPC into one `combat.ActorState`
- death is processed once: `CombatService` emits kill result, spawner marks the
  NPC dead and schedules replacement, loot service creates drops from the same
  kill source

Do not add authoritative `hp[]`, `shield[]`, or `energy[]` arrays to the
spawner unless `CombatService` is explicitly refactored to read/write that same
component store. Cached public combat summaries are allowed only as derived
read models.

## Data Structures/Contracts To Add Or Change

Add catalog data:

- `MapEnemyPoolDefinition`
  - `map_id`
  - `pool_id`
  - `npc_type`
  - `rank_band` or `level_band`
  - `spawn_area_ids`
  - `map_max_alive`
  - `pool_max_alive`
  - `initial_alive`
  - `spawn_interval_ms`
  - `kill_respawn_delay_ms`
  - `spawn_jitter_ms`
  - `spawn_mode`
  - `stat_template_id`
  - `drop_profile_id`
  - `aggro_profile_id`
  - `leash_profile_id`
  - `enabled`
- `MapSpawnAreaDefinition`
  - `map_id`
  - `spawn_area_id`
  - `shape`: circle, rectangle, or polygon
  - `bounds`
  - `safe_zone_excluded`
  - `pvp_zone_allowed`
  - `portal_exclusion_radius`
- `NPCStatTemplate`
  - `stat_template_id`
  - `npc_type`
  - `rank_band`
  - base `HP`, `Shield`, `Energy`, weapon range, damage, cooldown, accuracy,
    tracking, evasion, radar signature, speed, XP value, and public label key
- `NPCDropProfile`
  - `drop_profile_id`
  - `npc_type`
  - `map_id` or `risk_band`
  - `rank_band`
  - weighted loot table reference
  - rare drop caps and event/boss overrides
- `NPCAggroProfile`
  - aggro radius
  - assist radius
  - target memory duration
  - leash distance
  - reset behavior
  - safe-zone attack policy

Add live map-worker/spawner state:

- `npc_entity_ids[]`
- `npc_type[]`
- `spawn_area_id[]`
- `position_x[]`, `position_y[]`
- `velocity_x[]`, `velocity_y[]`
- `combat_actor_id[]` or `combat_state_ref[]`
- derived public combat summary cache, if needed
- `alive[]`, `dead_at[]`, `next_respawn_at[]`
- `aggro_target_entity_id[]`
- `leash_origin_x[]`, `leash_origin_y[]`
- `last_think_at[]`
- index maps from `EntityID` to row slot and from pool id to active row slots

Change contracts:

- Replace the training actor factory with a map/NPC catalog-backed actor
  projection. The map worker row supplies lifecycle, map ownership, position,
  signature, NPC type, and stat template identity. `CombatService` supplies or
  updates HP, shield, energy, cooldowns, contributions, death decision, and kill
  event through the corresponding `combat.ActorState`.
- Change NPC kill handling so the worker/spawner receives the death before the
  entity is hidden or removed. The spawner decrements alive count, records
  respawn state, and schedules/derives the next spawn.
- Change loot creation to call a selector such as:

```text
select_loot_table(npc_type, map_id, risk_band, rank_band, killed_at)
```

- Add public NPC metadata to AOI payloads only when safe:
  `entity_type`, `entity_id`, `position`, public display label, disposition,
  combat status, movement, status flags, and optional public rank/threat band.
  Do not send pool ids, spawn candidates, stat template internals, loot table
  ids, rare caps, or roll results.

## Implementation Tasks In Order

1. Define enemy pool, spawn area, NPC stat, drop profile, aggro, and leash
   catalog structures with validation for map id, bounded coordinates,
   non-negative caps, sane respawn delays, and safe-zone/PvP constraints.
2. Add seed catalog entries that preserve the current training NPC behavior as
   an explicit starter-map pool, not as a global hard-coded demo branch.
3. Add a map-worker spawner component with data-oriented storage local to the
   active map instance.
4. Teach the spawner to populate initial NPCs from enabled map pools while
   respecting `map_max_alive`, `pool_max_alive`, spawn area bounds, portal
   exclusion radii, and safe zone restrictions.
5. Replace `trainingNPCActor` projection with catalog-backed NPC actor
   projection from map-worker row state.
6. Route combat target sync through the owning map worker so cross-map or
   missing-map targets fail before combat service execution.
7. On NPC death, notify the spawner first, then create drops with the
   `npc_type + map_id/risk/rank_band` table selector, then broadcast AOI removal
   only to sessions in the same map.
8. Add periodic fill and kill-delay respawn scheduling/tick logic that restores
   NPCs after `spawn_interval_ms` or `kill_respawn_delay_ms` without exceeding
   map/pool caps.
9. Add aggro/leash tick logic as a narrow data-oriented system:
   acquire visible targets in radar/aggro range, chase only inside leash policy,
   reset when target leaves map/range/safe state, and never cross portals.
10. Add metrics and logs for spawn attempts, cap skips, kill/drop selection,
    respawn delays, aggro resets, and cross-map rejection counts without logging
    hidden roll data.
11. Add boss/event spawn hooks as disabled-by-default catalog entries with
    explicit event schedule, cap, map policy, and reward/drop profile.
12. Remove or quarantine debug/demo spawn paths from default real gameplay.
    Any retained helper must require explicit dev/test mode.
13. Update phase docs and module docs only after behavior is implemented and
    verified.

## Tests To Add/Update

- Catalog validation rejects invalid map ids, out-of-bounds spawn areas,
  negative caps, impossible respawn delays, unknown stat/drop/profile refs, and
  safe-zone-incompatible hostile pools.
- Starter map seed creates the former training NPC only through a map enemy
  pool definition.
- Spawner initial fill respects per-pool and per-map max alive counts.
- Periodic spawn fill respects `spawn_interval_ms`, `spawn_jitter_ms`,
  `map_max_alive`, and `pool_max_alive`.
- Respawn does not exceed caps after repeated kills or duplicate death events.
- Kill-triggered replacement waits for `kill_respawn_delay_ms`.
- NPC actor projection uses the configured stat template and preserves existing
  combat cooldown/contribution state.
- NPC HP/shield/energy have one authoritative owner; ECS/spawner caches cannot
  diverge from `CombatService`.
- Combat against an NPC in another map fails as not visible/not found and does
  not spend energy or mutate target state.
- NPC death selects a loot table by `npc_type + map_id/risk/rank_band`.
- Duplicate kill/drop creation returns the existing drops and does not double
  grant loot, cargo, XP, quest progress, or metrics.
- Drops remain visible only through same-map radar/AOI, preserve owner lock, and
  reject hidden/far pickup.
- Aggro starts only for visible/radar-valid targets and resets on leash break,
  safe-zone entry, portal transfer, death, or map mismatch.
- Boss/event spawns are disabled unless a catalog event enables them and tests
  cover their caps/reward profile.
- Debug/demo spawn commands are unavailable in default authenticated real mode.

## Migration/Doc Updates

- Update the active map catalog docs to include enemy pool, spawn area, stat,
  aggro, leash, and drop profile fields.
- Update combat/loot module docs to state that NPC stats and drop tables are
  selected from map content, while combat and pickup rules stay service-owned.
- Update quest docs where objectives reference `npc_type` so starter/training
  quest objectives use catalog NPC ids and map/rank constraints.
- Update `docs/2026-06-17-progression-economy-systems-design.md` to replace
  global NPC loot language with map/risk-aware drop profiles.
- Document that the training NPC is seed content for the starter map only, not
  a general runtime fallback.

## Risks And Acceptance Criteria

Risks:

- A global fallback NPC actor could accidentally keep the old training stats
  alive in advanced maps.
- Spawn caps can drift if kill, despawn, handoff, and duplicate event paths do
  not share one owner.
- Drop tables can leak through logs, admin payloads, or client protocol fields.
- ECS-style storage can become a premature full rewrite if it crosses service
  boundaries too early.
- Aggro/leash logic can create server tick pressure if every NPC scans every
  player every frame.

Acceptance criteria:

- No default gameplay path uses `trainingNPCActor` or one global
  `training_drone_salvage` table as the source of truth.
- Every live NPC belongs to exactly one current map, pool, and spawn area.
- Each map enforces enemy pool max alive counts and respawn delays under repeated
  kill and duplicate event conditions.
- Combat, loot, pickup, XP, quest progress, cargo, and wallet mutations remain
  server-owned and idempotent.
- Loot table selection is keyed by NPC type and map/risk/rank band, with no
  client-visible loot rolls, future spawn candidates, or table internals.
- Players can see and action only NPCs/drops in their current map and radar
  range.
- Full verification for the phase includes:

```bash
go test ./...
git diff --check
cd client
npm --cache /tmp/gameproject-npm-cache run check
```
