# Planet Detail Modal, Quick Navigation, And Disconnect Settlement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Planet list selection opens a real modal/detail window with quick
navigation, and disconnect settles active movement to the current
server-computed position before the session goes offline.

**Architecture:** Server owns planet visibility/detail and movement. Client
only requests planet detail and sends a `move_to` intent to a server-known
coordinate returned by `discovery.planet_detail`.

**Tech Stack:** Go runtime/world worker/realtime protocol, TypeScript client
state/reducer/HUD, Pixi world renderer.

---

## Required Reading

```text
AGENTS.md
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
docs/todo.md
output/mockups/final-mockup.png
```

## Current Behavior

- `planetsPanel` renders `planetDetailBlock(selected)` inline after the list.
- Selecting a planet only requests detail; there is no focused planet detail
  modal flow.
- Known planet memory markers can be selected, but the right-side planet list
  does not expose a "navigate to coordinates" action.
- `transport.go` calls `runtime.detachSession` on final socket unregister.
- `runtime.detachSession` submits `worker.DetachSessionCommand`, but it does
  not settle in-flight movement to a current position.
- `worker.stopPlayer` clears movement without advancing to a time-based current
  point; worker ticks advance by fixed delta only.

## Target UX

- Clicking a planet row opens a centered draggable modal/window titled with the
  planet name.
- The right rail remains a compact planet catalog/list.
- Detail modal contains:
  - name, type, biome, rarity, level
  - last known coordinates
  - owner status
  - production/storage summary if the server returns it
  - visible locked states for unsupported claim/build/route actions
  - `Navigate` quick action that sends a real move intent to the detail
    coordinate
- Clicking `Navigate` logs the route and closes or de-emphasizes the modal only
  if that matches the final HUD behavior.
- If the socket drops while movement is active, the server settles the player
  to the current server-computed location and clears the movement route before
  detaching the session.
- On reconnect, world snapshot reconciles to that settled position.

## Implementation Tasks

1. Add a planet detail modal mode in `client/src/ui/hud.ts`.
   - Keep `planetsPanel` as the compact right-side list.
   - Remove inline `planetDetailBlock(selected)` from the rail list.
   - Add a dedicated modal/window body for selected planet detail.
   - Add `data-action="planet-navigate"` with the selected detail coordinates.

2. Add a HUD handler and client command path for planet navigation.
   - Extend `HUDHandlers` with `onPlanetNavigate(planetID: string)`.
   - In `client/src/app/client-app.ts`, resolve the selected
     `planetIntel.selectedPlanet.coordinates`.
   - Call existing `sendMove({ x, y })`.
   - If detail is missing, request `discovery.planet_detail` and show a compact
     log line instead of sending a guessed coordinate.

3. Keep navigation server-authoritative.
   - Do not add `player_id`, `speed`, or client-computed arrival times.
   - Use the existing `move_to` payload shape only.
   - Ensure reducer reconciliation still comes from `position.corrected`,
     `aoi.entity_updated`, and world snapshots.

4. Add disconnect settlement at the server runtime/worker boundary.
   - Add a worker method or command, for example
     `SettleAndDetachSessionCommand`, that:
     - finds the player for the session
     - computes current movement position at the worker clock time
     - updates the entity position/index
     - clears movement
     - detaches the session
   - Avoid trusting any client timestamp or position.
   - Update `runtime.detachSession` to use this settlement path.
   - For multiple open connections, settle only when the last connection for a
     session is unregistered, matching `transport.go` connection count logic.

5. Add tests.
   - Go worker test: active movement is settled to an intermediate current
     position on detach, not origin and not target when only partial time passed.
   - Go server transport/runtime test: disconnect while moving, reconnect,
     bootstrap world snapshot shows settled position and no active movement.
   - Client reducer/HUD test: planet detail does not render inline in the rail.
   - Browser smoke: click planet row, modal opens, click Navigate, route log
     appears, disconnect/reconnect reconciles position.

## Files Likely Touched

```text
client/src/ui/hud.ts
client/src/app/client-app.ts
client/src/state/reducer.ts
client/src/state/types.ts
client/src/styles.css
client/tests/browser-smoke.mjs
internal/game/server/transport.go
internal/game/server/runtime.go
internal/game/server/server_test.go
internal/game/world/worker/commands.go
internal/game/world/worker/worker.go
internal/game/world/worker/worker_test.go
internal/game/world/movement.go
```

## Acceptance Checklist

- [x] Planet rail list no longer renders inline detail.
- [x] Planet detail opens as a draggable centered modal/window.
- [x] Planet navigation button uses only the selected server-returned
      coordinates.
- [x] Navigation sends `move_to` and uses the same movement logging/ETA path as
      normal map clicks.
- [x] Missing coordinates produce a safe "request detail first" state.
- [x] Socket disconnect during active movement settles the player to current
      server-computed position.
- [x] Reconnect snapshot shows no stale active movement unless the server really
      kept one.
- [x] Hidden or unknown planets cannot be navigated to through stale UI state.
- [x] Go tests cover movement settlement.
- [x] Browser smoke covers planet modal and navigate.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/world/worker ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
