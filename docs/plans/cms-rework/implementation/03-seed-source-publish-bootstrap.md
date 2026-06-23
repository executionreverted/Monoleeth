# Seed Source Publish Bootstrap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Seed empty content DB from existing code catalogs and create first published snapshot.

**Architecture:** `contentseed` compiles current MVP catalogs into CMS snapshot. `contentdb` transaction inserts rows/version/audit only when DB empty.

**Tech Stack:** Go, existing catalog packages, content snapshot validators, Postgres transaction.

---

### Task 1: Add Seed Package Skeleton

**Files:**
- Create: `internal/game/contentseed/doc.go`
- Create: `internal/game/contentseed/snapshot.go`
- Test: `internal/game/contentseed/snapshot_test.go`

**Steps:**
1. Add `BuildMVPSnapshot(worldID world.WorldID) (content.Snapshot, error)`.
2. Initially return empty groups with version `content_mvp_seed_v1`.
3. Test validates version and fails until required groups added.

### Task 2: Compile Items And Modules

**Files:**
- Create: `internal/game/contentseed/items.go`
- Create: `internal/game/contentseed/modules.go`
- Test: `internal/game/contentseed/items_modules_test.go`

**Steps:**
1. Pull module rows from `modules.MVPModuleDefinitions()`.
2. Pull item rows from `runtimeLootCatalog` logic by moving pure item seed code if needed.
3. Do not import `server` from seed package. Extract pure seed helpers into contentseed.
4. Test every module has item row.

### Task 3: Compile Ships Shop Loot Craft Production NPC

**Files:**
- Create: `internal/game/contentseed/ships.go`
- Create: `internal/game/contentseed/shop.go`
- Create: `internal/game/contentseed/loot.go`
- Create: `internal/game/contentseed/crafting.go`
- Create: `internal/game/contentseed/production.go`
- Create: `internal/game/contentseed/npc.go`

**Steps:**
1. Map existing MVP definitions into content DTOs.
2. Keep old display names/art keys where present.
3. Preserve old IDs in first migration.
4. Test `BuildMVPSnapshot` passes full content validation.

### Task 4: Add Bootstrap Service

**Files:**
- Create: `internal/game/contentseed/bootstrap.go`
- Test: `internal/game/contentseed/bootstrap_test.go`

**Steps:**
1. Add `EnsurePublishedSeed(ctx, store, snapshot)`:
   - if store has any content -> no-op
   - else validate snapshot
   - insert typed draft rows
   - insert current published version
   - write audit row
2. Use store transaction hook or method to avoid race.
3. Use DB/advisory lock in Postgres implementation.
4. Test empty seeds once, non-empty no-op, concurrent calls create one current version.

### Task 5: Wire Runtime Bootstrap Behind Config

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/config.go`

**Steps:**
1. If content DB enabled, open DB, migrate/verify, call seed bootstrap.
2. Still leave old runtime catalogs active only through explicit dev fallback until Phase 04.
3. Add test with fake store if runtime seams exist; else keep unit tests at contentseed/contentdb.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/contentdb -count=1
git diff --check
```

### Commit

```bash
git add internal/game/contentseed internal/game/server internal/game/contentdb
git commit -m "game: seed published content snapshot"
```
