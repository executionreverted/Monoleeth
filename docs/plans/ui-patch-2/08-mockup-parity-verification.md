# Mockup Parity And Verification Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make a final UI Patch 2 pass that compares the browser HUD against `final-mockup.png` and locks the new behavior into tests and screenshots.

**Status:** Completed 2026-06-19.

**Architecture:** Treat this as a QA/hardening phase, not a feature grab bag.
Only fix issues revealed by the Phase 01-07 screenshots, smoke tests, or
mockup comparison. Do not add new gameplay contracts here.

**Tech Stack:** Playwright smoke, screenshots, TypeScript tests, Go tests, git
diff checks.

---

## Files

- Modify: `client/tests/browser-smoke.mjs`
- Modify: `docs/plans/ui-patch-2-goal.md`
- Modify: relevant phase files in `docs/plans/ui-patch-2/`
- Modify: `docs/todo.md` only for real blockers that cannot be completed
- Screenshot output: `output/screenshots/ui-patch-2/08/`

## Checklist

1. [x] Capture screenshots:
   - unauthenticated desktop/mobile
   - authenticated desktop/tablet/mobile
   - selected planet
   - draggable centered window
   - scan mode active
   - movement ETA active
   - projectile frame
2. [x] Compare against `output/mockups/final-mockup.png`:
   - topbar height and density
   - left ship/nav rail size and icon language
   - right planet/target/minimap stack
   - bottom log/action rail position
   - central world object density
   - starfield color/contrast
   - action slot icon scale
   - panel border/corner treatment
3. [x] Fix only polish gaps that are in scope:
   - spacing
   - z-index
   - overflow
   - missing active/selected states
   - unreadable labels
   - mobile overlap
4. [x] Do not enable future gameplay:
   - no fake planet ownership
   - no fake route/build/upgrade
   - no fake rocket/shield/warp
   - no fake mail/social counts
5. [x] Update smoke assertions:
   - planet selection does not send move
   - dragged window remains in viewport
   - HUD click does not move
   - scan mode emits real scan intents
   - movement ETA appears
   - projectile effect appears
   - starfield debug offset changes with movement
6. [x] Run full verification:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

7. [x] Update `docs/plans/ui-patch-2-goal.md` checkboxes only for verified work.
8. [x] Move any incomplete non-blocking gaps to `docs/todo.md` with owner,
   unblock condition, and acceptance test.

No new non-blocking gaps were found in this phase; existing future-contract
items remain tracked in `docs/todo.md`.

## Acceptance

- UI Patch 2 goal checkboxes are complete or blockers are explicit.
- Screenshots show the starfield/mockup composition clearly.
- All new interactions are covered by real-server browser smoke or focused
  fixture smoke.
- Full verification passes.

## Implementation Notes

- Topbar and left nav now use the existing HUD SVG icon assets instead of
  text-only placeholders.
- Desktop action slots were resized toward the mockup's large square button
  treatment; tablet slots use a compact center-lane variant to avoid rail
  overlap.
- Browser smoke now asserts the mockup shell structure: six top status cells,
  icon-backed nav, six action slots, visible right rail/minimap, no horizontal
  overflow, and tablet action rail separation from the right rail.
- Phase 08 smoke saves final evidence under
  `output/screenshots/ui-patch-2/08/`.

## Verification

- `GOCACHE=/tmp/gameproject-go-cache go test ./...`
- `npm --cache /tmp/gameproject-npm-cache run check`
- `git diff --check`
- Screenshots inspected:
  - `output/screenshots/ui-patch-2/08/live-desktop.png`
  - `output/screenshots/ui-patch-2/08/live-tablet.png`
  - `output/screenshots/ui-patch-2/08/live-mobile.png`
  - `output/screenshots/ui-patch-2/08/window-desktop.png`
  - `output/screenshots/ui-patch-2/08/scan-mode-desktop.png`
  - `output/screenshots/ui-patch-2/08/movement-eta-desktop.png`
  - `output/screenshots/ui-patch-2/08/projectile-desktop.png`

## Commit

```bash
git add client/tests/browser-smoke.mjs docs/plans/ui-patch-2-goal.md docs/plans/ui-patch-2 docs/todo.md output/screenshots/ui-patch-2/08
git commit -m "client: verify ui patch 2 parity"
```
