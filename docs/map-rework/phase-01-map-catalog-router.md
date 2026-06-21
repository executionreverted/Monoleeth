# Phase 01: Map Catalog And Router

## Goal

Replace the old infinite coordinate-plane routing assumptions with a
server-owned bounded map catalog and a map-aware runtime router.

This phase defines the contracts and implementation order for the first server
slice of the map rework:

- every gameplay map is bounded to local coordinates `0..10000` on both axes
- `WorldID` remains the shard/universe id
- `ZoneID` may initially equal the server-only internal map id
- `MapDefinition` is server-owned catalog data
- runtime commands route by the player's active server-owned map
- clients never provide trusted `map_id`, `internal_map_id`, `public_map_key`,
  `player_id`, position, speed, cooldown, discovery, loot, damage, or economy
  truth

The visibility rule stays simple and strict: a player can see or action only
entities in the player's current map and radar range, after server-side stealth,
safe-zone, PvP, and interaction validation. The fog-of-war wave model is
removed.

## Current State To Replace/Reuse

Replace these assumptions:

- `docs/2026-06-17-world-system-design.md:23` says the world has no hard
  border, and `docs/2026-06-17-world-system-design.md:36` defines the universe
  as an infinite coordinate plane. The new model uses many bounded maps instead.
- `docs/2026-06-17-world-system-design.md:45` through
  `docs/2026-06-17-world-system-design.md:52` defines "Map" as a UI view into
  fog and remembered coordinates. The new server model needs a concrete
  `MapDefinition` catalog entry for each gameplay map.
- `docs/2026-06-17-world-system-design.md:27` and
  `docs/plans/ui-implementation/04-live-world-aoi-movement.md:84` preserve
  fog/intel language. Keep client-safe known intel where needed, but remove the
  expanding fog wave as a live visibility system.
- `internal/game/server/runtime.go:71` through
  `internal/game/server/runtime.go:80` configures one `WorldID` and one
  `ZoneID`.
- `internal/game/server/runtime.go:83` through
  `internal/game/server/runtime.go:95` stores a single `Worker`, `worldID`, and
  `zoneID` on `Runtime`.
- `internal/game/server/runtime.go:915` through
  `internal/game/server/runtime.go:927` builds `world.snapshot` from the single
  runtime world/zone instead of resolving a player map first.
- `internal/game/server/runtime.go:943` through
  `internal/game/server/runtime.go:978` creates the AOI viewer with
  `runtime.worldID` and `runtime.zoneID`, then queries the single worker window.
- `internal/game/server/runtime.go:1156` through
  `internal/game/server/runtime.go:1163` returns a hard-coded "Origin Fringe"
  sector projection.
- `internal/game/server/runtime.go:1367` through
  `internal/game/server/runtime.go:1384` resolves every authenticated socket
  to the runtime's single world/zone.
- `internal/game/server/handlers.go:209` through
  `internal/game/server/handlers.go:237` sends movement directly to
  `runtime.Worker`.
- `internal/game/server/handlers.go:265` through
  `internal/game/server/handlers.go:285` validates movement distance from the
  single worker position, but does not enforce map bounds.

Reuse these parts:

- `internal/game/server/handlers.go:20` through
  `internal/game/server/handlers.go:96` already rejects many client-supplied
  server-owned fields, including `world_id`, `zone_id`, `speed`, `damage`,
  hidden data, scan candidates, seeds, loot tables, and credentials. Add
  `map_id` to the same posture when code is changed.
- `internal/game/world/types.go:64` through
  `internal/game/world/types.go:75` provides finite coordinate validation.
  Extend the validation path with map bounds instead of replacing `Vec2`.
- `internal/game/world/types.go:100` through
  `internal/game/world/types.go:123` keeps `Entity` carrying `WorldID` and
  `ZoneID`; that shape can represent map-owned workers when `ZoneID == MapID`.
- `internal/game/world/types.go:207` through
  `internal/game/world/types.go:224` keeps movement intents limited to a target
  coordinate. Continue to treat movement as intent only.
- `internal/game/world/worker/worker.go:46` through
  `internal/game/world/worker/worker.go:55` already configures a worker with
  `WorldID` and `ZoneID`.
- `internal/game/world/worker/worker.go:727` through
  `internal/game/world/worker/worker.go:738` enforces that a worker only owns
  entities for its world/zone. This becomes useful map isolation once
  `ZoneID == MapID`.
- `internal/game/world/worker/worker.go:331` through
  `internal/game/world/worker/worker.go:384` has radius/window queries that can
  stay as the first AOI filter for a bounded map.
- `internal/game/server/runtime.go:1029` through
  `internal/game/server/runtime.go:1059` already emits per-session AOI diffs
  after visibility filtering. The router should preserve that behavior per map.

## Target Model

### Vocabulary

- `WorldID`: the shard/universe. It does not identify a playable map.
- `internal_map_id`: the server-only gameplay map id, for example `map_1_1`.
- `public_map_key`: the client-safe map key, for example `1-1`.
- `ZoneID`: the worker ownership id. Initially set equal to `internal_map_id`
  to avoid a second partitioning scheme.
