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
4. Use CMS DTO names such as `craft_duration_ms`; do not expose raw Go
   `time.Duration` fields as admin JSON.

### Task 2: Map Craft Recipes

**Files:**
- Modify: `internal/game/contentdb/map_crafting.go`
- Test: `internal/game/contentdb/map_crafting_test.go`

**Steps:**
1. Convert CMS recipe to `crafting.RecipeDefinition`.
2. Set `Source.Version` to CMS content version.
3. Build `crafting.NewRecipeCatalog`.
4. Test craft job source version uses CMS version.
5. Test old craft job completes after publish/restart or publish blocks while active.
6. Prove current-catalog edits cannot change refund, reservation, completion,
   duration, output, or location behavior for already-started jobs.

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

### Task 4: Map Production Catalog

**Files:**
- Modify: `internal/game/contentdb/map_production.go`
- Test: `internal/game/contentdb/map_production_test.go`

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

### Task 6: Safe Admin Edit Gate For Recipes

**Files:**
- Modify: `internal/game/admin` content validation/publish files
- Test: admin/content publish tests

**Steps:**
1. Allow `required_rank` recipe edits after validator proves bounds.
2. Keep `required_credits`, inputs, outputs, duration, enabled, recipe ID,
   output kind, and location/building gates read-only until active-job
   compatibility is enforced.
3. If those fields change in a draft while active jobs exist, publish must
   block with a clear validation report.
4. Test active-job incompatible recipe edit cannot publish.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentdb ./internal/game/crafting ./internal/game/production ./internal/game/server -run 'Craft|Production|Building|Content' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/content internal/game/contentdb internal/game/server internal/game/crafting internal/game/production
git commit -m "game: move crafting and production content to cms"
```
