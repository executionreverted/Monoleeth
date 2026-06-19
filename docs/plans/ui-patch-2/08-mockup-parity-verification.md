# Mockup Parity And Verification Gate Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make a final UI Patch 2 pass that compares the browser HUD against `final-mockup.png` and locks the new behavior into tests and screenshots.

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

1. Capture screenshots:
   - unauthenticated desktop/mobile
   - authenticated desktop/tablet/mobile
   - selected planet
   - draggable centered window
   - scan mode active
   - movement ETA active
   - projectile frame
2. Compare against `output/mockups/final-mockup.png`:
   - topbar height and density
   - left ship/nav rail size and icon language
   - right planet/target/minimap stack
   - bottom log/action rail position
   - central world object density
   - starfield color/contrast
   - action slot icon scale
   - panel border/corner treatment
3. Fix only polish gaps that are in scope:
   - spacing
   - z-index
   - overflow
   - missing active/selected states
   - unreadable labels
   - mobile overlap
4. Do not enable future gameplay:
   - no fake planet ownership
   - no fake route/build/upgrade
   - no fake rocket/shield/warp
   - no fake mail/social counts
5. Update smoke assertions:
   - planet selection does not send move
   - dragged window remains in viewport
   - HUD click does not move
   - scan mode emits real scan intents
   - movement ETA appears
   - projectile effect appears
   - starfield debug offset changes with movement
6. Run full verification:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

7. Update `docs/plans/ui-patch-2-goal.md` checkboxes only for verified work.
8. Move any incomplete non-blocking gaps to `docs/todo.md` with owner,
   unblock condition, and acceptance test.

## Acceptance

- UI Patch 2 goal checkboxes are complete or blockers are explicit.
- Screenshots show the starfield/mockup composition clearly.
- All new interactions are covered by real-server browser smoke or focused
  fixture smoke.
- Full verification passes.

## Commit

```bash
git add client/tests/browser-smoke.mjs docs/plans/ui-patch-2-goal.md docs/plans/ui-patch-2 docs/todo.md output/screenshots/ui-patch-2/08
git commit -m "client: verify ui patch 2 parity"
```
