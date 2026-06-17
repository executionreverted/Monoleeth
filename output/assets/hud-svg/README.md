# HUD SVG Assets

Mockup-aligned vector HUD assets for the browser-first 2D space MORPG.

The rejected raster HUD/game sprite pack was removed. This folder contains only
code-authored SVG interface assets:

- `icons/` - status, nav, planet action, and ability icons.
- `markers/` - minimap/world contact markers.
- `panels/` - major HUD panel frames.
- `buttons/` - action and ability button frames.
- `bars/` - segmented HUD stat bars.
- `preview/hud-svg-overview.svg` - quick style overview.
- `manifest.json` - machine-readable file list.

Colors are direct SVG/CSS class values so the files render consistently in
browsers, design tools, and command-line SVG renderers. To retheme the HUD,
edit `scripts/generate_hud_svg_assets.py` and regenerate the pack.

Attribution: `icons/build.svg` incorporates hammer and package path geometry
from Lucide Icons, licensed under ISC:

- https://lucide.dev/icons/hammer
- https://lucide.dev/icons/package
- https://github.com/lucide-icons/lucide/blob/main/LICENSE
