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

## Acceptance Criteria

- [ ] The live map/radar projection policy is documented and tested.
- [ ] Projection range/window is separate from player `stats.radar_range`, or
      the intentional radar stat buff and its combat/loot impact are tested.
- [ ] Worker exposes a bounded spatial query used by runtime projection.
- [ ] The widened window includes allowed visible entities near the player.
- [ ] Hidden entities remain absent even inside the widened window.
- [ ] Known planets and safe remembered intel render in map/radar.
- [ ] The radar is a stable circular surface with clickable contacts.
- [ ] Live radar contacts include stable entity ids/types.
- [ ] Remembered planet contacts include stable server-owned planet/detail ids.
- [ ] AOI diff events keep minimap contacts current or trigger a minimap refresh.
- [ ] Clicking a radar memory/contact opens detail or selects target without
      leaking movement.
- [ ] Navigate uses only server-returned coordinates.
- [ ] Client fog visual overlay is removed or disabled in real playtest mode.
- [ ] Server visibility/fog security tests still pass.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(AOI|Minimap|Fog|Visibility|Hidden)' -count=1
go test ./internal/game/world/... -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```
