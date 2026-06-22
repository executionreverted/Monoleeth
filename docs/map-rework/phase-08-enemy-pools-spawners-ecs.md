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

Replace/retire from default gameplay paths:

- `internal/game/server/combat_loot_repair.go:16-19` defines
  `trainingNPCType = "training_drone"` as the only runtime NPC type for the
  original browser loop. Phase08A/08B moved this into explicit starter-map
  catalog and spawner content rather than a global default population.
- `internal/game/server/combat_loot_helpers.go:93-108` turns any target world
  NPC entity into a combat actor by calling `trainingNPCActor` when no actor
  already exists. Phase08C removed this as the default runtime path:
  spawner-backed NPCs project actors from catalog state, while ad-hoc inserted
  NPC entities stay visible generic NPCs without combat actors.
- `internal/game/server/combat_loot_helpers.go:110-140` hard-codes the training
  NPC stat snapshot: type, level, HP, shield, energy, weapon range, accuracy,
  and radar range. Phase08C replaced this for spawner-backed NPC projection
  with `EnemySpawnRecord` + `NPCStatTemplate` data and preserved mutable combat
  state across resync.
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

- runtime actor projection from stat templates
- kill/respawn path and cap accounting
- map-aware loot selector
- aggro/leash ticks
- boss/event hooks

## Phase08B Landed Initial Spawner Slice

The landed Phase08B slice adds worker-local enemy spawner state under
`internal/game/world/worker`. The spawner stores server-only live NPC rows with
entity id, enemy pool id, spawn area id, NPC type, chosen MVP level, stat/drop,
aggro, leash profile ids, position, alive state, and spawn timestamp. It keeps
entity and pool alive indexes inside the map worker and exposes clone-safe
server/internal read APIs for tests; these structures are not client wire JSON.

Map workers now accept `InitializeEnemyPoolsCommand`, which performs the
deterministic initial-fill MVP from a validated `worldmaps.MapDefinition`.
Enabled, non-disabled pools seed up to `initial_alive` while respecting each
pool cap and the strictest configured map cap across enabled pools until a
future catalog-level map cap field exists. The Phase08B candidate is the
configured spawn area center; future RNG, jitter, and richer placement remain
deferred. Candidates are skipped without client leakage when they are outside
map bounds, inside a PvP-blocking safe zone for safe-zone-excluded areas, or
inside a visible portal exclusion radius. Spawned NPC world entities use
`world.EntityTypeNPC`, and server speed comes from the referenced NPC stat
template.

`runtime.seedWorld` now initializes NPCs through the spawner command instead of
manually inserting a default visible training NPC into every map. The starter
map still passes an explicit migration entity-id override so its first training
drone remains `entity_training_npc`; maps without enemy pools, including
`1-2`, do not receive a default training NPC. Hidden planet signal seeding is
unchanged. At the end of Phase08B, the runtime still used the old training NPC
combat actor projection and global training salvage loot table after the
spawner-created entity existed; catalog-backed actor projection and map-aware
loot selection were still deferred.

## Phase08C Landed Catalog-Backed Actor Projection Slice

The landed Phase08C slice replaces the default hard-coded `trainingNPCActor`
runtime path with catalog-backed NPC combat actor projection. Runtime seed now
projects every alive spawner-backed NPC from its `EnemySpawnRecord` and the
referenced `NPCStatTemplate`. Ad-hoc inserted NPC world entities remain visible
generic NPCs, but they do not receive combat actors just because their entity
type is `npc`.

Spawner-backed NPC AOI/public metadata can expose client-safe combat status.
The projection uses template combat stats and content-driven signature data,
while mutable `CombatService` state such as HP, shield, energy, cooldowns, and
contributions is preserved across resync instead of being reset from the
template on every sync.

Phase08C coverage was added or updated in:

- `internal/game/server/server_enemy_spawner_test.go`
- `internal/game/server/server_visibility_detection_test.go`
- `internal/game/server/server_world_movement_test.go`

## Phase08D Landed Enemy Death Accounting Slice

The landed Phase08D slice adds worker-owned kill/death accounting for
spawner-backed NPCs without adding respawn/fill spawning yet. The map worker now
accepts `MarkEnemyKilledCommand`, validates that the supplied map definition is
owned by the worker, resolves the server-only spawner row by NPC entity id, and
marks live rows dead exactly once. A live death records `DeadAt`, derives
`NextRespawnAt` from the owning pool `KillRespawnDelay`, updates the row
position from the current worker entity when available, decrements the pool and
map alive counts once, and removes the world entity. Duplicate death marks are
idempotent: original timing and counts remain unchanged, while any leftover
world entity with that id is removed. Unknown or non-spawner entities return the
worker unknown-entity error.

