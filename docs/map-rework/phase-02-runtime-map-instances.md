# Phase 02: Runtime Map Instances

## Goal

Implement the runtime slice that hosts one isolated simulation instance per
active map and routes all player commands, snapshots, worker ticks, AOI events,
and socket emissions through the player's active map.

This phase builds on the Phase 01 catalog/router contract. It should reuse the
existing worker, AOI, movement, visibility, session, and event patterns where
they already enforce server authority, but remove the single `Runtime.Worker`
assumption.

The runtime target is bounded multi-map gameplay:

- each map is `10000x10000`
- `WorldID` remains shard/universe
- `ZoneID` initially equals the server-only internal map id
- each map has its own worker, socket/AOI membership, enemy pools, safety
  rules, radar/stealth context, and rare planet scan/claim/production scope
- a player can see/action only entities in the current map and radar range
- client payloads are intents, never trusted map or gameplay facts

## Current State To Replace/Reuse

Replace these assumptions:

- `internal/game/server/runtime.go:83` through
  `internal/game/server/runtime.go:95` stores one `Worker`, one `worldID`, and
  one `zoneID` on the runtime.
- `internal/game/server/runtime.go:284` through
  `internal/game/server/runtime.go:321` creates exactly one worker during
  runtime construction.
- `internal/game/server/runtime.go:423` through
  `internal/game/server/runtime.go:424` stores only the configured runtime
  world/zone ids.
- `internal/game/server/runtime.go:525` through
  `internal/game/server/runtime.go:547` starts one ticker using
  `runtime.Worker.TickDelta()` and collects AOI events from a single worker.
- `internal/game/server/runtime.go:553` through
  `internal/game/server/runtime.go:567` seeds the world with one visible NPC
  and one hidden planet signal in the single worker.
- `internal/game/server/runtime.go:571` through
  `internal/game/server/runtime.go:621` spawns or attaches a player into the
  single worker at `world.Vec2{}`.
- `internal/game/server/runtime.go:640` through
  `internal/game/server/runtime.go:651` derives safe hangar area from distance
  to origin instead of map-defined safe zones.
- `internal/game/server/runtime.go:760` through
  `internal/game/server/runtime.go:797` bootstraps a session snapshot from the
  single world snapshot path.
- `internal/game/server/runtime.go:819` through
  `internal/game/server/runtime.go:843` emits movement correction and stop
  events from the single worker.
- `internal/game/server/runtime.go:943` through
  `internal/game/server/runtime.go:1015` builds one AOI snapshot by querying
  `runtime.Worker.EntitiesWithinWindow`.
- `internal/game/server/runtime.go:1029` through
  `internal/game/server/runtime.go:1041` ticks the one worker and diffs AOI for
  all sessions.
- `internal/game/server/runtime.go:1177` through
  `internal/game/server/runtime.go:1225` projects remembered planet intel
  without a bounded map id in the memory payload.
- `internal/game/server/handlers.go:209` through
  `internal/game/server/handlers.go:260` handles `move_to` and `stop` by
  submitting to `runtime.Worker`.
- `internal/game/server/handlers.go:265` through
  `internal/game/server/handlers.go:285` validates movement without checking
  map bounds or active map ownership.

Reuse these parts:

- `internal/game/world/worker/worker.go:46` through
  `internal/game/world/worker/worker.go:55` already accepts world/zone ids per
  worker, so map instances can each own a worker with `ZoneID == MapID`.
- `internal/game/world/worker/worker.go:57` through
  `internal/game/world/worker/worker.go:118` already stores entities,
  player-entity links, session-player links, spatial index, and map-local
  scheduled tasks inside one worker.
- `internal/game/world/worker/worker.go:210` through
  `internal/game/world/worker/worker.go:230` drains commands, advances
  movement, and runs scheduled tasks deterministically.
- `internal/game/world/worker/worker.go:313` through
  `internal/game/world/worker/worker.go:384` provides deterministic snapshots
  and spatial queries that can be scoped per map instance.
- `internal/game/world/worker/worker.go:585` through
  `internal/game/world/worker/worker.go:607` keeps movement mutation inside the
  owning worker.
