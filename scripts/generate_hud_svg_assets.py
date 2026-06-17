#!/usr/bin/env python3
"""Generate hand-coded SVG HUD assets for the mockup-style game UI."""

from __future__ import annotations

import json
import shutil
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "output" / "assets" / "hud-svg"


STYLE = """<style>
.line {
  fill: none;
  stroke: #d8ddde;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}
.thin { stroke-width: 1.25; }
.micro { stroke-width: 0.8; }
.muted { stroke: #7c8a90; opacity: 0.72; }
.panel-fill { fill: #03080a; fill-opacity: 0.78; }
.panel-fill-2 { fill: #081014; fill-opacity: 0.54; }
.panel-border {
  fill: none;
  stroke: #d8ddde;
  stroke-opacity: 0.46;
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}
.cyan { stroke: #1fdcf6; }
.green { stroke: #44e878; }
.red { stroke: #ff4236; }
.amber { stroke: #f6a40f; }
.purple { stroke: #b25fff; }
.fill-line { fill: #d8ddde; stroke: none; }
.fill-muted { fill: #7c8a90; stroke: none; opacity: 0.72; }
.fill-cyan { fill: #1fdcf6; stroke: none; }
.fill-green { fill: #44e878; stroke: none; }
.fill-red { fill: #ff4236; stroke: none; }
.fill-amber { fill: #f6a40f; stroke: none; }
.fill-purple { fill: #b25fff; stroke: none; }
.soft { opacity: 0.36; }
.ghost { opacity: 0.18; }
.label {
  font-family: Menlo, Consolas, Monaco, monospace;
  font-size: 13px;
  letter-spacing: 0;
  fill: #d8ddde;
}
.small-label {
  font-family: Menlo, Consolas, Monaco, monospace;
  font-size: 10px;
  letter-spacing: 0;
  fill: #7c8a90;
}
</style>"""


DEFS = """<defs>
  <filter id="hud-glow" x="-80%" y="-80%" width="260%" height="260%">
    <feGaussianBlur stdDeviation="1.8" result="blur"/>
    <feMerge>
      <feMergeNode in="blur"/>
      <feMergeNode in="SourceGraphic"/>
    </feMerge>
  </filter>
  <linearGradient id="panel-sheen" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#1fdcf6" stop-opacity="0.10"/>
    <stop offset="0.42" stop-color="#1fdcf6" stop-opacity="0.02"/>
    <stop offset="1" stop-color="#000000" stop-opacity="0"/>
  </linearGradient>
</defs>"""


manifest: list[dict[str, str]] = []


def write(path: str, body: str, view_box: str = "0 0 64 64", role: str = "asset", notes: str = "") -> None:
    target = OUT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    svg = (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{view_box}" '
        f'fill="none" shape-rendering="geometricPrecision">\n'
        f"{DEFS}\n{STYLE}\n{body}\n</svg>\n"
    )
    target.write_text(svg, encoding="utf-8")
    manifest.append({"path": path, "role": role, "notes": notes})


def icon(name: str, body: str, notes: str = "") -> None:
    write(f"icons/{name}.svg", body, role="icon", notes=notes)


def marker(name: str, body: str, notes: str = "") -> None:
    write(f"markers/{name}.svg", body, role="marker", notes=notes)


def panel(name: str, width: int, height: int, inner: str = "", notes: str = "") -> None:
    corner = 18
    body = f"""<rect x="0.5" y="0.5" width="{width - 1}" height="{height - 1}" rx="7" class="panel-fill"/>
<rect x="0.5" y="0.5" width="{width - 1}" height="{height - 1}" rx="7" fill="url(#panel-sheen)"/>
<rect x="0.5" y="0.5" width="{width - 1}" height="{height - 1}" rx="7" class="panel-border"/>
<path d="M8 {corner} V8 H{corner} M{width - corner} 8 H{width - 8} V{corner} M{width - 8} {height - corner} V{height - 8} H{width - corner} M{corner} {height - 8} H8 V{height - corner}" class="line thin muted"/>
{inner}"""
    write(f"panels/{name}.svg", body, view_box=f"0 0 {width} {height}", role="panel", notes=notes)


