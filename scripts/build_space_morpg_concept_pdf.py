from __future__ import annotations

import math
import os
import random
from dataclasses import dataclass
from typing import Iterable, List, Sequence, Tuple

from reportlab.lib import colors
from reportlab.lib.colors import Color
from reportlab.lib.pagesizes import A4, landscape
from reportlab.pdfbase.pdfmetrics import stringWidth
from reportlab.pdfgen import canvas


OUT_PATH = "output/pdf/space-morpg-concept-brief.pdf"
PAGE_SIZE = landscape(A4)
W, H = PAGE_SIZE

BG = Color(0.015, 0.023, 0.045)
BG_2 = Color(0.025, 0.036, 0.070)
CARD = Color(0.050, 0.070, 0.110)
CARD_2 = Color(0.070, 0.090, 0.130)
LINE = Color(0.230, 0.330, 0.430)
TEXT = Color(0.900, 0.940, 0.980)
MUTED = Color(0.600, 0.680, 0.760)
CYAN = Color(0.100, 0.790, 0.950)
GREEN = Color(0.270, 0.900, 0.560)
AMBER = Color(1.000, 0.690, 0.260)
MAGENTA = Color(0.900, 0.310, 0.730)
RED = Color(1.000, 0.340, 0.300)
PURPLE = Color(0.500, 0.430, 1.000)


@dataclass(frozen=True)
class Bullet:
    text: str
    accent: Color = CYAN


def lerp(a: float, b: float, t: float) -> float:
    return a + (b - a) * t


def blend(c1: Color, c2: Color, t: float) -> Color:
    return Color(
        lerp(c1.red, c2.red, t),
        lerp(c1.green, c2.green, t),
        lerp(c1.blue, c2.blue, t),
    )


def wrap_text(text: str, font: str, size: float, max_width: float) -> List[str]:
    lines: List[str] = []
    for raw in text.split("\n"):
        words = raw.split()
        if not words:
            lines.append("")
            continue
        line = words[0]
        for word in words[1:]:
            trial = f"{line} {word}"
            if stringWidth(trial, font, size) <= max_width:
                line = trial
            else:
                lines.append(line)
                line = word
        lines.append(line)
    return lines


def draw_text(
    c: canvas.Canvas,
    text: str,
    x: float,
    y: float,
    max_width: float,
    font: str = "Helvetica",
    size: float = 12,
    leading: float | None = None,
    color: Color = TEXT,
) -> float:
    leading = leading or size * 1.35
    c.setFont(font, size)
    c.setFillColor(color)
    for line in wrap_text(text, font, size, max_width):
        c.drawString(x, y, line)
        y -= leading
    return y


def draw_centered_text(
    c: canvas.Canvas,
    text: str,
    x: float,
    y: float,
    width: float,
    font: str = "Helvetica-Bold",
    size: float = 14,
    color: Color = TEXT,
) -> None:
    c.setFont(font, size)
    c.setFillColor(color)
    for i, line in enumerate(wrap_text(text, font, size, width)):
        line_w = stringWidth(line, font, size)
        c.drawString(x + (width - line_w) / 2, y - i * size * 1.25, line)


def rounded_rect(c: canvas.Canvas, x: float, y: float, w: float, h: float, radius: float, fill: Color, stroke: Color | None = None) -> None:
    c.setFillColor(fill)
    if stroke:
        c.setStrokeColor(stroke)
        c.roundRect(x, y, w, h, radius, stroke=1, fill=1)
    else:
        c.roundRect(x, y, w, h, radius, stroke=0, fill=1)


