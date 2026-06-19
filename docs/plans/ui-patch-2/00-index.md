# UI Patch 2 Plan Index

Date: 2026-06-19

## Purpose

UI Patch 2 fixes the issues found during real playtest after the first HUD
rework. The previous goal removed the page-stack/form hell and proved the real
server-backed client. This patch makes the HUD behave like a stable game
surface: selectable planets, real draggable windows, clean input isolation,
quick actions, scan mode, movement ETA, projectile feedback, and a real
starfield background.

## Primary References

```text
docs/plans/ui-patch-2-goal.md
output/mockups/final-mockup.png
output/assets/mockup-hud/background/starfield_2048x1152.png
output/assets/mockup-hud/background/grid_overlay_2048x1152.png
output/assets/hud-svg/
```

## Current Code Audit Findings

- `client/src/render/world-renderer.ts` only hit-tests `visibleEntities`.
  Known/discovered planets in `planetIntel.planets` are not selectable world
  objects unless they also exist as live `planet_signal` AOI entities.
- Known planet summaries do not carry coordinates; only `PlanetDetailSummary`
  has `coordinates`. Planet list clicks need to request `planet.detail` before
  the renderer can anchor a memory marker.
- The renderer correctly centers the camera on the player, so all world objects
  move on screen as the camera moves. The bug to fix is not "screen position
  changes"; it is missing stable world-memory anchoring, selection, and clear
  distinction between live AOI objects and known planet memory.
- `HUD` has a modal primitive but feature panels open as stacked `hud-window`
  elements with CSS offsets, no persisted positions, and no dragging.
- `.hud` uses `pointer-events: none` with only buttons/inputs/panels set to
  `pointer-events: auto`; gaps in overlays can still pass through to the
  canvas. A pointerdown/click suppression guard is needed around HUD/modal
  interaction.
- Quick action buttons are partially wired. Laser, scan, and gather map to
  real handlers. Rocket, shield, and warp must remain locked until real
  contracts exist.
- Scan is a single `scan.pulse` button. There is no client-local scan mode,
  no auto retry loop, and no renderer state for scan waves.
- Movement logs currently say generic command text like `Sent move_to`; they do
  not include from/to, distance, or ETA.
- `WorldRenderer.drawLaserEffect` renders an instantaneous line/flash. The
  user-facing request is a visible projectile moving from self to target.
- The world background is procedural stars/nebula/grid. It does not use the
  provided starfield asset and still reads far from the mockup.
- HUD distance helpers use local `Date.now()` interpolation separately from the
  renderer's server-time offset model. UI ETA/distance should share a single
  helper to avoid time drift.

## Phase List

1. [x] [Planet Map Selection And Stable Memory Markers](./01-planet-map-selection.md)
2. [x] [Draggable Modal Windows And Input Isolation](./02-window-modal-input-isolation.md)
3. [x] [Quick Action Input Contracts](./03-quick-actions-input-contracts.md)
4. [x] [Scan Mode Automation And Visual Indicator](./04-scan-mode-automation.md)
5. [x] [Movement Debug Logs And ETA Pill](./05-movement-debug-eta.md)
6. [x] [Projectile Combat Feedback](./06-projectile-combat-feedback.md)
7. [Starfield Parallax Background](./07-starfield-parallax-background.md)
8. [Mockup Parity And Verification Gate](./08-mockup-parity-verification.md)

## Suggested Worker Split

- World/data worker: Phase 01 planet markers, detail requests, selection model.
- HUD/window worker: Phase 02 draggable windows and input suppression.
- Interaction worker: Phases 03-05 quick actions, scan mode, movement ETA.
- Renderer worker: Phases 06-07 projectile and starfield/parallax.
- QA worker: Phase 08 screenshots, smoke assertions, mockup parity checklist.

Workers must follow `docs/symphony-worker-rules.md`, must not spawn subagents,
must not manage Symphony, and must not commit.

## Verification Commands

Use narrow commands while developing, then run the full gate:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

For UI slices, also run or update browser smoke and inspect screenshots:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run smoke
npm --cache /tmp/gameproject-npm-cache run smoke -- --fixture
```

Expected screenshot output for this patch:

```text
output/screenshots/ui-patch-2/
```

## Done Criteria

- All phase files are either completed or have explicit remaining blockers.
- `docs/plans/ui-patch-2-goal.md` checkboxes are all complete.
- No default fake gameplay state is introduced.
- Real-server browser smoke covers the changed behavior.
- Visual screenshots show clear progress toward `final-mockup.png`.
- Full verification passes.
