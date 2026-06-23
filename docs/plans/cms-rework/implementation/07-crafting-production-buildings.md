# Crafting Production Buildings Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Load craft recipes and production/building definitions from CMS snapshot.

**Architecture:** Definitions move to CMS. Player craft jobs and planet state stay existing services. Versioned source refs remain attached to long-lived state.

**Tech Stack:** Go content validators, `internal/game/crafting`, `internal/game/production`.

---

### Task 1: Validate Craft Recipes

**Files:**
- Modify: `internal/game/content/crafting.go`
- Test: `internal/game/content/crafting_test.go`

**Steps:**
1. Validate recipe ID/category/output/input/duration/rank/fees.
2. Validate location/building requirement enums.
3. Cross-ref input/output item/ship IDs.

### Task 2: Assemble Craft Recipes

**Files:**
- Modify: `internal/game/contentassembly/crafting.go`
- Test: `internal/game/contentassembly/crafting_test.go`

**Steps:**
1. Convert CMS recipe to `crafting.RecipeDefinition`.
2. Set `Source.Version` to CMS content version.
3. Build `crafting.NewRecipeCatalog`.
4. Test craft job source version uses CMS version.
5. Test old craft job completes after publish/restart or publish blocks while active.

### Task 3: Validate Production Buildings

**Files:**
- Modify: `internal/game/content/production.go`
- Test: `internal/game/content/production_test.go`

**Steps:**
1. Validate building type/category/level.
2. Validate input/output rates and energy cost.
3. Require positive production energy cost where current domain validator requires it.
4. Reject duplicate building type + level.
5. Cross-ref item IDs.

### Task 4: Assemble Production Catalog

**Files:**
- Modify: `internal/game/contentassembly/production.go`
- Test: `internal/game/contentassembly/production_test.go`

**Steps:**
1. Convert CMS rows to `production.BuildingProductionDefinition`.
2. Build `production.NewCatalog`.
3. Test DB rate affects settlement calculation.
4. Test existing building source/version resolves through injected/historical catalog.

### Task 5: Runtime Wiring

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/planet_building_handlers.go`
- Test: `internal/game/server/server_crafting_location_test.go`
- Test: `internal/game/server/server_production_summary_settlement_test.go`

**Steps:**
1. Ensure runtime crafting service receives assembled recipes.
2. Ensure production handlers use assembled production catalog, not `MustMVPCatalog`.
3. Replace direct `production.MustMVPCatalog()` and `production.MVPCatalog()` paths in runtime/transaction code.
4. Add guard test proving settlement/building transactions do not call MVP catalog.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentassembly ./internal/game/crafting ./internal/game/production ./internal/game/server -run 'Craft|Production|Building|Content' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentassembly internal/game/server internal/game/crafting internal/game/production
git commit -m "game: move crafting and production content to cms"
```