`combat.use_skill` now notifies the owning map worker/spawner before loot drop
creation and marks the NPC hidden only after the worker accepts the death. Quest
progress and XP remain on their existing kill-event flow. Loot selection is
intentionally unchanged in this slice and still uses the existing runtime loot
table until the map/risk/rank-aware selector lands.

Phase08D coverage was added or updated in:

- `internal/game/world/worker/enemy_spawner_test.go`
- `internal/game/server/server_combat_loot_death_test.go`

## Phase08E Landed Enemy Respawn And Periodic Fill Tick Slice

The landed Phase08E slice adds worker-local spawner tick logic for map-owned
enemy pools. `InitializeEnemyPoolsCommand` now installs a private, cloned map
definition for subsequent worker ticks, and `Worker.Tick()` evaluates due enemy
respawns and periodic fills after movement without exposing pool ids, spawn
candidates, timing seeds, or future rolls to clients.

Kill-delay respawn reuses the dead spawner row and entity id when the row's
`NextRespawnAt` is due, clears `DeadAt`/`NextRespawnAt`, updates `SpawnedAt`,
reinserts the NPC world entity, and restores stat-template speed. Duplicate
death marks remain idempotent and do not drift pool or map alive counts. The
runtime projection now resets stale dead combat actors and clears the killed
hidden flag when a reused respawn id becomes alive again.

Periodic fill is enabled only for enabled `SpawnModePeriodic` pools. It waits
for `SpawnInterval`, creates at most one new row per due pool tick, derives the
new entity id with the existing `enemySpawnEntityID` style and next per-pool
row index, and respects pool caps plus the same strictest enabled pool
`MapMaxAlive` cap used by initial fill. Dead rows with pending
`NextRespawnAt` reserve pool and map capacity so periodic fill cannot consume
the slot before kill-delay respawn restores the original entity id.
`SpawnModeKillReplacement` restores killed rows through kill-delay respawn but
does not periodic-fill extra rows, and `SpawnModeDisabled` stays inert. Respawn
and periodic fill reuse the Phase08B deterministic center candidate and skip
candidates inside forbidden safe-zone or visible portal exclusion positions
without leaking entities.

`SpawnJitter` is honored as a deterministic server-only timing offset derived
from map/pool/entity identifiers; `SpawnJitter=0` remains exact and tested.
Richer random placement inside spawn areas remains deferred.

Phase08E coverage was added or updated in:

- `internal/game/world/worker/enemy_spawner_test.go`
- `internal/game/server/npc_actor_projection.go`
- `internal/game/server/runtime_world_snapshot.go`

Phase08F picked up the map/risk/rank-aware loot selector that Phase08E
deferred. Other Phase08E deferred work remains:

- aggro/leash simulation
- boss/event spawn hooks

## Phase08F Landed Map/Risk/Rank-Aware Loot Selector Slice

The landed Phase08F slice replaces the runtime's single global NPC loot table
with a server-only loot table registry keyed by loot table id. The starter
`training_drone_salvage` table remains seeded as content, so the starter
training drone still drops `raw_ore x3`, but default NPC kill handling no
longer falls back to it.

On NPC kill, the runtime resolves the active map instance, the killed NPC's
`EnemySpawnRecord`, the record's `NPCDropProfile`, profile compatibility, and
the registry loot table before `MarkEnemyKilledCommand` marks/removes the NPC
and before realtime combat events are queued. Compatibility checks keep NPC
type, killed entity id, record level, and map risk band aligned with the drop
profile. Missing records, profiles, mismatches, or tables return safe runtime
errors, restore the pre-command combat actor state, leave the spawner row alive,
and do not create drops or queued stale combat events.

The selector keeps table ids, drop profile ids, pool ids, roll data, seeds, and
future spawn data server-only. Client loot payloads remain limited to drop id,
entity id, position, item id, quantity, state, and expiry.

Phase08F coverage was added or updated in:

- `internal/game/server/npc_loot_selector.go`
- `internal/game/server/npc_loot_selector_command_test.go`
- `internal/game/server/npc_loot_selector_test.go`
- `internal/game/server/server_combat_loot_death_test.go`

Deferred Phase08 work after Phase08F:

- boss/event spawn hooks

## Phase08G Landed Worker-Local Aggro/Leash Simulation Slice

The landed Phase08G slice adds worker-owned, server-only aggro/leash state to
spawner-backed NPC rows. Rows now track leash origin, aggro target entity id,
target acquired/last-seen timestamps, and last aggro tick time. These fields are
kept inside `EnemySpawnRecord`/`EnemySpawnSnapshot` server-only copies and are
not added to AOI, map, combat, minimap, or client payloads.