def segmented_bar(name: str, filled: int, color_class: str = "fill-line", width: int = 170, height: int = 18) -> None:
    gap = 4
    segments = 10
    seg_w = (width - gap * (segments - 1)) / segments
    rects = []
    for i in range(segments):
        klass = color_class if i < filled else "fill-muted ghost"
        x = i * (seg_w + gap)
        rects.append(f'<rect x="{x:.1f}" y="3" width="{seg_w:.1f}" height="{height - 6}" class="{klass}"/>')
    write(
        f"bars/{name}.svg",
        "\n".join(rects),
        view_box=f"0 0 {width} {height}",
        role="bar",
        notes=f"{filled}/10 segmented HUD bar.",
    )


def button(name: str, width: int, height: int, selected: bool = False) -> None:
    accent = "cyan" if selected else "muted"
    fill = '<rect x="6" y="6" width="{w}" height="{h}" rx="5" class="fill-cyan soft" filter="url(#hud-glow)"/>'.format(
        w=width - 12, h=height - 12
    ) if selected else ""
    body = f"""{fill}
<rect x="1" y="1" width="{width - 2}" height="{height - 2}" rx="5" class="panel-fill"/>
<rect x="1" y="1" width="{width - 2}" height="{height - 2}" rx="5" class="line thin {accent}"/>"""
    write(f"buttons/{name}.svg", body, view_box=f"0 0 {width} {height}", role="button", notes="HUD button frame.")