- `MapDefinition`: server-owned catalog data that defines one `10000x10000`
  map, its portals, safe/PvP areas, enemy pools, visual metadata, radar rules,
  and rare planet scan policy.
- `PlayerMapLocation`: server-owned active location for a player:
  `WorldID`, `internal_map_id`, `ZoneID`, local `Position`, movement state
  reference, and optional transition metadata.
- `MapRouter`: runtime service that resolves a player's active map before any
  command, query, snapshot, or event emission touches worker state.

### Map Bounds

All normal map-local coordinates use:

```text
min_x = 0
min_y = 0
max_x = 10000
max_y = 10000
```

The server must reject non-finite or out-of-bounds movement targets. The client
may render the bounds and may receive the current map id/name from snapshots,
but the client must never be trusted when it sends a map id back.

### Visibility And Actions

For all live gameplay commands:

```text
active_map = router.ActiveMapForPlayer(session.player_id)
worker = runtime.MapInstance(active_map).Worker
viewer = active_map + server-owned player position + server-owned radar stats
visible_entities = entities in active_map and radar range after stealth rules
```

An entity in another map is not visible, targetable, lootable, scannable,
repairable through proximity, or attackable even if its coordinates would fit in
the same local `0..10000` window.

### Portal Travel

Portals are server-owned map catalog entries. A client sends a portal-use
intent by portal id or by interacting with a visible portal entity. The server
resolves the player's active map, checks the portal belongs to that map, checks
range and state, then moves the player to the destination map spawn point.

The client does not choose:

- destination map
- destination position
- destination worker
- transition cooldown
- safe-zone immunity
- combat escape outcome

### Safe And PvP Zones

Safe/PvP status is derived from the active `MapDefinition` and server-owned
position, not from client labels. A map can contain:

- global map mode, such as safe, PvE, PvP, contested, or event
- local safe zones, such as station/hangar circles
- local PvP zones or danger volumes
- portal arrival protection windows

Combat, loot pickup, stealth reveal, portal use, hangar/repair, and market or
station actions must consult the map classifier where relevant.

### Per-Map Socket Information Isolation

Sockets are authenticated globally, but live world subscriptions are map-local.
The gateway should route each session to the player's current map instance and
emit only that map's client-safe snapshots/events. A session changing maps
receives a server-authored map transition event and a full replacement
`world.snapshot` for the destination map.

No event envelope should leak another map's worker tick, hidden entities, enemy
pool state, planet candidates, procedural seeds, or future spawn choices.

## Data Structures/Contracts To Add Or Change

### Server Catalog

Add a server-owned catalog shape similar to:

```text
MapDefinition
  internal_map_id
  public_map_key
  world_id
  zone_id
  display_name
  bounds = { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 }
  map_tier
  faction_or_region
  rules = { pvp_mode, safe_default, radar_modifier, stealth_modifier }
  safe_zones[]
  pvp_zones[]
  spawn_points[]
  portals[]
  enemy_pool_refs[]
  loot_pool_refs[]
  planet_scan_policy
  visual_theme_key
```

Only client-safe fields should be serialized to the browser:

```text
ClientMapProjection
  public_map_key
  display_name
  bounds
  pvp_mode
  safe_zone_summaries
  visible_portals
  visual_theme_key
```

Do not serialize enemy pool internals, spawn weights, scan seeds, hidden planet
candidates, or destination internals for undiscovered/special portals.

### Portal Definition

```text
PortalDefinition
  portal_id
  source_map_id
  source_position
  interaction_radius
  destination_map_id
  destination_spawn_id
  required_rank_or_faction
  cooldown
  combat_restrictions
  visibility_policy
```

Portal visibility is still server-filtered. A hidden, locked, or event-only
portal should not become actionable through a guessed id.

### Router Contracts

```text
MapCatalog.Get(internal_map_id) -> MapDefinition
MapCatalog.ByPublicKey(public_map_key) -> MapDefinition
MapCatalog.StarterMap(account/player) -> internal_map_id + spawn_id
MapCatalog.Portal(source_map_id, portal_id) -> PortalDefinition
MapCatalog.Classify(internal_map_id, position) -> map safety/pvp/radar modifiers
MapCatalog.ValidatePosition(internal_map_id, position) -> bounds result

MapRouter.ActiveLocation(player_id) -> PlayerMapLocation
MapRouter.RequireActiveInstance(player_id) -> MapInstance
MapRouter.RouteCommand(session, op) -> MapInstance + PlayerMapLocation
MapRouter.TransferThroughPortal(player_id, portal_id) -> transition result
```

The router owns current map truth. Handlers may accept a public map key only as
an optional optimistic/debug echo, and must ignore or reject it for gameplay.

### Payload Changes

`world.snapshot` should include a client-safe map projection:

```text
world.snapshot
  map
  sector/status
  player_position
  entities
  minimap
  snapshot_cursor
```

Movement response should remain intent-oriented:

```text
move_to response
  accepted
  public_map_key // server-authored current map label
  entities
  minimap
  correction
```

Add map transfer events when portal travel lands:

