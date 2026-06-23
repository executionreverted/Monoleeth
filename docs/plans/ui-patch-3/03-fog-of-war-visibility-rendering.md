# Radar, Stealth, And Known-Intel Map Rendering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

## 2026-06-23 Supersession Note

This phase title is legacy. The current DarkOrbit-style map direction does not
use a client-side fog-of-war wave or dark unexplored-space overlay. The desired
behavior is bounded-map visibility: the server sends only current-map entities
that pass AOI/radar/stealth checks, and the client renders live radar contacts
plus server-owned known-intel memory.

**Goal:** Nearby live entities and remembered discoveries render from
server-safe radar and known-intel payloads without leaking hidden gameplay data.

**Architecture:** Server decides what is visible and remembered. Client renders
contacts and remembered markers only from server-safe AOI/minimap/known-intel
payloads.

**Tech Stack:** Go visibility/AOI/minimap payloads, TypeScript state/reducer,
Pixi renderer radar/map markers, browser smoke screenshots.

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
- Renderer shows starfield/grid/entities without unexplored darkness.
- Known planet markers can render via `worldMapMemoryMarkers`, but this is not
  the same as live radar visibility.

## Target UX

- The world shows nearby live contacts only when the server says they pass
  current-map AOI/radar/stealth checks.
- Radar range is a functional contact/interaction boundary, not a fog reveal
  animation.
- Previously discovered planets/points can render as remembered markers if the
  server sends safe known-intel memory.
- Hidden live entities are never sent and never inferable from fog shape.
- Minimap shows live contacts and remembered markers separately.
- No visual fog overlay is required for the playtest; map readability and
  hidden-data filtering matter more than darkness.

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

2. Consider a dedicated `remembered_map` field only if minimap remembered data
   is too small for renderer needs.
   - Keep the payload coarse.
   - Avoid exact hidden entity shape.

3. Add Go tests.
   - Known planet appears in remembered payload after discovery/detail.
   - Hidden live signal does not appear.
   - Remembered payload does not grant live interaction permission.
   - Different players receive different remembered payloads.

## Client Rendering Tasks

1. Extend client state types and reducer if needed.
   - Preserve `minimap.remembered`.
   - Add renderer-facing remembered marker state derived from server payload.

2. Render radar contacts and remembered markers in
   `client/src/render/world-renderer.ts`.
   - Keep live contacts visually distinct from remembered intel.
   - Do not clamp far remembered planets into fake nearby radar contacts.
   - Keep entities and HUD readable without a dark fog layer.
   - Recalculate while moving using the same server-time interpolation as the
     player position.

3. Add debug/smoke hooks.
   - Renderer `debugSnapshot` should expose live contact count, remembered
     marker count, and whether visual fog is inactive.
   - Browser smoke should assert contacts/remembered markers come only from
     server payloads and visual fog stays inactive.

4. Add screenshot checks.
   - Desktop, tablet, mobile screenshots must show the radar/map surface without
     a fog-of-war wave.

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

- [x] World view uses radar/known-intel payloads for gameplay truth.
- [x] Radar/contact rendering follows server-owned player position and payloads.
- [x] Known planets can appear as remembered intel when server-safe.
- [x] Hidden planets/signals/entities are not serialized to power map visuals.
- [x] Minimap separates live contacts from remembered known-intel markers.
- [x] Browser smoke has a renderer-debug assertion that visual fog is inactive.
- [x] Screenshots under `output/screenshots/ui-patch-3/` show the map/radar on
      desktop, tablet, and mobile without a fog-of-war wave.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/world/visibility ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