def draw_background(c: canvas.Canvas, seed: int, title: str | None = None, page_num: int | None = None) -> None:
    c.setFillColor(BG)
    c.rect(0, 0, W, H, stroke=0, fill=1)

    c.setFillColor(BG_2)
    c.circle(W * 0.88, H * 0.82, 260, stroke=0, fill=1)
    c.setFillColor(Color(0.030, 0.030, 0.060))
    c.circle(W * 0.18, H * 0.10, 210, stroke=0, fill=1)

    rng = random.Random(seed)
    for _ in range(110):
        x = rng.uniform(22, W - 22)
        y = rng.uniform(24, H - 24)
        r = rng.choice([0.45, 0.55, 0.70, 0.85, 1.0])
        shade = rng.uniform(0.48, 0.95)
        c.setFillColor(Color(shade, shade, min(1, shade + 0.06)))
        c.circle(x, y, r, stroke=0, fill=1)

    c.setStrokeColor(Color(0.060, 0.120, 0.180))
    c.setLineWidth(0.4)
    for _ in range(12):
        x1 = rng.uniform(0, W)
        y1 = rng.uniform(0, H)
        x2 = x1 + rng.uniform(-70, 100)
        y2 = y1 + rng.uniform(-50, 80)
        c.line(x1, y1, x2, y2)

    if title:
        c.setFillColor(MUTED)
        c.setFont("Helvetica", 8.5)
        c.drawString(38, 24, title)
    if page_num is not None:
        c.setFillColor(MUTED)
        c.setFont("Helvetica", 8.5)
        c.drawRightString(W - 38, 24, f"{page_num:02d}")


def draw_header(c: canvas.Canvas, title: str, kicker: str, seed: int, page_num: int) -> None:
    draw_background(c, seed, "2D Space MORPG Concept Brief", page_num)
    c.setFillColor(MUTED)
    c.setFont("Helvetica-Bold", 9)
    c.drawString(40, H - 47, kicker.upper())
    c.setFillColor(TEXT)
    c.setFont("Helvetica-Bold", 28)
    c.drawString(40, H - 82, title)
    c.setStrokeColor(CYAN)
    c.setLineWidth(2.2)
    c.line(40, H - 100, 155, H - 100)


def draw_tag(c: canvas.Canvas, x: float, y: float, text: str, accent: Color = CYAN, width: float | None = None) -> float:
    font = "Helvetica-Bold"
    size = 8.5
    pad_x = 9
    tw = stringWidth(text.upper(), font, size)
    w = width or tw + pad_x * 2
    rounded_rect(c, x, y, w, 20, 8, blend(accent, CARD, 0.75), blend(accent, LINE, 0.25))
    c.setFillColor(blend(accent, TEXT, 0.35))
    c.setFont(font, size)
    c.drawCentredString(x + w / 2, y + 6.5, text.upper())
    return w


def draw_card(
    c: canvas.Canvas,
    x: float,
    y: float,
    w: float,
    h: float,
    title: str,
    body: str | Sequence[str],
    accent: Color = CYAN,
    title_size: float = 13,
    body_size: float = 9.8,
) -> None:
    rounded_rect(c, x, y, w, h, 9, CARD, Color(0.150, 0.210, 0.290))
    c.setFillColor(accent)
    c.rect(x, y + h - 4, w, 4, stroke=0, fill=1)
    c.setFillColor(TEXT)
    c.setFont("Helvetica-Bold", title_size)
    c.drawString(x + 15, y + h - 27, title)
    if isinstance(body, str):
        draw_text(c, body, x + 15, y + h - 47, w - 30, "Helvetica", body_size, body_size * 1.35, MUTED)
    else:
        draw_bullets(c, [Bullet(item, accent) for item in body], x + 15, y + h - 48, w - 30, body_size, body_size * 1.45)


def draw_bullets(
    c: canvas.Canvas,
    bullets: Iterable[Bullet],
    x: float,
    y: float,
    max_width: float,
    size: float = 10.5,
    leading: float = 15.0,
) -> float:
    for bullet in bullets:
        lines = wrap_text(bullet.text, "Helvetica", size, max_width - 18)
        c.setFillColor(bullet.accent)
        c.circle(x + 4, y + 3.5, 2.6, stroke=0, fill=1)
        c.setFillColor(TEXT)
        c.setFont("Helvetica", size)
        for i, line in enumerate(lines):
            c.drawString(x + 17, y - i * leading, line)
        y -= max(1, len(lines)) * leading + 4
    return y


def draw_flow_node(c: canvas.Canvas, x: float, y: float, label: str, accent: Color, w: float = 120, h: float = 46) -> None:
    rounded_rect(c, x - w / 2, y - h / 2, w, h, 12, Color(0.040, 0.060, 0.095), blend(accent, LINE, 0.15))
    c.setFillColor(accent)
    c.circle(x - w / 2 + 17, y, 4.2, stroke=0, fill=1)
    draw_centered_text(c, label, x - w / 2 + 28, y + 8, w - 38, "Helvetica-Bold", 10, TEXT)