```text
map.transfer_started
map.transfer_completed
map.transfer_failed
  from_public_map_key
  to_public_map_key
  position
  reason = "portal"
  snapshot_cursor
```

The event is server-authored. Client payloads for movement, combat, loot, scan,
claim, repair, and portal use must not contain trusted map, player, position,
speed, cooldown, damage, discovery, loot, or economy fields.

## Implementation Tasks In Order

1. Add a map catalog package or module with `MapDefinition`, bounded map
   validation, portal validation, and client-safe projection helpers.
2. Seed the first DarkOrbit-style starter map set as catalog data, with every
   map bounded to `0..10000`.
3. Define `internal_map_id`, `public_map_key`, and the initial
   `ZoneID == internal_map_id` mapping. Keep `WorldID` as the shard/universe id.
4. Add `PlayerMapLocation` storage in runtime memory first. Document the later
   persistence boundary before production.
5. Change session bootstrap design so a new player spawns in
   `MapCatalog.StarterMap`, not at infinite-world origin.
6. Add a `MapRouter` design that resolves active map and active worker before
   `world.snapshot`, `move_to`, `stop`, combat, loot, scan, planet, and
   proximity-based operations.
7. Add map-bounds validation to movement and server entity insertion paths.
   Reject out-of-bounds targets rather than clamping silently.
8. Add portal-use command/query contracts. The client sends intent; the server
   validates current map, range, restrictions, cooldowns, and destination.
9. Replace hard-coded sector projection with a map projection derived from
   `MapDefinition`.
10. Ensure payload rejection includes `map_id` and nested `map` fields where a
    client might try to submit server-owned map truth.
11. Define per-map event subscription behavior for the gateway. A session should
    receive events only for the player's active map instance.
12. Document how safe/PvP classification is used by combat, hangar, repair,
    portal travel, and station-like actions.

## Tests To Add/Update

- Catalog rejects any map definition whose bounds are not exactly `0..10000` on
  both axes.
- Catalog rejects portals whose source or destination spawn is outside bounds.
- Catalog rejects duplicate map ids, duplicate portal ids per source map, and
  missing destination maps.
- Starter spawn returns a server-owned map id and in-bounds spawn position.
- `move_to` rejects `x < 0`, `y < 0`, `x > 10000`, and `y > 10000`.
- `move_to` rejects any payload containing `map_id`, `player_id`, speed, or
  other server-owned truth.
- `world.snapshot` uses the player's active map, not a runtime global worker.
- Two players at the same local coordinates in different maps cannot see,
  attack, scan, loot, or target each other.
- Portal use succeeds only from the current source map, within interaction
  range, and with server-approved destination rules.
- Portal id guessing fails if the portal is not visible or not present in the
  player's active map.
- Safe-zone classification prevents combat where configured.
- PvP-zone classification permits PvP only where configured.
- Per-map socket/AOI events do not leak entities from other map instances.
- Client-safe map projection excludes enemy pool internals, hidden portals,
  hidden planet candidates, seeds, and worker topology.

## Migration/Doc Updates

This planning/documentation change does not update these files outside
`docs/map-rework`, but the implementation branch should update them when code
changes land:

- Replace or supersede the infinite-world assumptions in
  `docs/2026-06-17-world-system-design.md`.
- Update `docs/plans/ui-implementation/04-live-world-aoi-movement.md` so Phase
  04 names bounded maps, portals, and no fog wave.
- Update relevant module specs under `docs/plans/modules/`, especially world
  AOI/security, discovery/scanner, combat/loot, and production/planet specs.
- Update protocol/event documentation for `world.snapshot`, movement responses,
  and `map.transfer_started/completed/failed`.
- Update UI docs so minimap and world canvas render current-map bounds and
  server-authored map metadata only.

## Risks And Acceptance Criteria

Risks:

- Carrying both "zone" and "map" vocabulary can blur ownership. Until there are
  sub-map zones, keep the rule explicit: `ZoneID == internal_map_id`.
- Leaking internal `map_id` could tempt future handlers or the client to treat
  it as command truth. Handlers must resolve from session/player state every
  time.
- Portal transitions can duplicate or strand player entities if source and
  destination workers are not updated as one state transition.
- Existing scanner/planet code is coordinate-plane oriented and can leak old
  assumptions unless every query is scoped by active map.
- Safe-zone and PvP rules can become UI labels only unless combat and proximity
  services enforce them server-side.

Acceptance criteria:

- A server-owned `MapDefinition` model is documented for every bounded map.
- Every map uses local `0..10000` coordinate bounds.
- `WorldID` remains the shard/universe id, and initial `ZoneID` mapping to
  `internal_map_id` is documented.
- Runtime routing is defined by active player map, not by one
  `Runtime.Worker`.
- Visibility/action rules are scoped to current map plus radar range.
- The fog-of-war wave is explicitly removed from live map visibility.
- Client trust boundaries explicitly reject trusted map, position, combat,
  discovery, loot, cooldown, and economy fields.
- Test coverage is specified for catalog validation, routing, bounds, portal
  travel, safe/PvP zones, and per-map socket isolation.
