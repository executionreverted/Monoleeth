# HUD SVG Assets Design

Date: 2026-06-17

## Goal

Create a mockup-aligned HUD asset pack as hand-coded SVG files under
`output/assets/hud-svg/`. Keep the generated starfield background, but replace
the rejected raster HUD/game sprite pack with crisp, inspectable vector HUD
assets.

## Direction

The asset language follows `output/mockups/final-mockup.png`:

- dark transparent tactical panels
- thin pale gray strokes
- cyan friendly/status accents
- red hostile accents
- green outpost/owned accents
- amber unknown/warning accents
- purple portal accents
- compact geometric icons suitable for a browser game HUD

## Scope

Generate only HUD and map UI assets:

- icons
- panels
- action buttons
- ability slots
- segmented bars
- minimap/world markers

Do not generate game-world bitmap sprites here. Ships, planets, enemies, loot,
and other illustrated world assets should be handled separately.

## Output Location

Use `output/assets/hud-svg/` for now so the assets can be reviewed before the
browser client workspace exists.
