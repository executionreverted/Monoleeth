# Modal Interaction And Movement Input Isolation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** HUD windows and modals remain fully clickable while the ship is
moving, and HUD/modal clicks never leak into the world canvas as move/select
intents.

**Architecture:** UI focus and pointer ownership are client-local presentation
state. World movement and entity selection remain server-owned intents.

**Tech Stack:** TypeScript HUD, CSS pointer/event layers, Pixi world input,
Playwright smoke.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/ui-patch-2/02-window-modal-input-isolation.md
output/mockups/final-mockup.png
client/src/ui/hud.ts
client/src/render/world-renderer.ts
client/src/styles.css
```

## Current Behavior

- Patch 2 added draggable windows and a global HUD input suppression marker.
- `blocksWorldInput` and `markHUDInputSuppressed` exist in `hud.ts`.
- `world-renderer.ts` has `ignoreWorldInputUntil`.
- Playtest still reports modal clicks fail while moving.

## Target UX

- Movement never disables modal/window pointer events.
- Dragging a modal header while moving works.
- Clicking buttons, tabs, list rows, inputs, scroll areas, drag/drop slots, and
  category controls inside any HUD window never creates a world move.
- Clicking empty world still sends a move intent.
- Pressing quick action keys is ignored while a modal/window/input owns focus.
- Escape closes the top modal/window without affecting movement.

## Implementation Tasks

1. Audit event ordering from `hud.ts` to `world-renderer.ts`.
   - Track `pointerdown`, `pointerup`, `click`, `dragstart`, and keyboard
     events.
   - Confirm whether movement re-render replaces elements during click and
     drops the pointer/click target.

2. Make HUD/window layers own pointer events consistently.
   - Avoid holes in active modal/window surfaces.
   - Keep the full modal/window shell at `pointer-events: auto`.
   - Allow the transparent HUD host to remain pass-through where no UI exists.

3. Add a robust suppression bridge.
   - On capture-phase pointerdown inside `.hud`, `.hud-window`, `.hud-modal`,
     `.auth-panel`, or future drag/drop zones, set suppression until after the
     click event.
   - World renderer checks suppression on pointerdown and click before hit
     testing or move intent.
   - Suppression must not block HUD handlers.

4. Stabilize moving-state re-render.
   - Movement ETA updates should not recreate active modal content in a way
     that eats clicks.
   - Prefer targeted ETA/render updates or keep focus/selection stable across
     render.

5. Add tests and smoke.
   - Unit test `quickActionShortcutSafe` while modal/window/input is active.
   - Browser smoke:
     - start movement
     - open Inventory/Hangar/Planets/Shop/Quests
     - click a button/list row inside each
     - assert no unexpected `move_to` log was created
     - assert the intended modal/window action occurred

## Files Likely Touched

```text
client/src/ui/hud.ts
client/src/render/world-renderer.ts
client/src/styles.css
client/src/app/client-app.ts
client/tests/browser-smoke.mjs
```

## Acceptance Checklist

- [x] Modal/window clicks work while movement ETA is active.
- [x] Dragging windows works while movement ETA is active.
- [x] HUD clicks do not create `move_to` unless the clicked control explicitly
      sends navigation.
- [x] World clicks outside HUD still work.
- [x] Quick action keyboard shortcuts are ignored while inputs/modals/windows
      own focus.
- [x] Browser smoke proves no click leakage during movement.

## Verification

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
