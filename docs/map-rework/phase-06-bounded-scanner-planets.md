# Phase 06: Bounded Scanner And Planets

## Goal

Rework scanner-driven planet discovery from the old infinite coordinate-plane
model into bounded, per-map discovery. A scan only operates inside the player's
active `10000x10000` map, uses server-owned map profile rules, and never reveals
hidden candidates, procedural seeds, future planet rolls, or other-map data.

Preserve the useful existing trust boundary: players can see and act only on
entities in their current map and radar range, planet candidate generation stays
server-only, and discovered coordinates/intel are server-approved personal or
explicitly shared records.

## Current State To Replace/Reuse

- Replace the infinite-world assumptions in
  `docs/2026-06-17-world-system-design.md`: the universe has "no hard border",
  maps are a window into an infinite plane, planet level scales from origin
  distance, and a discovery horizon gates expansion.
- Replace fog-wave/fog-memory wording in
  `docs/2026-06-17-world-system-design.md` with same-map membership, radar
  range, stealth/jammer counterplay, and server-approved intel.
- Reuse the scanner trust boundary in
  `internal/game/discovery/scanner_types.go:59`,
  `internal/game/discovery/scanner.go:38`, and
  `internal/game/discovery/scanner.go:178`: start/resolve inputs already carry
  server-resolved player, world, zone, ship, pulse reference, position, stats,
  cooldown, and energy checks.
- Reuse hidden candidate safety in
  `internal/game/discovery/candidate.go:51` and
  `internal/game/discovery/candidate.go:126`: `PlanetCandidate` is hidden
  server truth and direct JSON serialization fails closed.
- Replace candidate horizon/origin logic in
  `internal/game/discovery/candidate.go:41`,
  `internal/game/discovery/candidate.go:73`,
  `internal/game/discovery/candidate.go:283`, and
  `internal/game/discovery/candidate.go:332`: `DiscoveryHorizon`,
  distance-from-origin level scaling, and origin-style distance labels do not
  fit bounded maps.
- Reuse materialization and intel primitives in
  `internal/game/discovery/store.go:14`,
  `internal/game/discovery/store.go:67`,
  `internal/game/discovery/store.go:158`,
  `internal/game/discovery/planet.go:62`, and
  `internal/game/discovery/intel.go:48`: discovered planets become persistent
  records and player intel stays personal.
- Replace single-runtime map assumptions in
  `internal/game/server/discovery_production_handlers.go:133`,
  `internal/game/server/discovery_production_handlers.go:189`, and
  `internal/game/server/discovery_production_handlers.go:204`: handlers
  currently use `runtime.worldID` / `runtime.zoneID` and do not consult a map
  router or map profile.
- Reuse scanner provider boundaries in
  `internal/game/server/scanner_providers.go`: runtime-owned module, stat,
  position, cooldown, energy, and stealth-reveal providers are the right
  integration points for map-aware checks.
- Keep the Phase 07 UI read model expectations in
  `docs/plans/ui-implementation/07-discovery-planets-production-routes.md`:
  scan, known planets, and planet detail must be server-backed and must not
  invent fake planets or coordinates.

## Target Model

Each playable map is a bounded `10000x10000` coordinate space with a server-owned
map profile. `WorldID` remains the shard/universe id. `ZoneID` may initially be
used as `MapID` until the map catalog phase introduces a distinct type.

Scanner discovery is local to the active map:

```text
client intent: scan.pulse
server resolves: session -> player -> active map -> map worker -> active ship
server validates: same map, in bounds, scanner equipped, stationary or slow scan,
                  energy/capacitor, cooldown, radar tier, scanner power
server rolls: hidden live-player reveal first, then bounded planet candidates
server emits: safe scan status, optional safe signal, optional planet id/intel
```

Planet candidate generation is still deterministic and server-only, but the
inputs are bounded by map profile rather than infinite horizon:

```text
candidate_key = hash(world_seed, map_id, profile_version, cell_x, cell_y, index)
candidate_position = map-local position inside 0..10000
candidate_level = map_profile level band + biome/type/rarity modifiers
candidate_density = map_profile density and spawn budget
candidate_detection = radar tier + scanner power + distance + map interference
```

Planet discovery should stay rare. Rarity is controlled through all of these,
not through one knob:

