# Draggable Modal Windows And Input Isolation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make feature panels open as centered, draggable HUD windows and stop HUD/form interactions from leaking into canvas movement.

**Architecture:** Replace the current offset-only window list with a small UI
window manager owned by `HUD`. Window state is client-local only: open/closed,
position, focus, z-order, and minimized/detail modal state. Input suppression is
handled both in HUD event capture and in `WorldRenderer` so a HUD-originating
pointer sequence cannot become a world click.

**Tech Stack:** TypeScript DOM events, CSS responsive layout, existing HUD
renderer, Playwright browser smoke.

---

## Files

- Modify: `client/src/ui/hud.ts`
- Modify: `client/src/styles.css`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/tests/browser-smoke.mjs`

## Steps

1. Introduce `HUDWindowState`:
   - `id`
   - `x`
   - `y`
   - `z`
   - `open`
   - `dragging` transient state outside render HTML
2. Default feature windows centered:
   - inventory/cargo
   - systems/hangar/loadout/crafting
   - quests
   - intel/scanner
   - economy/shop
   - admin ops
3. Preserve right rail core panels:
   - planets summary
   - target
   - sector map/minimap
   These can still have "inspect" modal behavior, but main feature screens open
   as centered windows.
4. Add draggable headers:
   - pointerdown on `.hud-window__header`
   - pointermove updates x/y
   - pointerup commits
   - clamp to viewport and keep header reachable
5. Add z-order focus:
   - pointerdown on a window focuses and raises it
   - nav toggle focuses existing window instead of duplicating
6. Keep mobile behavior:
   - under tablet breakpoint, windows become bottom sheets or full-width modal
     sheets
   - drag disabled or limited vertically on mobile
7. Input isolation:
   - HUD root capture marks pointer sequences that start in `.hud`, `.auth-panel`,
     `.hud-modal`, `.hud-window`, buttons, inputs, selects, textareas
   - `WorldRenderer.bindInput` ignores clicks not targeting the canvas and
     ignores the latest HUD-originated pointer sequence
   - while an input or modal has focus, keyboard action shortcuts do nothing
8. Tests:
   - smoke opens inventory/economy/quests/systems windows centered
   - drag one window and assert the position changes but stays in viewport
   - click inside a window body and assert no `move_to`
   - click a HUD button and assert no accidental canvas selection/move
   - mobile viewport has no horizontal body overflow
9. Screenshot:
   - save centered and dragged window states under
     `output/screenshots/ui-patch-2/02/`

## Acceptance

- Main feature windows open centered by default.
- Windows are draggable on desktop.
- Focus/z-order is stable.
- Modal close/Escape/backdrop behavior still works.
- HUD/form/modal clicks never trigger world movement.
- Mobile remains a clean sheet layout.

## Commit

```bash
git add client/src/ui/hud.ts client/src/styles.css client/src/render/world-renderer.ts client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/02
git commit -m "client: add draggable hud windows"
```