- `internal/game/world/worker/worker.go:694` through
  `internal/game/world/worker/worker.go:724` advances movement from
  server-owned speed and tick delta.
- `internal/game/world/worker/worker.go:727` through
  `internal/game/world/worker/worker.go:738` rejects entities owned by a
  different world/zone.
- `internal/game/server/runtime.go:1039` through
  `internal/game/server/runtime.go:1059` already keeps AOI diffs per
  authenticated session.
- `internal/game/server/handlers.go:372` through
  `internal/game/server/handlers.go:404` provides recursive rejection of
  server-owned payload fields.

## Target Model

### Runtime Shape

The runtime should own a catalog, a router, and a set of map instances:

```text
Runtime
  MapCatalog
  MapRouter
  MapInstances[internal_map_id]MapInstance
  PlayerLocations[player_id]PlayerMapLocation
  SessionLocations[session_id]internal_map_id
```

`Runtime.Worker` should stop being the route for gameplay commands. During
migration it may remain as a compatibility field for tests, but all real
handlers should call the router.

### Map Instance Shape

```text
MapInstance
  Definition MapDefinition
  Worker *worker.Worker
  ActiveSessions map[session_id]player_id
  LastAOI map[session_id]aoi.Snapshot
  HiddenEntities map[entity_id]bool
  HiddenPlayers map[player_id]bool
  StealthWitnesses map[witness_key]expires_at
  EnemyPoolRuntime
  LootRuntime
  PlanetScanRuntime
  TickDelta
```

Each map instance owns only entities and scheduled tasks for that map. Player
state that is global, such as account, wallet, inventory, hangar, loadout,
progression, quest state, and premium entitlements, stays on the parent runtime
or domain services. Position, AOI, local combat presence, local NPCs, local loot,
and local scan visibility are map-instance scoped.

### Command Routing

Every live operation follows this pattern:

```text
resolved_session = auth.ResolveSession(session_id)
location = MapRouter.ActiveLocation(resolved_session.player_id)
instance = Runtime.MapInstances[location.map_id]
handler validates payload as intent only
handler validates map bounds/range/visibility/safety with instance definition
handler submits command to instance.Worker or domain service
handler emits events only to sessions attached to that instance
```

Operations that must route by active map include:

- `world.snapshot`
- `move_to`
- `stop`
- `combat.use_skill`
- `loot.pickup`
- `scan.pulse`
- `planet.detail` for proximity or live scan information
- `planet.claim`
- `production` proximity actions, if any
- stealth toggle and reveal checks
- debug spawn/snapshot commands in dev mode

Queries for global systems such as wallet, inventory, market, auction,
progression, and premium can remain global, but any proximity-sensitive result
must ask the active map instance for location and safety context.

### Ticking And Event Emission

`StartWithEventSink` should tick all active map instances. It may use one
coordinating ticker initially, or one goroutine per map instance later. The
important behavior is isolation:

```text
for each active map instance:
  instance.Worker.Tick()
  for each session in instance.ActiveSessions:
    build AOI snapshot from that instance only
    diff against instance.LastAOI[session]
    emit events to that session only
```

No session in map A should receive worker ticks, entity ids, hidden flags,
minimap contacts, scanner signals, or NPC state from map B.

### Portal Transfer Runtime

Portal transfer is a server-owned state transition:

```text
1. Resolve player active source map.
2. Validate portal exists in source MapDefinition.
3. Settle source movement.
4. Validate player is within portal interaction radius.
5. Validate combat, cooldown, rank/faction, safe-zone, and event restrictions.
6. Remove or detach player entity and sessions from source instance.
7. Update PlayerMapLocation to destination map and spawn position.
8. Insert or attach the player's entity in destination instance.
9. Attach all active sessions to destination instance.
10. Emit `map.transfer_completed` and destination `world.snapshot`.
```

The transfer must be atomic from the player's perspective: a player cannot be
visible in both maps, and a failed transfer must leave the player in the source
map with source movement settled.

### Radar, Stealth, And Visibility

The existing visibility/AOI pipeline should stay, but its inputs become
map-local:

