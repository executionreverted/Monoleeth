# Planet Map Selection And Stable Memory Markers Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make discovered planets stable, selectable world/map objects without leaking hidden world truth.

**Architecture:** Keep live AOI entities and fog-memory planets separate. Live
objects still come from `visibleEntities`; known planets render from
server-owned `PlanetDetailSummary.coordinates` only after a safe detail query.
World hit-testing checks selectable memory markers before empty-space movement.

**Tech Stack:** TypeScript, PixiJS renderer, existing realtime command protocol,
Vitest, Playwright browser smoke.

---

## Status

- Completed: 2026-06-19
- Verification:
  - `npm --cache /tmp/gameproject-npm-cache run typecheck`
  - `npm --cache /tmp/gameproject-npm-cache run test`
  - `npm --cache /tmp/gameproject-npm-cache run smoke`

## Current Symptoms

- A discovered planet appears visually, but cannot be selected as a planet.
- Clicking near the planet can become a movement intent.
- When the player moves, the planet appears to drift or come to center because
  there is no clear stable world-memory marker model.

## Files

- Modify: `client/src/state/types.ts`
- Modify: `client/src/state/reducer.ts`
- Modify: `client/src/state/reducer.test.ts`
- Modify: `client/src/app/client-app.ts`
- Modify: `client/src/render/world-view.ts`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/src/ui/hud.ts`
- Modify: `client/tests/browser-smoke.mjs`
- Reference: `docs/plans/modules/14-world-aoi-fog-security.md`

## Steps

1. [x] Add a client-safe selectable object model for non-live map memory:
   - `WorldMapMemoryMarker`
   - fields: `id`, `kind`, `label`, `position`, `detailID`, `state`
   - allowed kinds for this slice: `known_planet`
2. [x] Add `worldMemoryMarkers` or equivalent derived state to `WorldViewState`.
   Do not merge it into `visibleEntities`.
3. [x] Add reducer helpers that derive markers from
   `state.planetIntel.selectedPlanet.coordinates` and any known planet detail
   records that include coordinates.
4. [x] Extend `ClientApp` handlers:
   - planet list/detail click sends `discovery.planet_detail` with `planet_id`
   - world memory marker click selects the planet detail, not `move_to`
5. [x] Update `HUD` planet rows:
   - rows become buttons with `data-action="planet-detail"`
   - selected planet row gets visible focus state
   - selected planet detail appears in the right rail or modal body
6. [x] Update `WorldRenderer`:
   - render memory planet markers in their own layer
   - hit-test memory markers before empty-space movement
   - label them as known/intel memory, not live AOI entities
   - expose memory marker positions in `debugSnapshot`
7. [x] Make map marker coordinates stable:
   - world-to-screen uses the same camera center as entities
   - marker world position never mutates during player movement
8. [x] Tests:
   - reducer test: planet detail with coordinates produces stable selected
     planet state
   - renderer/browser smoke: after scan and planet detail, marker can be
     selected and no `move_to` is sent for marker click
   - smoke state verifies hidden planet entity is still absent
9. [x] Screenshot:
   - save selected planet state under `output/screenshots/ui-patch-2/01/`

## Acceptance

- [x] Clicking discovered planet list opens/requests safe detail.
- [x] Clicking planet marker selects planet detail and does not move the ship.
- [x] Empty world click still moves.
- [x] The selected planet keeps the same world coordinates while the ship moves.
- [x] Hidden planet/future spawn/procedural seed data is not present in DOM, smoke
  state, browser storage, or WebSocket payloads.

## Commit

```bash
git add client/src/state client/src/app client/src/render client/src/ui client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/01
git commit -m "client: stabilize planet map selection"
```