def draw_arrow(c: canvas.Canvas, x1: float, y1: float, x2: float, y2: float, color: Color = LINE) -> None:
    c.setStrokeColor(color)
    c.setLineWidth(1.25)
    c.line(x1, y1, x2, y2)
    angle = math.atan2(y2 - y1, x2 - x1)
    size = 6
    p1 = (x2 - size * math.cos(angle - 0.55), y2 - size * math.sin(angle - 0.55))
    p2 = (x2 - size * math.cos(angle + 0.55), y2 - size * math.sin(angle + 0.55))
    c.setFillColor(color)
    c.line(x2, y2, p1[0], p1[1])
    c.line(x2, y2, p2[0], p2[1])


def page_cover(c: canvas.Canvas) -> None:
    draw_background(c, 1)
    c.setFillColor(Color(0.025, 0.050, 0.080))
    c.circle(W * 0.70, H * 0.48, 180, stroke=0, fill=1)
    c.setStrokeColor(CYAN)
    c.setLineWidth(1.5)
    for r in [85, 118, 152]:
        c.circle(W * 0.70, H * 0.48, r, stroke=1, fill=0)
    c.setStrokeColor(MAGENTA)
    c.setLineWidth(1)
    c.arc(W * 0.70 - 160, H * 0.48 - 160, W * 0.70 + 160, H * 0.48 + 160, 15, 115)
    c.setFillColor(AMBER)
    c.circle(W * 0.70 + 64, H * 0.48 + 42, 8, stroke=0, fill=1)
    c.setFillColor(GREEN)
    c.circle(W * 0.70 - 82, H * 0.48 - 48, 5, stroke=0, fill=1)
    c.setFillColor(RED)
    c.circle(W * 0.70 + 13, H * 0.48 - 108, 4.5, stroke=0, fill=1)

    draw_tag(c, 48, H - 86, "Concept Brief", CYAN)
    c.setFillColor(TEXT)
    c.setFont("Helvetica-Bold", 46)
    c.drawString(48, H - 154, "2D Space MORPG")
    c.setFont("Helvetica-Bold", 25)
    c.setFillColor(CYAN)
    c.drawString(50, H - 190, "Fog. Energy. Ships. Risk.")
    draw_text(
        c,
        "A server-authoritative browser and desktop space game that combines real-time ship control, radar-gated fog of war, planet discovery, energy economy, PvP risk, and DarkOrbit-like ship progression.",
        52,
        H - 242,
        430,
        "Helvetica",
        13,
        18,
        TEXT,
    )

    y = H - 338
    x = 54
    for label, color in [
        ("Browser + Desktop", CYAN),
        ("Server Authoritative", GREEN),
        ("Pixel Space Canvas", AMBER),
        ("Persistent Economy", MAGENTA),
    ]:
        w = draw_tag(c, x, y, label, color)
        x += w + 10

    c.setFillColor(MUTED)
    c.setFont("Helvetica", 9)
    c.drawString(52, 38, "Prepared as an early concept deck - June 2026")
    c.showPage()


def page_pitch(c: canvas.Canvas) -> None:
    draw_header(c, "The Pitch", "Why it is interesting", 2, 2)
    draw_text(
        c,
        "A real-time space game where information is the most valuable resource.",
        56,
        H - 150,
        620,
        "Helvetica-Bold",
        24,
        31,
        TEXT,
    )
    draw_text(
        c,
        "Players fly one active ship through a fog-covered galaxy, discover planets and resources, build an energy network, upgrade their hangar, and risk cargo in dangerous PvP sectors.",
        58,
        H - 225,
        680,
        "Helvetica",
        13,
        18,
        MUTED,
    )
    cards = [
        ("Radar-Gated Interaction", "You cannot attack, loot, gather, or even know about entities unless your ship or network can detect them.", CYAN),
        ("Energy Economy", "Discovered planets and stations produce energy that powers crafting, laser ammo, wormholes, upgrades, and strategic expansion.", AMBER),
        ("Ship Identity", "Every chassis has different slots, stats, passive bonuses, and saved loadouts. One active ship, many hangar choices.", GREEN),
        ("Risk Zones", "Safe space teaches the game. Deep space pays better, enables PvP, and can drop cargo when you die.", RED),
    ]
    x0, y0 = 52, 72
    cw, ch = 176, 148
    for i, (title, body, accent) in enumerate(cards):
        draw_card(c, x0 + i * (cw + 16), y0, cw, ch, title, body, accent, 11.5, 9.2)
    c.showPage()