def generate_icons() -> None:
    icon("sector", """<circle cx="32" cy="32" r="14" class="line"/>
<path d="M32 10v11M32 43v11M10 32h11M43 32h11" class="line"/>
<path d="M32 24l8 8-8 8-8-8z" class="line thin cyan"/>""", "Sector/crosshair status icon.")

    icon("danger", """<path d="M22 18l10-6 10 6 4 13-8 11v8H26v-8l-8-11z" class="line"/>
<path d="M24 27l6-3v7h-5zM40 27l-6-3v7h5z" class="fill-line"/>
<path d="M29 37h6M26 50h12M18 14l-5-5M46 14l5-5M17 46l-5 5M47 46l5 5" class="line thin red"/>
<path d="M32 34l-3 4h6z" class="line thin muted"/>""", "Danger/skull status icon.")

    icon("energy", """<path d="M32 8l-13 25h10l-5 23 21-31H34l7-17z" class="fill-line"/>
<path d="M32 8l-13 25h10l-5 23 21-31H34l7-17z" class="line cyan soft" filter="url(#hud-glow)"/>
<path d="M15 21a22 22 0 0 1 11-10M49 43a22 22 0 0 1-11 10M13 33h7M44 33h7" class="line thin cyan"/>
<path d="M20 49l-4 4M48 15l-4 4" class="line thin muted"/>""", "Energy lightning icon.")

    icon("cargo", """<path d="M32 8l18 10v24L32 54 14 42V18z" class="line"/>
<path d="M14 18l18 11 18-11M32 29v25" class="line thin muted"/>
<path d="M32 8v21" class="line thin"/>""", "Cargo cube icon.")

    icon("credits", """<circle cx="32" cy="32" r="19" class="line"/>
<circle cx="32" cy="32" r="9" class="line"/>
<circle cx="32" cy="32" r="2.5" class="fill-line"/>
<path d="M32 13v6M32 45v6M13 32h6M45 32h6" class="line thin muted"/>""", "Credits/currency icon.")

    icon("capacity", """<circle cx="32" cy="32" r="18" class="line"/>
<path d="M20 36a13 13 0 0 1 24-8" class="line cyan"/>
<path d="M32 32l10-8" class="line"/>
<circle cx="32" cy="32" r="2" class="fill-line"/>""", "Capacity gauge icon.")

    icon("mail", """<rect x="10" y="18" width="44" height="29" rx="2" class="line"/>
<path d="M11 19l21 17 21-17M11 47l16-15M53 47L37 32" class="line thin muted"/>
<circle cx="48" cy="15" r="6" class="line cyan"/>
<circle cx="48" cy="15" r="2.2" class="fill-cyan"/>""", "Mail icon with notification indicator.")

    icon("users", """<path d="M32 13l7 17-7-4-7 4z" class="line"/>
<path d="M17 29l6 15-6-3-6 3zM47 29l6 15-6-3-6 3z" class="line thin muted"/>
<path d="M23 39c4 4 14 4 18 0M20 48c7 5 17 5 24 0" class="line thin cyan"/>
<circle cx="32" cy="35" r="2" class="fill-cyan"/>""", "Fleet/social formation icon.")

    icon("menu", """<path d="M15 20h34M15 32h34M15 44h34" class="line"/>""", "Menu icon.")

    icon("inventory", """<path d="M32 8l18 10v24L32 54 14 42V18z" class="line"/>
<path d="M14 18l18 11 18-11M32 29v25" class="line thin muted"/>""", "Inventory cube icon.")

    icon("shop", """<rect x="13" y="18" width="38" height="28" rx="2" class="line"/>
<path d="M18 25h16M18 32h10M18 39h14" class="line thin muted"/>
<circle cx="43" cy="32" r="7" class="line cyan"/>
<path d="M43 26v12M39 29h7c2 0 3 1 3 3s-1 3-3 3h-7" class="line thin cyan"/>
<path d="M23 51h18M27 46v5M37 46v5" class="line"/>""", "Market/trade terminal icon.")

    icon("galaxy", """<path d="M47 16c-8-6-22-4-29 6-8 11-2 25 10 28 12 3 23-8 19-19-3-8-14-10-19-4-4 5-1 12 5 13" class="line"/>
<path d="M18 45c8 5 20 4 27-5M20 23c7-5 18-6 26-1" class="line thin cyan"/>
<path d="M32 32h.1M15 34h.1M49 28h.1M40 49h.1" class="line cyan"/>
<circle cx="32" cy="32" r="2.5" class="fill-line"/>""", "Galaxy spiral icon.")

    icon("wormholes", """<path d="M31 8c13 0 23 9 24 21M50 42c-6 12-21 17-33 9M10 34c-2-13 7-25 20-26" class="line purple" filter="url(#hud-glow)"/>
<path d="M18 20c9-8 24-6 31 4M46 43c-8 8-22 9-31 1M14 36c-3-9 0-18 8-23" class="line thin purple"/>
<path d="M25 22c7-4 16-1 20 6M40 41c-6 5-15 5-21 0M20 33c0-7 5-13 12-14" class="line cyan"/>
<circle cx="32" cy="32" r="9" class="panel-fill"/>
<circle cx="32" cy="32" r="9" class="line"/>
<circle cx="32" cy="32" r="3" class="fill-cyan soft"/>
<path d="M32 9v6M32 49v6M9 32h6M49 32h6" class="line thin muted"/>""", "Wormhole vortex icon.")

    icon("planets", """<circle cx="32" cy="32" r="14" class="line"/>
<path d="M8 35c10 7 38 7 48-6" class="line"/>
<path d="M11 31c12-7 35-7 45 2" class="line thin muted"/>
<circle cx="32" cy="32" r="4" class="fill-muted"/>""", "Planet with orbital ring icon.")

    icon("hangar", """<path d="M12 50h40M17 50V28l15-13 15 13v22" class="line"/>
<path d="M22 50V32h20v18" class="line thin muted"/>
<path d="M32 24l7 19-7-5-7 5z" class="line cyan"/>
<path d="M32 15v9M18 28h28" class="line thin"/>""", "Hangar/ship bay icon.")

    icon("build", """<path d="M7 48l18-10 18 10-18 10z" class="line"/>
<path d="M7 48v10l18 6V58M43 48v10l-18 6" class="line"/>
<path d="M16 43l18 10" class="line thin muted"/>
<path d="M18 21c4-5 6-7 7-11 2-3 6-4 10-4h13c-6 6-8 13-8 20l-6 6h-5c-4 1-7 5-10 9l-9-8z" class="line cyan"/>
<path d="M40 27l16 16c3 3 3 7 0 10s-7 3-10 0L32 39" class="line"/>
<path d="M36 35l15 15M10 33l7 7" class="line thin muted"/>""", "Hammer centered over lower-left package build icon.")

    icon("upgrade", """<path d="M17 47l15-15 15 15M17 35l15-15 15 15M17 23l15-15 15 15" class="line"/>
<path d="M32 20v34" class="line thin cyan"/>""", "Upgrade chevrons icon.")

    icon("route", """<circle cx="16" cy="44" r="5" class="line"/>
<circle cx="32" cy="16" r="5" class="line"/>
<circle cx="48" cy="44" r="5" class="line"/>
<path d="M20 40l9-18M35 21l10 19" class="line"/>
<path d="M21 44h22" class="line thin cyan"/>""", "Route nodes icon.")

    icon("auto", """<circle cx="32" cy="32" r="18" class="line"/>
<circle cx="32" cy="32" r="5" class="line cyan"/>
<path d="M32 9v12M32 43v12M9 32h12M43 32h12" class="line"/>
<path d="M18 18l7 7M46 18l-7 7M18 46l7-7M46 46l-7-7" class="line thin muted"/>""", "Auto target icon.")

    icon("laser", """<path d="M15 44l24-24 10 10-24 24z" class="line"/>
<path d="M38 15l11 11M33 20l11 11M20 39l10 10" class="line thin muted"/>
<circle cx="45" cy="24" r="6" class="line cyan"/>
<path d="M48 21l9-9M52 25l6-6M41 28l-3 3" class="line cyan" filter="url(#hud-glow)"/>
<path d="M17 50l-7 4M21 54l-4 6" class="line thin muted"/>""", "Laser cannon weapon icon.")

    icon("rocket", """<path d="M32 7c8 8 12 20 12 33H20c0-13 4-25 12-33z" class="line"/>
<path d="M24 40l-9 10V37l5-5M40 40l9 10V37l-5-5" class="line"/>
<circle cx="32" cy="22" r="4.5" class="line cyan"/>
<path d="M24 33h16M28 40v8M36 40v8" class="line thin muted"/>
<path d="M28 50l4 8 4-8" class="line cyan" filter="url(#hud-glow)"/>
<path d="M22 55l-5 5M42 55l5 5" class="line thin muted"/>""", "Rocket/missile ability icon.")

    icon("scan", """<circle cx="32" cy="32" r="24" class="line"/>
<circle cx="32" cy="32" r="15" class="line thin"/>
<circle cx="32" cy="32" r="5" class="line cyan"/>
<path d="M32 8v9M32 47v9M8 32h9M47 32h9" class="line thin muted"/>""", "Radar scan ability icon.")

    icon("shield", """<path d="M32 8l18 8v16c0 11-7 19-18 25-11-6-18-14-18-25V16z" class="line"/>
<path d="M32 14v36" class="line thin muted"/>
<path d="M22 23h20" class="line thin cyan"/>""", "Shield ability icon.")

    icon("warp", """<path d="M13 15l16 17-16 17h10l16-17-16-17z" class="fill-line"/>
<path d="M35 15l16 17-16 17" class="line"/>
<path d="M42 32H14" class="line thin cyan"/>""", "Warp ability icon.")

    icon("gather", """<path d="M11 42l10-13 16 5 7 13-12 9-16-3z" class="line"/>
<path d="M20 43l8-8 9 4M24 52l11-9M16 52l6-10" class="line thin muted"/>
<path d="M48 9l10 10-10 10-10-10z" class="line"/>
<path d="M48 14v10M43 19h10" class="line thin cyan"/>
<path d="M40 26L28 37M46 30L34 44M53 25L38 49" class="line thin cyan" filter="url(#hud-glow)"/>
<circle cx="22" cy="31" r="2" class="fill-cyan"/>
<circle cx="37" cy="35" r="1.6" class="fill-cyan"/>
<path d="M8 35h6M39 56h7" class="line thin muted"/>""", "Mining/gather beam icon.")

    icon("plus", """<path d="M32 14v36M14 32h36" class="line"/>""", "Plus control icon.")
    icon("minus", """<path d="M14 32h36" class="line"/>""", "Minus control icon.")


