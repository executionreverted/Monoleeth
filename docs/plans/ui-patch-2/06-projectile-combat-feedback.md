# Projectile Combat Feedback Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show a visible projectile traveling from the player to the enemy when a fire command is accepted by server combat events.

**Architecture:** Combat truth remains event-owned. The client renders a
presentation-only projectile using server-safe source/target IDs and effect
timestamps. Damage, miss, death, cooldown, energy, HP, shield, and loot remain
server-owned.

**Tech Stack:** PixiJS renderer, existing `WorldFeedbackEffect`, reducer tests,
Playwright smoke.

**Status:** Complete on 2026-06-19.

---

## Files

- Modify: `client/src/state/types.ts`
- Modify: `client/src/state/reducer.ts`
- Modify: `client/src/state/reducer.test.ts`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/tests/browser-smoke.mjs`
- Reference: `docs/plans/modules/05-combat-damage-targeting.md`

## Steps

1. [x] Extend `WorldFeedbackEffect` if needed:
   - keep `laser` kind or add `projectile`
   - include `sourceID`, `targetID`, `createdAt`, `expiresAt`
   - no damage/hit fields unless provided by server events
2. [x] In reducer, create projectile effect only from safe server events:
   - `combat.cooldown_started`
   - or accepted combat event that includes source/target
3. [x] Renderer:
   - compute source and target screen positions each frame
   - draw a small bright bolt/head traveling along the line over 180-280ms
   - draw faint tail, target flash, and then let existing hit/miss/damage text
     handle results
   - if source/target disappeared, fall back to last known effect position
4. [x] Prevent duplicated visual spam:
   - dedupe by event id/effect id
   - expire effects deterministically
5. [x] Tests:
   - reducer test creates projectile effect from cooldown/combat event
   - smoke fires at visible NPC and waits for a projectile effect in smoke
     state plus nonblank projectile pixels if practical
6. [x] Screenshot:
   - save a fire frame under `output/screenshots/ui-patch-2/06/`

## Acceptance

- [x] Fire shows a moving projectile from self to target.
- [x] Projectile is visibly separate from floating damage text.
- [x] No client damage/hit result is invented.
- [x] If the target dies, death/loot feedback still appears from server events.

## Verification

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run typecheck
npm --cache /tmp/gameproject-npm-cache test -- --run src/state/reducer.test.ts
npm --cache /tmp/gameproject-npm-cache run smoke
```

Screenshot captured:

```text
output/screenshots/ui-patch-2/06/projectile-desktop.png
```

## Commit

```bash
git add client/src/state client/src/render/world-renderer.ts client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/06
git commit -m "client: animate combat projectile"
```