def page_loop(c: canvas.Canvas) -> None:
    draw_header(c, "Core Game Loop", "The player's long-term engine", 3, 3)
    center = (W / 2, H / 2 - 12)
    labels = [
        ("Explore Fog", CYAN),
        ("Discover Planets", GREEN),
        ("Produce Energy", AMBER),
        ("Upgrade Ship", MAGENTA),
        ("Fight / Gather", RED),
        ("Haul / Sell", PURPLE),
        ("Expand Network", Color(0.55, 0.95, 0.95)),
    ]
    radius = 176
    points = []
    for i, (label, color) in enumerate(labels):
        angle = math.radians(100 - i * 360 / len(labels))
        points.append((center[0] + math.cos(angle) * radius, center[1] + math.sin(angle) * radius, label, color))
    for i in range(len(points)):
        x1, y1, _, color = points[i]
        x2, y2, _, _ = points[(i + 1) % len(points)]
        draw_arrow(c, x1, y1, x2, y2, blend(color, LINE, 0.35))
    for x, y, label, color in points:
        draw_flow_node(c, x, y, label, color, 124, 46)

    rounded_rect(c, center[0] - 138, center[1] - 44, 276, 88, 14, Color(0.035, 0.055, 0.088), Color(0.150, 0.250, 0.320))
    draw_centered_text(c, "Player Fantasy", center[0] - 122, center[1] + 19, 244, "Helvetica-Bold", 16, TEXT)
    draw_centered_text(c, "Push into unknown space, return richer or lose cargo trying.", center[0] - 118, center[1] - 7, 236, "Helvetica", 10.5, MUTED)
    c.showPage()


def page_moment(c: canvas.Canvas) -> None:
    draw_header(c, "Moment-to-Moment Play", "What the player does on the canvas", 4, 4)
    bullets = [
        Bullet("Click or command the ship to move across a 2D space map.", CYAN),
        Bullet("Scan fog, classify signals, and reveal planets, NPCs, loot, or enemy ships.", GREEN),
        Bullet("Choose tasks such as mining, salvaging, scanning, hauling, or fighting.", AMBER),
        Bullet("Use default auto-attack for farming, then manually trigger rockets, modules, scanners, or defensive tools during harder fights.", MAGENTA),
        Bullet("Return to stations or planets to sell cargo, repair ships, swap loadouts, and craft upgrades.", RED),
    ]
    draw_bullets(c, bullets, 56, H - 155, 360, 12.2, 17.5)

    x, y = 484, 108
    rounded_rect(c, x, y, 300, 318, 16, Color(0.032, 0.050, 0.080), Color(0.160, 0.240, 0.320))
    c.setStrokeColor(Color(0.090, 0.160, 0.240))
    c.setLineWidth(0.8)
    for i in range(1, 6):
        c.line(x + 18, y + i * 48, x + 282, y + i * 48)
        c.line(x + i * 46, y + 18, x + i * 46, y + 300)
    c.setFillColor(CYAN)
    c.circle(x + 155, y + 170, 11, stroke=0, fill=1)
    c.setStrokeColor(CYAN)
    c.setLineWidth(1.1)
    c.circle(x + 155, y + 170, 72, stroke=1, fill=0)
    c.setFillColor(RED)
    c.circle(x + 222, y + 206, 6, stroke=0, fill=1)
    c.setFillColor(AMBER)
    c.circle(x + 92, y + 96, 7, stroke=0, fill=1)
    c.setFillColor(GREEN)
    c.circle(x + 196, y + 102, 5, stroke=0, fill=1)
    c.setFillColor(MUTED)
    c.setFont("Helvetica", 8.8)
    c.drawString(x + 26, y + 290, "Canvas map: live entities only")
    draw_tag(c, x + 26, y + 27, "Fog hides server-side", CYAN)
    c.showPage()


