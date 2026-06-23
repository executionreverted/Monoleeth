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
- Implemented slice: Task 001 playtest live projection is a server-owned
  circular `defaultLiveProjectionRadius = 1000`, giving a 2000-unit diameter
  window around the authoritative player position. This is deliberately
  separate from `stats.radar_range`.
- Scanner discovery radius is `2000`, but that is for scan/discovery, not live
  AOI.
- Current AOI projection now uses worker-owned `SnapshotWithinRadius`, backed by
  the spatial index, before visibility filtering.
- Minimap payload is built from live AOI plus remembered planet intel and now
  exposes both `radar_range` and `projection_radius`.
- Remembered minimap entries now carry stable `planet_id`/`detail_id`.
- Client fog overlay is visually disabled in `world-renderer.ts`; renderer debug
  reports inactive fog while remembered planet markers still render.
- Server hidden filtering must remain fail-closed.
- Combat and loot prechecks now use stat-radar visibility, not the widened
  projection snapshot, before domain range/capacity validation.
- AOI diff events reconcile minimap live contacts in the reducer.
- HUD minimap plotting uses `projection_radius` with `radar_range` retained as
  the displayed/stat value.

## Implementation Plan

1. Define the Task 001 live visibility window.
   - Decide whether `2000x2000` means a square window centered on the player or
     a 2000-unit radius.
   - Document the policy in code comments/tests.
   - Keep it server-owned and capped; do not let the client decide the real
     range.
   - Prefer a separate server-owned `live_projection_range` or
     `projection_window_size` over changing `stats.radar_range`.
   - Add `projection_radius` or `projection_window` to `world.snapshot/minimap`
     and use that field for map plotting instead of combat/stat `radar_range`.
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
   - Preserve/reject remembered contacts at the reducer boundary based on those
     ids; the UI must never infer a planet id from coordinates.
   - Render minimap contacts as pointer-enabled buttons/elements with
     `data-entity-id`, `data-entity-type`, or `data-detail-id`.
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

## Acceptance Criteria

- [x] The live map/radar projection policy is documented and tested.
- [x] Projection range/window is separate from player `stats.radar_range`, or
      the intentional radar stat buff and its combat/loot impact are tested.
- [x] `world.snapshot/minimap` exposes `projection_radius` or
      `projection_window` and HUD plotting uses it instead of combat/stat
      radar range.
- [x] Widened playtest projection renders edge contacts without widening
      attack, pickup, or other interaction ranges.
- [x] Worker exposes a bounded spatial query used by runtime projection.
- [x] The widened window includes allowed visible entities near the player.
- [x] Hidden entities remain absent even inside the widened window.
- [x] Known planets and safe remembered intel render in map/radar.
- [x] The radar is a stable circular surface with clickable contacts.
- [x] Live radar contacts include stable entity ids/types.
- [x] Remembered planet contacts include stable server-owned planet/detail ids.
- [x] Reducer rejects remembered real-mode contacts that lack server-owned
      planet/detail ids.
- [x] Minimap/radar contact DOM nodes are clickable controls keyed by
      server-owned ids, not passive spans or guessed coordinates.
- [x] AOI diff events keep minimap contacts current or trigger a minimap refresh.
- [x] Clicking a radar memory/contact opens detail or selects target without
      leaking movement.
- [x] Navigate uses only server-returned coordinates.
- [x] Client fog visual overlay is removed or disabled in real playtest mode.
- [x] Server visibility/fog security tests still pass.

## Verified In This Slice

```bash
go test ./internal/game/world/worker -run 'TestSnapshotWithinRadiusUsesWorkerSpatialIndex' -count=1
go test ./internal/game/server -run 'Test(WorldSnapshotCarriesSectorMinimapAndPublicEntityContract|WorldProjectionIncludesEdgeContactsWithoutHiddenLeaks|AOIDiffEventsAreFilteredPerSession|TwoPlayersWithDifferentRadarReceiveDifferentFilteredSnapshots|CombatRejectsHiddenOutOfRangeAndDisabledWithoutEnergySpend|LootPickupRejects.*Projection|LootPickupRejectsOutOfRangeDropWithoutCargoMutation)' -count=1
go test ./internal/game/server ./internal/game/world/... -run 'Test.*(Projection|SnapshotWithinRadius|WorldSnapshot|AOI|Hidden|Visibility|Combat|LootPickup)' -count=1
go test ./internal/game/server ./internal/game/world/... -count=1
cd client && npm --cache /tmp/gameproject-npm-cache run test -- --run src/state/reducer.test.ts
cd client && npm --cache /tmp/gameproject-npm-cache run typecheck
cd client && npm --cache /tmp/gameproject-npm-cache run smoke
cd client && npm --cache /tmp/gameproject-npm-cache run check
```

Navigation evidence: `client/src/app/client-app.ts` `navigateToPlanet` only calls
`sendMove` after the selected server planet detail for that `planet_id` contains
finite coordinates; otherwise it requests server detail and does not infer from
minimap position.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden)' -count=1
go test ./internal/game/world/... -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```
