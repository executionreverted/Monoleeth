# Curated Entity Asset Catalog Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the generated isometric entity assets in the real browser playtest without shipping the full source asset pool.

**Architecture:** Treat `client/src/assets/entities/` as source art only. Copy a tiny curated runtime set into `client/src/assets/world/entities/` under deploy-safe names, then route the existing world renderer through a small catalog module. Keep server state unchanged; only visual asset URLs change.

**Tech Stack:** Browser client, Vite static asset imports, Pixi renderer registry, existing bundle scan and E2E playtest screenshot proof.

**Directional Follow-up:** Runtime now ships eight deploy-safe directions for
each curated entity family. Pixi keeps one `Sprite` per entity and swaps its
texture by server movement vector:

- `00` southwest
- `02` west
- `04` northwest
- `06` north
- `08` northeast
- `10` east
- `12` southeast
- `14` south

The source `client/src/assets/entities/` folders remain source art. The runtime
imports only `client/src/assets/world/entities/*_iso_XX.png` names so bundle
scan does not expose source model tokens.

---

## Task 1: Runtime-Safe Asset Selection

**Files:**
- Create: `client/src/assets/world/entities/ship_player_iso_east.png`
- Create: `client/src/assets/world/entities/npc_hostile_iso_east.png`
- Create: `client/src/assets/world/entities/loot_cache_iso_east.png`

**Steps:**
1. Copy frames `00`, `02`, `04`, `06`, `08`, `10`, `12`, and `14` from the
   chosen source assets.
2. Use deploy-safe names that do not include source folder tokens or `spin_512`.
3. Keep the full source directories untouched.

## Task 2: Catalog And Renderer Wiring

**Files:**
- Create: `client/src/render/world-entity-asset-catalog.ts`
- Modify: `client/src/render/world-renderer-assets.ts`
- Modify: `client/src/render/world-renderer-assets.test.ts`

**Steps:**
1. Add the direction convention for generated frames.
2. Add three runtime catalog entries: player ship, hostile NPC, loot cache.
3. Import direction URL maps into `world-renderer-assets.ts`.
4. Resolve facing from movement vectors and swap `Sprite.texture` in place.
5. Assert the renderer maps player/NPC/loot keys to curated PNG URLs.
6. Assert source asset tokens and `spin_512` are absent from runtime descriptors.

## Task 3: Docs And Verification

**Files:**
- Modify: `docs/playtest-vertical-slice-status.md`
- Modify: `docs/plans/2026-06-27-playable-v1-gap-closure.md`

**Steps:**
1. Record the selected assets and source manifests in docs only.
2. Mark only verified Milestone 7 checklist items.
3. Run:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run check
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
scripts/ci_playtest_artifact_gate.sh
git diff --check
```

Done when the browser playtest screenshot proof renders the selected curated PNGs and bundle scan stays green.