def page_fog(c: canvas.Canvas) -> None:
    draw_header(c, "Fog of War as a Rule", "Not just a visual effect", 5, 5)
    draw_text(
        c,
        "The server has the truth. Each client receives only the entities and intel that player is allowed to know. If radar cannot detect it, the browser never receives it.",
        54,
        H - 150,
        705,
        "Helvetica-Bold",
        15,
        21,
        TEXT,
    )
    states = [
        ("Unknown", "No data is sent. The client cannot inspect, target, loot, or infer the entity.", RED),
        ("Weak Signal", "Approximate contact. Maybe a planet, ship, loot, anomaly, or noise.", AMBER),
        ("Visible", "Live authoritative updates: position, velocity, HP, shield, state.", GREEN),
        ("Last Known", "Stale intel after contact is lost. Useful, but no longer live truth.", CYAN),
    ]
    x = 52
    for title, body, accent in states:
        draw_card(c, x, 240, 178, 122, title, body, accent, 12, 9.3)
        x += 192
    draw_card(
        c,
        74,
        74,
        692,
        112,
        "Visibility Pipeline",
        [
            "Spatial grid candidates -> exact distance -> sensor strength -> stealth/signature -> nebula/jammer penalties -> line of sight -> per-player replication delta.",
            "Interactions such as attack, loot, gather, and portal use are rejected unless the relevant entity is currently visible or sensor-locked.",
        ],
        CYAN,
        13,
        10.2,
    )
    c.showPage()


def page_energy(c: canvas.Canvas) -> None:
    draw_header(c, "Planets and Energy", "The strategic layer", 6, 6)
    draw_text(
        c,
        "Exploration is not cosmetic. New planets unlock production, crafting paths, radar coverage, and long-distance logistics.",
        54,
        H - 145,
        560,
        "Helvetica-Bold",
        16,
        22,
        TEXT,
    )
    draw_card(
        c,
        52,
        256,
        348,
        160,
        "Planetary Energy",
        [
            "Produced by claimed or developed planets.",
            "Spent on laser ammo production, buildings, upgrades, crafting, wormhole upkeep, station boosts, and strategic projects.",
            "Creates a reason to explore and defend territory.",
        ],
        AMBER,
    )
    draw_card(
        c,
        438,
        256,
        348,
        160,
        "Ship Capacitor",
        [
            "Local combat energy pool on the active ship.",
            "Spent by lasers, shields, radar pulses, warp bursts, jammers, and active modules.",
            "Determined by chassis, generators, modules, and buffs.",
        ],
        CYAN,
    )
    draw_arrow(c, 400, 333, 438, 333, AMBER)
    draw_text(
        c,
        "Key balance idea: planets help you build and sustain power, but combat is still limited by the ship's capacitor and fitted modules. Economy fuels progression; moment-to-moment fights remain tactical.",
        82,
        126,
        670,
        "Helvetica",
        13,
        18,
        MUTED,
    )
    c.showPage()


def page_ships(c: canvas.Canvas) -> None:
    draw_header(c, "Ships, Hangar, and Roles", "One active ship, many choices", 7, 7)
    draw_text(
        c,
        "The player owns multiple ships but deploys one active ship at a time. Each chassis has different slots, stats, passives, and saved loadouts.",
        54,
        H - 144,
        700,
        "Helvetica-Bold",
        15,
        21,
        TEXT,
    )
    roles = [
        ("Support Relay", "Energy regen aura, radar support, fleet sustain.", GREEN),
        ("Bulwark Hauler", "High shield and cargo, few lasers, slow but valuable.", AMBER),
        ("Interceptor", "Fast, armed, low cargo. Built for PvP hunters.", RED),
        ("Explorer", "Strong radar, anomaly detection, fog scouting.", CYAN),
        ("Industrial Miner", "Mining bonuses, cargo modules, weak in combat.", PURPLE),
    ]
    x0, y0 = 44, 240
    for i, (title, body, accent) in enumerate(roles):
        draw_card(c, x0 + i * 157, y0, 143, 130, title, body, accent, 10.5, 8.8)
    draw_card(
        c,
        62,
        74,
        716,
        112,
        "Loadout Promise",
        [
            "A more expensive ship is not automatically best at everything. It unlocks a different identity: more laser slots, stronger shields, bigger cargo, radar dominance, support auras, or specialist utility.",
            "If your active ship is destroyed, it must be repaired. You can deploy another hangar ship while waiting, which keeps death meaningful without ending the session.",
        ],
        MAGENTA,
        13,
        10.2,
    )
    c.showPage()


