# Runtime Loader Catalog Assembly Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Load runtime content from a DB-backed `content.Repository`.

**Architecture:** Runtime already installs `content.GameplayContent` from `content.LoadPublishedContent`. Add a `contentdb` repository adapter that maps current published CMS rows/snapshot into that bundle. Keep `content.StaticRepository` only for explicit dev/test fallback.

**Tech Stack:** Go domain constructors, existing `internal/game/content` validation, contentdb snapshot loader.

**Progress (TASK-0454):**
- [x] Add DB-backed `content.Repository` adapter in `internal/game/contentdb`.
- [x] Map enabled item, module, ship, loot, craft, production, and shop snapshot rows into `content.GameplayContent`.
- [x] Verify published map/NPC rows against `worldmaps.StarterCatalog(worldID)` with no silent fallback.
- [ ] Wire runtime/config to use `contentdb.Repository`.
- [ ] Add server fail-closed runtime/config tests.

---

### Task 1: Add DB Repository Adapter

**Files:**
- Modify/Create: `internal/game/contentdb/repository.go`
- Test: `internal/game/contentdb/repository_test.go`

**Steps:**
1. Implement a type that satisfies `content.Repository`.
2. Load only the current published version row selected by `is_current=true`.
3. Map snapshot/typed rows into `content.GameplayContent`.
4. Return an error when no current published version exists.
5. Test invalid mapped content fails through `content.LoadPublishedContent`.

### Task 2: Map Items Modules Ships Shop

**Files:**
- Create/Modify: `internal/game/contentdb/map_items.go`
- Create/Modify: `internal/game/contentdb/map_modules.go`
- Create/Modify: `internal/game/contentdb/map_ships.go`
- Create/Modify: `internal/game/contentdb/map_shop.go`
- Test: `internal/game/contentdb/map_items_modules_shop_test.go`

**Steps:**
1. Convert content items -> `economy.ItemDefinition`.
2. Convert modules -> `modules.ModuleDefinition`, then `modules.NewCatalog`.
3. Convert ships -> `ships.Catalog`.
4. Convert shop -> `catalog.ContentRegistry`.
5. Test LC1 damage/range/cooldown value survives assembly.

### Task 3: Map Loot Craft Production NPC

**Files:**
- Create/Modify: `internal/game/contentdb/map_loot.go`
- Create/Modify: `internal/game/contentdb/map_crafting.go`
- Create/Modify: `internal/game/contentdb/map_production.go`
- Create/Modify: `internal/game/contentdb/map_maps_npc.go`
- Test: `internal/game/contentdb/map_gameplay_test.go`

**Steps:**
1. Loot rows must resolve item definitions.
2. Craft recipes must resolve item/ship refs.
3. Production rates must resolve item refs.
4. Enemy pools/templates must preserve server-only fields.
5. Test missing ref rejects build.

### Task 4: Wire Runtime Loader

**Files:**
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/config.go`
- Test: `internal/game/server/server_content_runtime_test.go`

**Steps:**
1. Add `ContentRepository content.Repository` or content DB config seam.
2. Required DB path:
   - load current published snapshot by `is_current=true`
   - return validated `GameplayContent`
   - install into runtime exactly like static bundle
3. DB-disabled path allowed only for explicit `dev_fallback`/tests via `content.StaticRepository`.
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
5. Add scan/test for migrated runtime paths not calling MVP helpers outside `internal/game/content`.

### Verify

```bash
go test ./internal/game/content ./internal/game/contentdb ./internal/game/server -run 'Content|Runtime|Catalog' -count=1
git diff --check
```

### Commit

```bash
git add internal/game/contentdb internal/game/server
git commit -m "game: load runtime catalogs from content db"
```
