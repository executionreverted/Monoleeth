# Mockup And DarkOrbit Parity Verification Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Verify UI Patch 3 as a cohesive game HUD against
`output/mockups/final-mockup.png`, using DarkOrbit references only for
game-window interaction patterns.

**Architecture:** This is a QA/hardening phase. It should not introduce new
gameplay contracts unless a blocker is found and fixed in a focused slice.

**Tech Stack:** Browser smoke, Playwright/screenshots, TypeScript checks, Go
tests, visual review.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
output/mockups/final-mockup.png
output/assets/hud-svg/
client/tests/browser-smoke.mjs
```

## Verification Targets

- First viewport is the game, not a form/list page.
- Top bar, left nav, right panels, bottom action bar, log, minimap, and modal
  windows visually follow the mockup density and placement.
- Modals/windows feel like game cockpit surfaces.
- Radar/known-intel markers, parallax, projectiles, scan waves, and movement
  ETA work together without overlap. Visual fog-of-war remains inactive for the
  bounded-map playtest.
- Inventory, Hangar, Planets, Quests, and Shop can each be opened and used.
- Moving does not break modal interaction.
- No fake gameplay values appear in real mode.

## Screenshot Matrix

Capture authenticated real-server screenshots:

```text
output/screenshots/ui-patch-3/hangar-desktop.png           1440x900
output/screenshots/ui-patch-3/planets-catalog-desktop.png  1440x900
output/screenshots/ui-patch-3/quests-desktop.png           1440x900
output/screenshots/ui-patch-3/shop-desktop.png             1440x900
output/screenshots/ui-patch-3/hangar-tablet.png            1024x768
output/screenshots/ui-patch-3/planets-catalog-tablet.png   1024x768
output/screenshots/ui-patch-3/quests-tablet.png            1024x768
output/screenshots/ui-patch-3/shop-tablet.png              1024x768
output/screenshots/ui-patch-3/hangar-mobile.png            390x844
output/screenshots/ui-patch-3/planets-catalog-mobile.png   390x844
output/screenshots/ui-patch-3/quests-mobile.png            390x844
output/screenshots/ui-patch-3/shop-mobile.png              390x844
```

## QA Tasks

1. Run full server/client verification.
2. Run browser smoke in real authenticated mode.
3. Run fixture smoke only as supplemental UI coverage.
4. Capture screenshot matrix.
5. Compare against `final-mockup.png`:
   - spacing
   - panel placement
   - border treatment
   - typography scale
   - action slot shape
   - right rail density
   - log/minimap placement
   - background/radar readability
6. Audit no-fake-state rules.
   - No fake HP/shield/energy/cargo/wallet/quest counts.
   - No fake planets/NPC/loot/market products.
   - Locked actions are disabled and explain missing server contract only in
     tooltips/compact states, not as fake features.
7. Update completed checkboxes in phase files and goal only for verified work.

## Files Likely Touched

```text
client/tests/browser-smoke.mjs
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/*.md
output/screenshots/ui-patch-3/
```

## Acceptance Checklist

- [x] Full verification commands pass.
- [x] Browser smoke passes in real mode.
- [x] Screenshot matrix exists.
- [x] Screenshots visibly match mockup HUD composition more closely than
      pre-patch state.
- [x] Inventory, Hangar, Planets, Quests, and Shop are all usable surfaces.
- [x] Movement plus modal interaction is verified.
- [x] Visual fog is inactive; radar/known-intel rendering does not leak hidden
      data.
- [x] No fake gameplay data appears in real mode.
- [x] Any remaining gaps are documented as explicit follow-up TODOs.

## Implementation Notes

- Full verification passed with `GOCACHE=/tmp/gameproject-go-cache go test ./...`,
  `npm --cache /tmp/gameproject-npm-cache run check`, and `git diff --check`.
- Browser smoke covers real authenticated desktop/tablet/mobile sessions,
  screenshots for Hangar, Planets, Quests, and Shop, movement interpolation,
  modal/window interaction while moving, visual-fog inactivity, and
  no-fake-state assertions.
- Remaining non-goal polish is tracked in Phase 04: module category filtering
  and a browser invalid-equip fixture once seeded incompatible module data
  exists.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