def page_combat(c: canvas.Canvas) -> None:
    draw_header(c, "Combat and PvP Risk", "Server truth decides every hit", 8, 8)
    draw_card(
        c,
        52,
        260,
        348,
        154,
        "Attack Validation",
        [
            "Target visible or sensor-locked.",
            "Weapon equipped and cooldown ready.",
            "Range, line of sight, safe-zone, and faction rules pass.",
            "Hit chance, shield, armor, and damage calculated server-side.",
        ],
        RED,
    )
    draw_card(
        c,
        438,
        260,
        348,
        154,
        "Auto + Manual Combat",
        [
            "Default lasers can auto-fire for farming.",
            "Manual rockets, modules, scans, jammers, shields, and burst skills create skill expression.",
            "Energy/capacitor pressure becomes the combat rhythm.",
        ],
        AMBER,
    )
    zones = [
        ("Safe", "Low reward, no open PvP.", GREEN),
        ("Contested", "PvP enabled, better resources.", AMBER),
        ("Deep Space", "Higher reward, cargo drop risk.", RED),
        ("Dead Space", "Radar distortion, rare finds, highest danger.", MAGENTA),
    ]
    x = 60
    for title, body, accent in zones:
        draw_card(c, x, 76, 170, 112, title, body, accent, 11.5, 9.2)
        x += 188
    c.showPage()


def page_wormholes(c: canvas.Canvas) -> None:
    draw_header(c, "Wormholes and Logistics", "Exploration becomes infrastructure", 9, 9)
    draw_text(
        c,
        "Discovered planets can become strategic anchors. Wormholes are not free teleporters; they are expensive logistics links that consume energy and create territory value.",
        54,
        H - 145,
        690,
        "Helvetica-Bold",
        15,
        21,
        TEXT,
    )
    left_x, right_x, cy = 188, 650, 292
    for x, label, accent in [(left_x, "Planet A", GREEN), (right_x, "Planet B", CYAN)]:
        c.setFillColor(Color(0.040, 0.075, 0.105))
        c.circle(x, cy, 52, stroke=0, fill=1)
        c.setStrokeColor(accent)
        c.setLineWidth(2.4)
        c.circle(x, cy, 62, stroke=1, fill=0)
        c.setFillColor(accent)
        c.circle(x, cy, 7, stroke=0, fill=1)
        draw_centered_text(c, label, x - 55, cy - 82, 110, "Helvetica-Bold", 12, TEXT)
    c.setStrokeColor(MAGENTA)
    c.setLineWidth(3.0)
    c.bezier(left_x + 64, cy + 8, 330, cy + 88, 510, cy - 88, right_x - 64, cy - 8)
    draw_tag(c, 350, cy + 14, "Energy upkeep", AMBER)
    draw_tag(c, 362, cy - 32, "Cargo / mass limit", MAGENTA)
    draw_card(
        c,
        82,
        72,
        680,
        118,
        "Strategic Use",
        [
            "Build routes between claimed or discovered worlds.",
            "Move faster across your own network, but pay upkeep and expose infrastructure.",
            "PvP groups can raid supply routes, disrupt anchors, or fight over high-value planets.",
        ],
        CYAN,
        13,
        10.4,
    )
    c.showPage()


