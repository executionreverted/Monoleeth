# Old DarkOrbit Balance Seed Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Seed the gameplay content database with Old DarkOrbit 2009-style balance values for XP, ships, shop items, NPC stats, ores, and NPC metal drops.

**Architecture:** Keep runtime server-authoritative by compiling a typed seed bundle through `contentseed.BuildMVPSnapshot` only when the content DB is empty. Runtime content must load from either the published content DB rows or an explicitly injected test repository; it must not silently fall back to legacy/static catalogs.

**Tech Stack:** Go content bundle, contentseed snapshot rows, Postgres content DB seed path, Go tests.

---

### Task 1: Progression XP Table

Modify `internal/game/progression/xp_table.go` so the main XP thresholds match the legacy `experiencias.nave` curve for levels 1-33. Keep rank requirements MVP-scoped for now.

Test with `go test ./internal/game/progression -run TestMainXPTable -count=1`.

### Task 2: Legacy Ores And Drops

Add stackable item definitions for `xenomit`, `prometium`, `terbium`, `endurium`, `prometid`, `duranium`, and `promerium`. These are cargo resources, not normal inventory products: NPC loot pickup should place them in the active ship cargo hold and consume cargo capacity, and shop content must not sell them. Update NPC loot tables so existing spawned NPC archetypes drop the legacy metal rows from their corresponding legacy NPC balance.

Test with `go test ./internal/game/content ./internal/game/contentseed ./internal/game/contentdb -run 'TestDefaultGameplayContent|TestBuildMVPSnapshot|TestRepository' -count=1`.

### Task 3: Legacy Names For Temporary Catalog Parity

Rename starter ships/modules/shop display metadata to the legacy names for now, and keep IDs stable where existing runtime logic depends on them.

Test with `go test ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestNewRuntimeUsesPublishedItemShipShopRuntimeContent|TestNewRuntimeSeedsStarterLaserAndScannerLoadout' -count=1`.

### Task 4: Documentation

Update Phase 05 checklist and `docs/todo.md` to record that empty-DB seeding now includes the Old DarkOrbit-inspired balance slice, runtime no longer falls back to static content without an explicit test repository, and full CMS editing remains open.

Current DB-backed snapshot rows cover items, modules, ships, shop products, loot tables, NPC/map spawn content, craft recipes, production buildings, quest templates, quest rewards, scanner config, starter config, route policy, production rules, and combat rules. Typed defaults remain only as first-run seed builders and explicit test helpers.

### Task 5: Verification

Run:

```bash
go test ./...
git diff --check
```

For client-impacting catalog changes, run:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
```
