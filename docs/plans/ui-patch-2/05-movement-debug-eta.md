# Movement Debug Logs And ETA Pill Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make movement debuggable by logging from/to coordinates and showing a top-center arrival ETA pill during server-owned movement.

**Architecture:** Derive movement presentation from authoritative entity
movement fields: origin, target, speed, started_at_ms, and arrive_at_ms. Use a
shared server-time helper so HUD ETA, distance display, and renderer
interpolation agree.

**Tech Stack:** TypeScript state helpers, HUD rendering, existing server-timed
movement payloads, Vitest, Playwright smoke.

**Status:** Complete on 2026-06-19.

---

## Files

- Create: `client/src/state/movement.ts`
- Modify: `client/src/state/reducer.ts`
- Modify: `client/src/state/reducer.test.ts`
- Modify: `client/src/app/client-app.ts`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/src/ui/hud.ts`
- Modify: `client/src/styles.css`
- Modify: `client/tests/browser-smoke.mjs`

## Steps

1. [x] Add a shared helper module:
   - estimate server time from `lastServerTime` and local monotonic time
   - compute current position for an entity movement payload
   - compute distance, duration, remaining ms, and arrival ratio
2. [x] Replace local duplicated interpolation helpers in `HUD` with the shared
   movement helper.
3. [x] In `ClientApp.sendMove`, compute:
   - current self display position
   - target
   - distance
   - rough ETA from `stats.speed` if available
   Log: `Move x1,y1 -> x2,y2, 180u, eta 1.0s`.
4. [x] When server correction/snapshot with movement arrives, log accepted route
   if the server origin/target differs meaningfully from the client estimate.
5. [x] Add `movementEtaPanel` to HUD:
   - top-center pill, below topbar
   - destination x/y
   - remaining seconds
   - progress fill
   - hidden when no active self movement
6. [x] Add rejection logs:
   - rate limit
   - offline
   - disabled ship
   - command error
7. [x] Tests:
   - unit tests for ETA helper
   - smoke clicks movement and asserts ETA pill appears, counts down, and
     disappears after arrival or stop
   - smoke verifies movement logs and visible move rejection output
8. [x] Screenshot:
   - save movement ETA state under `output/screenshots/ui-patch-2/05/`

## Acceptance

- [x] Every move intent logs from/to/distance/ETA.
- [x] ETA pill uses server-time movement fields, not client-authored truth.
- [x] Mid-route reclick logs new from/to based on server-computed in-flight
  position.
- [x] Move spam/rate-limit rejections are visible and do not corrupt route state.

## Verification

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
```

Screenshot captured:

```text
output/screenshots/ui-patch-2/05/movement-eta-desktop.png
```

## Commit

```bash
git add client/src/state client/src/app client/src/render client/src/ui client/src/styles.css client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/05
git commit -m "client: add movement eta debug ui"
```