def generate_markers() -> None:
    marker("player-triangle", """<path d="M32 9l17 44-17-10-17 10z" class="fill-line"/>
<path d="M32 19l8 24-8-5-8 5z" class="panel-fill"/>
<path d="M32 9l17 44-17-10-17 10z" class="line"/>""", "Player minimap/world marker.")

    marker("friendly-diamond", """<path d="M32 10l22 22-22 22-22-22z" class="line cyan" filter="url(#hud-glow)"/>
<path d="M32 22l10 10-10 10-10-10z" class="fill-cyan soft"/>""", "Friendly marker.")

    marker("outpost-diamond", """<path d="M32 10l22 22-22 22-22-22z" class="line green" filter="url(#hud-glow)"/>
<path d="M32 22l10 10-10 10-10-10z" class="fill-green soft"/>""", "Outpost marker.")

    marker("hostile-diamond", """<path d="M32 10l22 22-22 22-22-22z" class="line red" filter="url(#hud-glow)"/>
<path d="M32 22l10 10-10 10-10-10z" class="fill-red soft"/>
<path d="M32 4v8M32 52v8M4 32h8M52 32h8" class="line thin red"/>""", "Hostile marker.")

    marker("portal-circle", """<circle cx="32" cy="32" r="19" class="line purple" filter="url(#hud-glow)"/>
<circle cx="32" cy="32" r="12" class="line thin purple"/>
<circle cx="32" cy="32" r="6" class="line cyan soft"/>""", "Portal/wormhole marker.")

    marker("unknown-question", """<path d="M20 14a25 25 0 0 0-7 12M13 39a25 25 0 0 0 8 12M44 51a25 25 0 0 0 8-12M52 26a25 25 0 0 0-7-12" class="line amber"/>
<text x="32" y="42" text-anchor="middle" class="label" style="font-size:31px;fill:#f6a40f">?</text>""", "Unknown signal marker.")


