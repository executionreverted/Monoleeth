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
  must not rely on the old radar-only feel or full-snapshot filtering. Expose a
  worker-owned bounded spatial projection and document whether the playtest
  policy is radius or square window.
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

- Define projection input sources, not just projection size. The widened view
  must specify how worker-owned entities, DB overlay entities, procedural/cache
  materialization, known planet intel, NPCs, loot, and player entities enter the
  bounded projection before visibility filtering.
- Add tests that prove the widened projection is not only showing current
  in-memory seed fixtures. A live/procedural/DB-backed entity inside the policy
  window must appear when visible, while a hidden entity from the same source
  remains filtered.
- Document refresh behavior when movement crosses materialization boundaries:
  old live contacts leave, newly materialized contacts enter, and minimap state
  reconciles through AOI diffs or a safe snapshot refresh.

## Third Subagent Review Additions - 2026-06-20

- Current implementation evidence proves a larger worker snapshot, but not the
  full projection source contract. Runtime still queries only
  `Worker.EntitiesWithinWindow`, so Task 001 must document or implement how DB
  overlays, procedural/live materialization, known intel, seeded NPCs, loot,
  and player entities enter the `2000x2000` projection before visibility
  filtering.
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
- Document square projection versus circular radar semantics. The server
  `2000x2000` square gives corner contacts beyond a 1000-unit circular radius;
  payload naming, radar scale, and contact rendering must make that policy
  deterministic.
- Add browser/server tests for scan discovery without manual Sync, far memory,
  invalidated or wrong-zone memory, and corner contacts inside the square
  projection but outside a strict radar circle.

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

1. Define the Task 001 live visibility window.
   - Decide whether `2000x2000` means a square window centered on the player or
     a 2000-unit radius.
   - Document the policy in code comments/tests.
   - Keep it server-owned and capped; do not let the client decide the real
     range.
   - Prefer a separate server-owned `live_projection_range` or
     `projection_window_size` over changing `stats.radar_range`.
   - If radar stats are intentionally buffed instead, document the combat/loot
     range impact and add regression tests proving interactions still re-check
     their own ranges.

2. Add safe map/radar projection.
   - Add a worker-owned bounded spatial query API instead of scanning the full
     worker snapshot for every projection.
   - Use spatial index lookup for the widened window.
   - Run all candidates through existing visibility/hidden filters.
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
   - Assert visible edge contacts appear inside the policy window.
   - Assert hidden contacts inside the same window do not appear.
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
client/tests/browser-smoke.mjs
docs/plans/task-001/02-aoi-radar-map-visibility.md
```

## Implementation Evidence - 2026-06-20

- Live projection policy is a server-owned `2000x2000` square window:
  `runtimeLiveProjectionDiameter = 2000`, half extent `1000`.
- Player `stats.radar_range` remains a ship/stat value; it is not mutated to
  widen the playtest map projection.
- Runtime projection uses worker `EntitiesWithinWindow`, backed by the spatial
  index `QueryWindow`, before AOI/security filtering.
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

## Acceptance Criteria

- [x] The live map/radar projection policy is documented and tested.
- [x] Projection range/window is separate from player `stats.radar_range`, or
      the intentional radar stat buff and its combat/loot impact are tested.
- [x] Worker exposes a bounded spatial query used by runtime projection.
- [x] The widened window includes allowed visible entities near the player.
- [x] Hidden entities remain absent even inside the widened window.
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
- [x] Server visibility/fog security tests still pass.
- [ ] Projection input sources for worker, DB overlay, procedural/live
      materialization, known intel, NPCs, loot, and players are documented and
      tested.
- [ ] Projection smoke proves the widened view is not only direct worker
      fixture insertion and reconciles source changes after movement.
- [ ] Scan/known-planet updates refresh remembered minimap and world-map markers
      without requiring manual Sync.
- [ ] Far remembered planets do not clamp into fake nearby radar contacts.
- [ ] Stale, invalidated, and wrong-zone remembered intel have explicit render
      and click behavior.
- [ ] Square projection and circular radar semantics are documented and tested,
      including corner contacts.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden)' -count=1
go test ./internal/game/world/... -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```
