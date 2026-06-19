# Starfield Parallax Background Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the procedural dark grid feel with the provided starfield asset as an endless mirrored parallax background.

**Architecture:** Load a client-build-safe copy of
`starfield_2048x1152.png`, render it in a background layer behind the grid and
world entities, and tile it around the camera using mirrored neighboring tiles.
The grid/radar layer stays above the starfield for mockup readability.

**Tech Stack:** PixiJS texture/sprite rendering, Vite asset handling, CSS/Pixi
visual verification. Before implementing Pixi texture APIs, fetch current Pixi
docs with Context7 per `AGENTS.md`.

---

## Files

- Create or copy asset: `client/src/assets/starfield_2048x1152.png`
- Optional copy asset: `client/src/assets/grid_overlay_2048x1152.png`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/src/styles.css`
- Modify: `client/tests/browser-smoke.mjs`
- Reference: `output/assets/mockup-hud/background/starfield_2048x1152.png`
- Reference: `output/assets/mockup-hud/background/grid_overlay_2048x1152.png`

## Steps

1. Copy the starfield asset into a Vite-served client asset path.
2. Add a Pixi background texture layer:
   - far starfield layer
   - optional mid dust layer using same texture at lower alpha/scale
   - existing procedural stars can become a subtle near-particle layer or be
     removed if they fight the asset
3. Implement endless mirrored tiling:
   - world tile size is `2048x1152`
   - draw enough tiles to cover viewport plus one margin
   - odd x tile mirrors horizontally
   - odd y tile mirrors vertically
   - offset based on camera center and parallax depth
4. Preserve grid/radar overlay:
   - grid stays above starfield
   - alpha tuned against mockup
   - no one-note color wash
5. Add parallax:
   - far layer slow
   - mid layer medium
   - near particles/grid faster
6. Tests:
   - browser smoke pixel sampling verifies canvas is nonblank and background is
     not just black/grid
   - movement smoke compares background debug offset before/after movement
   - screenshots desktop/tablet/mobile
7. Screenshot:
   - save `live-desktop.png`, `live-tablet.png`, `live-mobile.png` under
     `output/screenshots/ui-patch-2/07/`

## Acceptance

- Starfield asset is visible in the first viewport.
- Moving the player creates clear parallax motion.
- Tiling is endless with mirrored edges; no blank seams in common viewports.
- Grid and HUD remain readable.
- Production build includes the asset and `npm run bundle-scan` remains clean.

## Commit

```bash
git add client/src/assets client/src/render/world-renderer.ts client/src/styles.css client/tests/browser-smoke.mjs output/screenshots/ui-patch-2/07
git commit -m "client: add mirrored starfield parallax"
```
