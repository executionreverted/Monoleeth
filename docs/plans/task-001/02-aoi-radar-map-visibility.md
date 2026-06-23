# Phase 02 - AOI, Radar, Map Visibility, And Fog Removal

## Goal

Make the playtest world/radar useful. The client should receive enough
server-owned nearby data to avoid objects popping in late, the minimap/radar
should show known and visible entities, and the current visual fog overlay
should be removed for now without weakening server visibility security.

## Problems Covered

- Nearby objects are too easy to miss.
- The current effective live AOI feels like `420` radar range.
- Scanner radius `2000` exists but is not live object/radar visibility.
- Radar/minimap is inert and often empty.
- Known planets and visible enemies/loot/players are not consistently shown.
- Fog of war should be disabled for this playtest.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/06-loot-drop-ownership.md
docs/2026-06-17-world-system-design.md
internal/game/server/runtime.go
internal/game/world/visibility/visibility.go
internal/game/world/visibility/fog.go
internal/game/world/spatial/index.go
client/src/render/world-renderer.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/state/world-memory.ts
client/src/ui/hud.ts
```

## Current Findings

- `defaultRadarRange = 420` seeds the current player radar feel.
- Scanner discovery radius is `2000`, but that is for scan/discovery, not live
  AOI.
- Current AOI builds from worker snapshot then filters; widening it should use
  or expose spatial index queries instead of scanning too much work.
- Minimap payload is built from live AOI plus remembered planet intel.
- Remembered minimap entries need stable ids such as `planet_id` or
  `detail_id`.
- Client fog overlay lives in `world-renderer.ts` and can be disabled visually.
- Server hidden filtering must remain fail-closed.
- Current runtime uses `state.Stats.RadarRange` for AOI filtering and minimap
  scale; widening this directly can accidentally widen combat and loot
  interaction checks.
- AOI diff events update visible entities but do not currently reconcile
  minimap live contacts.

## Subagent Review Additions - 2026-06-20

- Make the projection implementation task exact: `Runtime.aoiSnapshotForPlayerLocked`
  must not rely on the old radar-only feel or client-selected full-snapshot
  filtering. Current bounded-map follow-up should document server-owned active
  `0..10000` map membership plus AOI/radar/stealth filtering.
- Add minimap reconciliation from `aoi.entity_entered`,
  `aoi.entity_updated`, and `aoi.entity_left`. The HUD renders from minimap
  state, so visible entity changes must also update live contacts or trigger a
  safe minimap refresh.
- Add a stable identity contract for minimap interactions. Live contacts need
  `data-entity-id` and `data-entity-type`; remembered planet contacts need
  server-owned `planet_id` or `detail_id`, not client-guessed coordinates.
- Rewrite smoke expectations that still assume active visual fog or a fixed
  `radar_range === 420`. Task 001 smoke should assert fog overlay absence and
  preserved hidden-data filtering instead.

## Second Subagent Review Additions - 2026-06-20

- Define active-map input sources, not a legacy projection size. The current
  bounded-map model must specify how worker-owned entities, DB overlay entities,
  procedural/cache materialization, known planet intel, NPCs, loot, and player
  entities enter the `0..10000` map candidate set before AOI/radar/stealth
  visibility filtering.
- Add tests that prove current-map rendering is not only showing in-memory seed
  fixtures. A live/procedural/DB-backed entity on the active map must appear
  only when visible or remembered by server-owned intel, while a hidden entity
  from the same source remains filtered.
- Document refresh behavior when movement crosses materialization boundaries:
  old live contacts leave, newly materialized contacts enter, and minimap state
  reconciles through AOI diffs or a safe snapshot refresh.

## Third Subagent Review Additions - 2026-06-20

- Current implementation evidence originally proved a larger worker snapshot,
  but not the full projection source contract. Runtime still queries only
  `Worker.EntitiesWithinWindow`, so future bounded-map work must document or
  implement how DB overlays, procedural/live materialization, known intel,
  seeded NPCs, loot, and player entities enter the active `0..10000` map before
  AOI/radar/stealth visibility filtering.
- Add a server test proving a non-fixture materialized or DB-backed visible
  entity enters the projection while a hidden entity from the same source stays
  filtered.
- Add browser smoke covering projection beyond static fixtures: live contact,
  remembered planet/intel marker, hidden absence, and movement/materialization
  boundary refresh.

## Fourth Subagent Review Additions - 2026-06-20

- `known_planets` or scanner intel updates must refresh minimap remembered
  contacts and main world memory markers immediately, or trigger a safe
  `world.snapshot`/memory refresh. Smoke must not mask this gap by pressing
  Sync before checking the newly discovered marker.
- Define the far-memory policy. Remembered planets outside the current
  projection must either render as an explicit off-ring/distance marker or stay
  off radar; they must not be clamped onto the minimap edge as fake nearby
  contacts.
- Define stale/wrong-zone/invalidated remembered intel behavior before render:
  current world/zone filtering, stale styling, click behavior, and whether
  colonized/claimed memories remain visible.
- Document active-map projection versus circular radar/contact semantics.
  Legacy Phase 02 used a `2000x2000` square proof; current bounded maps use
  active `0..10000` map membership plus AOI/radar/stealth filtering. Payload
  naming, radar scale, and contact rendering must make that policy
  deterministic.
- Add browser/server tests for scan discovery without manual Sync, far memory,
  invalidated or wrong-map memory, and active-map contacts that are renderable
  as map memory while still requiring radar/visibility checks for live
  interaction.

## Fifth Subagent Review Additions - 2026-06-20

- Server responses that replace `entities` without a `minimap` payload can leave
  `minimap.live_contacts` stale. Fix by including a fresh minimap projection in
  those responses or by rebuilding live contacts from replacement entities in
  the reducer while preserving remembered intel.
- Minimap live contact actions need a stable type contract. Hostile NPC/player
  contacts select, loot contacts use selection-only `loot-select`, remembered
  planet contacts open detail, and self/friendly/unknown non-detail contacts
  should no-op or be explicitly disabled.
- Scan/known-planet discovery still needs no-manual-Sync smoke: newly learned
  remembered markers should appear and open detail from the event/query flow
  that discovered them.

## Implementation Plan

1. Define the Task 001 live visibility projection.
   - Treat the old `2000x2000` window as historical Phase 02 proof, not the
     target map size.
   - Use bounded active-map membership for the `0..10000` local map, then apply
     server-owned AOI/radar/stealth filtering.
   - Document the policy in code comments/tests.
   - Keep active-map projection server-owned and bounded; do not let the client
     decide map membership, visibility, or radar range.
   - Do not buff `stats.radar_range` to fake broader map knowledge. Radar stays
     a ship/stat value used by visibility and interaction gates.
   - Add regression tests proving interactions still re-check their own
     visibility, radar, and range gates even when the map surface can render
     remembered intel.

2. Add safe map/radar projection.
   - Add a worker-owned active-map projection API instead of scanning or
     exposing an unfiltered full worker snapshot.
   - Use the map's bounded `0..10000` membership as the coarse candidate set,
     then apply AOI/radar/stealth and hidden-data filters before serialization.
   - Treat spatial indexes as optimization only; they must not redefine the
     visibility policy or leak out-of-map candidates.
   - Include live contacts by type: NPC, loot, hostile, player, planet signal,
     known planet memory.
   - Do not serialize internal hidden flags, seeds, future spawns, or scan rolls.

3. Make minimap/radar useful.
   - Make the radar visually circular inside a stable square sector-map frame.
   - Render live contacts and remembered known planets.
   - Give live contacts stable `data-entity-id` and `data-entity-type`
     attributes.
   - Give remembered planet contacts stable `planet_id` or `detail_id` from
     player-owned intel records only.
   - Click known planet/contact opens detail first; navigation uses
     server-returned coordinates only.
   - Reconcile minimap live contacts after AOI entered/updated/left events or
     trigger a safe minimap refresh.
   - Render all known planet memory markers on the main map, not only the
     selected planet.

4. Disable visual fog.
   - Remove or feature-disable the dark circular fog overlay in the renderer.
   - Remove fog debug display from normal HUD if present.
   - Keep server-side hidden-data filtering and intel permission checks.
   - Update browser smoke from `fog.active === true` to “fog overlay inactive,
     remembered/minimap markers still render, hidden fields remain absent.”

5. Update tests and smoke.
   - Assert current-map contacts can be represented as map memory or live
     contacts only after the server-owned AOI/radar/stealth filters allow them.
   - Assert hidden contacts on the same map do not appear unless visibility
     rules explicitly reveal them.
   - Assert minimap clicks do not guess coordinates.

## Likely Files

```text
internal/game/server/runtime.go
internal/game/server/server_test.go
internal/game/world/worker/worker.go
internal/game/world/worker/worker_test.go
internal/game/world/spatial/index.go
internal/game/world/visibility/visibility.go
internal/game/world/aoi/snapshot.go
client/src/render/world-renderer.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/state/world-memory.ts
client/src/ui/hud.ts
client/src/styles.css
docs/plans/task-001/02-aoi-radar-map-visibility.md
```

## Implementation Evidence - 2026-06-20

- Phase 02 originally proved a server-owned `2000x2000` square projection
  window with `runtimeLiveProjectionDiameter = 2000`, half extent `1000`.
  That proof is now historical. The bounded multi-map direction supersedes it
  with full active-map `0..10000` membership plus AOI/radar/stealth filtering.
- Player `stats.radar_range` remains a ship/stat value; it is not mutated to
  fake larger map knowledge.
- Runtime projection must stay server-owned before AOI/security filtering.
- Visibility still runs through `aoi.BuildVisibleSnapshot` and hidden entities
  inside the projection window remain absent from world entities and minimap
  contacts.
- Minimap live contacts reconcile from AOI enter/update/left/correction
  events, carry stable entity ids/types, and are clickable local target selects.
- Remembered planet minimap contacts now carry server-owned `planet_id` and
  `detail_id`; clicking them opens planet detail without sending movement.
- World-map memory markers include selected planet detail and all server-owned
  remembered minimap planet memories with coordinates.
- Planet navigation buttons carry only `planet_id`; the client resolves the
  final navigation destination from selected `discovery.planet_detail`
  coordinates only. Cross-planet stale coordinate fallback is regression-tested
  so a detail response without coordinates cannot inherit another planet's last
  known coordinates.
- Long-range planet navigation may still send bounded movement waypoints, but
  those waypoints are derived from the server-timed current ship position and
  the server-returned final planet coordinate. Browser smoke asserts the final
  navigation target equals the selected server detail coordinates.
- Minimap live contacts are regression-tested to mirror the matching server AOI
  snapshot entity id, type, and position.
- `move_to` and `stop` responses now include the server-owned minimap projection
  alongside replacement entities. The client also rebuilds `minimap.live_contacts`
  from server-owned replacement entities when no fresh minimap payload is
  present, preserving remembered planet intel while removing stale live
  contacts.
- Scan discovery now refreshes remembered planet minimap contacts without a
  manual Sync. `discovery.known_planets` events and query responses include a
  server-owned `minimap` payload while `known_planets.planets[]` remains
  coordinate-free.
- Client reducer coverage proves a `discovery.known_planets` event with
  `minimap.remembered` immediately produces `worldMapMemoryMarkers()` without
  `planet_detail` or `world.snapshot`.
- Browser smoke now asserts real scan discovery creates remembered minimap and
  world memory markers before any manual Sync path can mask the bug.
- Minimap loot contacts are selection-only at the radar layer and are covered by
  browser smoke so they cannot accidentally issue movement or pickup commands.
- Visual fog overlay is disabled in the renderer while server-side visibility
  and hidden-data filtering remain active.
- The old square-window regression (`1000,1000` admitted and `1001,0`
  excluded) was a Phase 02 proof of server-owned projection boundaries, not the
  target map size. Current follow-up work should preserve the same
  server-owned filtering discipline over bounded `0..10000` maps.
- Far remembered planets remain server-owned map memory, not fake live radar
  contacts. Backend regression covers a far materialized planet/intel row as
  `minimap.remembered` with unclipped coordinates and no matching
  `live_contacts` entry; the client filters remembered minimap points to the
  current projection window while preserving world-map memory markers.
- Client projection helper coverage preserved the legacy Phase 02 square-window
  proof for `+1000,+1000`, but current follow-up work should validate bounded
  `0..10000` active-map membership plus radar/stealth filtering. Fixture
  browser smoke continues to keep far remembered intel off live radar.
- Remembered intel policy is explicit client-side: stale known-planet memories
  remain renderable/clickable as detail links with stale styling and preserved
  `projection_source` evidence, while `invalidated`, wrong-zone, expired, or
  revoked memories fail closed before minimap/world-map rendering. Wrong-sector
  remembered memory is also filtered from HUD/world-map surfaces even when its
  freshness is otherwise `fresh`. Reducer and fixture smoke cover this behavior
  so stale memory clicks cannot emit movement and bad memory cannot appear as
  fake radar/map contacts.
- Runtime projection source contract is now explicit and tested. Live worker
  projection entities and minimap contacts carry `projection_source:
  worker_projection`; this covers the current live materialization path for
  players, NPCs, loot, and planet signals after they enter the authoritative
  world worker. Materialized discovery/DB overlay planet intel enters radar/map
  memory as `projection_source: known_intel` with `sector_key:
  origin-fringe`, and never becomes a fake live contact without worker
  materialization.
- AOI entity payloads now carry the same projection source as live minimap
  contacts, so `aoi.entity_entered`/`aoi.entity_updated`, `move_to`,
  `stop`, and `world.snapshot` replacement paths can rebuild live contacts
  without dropping source evidence.
- Server regression now covers worker NPC, loot, and player projection sources,
  hidden same-window filtering, materialized discovery planet intel memory,
  source/sector metadata, and server-owned movement crossing a projection
  boundary where the old entity leaves and the new entity enters with
  `worker_projection`.
- Browser smoke now asserts real-server entity/contact source metadata and DOM
  `data-projection-source` after authenticated bootstrap, remembered-minimap
  refresh, and movement projection refresh. Fixture smoke also proves fallback
  replacement snapshots preserve `worker_projection` and `known_intel` source
  evidence.

## Acceptance Criteria

- [x] The live map/radar projection policy is documented and tested as
      server-owned bounded active-map visibility plus AOI/radar/stealth
      filtering.
- [x] Active-map visibility does not buff player `stats.radar_range`; radar
      remains a ship/stat value for visibility and interaction gates.
- [x] Worker/runtime projection stays server-owned and uses bounded active-map
      membership before client-safe serialization.
- [x] Current-map contacts appear only when map memory or live visibility rules
      allow them.
- [x] Hidden same-map entities remain absent unless radar/stealth/visibility
      rules explicitly reveal them.
- [x] Known planets and safe remembered intel render in map/radar.
- [x] The radar is a stable circular surface with clickable contacts.
- [x] Live radar contacts include stable entity ids/types.
- [x] Remembered planet contacts include stable server-owned planet/detail ids.
- [x] AOI diff events and entity-replacement responses keep minimap live
      contacts current or trigger a minimap refresh.
- [x] Clicking a radar memory/contact opens detail or selects target without
      leaking movement.
- [x] Navigate uses only server-returned coordinates.
- [x] Client fog visual overlay is removed or disabled in real playtest mode.
- [x] Server visibility/security tests still pass, including legacy fog-memory
      primitives where they remain as known-intel memory helpers.
- [x] Active-map input sources for worker, DB overlay, procedural/live
      materialization, known intel, NPCs, loot, and players are documented and
      tested.
- [x] Visibility smoke proves current-map rendering is not only direct worker
      fixture insertion and reconciles source changes after movement.
- [x] Scan/known-planet updates refresh remembered minimap and world-map markers
      without requiring manual Sync.
- [x] Far remembered planets do not clamp into fake nearby radar contacts.
- [x] Stale, invalidated, and wrong-zone remembered intel have explicit render
      and click behavior.
- [x] Active-map membership and circular radar/contact semantics are documented
      and tested, with legacy square-window proofs framed as historical.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden)' -count=1
go test ./internal/game/world/... -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
```