- viewer map id is the active map id
- viewer position is the server-owned map-local position
- radar range comes from effective stats plus map modifiers
- stealth witnesses are scoped to the current map
- map enemy/loot/planet signals are considered only inside the same map

The fog-of-war wave does not drive live visibility. Known planet intel may still
appear as server-approved memory, but live contacts require current map plus
radar range.

### Safe And PvP Runtime Rules

Replace origin-radius checks with map-definition classifiers. A map instance
should answer:

```text
ClassifyPosition(position) -> safe_zone, pvp_zone, contested, station_actions
CanAttack(attacker, target)
CanLoot(player, loot)
CanUsePortal(player, portal)
CanUseHangarOrRepair(player)
```

Combat and proximity services must call these checks server-side. UI labels are
not enforcement.

### Per-Map Enemy Pools

The current `seedWorld` starter NPC becomes map catalog/runtime seeding. Each
map definition references enemy pools, spawn areas, caps, and respawn cadence.
The runtime instance materializes only the NPCs that belong to that map.

The client may receive visible hostile entities after AOI filtering, but must
not receive pool weights, future spawn candidates, spawn timers, or spawn seeds.

### Rare Planet Scan, Claim, And Production

Planet gameplay moves from infinite coordinate assumptions to map-local rare
content:

- rare planet candidates are scoped by `WorldID + MapID + local coordinates`
- scan pulses resolve active map from the player, not from client payload
- discovery materializes persistent planet records with map id and bounded
  coordinates
- claim checks current map, proximity, rank, X Core cost, safe/PvP rules, and
  idempotency
- production and planet storage queries must verify player ownership and map
  scope before showing live/proximity information

Known planet intel can remain personal, but browser memory payloads need a
public map key so the client can place remembered planets only on the correct
map. Server storage keeps the internal map id.

## Data Structures/Contracts To Add Or Change

### Runtime Containers

```text
MapInstanceID = WorldID + MapID

Runtime
  mapCatalog MapCatalog
  mapRouter *MapRouter
  mapInstances map[MapID]*MapInstance
  playerLocations map[PlayerID]PlayerMapLocation
  sessionLocations map[SessionID]MapID
```

### Player Location

```text
PlayerMapLocation
  player_id
  world_id
  internal_map_id
  zone_id
  entity_id
  position
  movement_state
  last_transition_at
  transition_cooldown_until
```

Position and movement are copied from the owning worker when snapshots are
created or transitions settle. The router remains the source for which map owns
the player.

### Map Instance

```text
MapInstance
  definition MapDefinition
  worker *worker.Worker
  active_sessions map[auth.SessionID]foundation.PlayerID
  last_aoi map[auth.SessionID]aoi.Snapshot
  hidden_entities map[world.EntityID]bool
  hidden_players map[foundation.PlayerID]bool
  stealth_witnesses map[hiddenPlayerWitnessKey]time.Time
  npc_spawner MapEnemyPoolRuntime
  loot_runtime MapLootRuntime
  planet_runtime MapPlanetRuntime
```

### Snapshot Payload

Extend `world.snapshot` to be map-aware:

```text
world.snapshot
  map
    public_map_key
    display_name
    bounds
    pvp_mode
    safe_status
    visual_theme_key
  sector/status
  player_position
  entities
  minimap
  snapshot_cursor
```

`snapshot_cursor` should either remain session-local or include a map-safe
cursor prefix. It must not expose other map worker state.

### Event Payloads

```text
map.transfer_completed
  from_public_map_key
  to_public_map_key
  position
  map_subscription_epoch
  reason
  snapshot_cursor

aoi.entity_entered / updated / left
  emitted only for active destination map after transfer

position.corrected
  emitted from active map worker only
```

### Handler Changes

Handlers should no longer call `runtime.Worker` directly. Use:

```text
instance, location = runtime.MapRouter.RequireActiveInstance(ctx.PlayerID)
```

Then validate:

- payload contains no trusted server-owned fields
- target is finite
- target is inside `location.map_id` bounds
- movement distance/rate is valid
- ship can move
- safe/PvP/portal/radar rules allow the action

## Implementation Tasks In Order