def page_architecture(c: canvas.Canvas) -> None:
    draw_header(c, "Server Architecture", "How the game stays authoritative", 10, 10)
    layers = [
        ("Browser / Tauri Client", "Sends intent only: move, scan, attack, gather, loot.", CYAN),
        ("WebSocket Gateway", "Owns live connections and routes commands.", GREEN),
        ("World Router", "Knows which map worker owns each player.", AMBER),
        ("Map Workers", "Own map state, spatial grid, fog, combat, loot, timers.", MAGENTA),
        ("PostgreSQL / Redis / NATS", "Persistence, sessions, cache, events, async jobs.", PURPLE),
    ]
    x, y = 116, H - 162
    prev = None
    for i, (title, body, accent) in enumerate(layers):
        draw_card(c, x, y - i * 78, 610, 56, title, body, accent, 12, 9.3)
        if prev:
            draw_arrow(c, W / 2, prev - 4, W / 2, y - i * 78 + 58, blend(accent, LINE, 0.25))
        prev = y - i * 78
    draw_text(
        c,
        "Portal transitions are logical routing changes. The socket can stay connected while the player moves from Map A to Map B.",
        108,
        65,
        624,
        "Helvetica-Bold",
        13,
        18,
        TEXT,
    )
    c.showPage()


def page_mvp(c: canvas.Canvas) -> None:
    draw_header(c, "First Vertical Slice", "Build the hardest truth early", 11, 11)
    draw_text(
        c,
        "The first prototype should prove the core architecture before adding content volume.",
        54,
        H - 143,
        640,
        "Helvetica-Bold",
        16,
        22,
        TEXT,
    )
    items = [
        Bullet("One gateway, one server process, two maps, one portal.", CYAN),
        Bullet("One active player ship with server-authoritative movement.", GREEN),
        Bullet("One NPC enemy with range, damage, death, respawn, and loot drop.", RED),
        Bullet("One resource node with gather ticks, cargo limit, and respawn.", AMBER),
        Bullet("Server-side fog: hidden entities are never sent to the client.", MAGENTA),
        Bullet("PixiJS canvas client with ship, radar radius, fog overlay, loot, and panels.", PURPLE),
    ]
    draw_bullets(c, items, 70, H - 205, 420, 12, 17)
    draw_card(
        c,
        530,
        124,
        248,
        268,
        "Success Criteria",
        [
            "A player can explore unknown space.",
            "Detection controls what appears on screen.",
            "Portal travel moves the player between maps.",
            "Combat, loot, and gathering are all validated by the server.",
            "The prototype already feels like the final game loop in miniature.",
        ],
        CYAN,
        13,
        10.5,
    )
    c.showPage()


def page_refs(c: canvas.Canvas) -> None:
    draw_header(c, "Inspirations and Differentiation", "What we borrow and what we change", 12, 12)
    refs = [
        ("Dark Forest", "Cryptographic fog of war, exploration, planets, artifacts, wormholes. We borrow the information-war fantasy without using blockchain.", CYAN),
        ("DarkOrbit", "Readable 2D space combat, portals, ship identity, lasers, PvE/PvP farming loops. We make it more server-authoritative and exploration-heavy.", RED),
        ("OGame / Travian", "Long-term browser strategy, resource loops, alliances, timers, and macro progression. We add a live ship and tactical map layer.", AMBER),
        ("EVE Online / EVE Echoes", "Mining, hauling, PvP risk, player economy, space roles. We keep the client lighter and more approachable.", GREEN),
    ]
    y = 354
    for title, body, accent in refs:
        draw_card(c, 58, y, 724, 74, title, body, accent, 13, 10.2)
        y -= 88
    c.setFillColor(MUTED)
    c.setFont("Helvetica", 8.4)
    c.drawString(60, 52, "Reference URLs: darkforest.zkga.me/blog, darkorbit.com, gameforge.com/ogame, kingdoms.com, eveonline.com")
    c.showPage()


def build_pdf(path: str = OUT_PATH) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    c = canvas.Canvas(path, pagesize=PAGE_SIZE)
    c.setTitle("2D Space MORPG Concept Brief")
    c.setAuthor("Codex")
    page_cover(c)
    page_pitch(c)
    page_loop(c)
    page_moment(c)
    page_fog(c)
    page_energy(c)
    page_ships(c)
    page_combat(c)
    page_wormholes(c)
    page_architecture(c)
    page_mvp(c)
    page_refs(c)
    c.save()


if __name__ == "__main__":
    build_pdf()
    print(OUT_PATH)
