# Runtime Loader Catalog Assembly Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build runtime domain catalogs from current published CMS snapshot.

**Architecture:** `contentassembly` maps CMS DTOs into existing validated domain definitions. Runtime installs assembled catalogs instead of directly calling MVP catalog funcs when DB content enabled.

**Tech Stack:** Go domain constructors, existing validation, content snapshot loader.

---

### Task 1: Add Assembly Result

**Files:**
- Create: `internal/game/contentassembly/catalogs.go`
- Test: `internal/game/contentassembly/catalogs_test.go`

**Steps:**
1. Define `RuntimeCatalogs` with item/module/ship/shop/loot/recipe/production/map catalogs.
2. Include historical resolver for old content versions needed by durable state.
3. Add `Build(snapshot content.Snapshot, worldID world.WorldID)`.
4. First test returns error on invalid snapshot.

### Task 2: Assemble Items Modules Ships Shop

**Files:**
- Create: `internal/game/contentassembly/items.go`
- Create: `internal/game/contentassembly/modules.go`
- Create: `internal/game/contentassembly/ships.go`
- Create: `internal/game/contentassembly/shop.go`
- Test: `internal/game/contentassembly/items_modules_shop_test.go`

**Steps:**
1. Convert content items -> `economy.ItemDefinition`.
2. Convert modules -> `modules.ModuleDefinition`, then `modules.NewCatalog`.
3. Convert ships -> `ships.Catalog`.
4. Convert shop -> `catalog.ContentRegistry`.
5. Test LC1 damage/range/cooldown value survives assembly.

### Task 3: Assemble Loot Craft Production NPC

**Files:**
- Create: `internal/game/contentassembly/loot.go`
- Create: `internal/game/contentassembly/crafting.go`
- Create: `internal/game/contentassembly/production.go`
- Create: `internal/game/contentassembly/maps_npc.go`
- Test: `internal/game/contentassembly/gameplay_catalogs_test.go`

**Steps:**
1. Loot rows must resolve item definitions.
2. Craft recipes must resolve item/ship refs.
3. Production rates must resolve item refs.
4. Enemy pools/templates must preserve server-only fields.
5. Test missing ref rejects build.

### Task 4: Wire Runtime Loader

**Files:**
- Modify: `internal/game/server/runtime.go`
- Test: `internal/game/server/server_content_runtime_test.go`

**Steps:**
1. Add helper `runtimeContentCatalogs(config, db, worldID)`.
2. Required DB path:
   - load current published snapshot by `is_current=true`
   - assemble catalogs
   - install into runtime
3. DB-disabled path allowed only for explicit `dev_fallback`/tests.
4. Test fake snapshot changes module stat in runtime module catalog.

### Task 5: Fail Closed

**Files:**
- Modify: `internal/game/server/runtime.go`
- Test: `internal/game/server/server_content_runtime_test.go`

**Steps:**
1. DB enabled + invalid published snapshot -> `NewRuntime` error.
2. DB enabled + no published content after seed -> error.
3. Required mode + missing DB URL -> error.
4. No silent fallback.
5. Add scan/test for migrated runtime paths not calling MVP helpers.

### Verify

```bash
go test ./internal/game/contentassembly ./internal/game/server -run 'Content|Runtime|Catalog' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/contentassembly internal/game/server
git commit -m "game: load runtime catalogs from content db"
```