1. Introduce `MapInstance` construction from Phase 01 `MapDefinition`.
2. Create runtime-owned `mapInstances` and `playerLocations` maps.
3. Build all configured map instances during `NewRuntime`, with each worker
   configured as `WorldID = runtime world` and `ZoneID = MapID`.
4. Replace `seedWorld` with per-map seed/materialization from map definitions:
   starter NPCs, portal entities, hidden planet signals, enemy pools, and safe
   station areas.
5. Update session bootstrap so new players spawn at the starter map spawn point
   and reconnecting players attach to their existing active map instance.
6. Move session attach/detach bookkeeping from global single-worker state to
   map-instance membership.
7. Update `runtimeSessionResolver` so command context uses the player's active
   `WorldID` and `ZoneID`, where `ZoneID == MapID` initially.
8. Update `world.snapshot`, minimap, AOI snapshot, movement correction, and
   post-command AOI diff paths to resolve the active map instance first.
9. Replace fixed projection constants with authoritative effective radar range
   when building map AOI. Multi-map snapshots are not complete while the old
   fixed runtime projection still decides visibility.
10. Update `move_to` and `stop` to submit to the active map worker.
11. Add bounded movement validation using the active map definition.
12. Replace origin-radius hangar safety checks with map safe-zone
    classification.
13. Implement portal transfer as a single runtime transition across source and
    destination instances.
14. Scope stealth, radar witnesses, hidden entities, and scan reveals to the map
    instance.
15. Route scan pulse, known planet minimap memory, planet detail, claim, and
    production proximity checks through active map context.
16. Tick all active map instances in `StartWithEventSink` and emit events only
    to sessions attached to each instance.
17. Keep dev/debug commands map-aware. Debug spawn should spawn into the
    caller's active map unless an explicit server-side admin target is added.

## Tests To Add/Update

- Runtime constructs one worker per map definition with `ZoneID == MapID`.
- Startup fails if any map instance cannot be constructed or has invalid bounds.
- New authenticated session spawns in the configured starter map at an in-bounds
  spawn point.
- Reconnect attaches to the player's existing active map instance without
  duplicating the player entity.
- `world.snapshot` returns only entities from the active map.
- `world.snapshot` AOI range comes from authoritative effective radar stats,
  not the old fixed projection constant.
- Two players at identical local coordinates in different maps do not receive
  each other's AOI events.
- `move_to` updates only the active map worker.
- `move_to` rejects out-of-bounds targets and leaves source state unchanged.
- `stop` settles only the active map worker entity.
- Worker tick loop emits AOI diffs only to sessions attached to that map.
- Portal transfer removes the player from the source instance and inserts or
  attaches the player in the destination instance at the server-owned spawn.
- Failed portal transfer leaves the player in the source instance.
- Multi-tab sessions for one player transfer together.
- Safe-zone combat is rejected server-side.
- PvP-zone combat is allowed only where the active map definition permits it.
- Stealthed players are hidden only according to same-map radar/witness rules.
- Scan pulse cannot discover or reveal planets outside the active map.
- Known planet minimap memory includes map id and appears only on the matching
  map.
- Enemy pool seeding never serializes spawn weights, future spawns, or seeds to
  clients.
- Debug spawn/snapshot commands are disabled outside dev mode and map-aware in
  dev mode.

## Migration/Doc Updates

This planning/documentation change does not update files outside
`docs/map-rework`. When implementation begins, update:

- `docs/2026-06-17-world-system-design.md` to mark the infinite coordinate
  plane and fog wave as superseded by bounded multi-map gameplay.
- `docs/plans/ui-implementation/04-live-world-aoi-movement.md` to describe
  map-local AOI, bounded movement, portal transitions, and no fog-wave
  visibility.
- `docs/plans/modules/14-world-aoi-fog-security.md` to rename or split fog
  security into map/radar/stealth security.
- Discovery/scanner and planet/production module specs so planet records,
  scan candidates, claims, and known intel include map scope.
- Combat/loot specs so safe/PvP map classifiers are part of validation.
- Realtime protocol docs for `map.transfer_started/completed/failed`, map-aware
  `world.snapshot`, and map-scoped event delivery.
- Client docs so the browser treats `public_map_key` as display/server echo
  only, not as a trusted command input. Internal `map_id` remains server-only.

