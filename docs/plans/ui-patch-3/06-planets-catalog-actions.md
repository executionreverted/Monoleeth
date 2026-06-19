# Planets Catalog Detail And Planet Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** The Planets surface becomes a catalog of discovered planets with
detail tabs and real/locked planet actions, not a simple radar expansion.

**Architecture:** Discovery, planet detail, production, storage, and routes are
server-owned. The client opens catalog/detail/action UI only from server
snapshots and commands.

**Tech Stack:** Go discovery/production/routes runtime handlers, TypeScript
HUD/reducer, CSS game catalog layout, browser smoke.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/ui-implementation/07-discovery-planets-production-routes.md
docs/2026-06-17-world-system-design.md
output/mockups/final-mockup.png
```

## Current Behavior

- Planet rail and intel panel list a few known planets.
- Planet detail exists but is small and mostly inline.
- `planetDetailPayload` can include coordinates, production, routes,
  `production_locked`, and `available_commands`.
- Planet actions such as claim/build/upgrade/routes remain open in
  `docs/todo.md` unless implemented.

## Target UX

- Planets menu opens a centered modal/window with:
  - left catalog list of discovered/known planets
  - search/filter by owned/stale/type/rarity
  - selected planet detail header with orb/portrait
  - tabs: Overview, Production, Storage, Routes, Intel
  - action bar: Navigate, Claim, Build, Upgrade, Route, Auto
- Each action is either real and server-backed or visibly locked with a concise
  reason.
- Owned planets show production/storage summaries if server provides them.
- Stale intel is visually different from live visibility.

## Implementation Tasks

1. Build planet catalog UI.
   - Replace simple `planetsPanel` detail with catalog/list/detail layout.
   - Keep the right rail as compact quick list only.
   - Use selected planet modal from phase 01.

2. Wire real read models.
   - `discovery.known_planets`
   - `discovery.planet_detail`
   - `planet.production_summary`
   - `planet.storage_summary`
   - `route.list`
   - `route.snapshot`

3. Action state matrix.
   - Navigate: enabled when selected detail has coordinates.
   - Claim: locked unless `discovery.claim_planet` is implemented in the same
     slice.
   - Build/Upgrade: locked unless `planet.building_build` and
     `planet.building_upgrade` are implemented.
   - Route/Auto: locked unless route mutation contracts are implemented.
   - Show server-provided `available_commands` only as capability hints, not as
     proof to skip handler validation.

4. Optional mutation slice if scope allows.
   - If enabling claim/build/route, implement the matching server contracts
     fully in this phase.
   - Otherwise keep them disabled and leave explicit TODO links.

5. Tests.
   - Reducer parses planet detail/production/storage/routes.
   - Browser smoke selects planet catalog row, opens detail, switches tabs,
     clicks Navigate.
   - Smoke asserts locked actions are disabled when contracts are absent.

## Files Likely Touched

```text
internal/game/server/discovery_production_handlers.go
internal/game/server/server_test.go
client/src/state/types.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
client/src/styles.css
client/src/app/client-app.ts
client/tests/browser-smoke.mjs
docs/todo.md
```

## Acceptance Checklist

- [x] Planets menu is a catalog/detail/action surface.
- [x] Right rail remains compact and does not inline full detail.
- [x] Selected planet detail has Overview, Production, Storage, Routes, Intel
      sections or tabs.
- [x] Navigate works through real `move_to`.
- [x] Unsupported claim/build/upgrade/route/auto actions are disabled and do
      not send fake commands.
- [x] Owned planet production/storage data comes from server summaries.
- [x] Stale intel is clearly marked and does not imply live visibility.
- [x] Browser smoke covers catalog selection and action locking.

## Implementation Notes

- The Planets window now renders a real catalog/detail/action surface from
  `planetIntel`, `production`, and `routes` state. Catalog row selection sends
  `discovery.planet_detail` only; it does not emit movement.
- The right rail remains a compact quick list and no longer inlines full planet
  detail. Full detail is in the Planets window or the existing draggable planet
  detail modal.
- Navigate remains enabled only after server detail provides coordinates and
  still sends the existing server-owned `move_to` intent. Claim, Build,
  Upgrade, Route, and Auto are visibly disabled because their mutation
  contracts are not implemented in this slice.
- Browser smoke covers row selection, section visibility, Navigate availability,
  locked action buttons, and screenshots under
  `output/screenshots/ui-patch-3/`.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/discovery ./internal/game/production ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