def generate_panels() -> None:
    panel("top-status-bar", 2048, 50, """<path d="M290 8v34M570 8v34M900 8v34M1180 8v34M1470 8v34M1800 8v34" class="line micro muted"/>""", "Full-width top status HUD bar.")
    panel("ship-status", 260, 220, """<path d="M18 46h224M18 112h224M18 178h224" class="line micro muted"/>""", "Left ship status frame.")
    panel("nav-menu", 180, 330, """<path d="M18 58h144M18 112h144M18 166h144M18 220h144M18 274h144" class="line micro muted"/>""", "Left navigation menu frame.")
    panel("log", 430, 235, """<path d="M18 42h394M18 200h394" class="line micro muted"/>
<text x="22" y="31" class="small-label">LOG</text>""", "Terminal log panel.")
    panel("planet-list", 365, 295, """<path d="M18 86h329M18 142h329M18 198h329M18 254h329" class="line micro muted"/>
<text x="20" y="33" class="small-label">PLANETS</text>""", "Right planet list panel.")
    panel("selected-planet", 365, 345, """<path d="M18 86h329M18 268h329" class="line micro muted"/>
<text x="20" y="33" class="small-label">SELECTED PLANET</text>""", "Selected planet detail panel.")
    panel("sector-map", 430, 340, """<circle cx="284" cy="154" r="40" class="line micro muted"/>
<circle cx="284" cy="154" r="74" class="line micro muted"/>
<circle cx="284" cy="154" r="110" class="line micro muted"/>
<path d="M284 30v248M160 154h248" class="line micro muted"/>
<text x="22" y="31" class="small-label">SECTOR MAP</text>""", "Sector minimap panel.")

    button("button-88x72", 88, 72)
    button("button-88x72-selected", 88, 72, selected=True)
    button("ability-slot-96", 96, 96)
    button("ability-slot-96-selected", 96, 96, selected=True)


