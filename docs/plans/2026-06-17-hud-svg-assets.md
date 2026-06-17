# HUD SVG Assets Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a mockup-aligned SVG HUD asset pack under `output/assets/hud-svg/`.

**Architecture:** Use a small deterministic generator script to write individual SVG files with shared style tokens. The generated files stay plain SVG so they can be opened, edited, imported, or later converted into a sprite sheet.

**Tech Stack:** Python standard library for generation, SVG markup, existing `output/` asset workspace.

---

### Task 1: Generator

**Files:**
- Create: `scripts/generate_hud_svg_assets.py`
- Create output: `output/assets/hud-svg/**`

**Steps:**
1. Define shared SVG colors, stroke styles, and helpers.
2. Generate icon, marker, panel, button, slot, and segmented bar SVG files.
3. Write a manifest and README into the output folder.
4. Run the generator.

### Task 2: Roadmap Note

**Files:**
- Modify: `docs/roadmap/11-browser-client-prototype.md`

**Steps:**
1. Update asset prep notes to mention the SVG HUD pack.
2. Keep Phase 11 state and checklist untouched because client implementation is still not started.

### Task 3: Verification

**Steps:**
1. Parse every generated SVG as XML.
2. Count generated SVG files and inspect the manifest.
3. Run `go test ./...`.
4. Run `git diff --check`.
