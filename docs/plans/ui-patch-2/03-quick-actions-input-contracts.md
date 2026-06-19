# Quick Action Input Contracts Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

## Status

- State: Completed 2026-06-19 for quick action contracts; scan mode
  automation remains Phase 04.
- Verification:
  - `npm --cache /tmp/gameproject-npm-cache run smoke`
  - desktop/tablet/mobile screenshots under
    `output/screenshots/ui-patch-2/03/`

**Goal:** Make the action bar feel like real game controls while keeping unimplemented actions locked and preventing focus/input conflicts.

**Architecture:** Treat the bottom action rail as the single quick-action
contract for implemented player intents. Slots can send commands, toggle
client-local modes, or render locked. Keyboard shortcuts are optional in this
phase but must obey focus guards.

**Tech Stack:** TypeScript HUD event handling, existing `CommandBuilder`,
Vitest/Playwright smoke.

---

## Files

- Modify: `client/src/ui/hud.ts`
- Modify: `client/src/app/client-app.ts`
- Modify: `client/src/styles.css`
- Modify: `client/tests/browser-smoke.mjs`
- Optional Modify: `client/src/protocol/request-id.test.ts`

## Slot Contract

- Slot 1 Laser: real `combat.use_skill` only when a visible hostile target is
  selected and server-owned energy/cooldown/range hints allow the action.
- Slot 2 Rocket: locked until a real skill contract exists.
- Slot 3 Scan: toggles scan mode in Phase 04. In this phase it can still call
  one-shot `scan.pulse` until scan mode lands.
- Slot 4 Shield: locked until a real defensive skill contract exists.
- Slot 5 Warp: locked until route/warp contracts exist.
- Slot 6 Gather: real `loot.pickup` or approach intent for visible loot.

## Steps

1. [x] Add a single `QuickActionState` helper so target panel and action bar do not
   duplicate action availability logic.
2. [x] Ensure every enabled action has:
   - stable `data-action`
   - public label
   - compact detail line
   - disabled reason in `title`
3. [x] Add action feedback states:
   - pending command
   - cooldown
   - no lock
   - no drop
   - offline
   - locked
4. [x] Add optional keyboard shortcuts `1..6` only if focus is safe:
   - ignore when `input`, `textarea`, `select`, contenteditable, or modal is
     active
   - ignore while dragging a HUD window
5. [x] Keep unimplemented slots disabled. Do not add fake rocket/shield/warp logic.
6. [x] Update smoke:
   - click laser action and verify `combat.use_skill`
   - verify gather slot maps to the real `loot.pickup`/`move_to` path; the
     existing real smoke still covers loot approach/pickup reconciliation
   - click locked slots and verify no command is sent
   - focus an input/window and press shortcuts; verify no command or movement
7. [x] Update visual states to better match mockup:
   - use `output/assets/hud-svg/icons/laser.svg`
   - use `scan.svg`, `shield.svg`, `warp.svg`, `gather.svg`, `rocket.svg`
   - keep text compact and non-overlapping

## Acceptance

- [x] Implemented quick actions are clearly live and backed by real handlers.
- [x] Unimplemented quick actions are visually locked and inert.
- [x] No quick action fires while typing or while a modal/window has focus.
- [x] Browser smoke proves no accidental world click is produced by action-bar
  interaction.

## Commit

```bash
git add client/src/ui/hud.ts client/src/app/client-app.ts client/src/styles.css client/tests/browser-smoke.mjs client/src/protocol/request-id.test.ts
git commit -m "client: tighten quick action contracts"
```