Spawn and respawn initialize leash origin at the spawn position. Death and
respawn clear stale target memory and aggro tick state before the row can
participate again. The starter training drone remains passive and stationary
because its catalog aggro radius is `0` and speed is `0`.

`Worker.Tick()` now runs a narrow aggro system after movement and spawner
respawn/fill handling. The system only considers alive spawner-backed NPC rows,
uses same-worker `playerEntities` as the only target source, excludes
worker-marked hidden/stealthed aggro-ineligible players, chooses the nearest
eligible player inside `NPCAggroProfile.AggroRadius` deterministically, and
drives chase or return movement through existing server-owned
`world.MovementState` and `entitySpeeds`. It does not traverse portals or query
destination maps. Runtime stealth sync updates this worker-owned eligibility
state before aggro ticks so NPC public movement targets cannot retain hidden
player coordinates.

NPCs using `SafeZoneAttackPolicy == "never"` do not acquire or keep targets
when either the NPC or the target is inside a PvP-blocking safe zone. Targets
that leave aggro range are remembered only until `TargetMemory` expires. If the
NPC or target breaks `NPCLeashProfile.LeashDistance` and `ResetOnBreak` is true,
the target is cleared and the NPC returns toward its leash origin when speed
permits. Phase08G deliberately does not add auto-attack, damage ticks, assist
aggro, public combat events, client UI, metrics/logging, or boss/event spawn
hooks.

Phase08G coverage was added in:

- `internal/game/world/worker/enemy_aggro.go`
- `internal/game/world/worker/enemy_aggro_test.go`

Phase08H picked up the boss/event spawn hooks that Phase08G deferred.

Deferred Phase08 work after Phase08G:

- metrics/logging for spawn, cap, death, drop, respawn, aggro reset, and
  cross-map rejection paths
- debug/demo spawn path quarantine remains to be audited outside the event hook
  slice

## Phase08H Landed Boss/Event Spawn Hooks Slice

The landed Phase08H slice adds server-owned disabled-by-default boss/event spawn
catalog hooks without changing starter gameplay. `MapDefinition` now carries
server-only `NPCEventSpawnDefinition` entries and `SpawnModeEventScheduled`
enemy pools. These fields remain excluded from `ClientMapProjection`; browser
map JSON still does not receive event ids, pool ids, stat template ids, drop
profile ids, schedules, caps, spawn candidates, loot table ids, rare caps, or
roll data.

The starter map now includes a disabled overseer event hook and event-scheduled
pool as validation content. Because event-scheduled pools are skipped by
initial fill, periodic fill, and automatic kill-delay respawn, the default
starter runtime still spawns only the existing training drone unless a
server-owned trigger explicitly accepts an enabled, due event hook.

Map workers now accept `TriggerEnemyEventSpawnCommand`. The command is an
in-process server hook, not a client operation. It validates worker map
ownership, event hook existence, hook enablement, schedule delay, event cap,
pool cap, map cap, event-owned pool mode, spawn area validity, safe-zone
exclusion, visible portal exclusion, stat template presence/compatibility,
event map policy, and event drop profile compatibility before inserting a
normal spawner-backed NPC row and world NPC entity. Spawned rows carry
server-only event metadata plus the normal pool/stat/drop/aggro/leash metadata,
so existing actor projection, aggro/leash, death accounting, and Phase08F loot
selection continue to work without a combat or loot rewrite. Trigger no-ops for
disabled, not-yet-due, capped, or forbidden-candidate hooks do not insert stale
rows or entities.

The Phase08H review retry hardened event hooks so newly introduced event ids
initialize and respect their `StartsAfter` due cache, event entity ids use an
opaque deterministic hash suffix instead of reversible map/pool/event text, and
event triggers enforce the strictest enabled map cap across mixed normal/event
pools before spawning. The final rereview fix also keeps raw worker
`MapDefinition` inputs fail-closed unless `ZoneID` equals
`InternalMapID.ZoneID()` and that zone is the worker-owned current map, so
event triggers and shared enemy pool commands cannot seed rows for a different
internal map.

Phase08H coverage was added or updated in:

- `internal/game/world/maps/enemy_catalog.go`
- `internal/game/world/maps/enemy_catalog_test.go`
- `internal/game/world/maps/catalog_test.go`
- `internal/game/world/worker/enemy_spawner.go`
- `internal/game/world/worker/enemy_event_spawner_test.go`
- `internal/game/world/worker/enemy_spawner_test.go`

Deferred Phase08 work after Phase08H:

- metrics/logging for spawn attempts, cap skips, kill/drop selection, respawn
  delays, aggro resets, and cross-map rejection counts without logging hidden
  roll data