def generate_bars() -> None:
    segmented_bar("segmented-empty-170x18", 0)
    segmented_bar("segmented-hull-76-170x18", 8)
    segmented_bar("segmented-shield-54-170x18", 5)
    segmented_bar("segmented-cap-68-170x18", 7)
    segmented_bar("segmented-energy-cyan-68-170x18", 7, "fill-cyan")


def generate_preview() -> None:
    body = """<rect width="1200" height="760" fill="#05090b"/>
<text x="32" y="44" class="label" style="font-size:24px">HUD SVG Asset Pack</text>
<text x="32" y="74" class="small-label">Individual files live in icons, markers, panels, buttons, and bars.</text>
<rect x="28" y="102" width="1144" height="1" class="fill-muted ghost"/>
<text x="32" y="138" class="label">Palette</text>
<circle cx="44" cy="170" r="10" class="fill-cyan"/><text x="64" y="175" class="small-label">cyan friendly/status</text>
<circle cx="244" cy="170" r="10" class="fill-green"/><text x="264" y="175" class="small-label">green owned/outpost</text>
<circle cx="464" cy="170" r="10" class="fill-red"/><text x="484" y="175" class="small-label">red hostile</text>
<circle cx="654" cy="170" r="10" class="fill-amber"/><text x="674" y="175" class="small-label">amber unknown</text>
<circle cx="854" cy="170" r="10" class="fill-purple"/><text x="874" y="175" class="small-label">purple portal</text>
<rect x="32" y="220" width="350" height="180" rx="7" class="panel-fill"/>
<rect x="32" y="220" width="350" height="180" rx="7" class="panel-border"/>
<path d="M40 238V228h10M364 228h10v10M374 382v10h-10M50 392H40v-10" class="line thin muted"/>
<text x="52" y="254" class="small-label">PANEL FRAME SAMPLE</text>
<rect x="52" y="288" width="116" height="18" class="fill-line"/>
<rect x="52" y="322" width="164" height="10" class="fill-cyan"/>
<rect x="52" y="350" width="250" height="1" class="fill-muted ghost"/>
<g transform="translate(470 230)">
  <path d="M32 9l17 44-17-10-17 10z" class="fill-line"/>
  <path d="M112 10l22 22-22 22-22-22z" class="line cyan"/>
  <path d="M192 10l22 22-22 22-22-22z" class="line red"/>
  <circle cx="272" cy="32" r="19" class="line purple"/>
  <text x="352" y="42" text-anchor="middle" class="label" style="font-size:31px;fill:#f6a40f">?</text>
</g>
<text x="470" y="332" class="small-label">markers</text>
<g transform="translate(470 382)">
  <circle cx="32" cy="32" r="24" class="line"/>
  <circle cx="32" cy="32" r="15" class="line thin"/>
  <circle cx="32" cy="32" r="5" class="line cyan"/>
  <path d="M113 15l16 17-16 17h10l16-17-16-17z" class="fill-line"/>
  <path d="M215 8l18 8v16c0 11-7 19-18 25-11-6-18-14-18-25V16z" class="line"/>
  <path d="M294 49l35-35M297 17l30 30" class="line"/>
</g>
<text x="470" y="488" class="small-label">sample ability icons</text>"""
    write("preview/hud-svg-overview.svg", body, view_box="0 0 1200 760", role="preview", notes="Self-contained overview preview.")


def generate_readme() -> None:
    readme = """# HUD SVG Assets

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
"""
    (OUT / "README.md").write_text(readme, encoding="utf-8")


def main() -> None:
    if OUT.exists():
        shutil.rmtree(OUT)
    OUT.mkdir(parents=True, exist_ok=True)
    generate_icons()
    generate_markers()
    generate_panels()
    generate_bars()
    generate_preview()
    generate_readme()
    (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"generated {len(manifest)} SVG assets under {OUT}")


if __name__ == "__main__":
    main()