## Risks And Acceptance Criteria

Risks:

- Direct references to `runtime.Worker` can survive in handlers and silently
  bypass map routing.
- Player transfer can duplicate entities if source removal and destination
  insertion are not treated as one state transition.
- Global maps such as `lastAOI`, `hiddenPlayers`, and stealth witnesses can
  leak behavior across maps unless moved into `MapInstance` or keyed by map.
- Existing scanner/discovery code may still assume infinite coordinates and
  discovery horizon unless map scope is added to every candidate and planet
  lookup.
- Tick scheduling can become noisy if every configured map runs forever. The
  first implementation can tick active maps only, but must define activation and
  shutdown behavior clearly.
- Client-visible map ids are necessary for UI, but any handler that trusts them
  reopens cross-map exploits.

Acceptance criteria:

- Runtime owns multiple map instances created from server-owned
  `MapDefinition` data.
- No gameplay handler routes through a single global `Runtime.Worker`.
- New and reconnecting players attach to the correct active map instance.
- Movement, stop, combat, loot, scan, stealth, planet, and proximity operations
  resolve active map server-side before validation.
- Coordinates are finite and inside `0..10000` for the active map before worker
  mutation.
- AOI snapshots and events are generated from only the active map instance.
- Portal transitions atomically move player entity/session ownership between
  map instances.
- Safe/PvP classification is enforced server-side.
- Radar/stealth visibility stays current-map plus radar range.
- Rare planet discovery, claim, and production contracts are map-scoped.
- Tests cover multi-map isolation, portal transfer, bounds, reconnect,
  safe/PvP rules, scanner scope, and socket event isolation.

## Progress Notes

2026-06-21 local TASK-0259 server slice:

- Implemented runtime-owned map instances for every configured map definition,
  with each worker using `ZoneID == internal_map_id`.
- Moved live session membership, last AOI cursors, hidden entities, hidden
  players, and scanner witness state into `MapInstance` runtime state.
- Updated session bootstrap/reconnect/detach to attach sessions to the active
  map instance, detach them from stale instances, and clear stale AOI cursors.
- Updated `StartWithEventSink`/tick collection to tick map instances and emit
  AOI diffs only to sessions attached to the instance being diffed.
- Updated `world.snapshot`, post-command AOI diffs, movement, stop, combat,
  loot, scanner reveal, and debug spawn paths to resolve or use the active map
  instance.
- Replaced fixed AOI projection-window snapshots with server-owned effective
  radar range from runtime player stats, with a conservative `defaultRadarRange`
  fallback for bootstrap/test paths before stats are materialized.
- Added map safe-zone definitions and switched hangar safety classification from
  origin-radius distance to map-definition safe zones.
- Added focused tests for per-map worker construction, cross-map AOI isolation,
  reconnect/session AOI scoping, active-map snapshots, active-map movement/stop,
  out-of-bounds rejection, and safe-zone classification.

Intentionally deferred:

- Public `portal.enter` protocol and atomic portal transfer events remain Phase
  03 work; this slice did not add a partial client operation.
- Full safe/PvP combat policy enforcement remains Phase 04.
- Advanced radar/stealth gameplay and bounded scanner tuning remain later Phase
  05/06 work.

2026-06-21 local TASK-0262 review-blocker fix:

- Scoped scanner materialization keys, materialized planet records, and
  personal planet intel memory by authoritative world/zone map context so the
  same local scan cell in different runtime maps no longer collides.
- Filtered known-planet query payloads and remembered minimap memory to the
  authenticated player's active map only, while exposing only the client-safe
  `public_map_key`.
- Replaced the hidden-player scan reveal projection gate with the active-map
  effective radar/scan range rule, so same-map targets inside authoritative
  range can be revealed beyond the old 1000-unit window and out-of-range
  targets are not revealed.

2026-06-21 local TASK-0265 production/storage scope fix:

- Filtered planet production and storage summary payloads through the
  server-owned active map location before exposing owned live production/storage
  data.
- Added focused server regression coverage for owned planets in `map_1_1` and
  `map_1_2`, including explicit `planet_id` requests outside the active map.