- debug/demo spawn command quarantine from default real gameplay remains open

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
- `NPCEventSpawnDefinition`
  - `event_spawn_id`
  - `enemy_pool_id`
  - `drop_profile_id`
  - `enabled`
  - `starts_after`
  - `max_alive`
  - `map_policy`
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
  projection. Phase08C landed this for spawner-backed NPCs: the map worker row
  supplies lifecycle, map ownership, position, NPC type, and stat template
  identity; catalog/template data supplies stats and signature; and
  `CombatService` supplies or updates HP, shield, energy, cooldowns,
  contributions, death decision, and kill event through the corresponding
  `combat.ActorState`.
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
   projection from map-worker row state. Landed in Phase08C for alive
   spawner-backed NPCs; ad-hoc inserted NPCs remain generic visible NPCs
   without combat actors.
6. Route combat target sync through the owning map worker so cross-map or
   missing-map targets fail before combat service execution.
7. On NPC death, notify the spawner first, then create drops with the
   `npc_type + map_id/risk/rank_band` table selector, then broadcast AOI removal
   only to sessions in the same map. Landed in Phase08F for
   spawner-record/drop-profile-based loot table selection.
8. Add periodic fill and kill-delay respawn scheduling/tick logic that restores
   NPCs after `spawn_interval_ms` or `kill_respawn_delay_ms` without exceeding
   map/pool caps. Landed in Phase08E for worker-local pool ticks.
9. Add aggro/leash tick logic as a narrow data-oriented system:
   acquire visible targets in radar/aggro range, chase only inside leash policy,
   reset when target leaves map/range/safe state, and never cross portals.
   Landed in Phase08G for worker-local same-map player acquisition, target
   memory, safe-zone reset, leash break reset, and chase/return movement.
10. Add metrics and logs for spawn attempts, cap skips, kill/drop selection,
    respawn delays, aggro resets, and cross-map rejection counts without logging
    hidden roll data.
11. Add boss/event spawn hooks as disabled-by-default catalog entries with
    explicit event schedule, cap, map policy, and reward/drop profile. Landed
    in Phase08H for server-only catalog hooks and explicit worker trigger
    spawning.
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
- Periodic spawn fill respects `spawn_interval_ms`, deterministic
  `spawn_jitter_ms`, `map_max_alive`, and `pool_max_alive`. Phase08E landed the
  worker-local deterministic center-candidate MVP.
- Respawn does not exceed caps after repeated kills or duplicate death events.
  Phase08E landed this for worker-local spawner rows.
- Kill-triggered replacement waits for `kill_respawn_delay_ms`. Phase08E landed
  this for worker-local spawner rows.
- NPC actor projection uses the configured stat template and preserves existing
  combat HP/shield/energy, cooldown, and contribution state. Phase08C landed
  this for spawner-backed NPC projection.
- NPC HP/shield/energy have one authoritative owner; ECS/spawner caches cannot
  diverge from `CombatService`.
- Combat against an NPC in another map fails as not visible/not found and does
  not spend energy or mutate target state.
- NPC death selects a loot table by `npc_type + map_id/risk/rank_band`.
  Phase08F landed this through the active map spawner record and
  `NPCDropProfile.LootTableID`.
- Duplicate kill/drop creation returns the existing drops and does not double
  grant loot, cargo, XP, quest progress, or metrics.
- Drops remain visible only through same-map radar/AOI, preserve owner lock, and
  reject hidden/far pickup.
- Aggro starts only for visible/radar-valid targets and resets on hidden/stealth
  ineligibility, leash break, safe-zone entry, portal transfer, death, or map
  mismatch. Phase08G landed the worker-local same-worker target acquisition,
  hidden/stealth ineligibility filtering, safe-zone reset, target-memory,
  leash-break, dead-row, and respawn clearing coverage; portal transfer remains
  covered by same-worker ownership rather than cross-map traversal.
- Boss/event spawns are disabled unless a catalog event enables them and tests
  cover their caps/reward profile. Phase08H landed disabled starter event
  catalog content, event-owned pool validation, due/enabled trigger behavior,
  event/pool/map caps, and forbidden candidate no-op coverage.
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

Phase08C satisfies the `trainingNPCActor` replacement portion of the first
criterion for spawner-backed NPCs. Phase08D satisfies narrow death accounting
for spawner rows and alive counters. Phase08E satisfies worker-local
kill-delay respawn and periodic fill tick behavior. Phase08F satisfies the
global loot table replacement for default NPC kill drops. Phase08G satisfies
worker-local aggro/leash movement simulation. Phase08H satisfies the
disabled-by-default boss/event hook slice with explicit server-owned trigger
spawning. Metrics/logging and debug/demo quarantine remain open, so the full
Phase 08 acceptance criteria are not complete.

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