- scan cooldown and energy cost
- low candidate density per map profile
- radar tier minimums
- scanner power and scan radius
- map-specific interference, safe/PvP risk, and rare-profile weights
- deterministic detection rolls that do not expose roll data to the client

Initial tuning targets:

| Profile | Cooldown | Candidate density | Eligible pulse success | Expected feel |
| --- | ---: | ---: | ---: | --- |
| Dev/test | 5-15s | deterministic fixture | forced or high | reliable automation |
| Starter safe map | 10-15m | 1-3 candidates per map | 5-12% | rare, memorable first discovery |
| Normal PvE map | 8-12m | 2-5 candidates per map | 8-16% | steady exploration value |
| PvP/high-risk map | 6-10m | 3-7 candidates per map | 10-20% | higher reward for higher risk |
| Event map | event-configured | event-configured | event-configured | live-ops controlled |

These numbers are starting targets, not balance law. Production tuning must keep
test/dev overrides explicit so browser smoke can force deterministic discovery
without making real play too generous.

The playable map surface has no fog-of-war wave. Visibility is same-map plus
radar/stealth rules. Personal/shared planet intel remains valid strategic
memory, but it is not live visibility. A player may have known planets in other
maps; live actions such as scanning, claiming, and interacting must re-check the
active map and range.

Per-map worker and socket isolation is mandatory. A session connected to one map
must not receive hidden candidates, scan events, AOI diffs, minimap contacts, or
other players' stealth reveal outcomes from another map.

## Data Structures/Contracts To Add Or Change

- Add or reuse a canonical map identifier:
  - MVP option: `MapID` is represented by `foundation.ZoneID`.
  - Later option: add `foundation.MapID` and keep `ZoneID` for future
    sub-partitions inside a map.
- Add a map catalog/profile contract from earlier phases:
  - `map_id`
  - `width = 10000`
  - `height = 10000`
  - `scan_cell_size`
  - `candidate_density`
  - `spawn_budget`
  - `allowed_planet_biomes`
  - `planet_type_weights`
  - `rarity_weights`
  - `level_min` / `level_max`
  - `min_radar_bonus`
  - `scanner_interference`
  - `safe_or_pvp_policy`
  - `profile_version`
- Replace `CandidateGenerationOptions.DiscoveryHorizon` with map-bounded
  generation options:
  - `MapID`
  - `MapBounds`
  - `ProfileVersion`
  - `Density`
  - `RarityWeights`
  - `LevelBand`
  - `ScannerInterference`
  - `ScanCellSize`
  - `ChunkSize` only if still needed for caching
- Add map identity to `PlanetCandidate` hidden state. Keep it non-serializable.
- Change candidate materialization keys to include `world_id`, `map_id`,
  `profile_version`, scan cell, and local candidate index. Do not include a
  client-supplied coordinate or planet id.
- Add map identity to `discovery.Planet` or clearly define `ZoneID` as the map
  id for this phase. Coordinates are map-local and must validate inside
  `0..10000`.
- Add map identity to `PlayerPlanetIntel` and read models so known planets can
  be grouped by map without treating all coordinates as one global plane.
- Make scanner cooldown and pulse identity map-scoped:
  `player_id + ship_id + world_id + map_id`.
- Add public map identity to safe scanner and planet payloads:
  - `scan.pulse_started`
  - `scan.pulse_resolved`
  - `scan.planet_discovered`
  - `discovery.known_planets`
  - `discovery.planet_detail`
  Use `public_map_key` on wire payloads. Keep `internal_map_id` for server
  storage, candidate keys, cooldown keys, and materialization.
- Keep scan input client-minimal. The client must not send trusted `map_id`,
  position, candidate data, planet coordinates, radar level, scanner power,
  cooldown, or energy facts.

## Implementation Tasks In Order

1. Confirm phases 01-05 have a server-owned active map per session and a map
   profile lookup available to runtime scanner providers.
2. Add map bounds validation helpers for map-local coordinates. Use them in
   scanner position checks, planet validation, and candidate generation.
3. Replace `DiscoveryHorizon` candidate generation with a map profile input.
   Candidate generation must return no candidates outside `0..10000` even when
   the scan cell overlaps a map edge.
4. Update candidate key generation so the same cell in two different maps cannot
   produce the same materialization identity.
