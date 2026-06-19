# Fog Of War And Remembered Map Rendering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unexplored space reads as dark/fogged, current radar area reads clear,
and remembered discoveries render as safe fog memory without leaking hidden
gameplay data.

**Architecture:** Server decides what is visible and remembered. Client renders
darkness, haze, and revealed zones only from server-safe AOI/minimap/fog memory
payloads.

**Tech Stack:** Go visibility/AOI/minimap payloads, TypeScript state/reducer,
Pixi renderer fog overlay, browser smoke screenshots.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/2026-06-17-world-system-design.md
internal/game/world/visibility/fog.go
internal/game/server/runtime.go
client/src/render/world-renderer.ts
output/mockups/final-mockup.png
```

## Current Behavior

- `FogMemory` exists in `internal/game/world/visibility/fog.go`.
- `minimapPayload` includes `Remembered []minimapMemoryPayload`.
- `minimapFromAOI` always returns an empty `Remembered` slice.
- Renderer shows starfield/grid/entities but no unexplored darkness.
- Known planet markers can render via `worldMapMemoryMarkers`, but this is not
  the same as a fog layer.

## Target UX

- The world is dark and slightly hazy outside the player's safe visibility
  region.
- Radar range around the player has a clear circular/soft-edged reveal.
- Previously discovered planets/points can create faint remembered reveal
  pockets if the server sends safe memory.
- Hidden live entities are never sent and never inferable from fog shape.
- Minimap shows live contacts and remembered markers separately.
- Fog visually matches the mockup tone: space remains readable but exploration
  has real darkness.

## Server Payload Tasks

1. Populate remembered minimap data from safe known planet intel.
   - Use discovery/intel records already returned by
     `runtime.knownPlanetsPayload`.
   - Include only:
     - kind
     - label
     - last known coordinate
     - freshness/stale state
   - Do not include current resources, hidden candidates, owner internals,
     procedural seeds, or future spawn data.

2. Consider a dedicated `fog` or `remembered_map` field only if minimap
   remembered data is too small for renderer needs.
   - Keep the payload coarse.
   - Avoid exact hidden entity shape.

3. Add Go tests.
   - Known planet appears in remembered payload after discovery/detail.
   - Hidden live signal does not appear.
   - Remembered payload does not grant interaction permission.
   - Different players receive different remembered payloads.

## Client Rendering Tasks

1. Extend client state types and reducer if needed.
   - Preserve `minimap.remembered`.
   - Add a renderer-facing fog memory model derived from server payload.

2. Add fog overlay to `client/src/render/world-renderer.ts`.
   - Use a dark full-screen layer.
   - Cut/clear a soft circle around current player position using radar range.
   - Add faint remembered pockets for server-provided memory markers.
   - Keep entities and HUD readable.
   - Recalculate while moving using the same server-time interpolation as the
     player position.

3. Add debug/smoke hooks.
   - Renderer `debugSnapshot` should expose fog active state, reveal center,
     reveal radius, and remembered pocket count.
   - Browser smoke should assert fog is non-empty and reveal follows movement.

4. Add screenshot checks.
   - Desktop, tablet, mobile screenshots must show visible dark/fogged
     unexplored areas.

## Files Likely Touched

```text
internal/game/server/runtime.go
internal/game/server/discovery_production_handlers.go
internal/game/server/server_test.go
internal/game/world/visibility/fog.go
internal/game/world/visibility/fog_test.go
client/src/state/types.ts
client/src/state/reducer.ts
client/src/render/world-renderer.ts
client/src/render/world-view.ts
client/tests/browser-smoke.mjs
client/src/styles.css
```

## Acceptance Checklist

- [x] World view has visible fog/darkness outside current safe reveal.
- [x] Fog reveal follows the interpolated server-owned player position.
- [x] Known planets can appear as remembered fog memory when server-safe.
- [x] Hidden planets/signals/entities are not serialized to produce fog.
- [x] Minimap separates live contacts from remembered fog/intel.
- [x] Browser smoke has a canvas-pixel or renderer-debug assertion for fog.
- [x] Screenshots under `output/screenshots/ui-patch-3/` show fog on desktop,
      tablet, and mobile.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/world/visibility ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