5. Replace origin-distance level and signal labels with map-profile level bands
   and map-local safe signal labels. Exact coordinates stay hidden until a
   server-approved intel/detail response.
6. Update `ScannerService.StartScanPulse` integration so runtime passes the
   active map/worker identity from server session state, not from client input.
7. Update scanner cooldown/energy accounting to include the active map and to
   spend or reserve energy exactly once per accepted pulse reference.
8. Update `ScannerService.ResolveScanPulse` so planet detection only considers
   candidate cells clipped to the active map. The MVP may scan only the player's
   current cell, but it must not discover candidates outside the active map or
   in another map.
9. Preserve hidden-player scanner reveal behavior, but require same active map,
   radar/scan radius, stealth rules, and map worker ownership before creating a
   witness.
10. Add map identity to planet materialization and player intel writes. Existing
    personal/shared intel rules continue to apply.
11. Update known-planet and planet-detail read models to include
    `public_map_key` on the wire and internal map id in storage, and to treat
    live actions as locked unless the player is currently in the planet's map
    and passes the relevant range/radar checks.
12. Update realtime events and socket fanout so scan events are queued only to
    the acting session and safe map-local subscribers. Never broadcast hidden
    candidate state.
13. Update the UI phase doc after implementation to clarify that the browser
    renders known planet memory by map, but the active map surface only renders
    server-visible current-map entities.

## Tests To Add/Update

- Candidate generation returns only map-local positions inside `0..10000`.
- Candidate generation for identical cell coordinates in different maps produces
  distinct materialization keys.
- Direct JSON serialization of `PlanetCandidate` still fails.
- Scan command ignores or rejects client-supplied trusted map, position,
  candidate, radar, cooldown, and energy fields.
- Scan at each map edge cannot discover a planet outside the active map.
- Scan in map A cannot discover or reveal entities in map B.
- Scanner cooldown and energy spend are idempotent per pulse reference and
  scoped by active map.
- Radar tier below candidate minimum cannot discover the planet.
- Higher scanner power/radar tier can improve detection without leaking rolls.
- Known planets and planet detail include public map key and never invent
  coordinates.
- Planet detail for unknown or unshared intel returns a safe not-found/locked
  response.
- Hidden-player reveal tests cover same-map visibility, cross-map isolation, and
  stealth witness expiry.
- Browser/realtime tests verify no fog-wave payloads are emitted and no
  other-map scan/minimap events leak.

## Migration/Doc Updates

- Rewrite or supersede the world design sections that describe infinite maps,
  discovery horizon, origin-distance scaling, and fog-wave/fog-memory gameplay.
- Update `docs/plans/ui-implementation/07-discovery-planets-production-routes.md`
  after implementation to include internal map scope, public map key,
  current-map action locking, and the removal of fog-wave language.
- Update module/security docs that mention fog to use current-map membership,
  radar range, stealth, and server-approved intel.
- If any existing local seed/test planets exist, backfill them into a starter
  map id, with coordinates validated inside `0..10000`.
- Add tuning docs for starter map density, scan cooldown, scanner power, radar
  tiers, and rare planet rates.

## Risks And Acceptance Criteria

Risks:

- Old origin-distance code may silently keep generating level bands that do not
  match map difficulty.
- Treating `ZoneID` as `MapID` can become confusing once one map needs multiple
  simulation partitions. Keep naming explicit in docs and adapters.
- Known intel across maps can be mistaken for live visibility if payload names
  are vague.
- Scanner rarity can become either too generous or impossible if density,
  cooldown, radar tier, and scan power are not tuned together.
- Edge scans are a common source of out-of-bounds or adjacent-map leaks.

Acceptance criteria:

- `scan.pulse` can only operate in the authenticated player's active map.
- No client payload can provide trusted map, position, candidate, coordinate,
  radar, energy, cooldown, or discovery truth.
- Candidate generation is server-only, deterministic, map-bounded, and profile
  driven.
- Planet discovery is rare through cooldown, density, radar tier, scanner power,
  and map profile controls.
- Materialized planets and player intel carry map identity and map-local
  coordinates.
- Same-map plus radar/stealth visibility remains the only live visibility rule.
- No fog-of-war wave remains in scanner contracts or map surface payloads.
- Realtime scan, minimap, and AOI events do not leak other-map data.
