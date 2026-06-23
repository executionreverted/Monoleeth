# Content Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a validated static gameplay content bundle that prepares monsters, drops, items, recipes, maps, and production content for later DB/CMS editing.

**Architecture:** Add an `internal/game/content` package that aggregates existing catalog types without becoming gameplay logic. The server runtime will load item and loot catalogs through this bundle while behavior stays unchanged.

**Tech Stack:** Go domain packages, existing `internal/game/*` catalogs, focused `go test` package tests.

**Demo Balance Direction:** Keep content DarkOrbit-like for the vertical slice:
starter PvE drones, tougher border PvP NPCs, low-stack salvage drops, bounded
sector travel through portals, safe-zone protection, scanner/radar-driven
discovery, and generic placeholder names that can be rebalanced later.

---

### Task 1: Static Bundle And Cross-Reference Validator

**Files:**
- Create: `internal/game/content/bundle.go`
- Create: `internal/game/content/validation.go`
- Test: `internal/game/content/bundle_test.go`

**Step 1: Write failing tests**

Create tests for:

- `DefaultGameplayContent()` succeeds.
- Removing an item used by a loot table fails.
- Changing an enemy drop profile to an unknown loot table fails.
- Changing a recipe input to an unknown item fails.
- Changing a ship-unlock recipe to an unknown ship fails.
- Changing a production output to an unknown item fails.

**Step 2: Run focused failing test**

Run:

```bash
go test ./internal/game/content -count=1
```

Expected: fail because package does not exist.

**Step 3: Implement minimal bundle**

Add `GameplayContent` with:

- `Items map[foundation.ItemID]economy.ItemDefinition`
- `LootTables map[string]loot.LootTable`
- `Modules modules.Catalog`
- `Ships ships.Catalog`
- `Recipes crafting.RecipeCatalog`
- `Production production.Catalog`
- `Maps *maps.Catalog`
- `Scanner ScannerContent`

Add `DefaultGameplayContent(worldID world.WorldID)` to assemble current static
content.

**Step 4: Implement validator**

Add `Validate() error` and helpers for:

- known item refs
- known ship refs
- known loot-table refs from map drop profiles
- recipe item/ship refs
- production item refs
- server-only scanner seed, bounded candidate options, radar-level unit, and
  discovery XP amount

**Step 5: Run focused test**

Run:

```bash
go test ./internal/game/content -count=1
```

Expected: pass.

**Step 6: Commit**

```bash
git add internal/game/content docs/plans/2026-06-24-content-foundation-design.md docs/plans/2026-06-24-content-foundation.md
git commit -m "game: add validated content bundle"
```

### Task 2: Runtime Loading Boundary

**Files:**
- Modify: `internal/game/server/combat_loot_catalog.go`
- Modify: runtime constructor file that currently calls `runtimeLootCatalog`
- Test: existing server/content focused tests

**Step 1: Route runtime content through bundle**

Replace scattered item/loot/scanner assembly calls with
`content.DefaultGameplayContent`.

**Step 2: Preserve existing runtime data**

Keep returned item, loot tables, scanner seed, bounded candidate options,
radar-level unit, and discovery XP identical to current playtest behavior.

**Step 3: Run focused runtime tests**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'Test.*Content|TestNPCLootSelector|TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner' -count=1
```

Expected: pass.

**Step 4: Commit**

```bash
git add internal/game/content internal/game/server
git commit -m "game: load runtime catalogs through content bundle"
```

### Task 3: Docs And Final Checks

**Files:**
- Modify: `docs/todo.md`
- Modify: optional content status doc if needed

**Step 1: Record CMS prerequisites**

Document remaining work:

- DB content repository
- revisioned drafts/publish/rollback
- admin UI
- balancing workflow

**Step 2: Run narrow checks**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'Test.*Content|TestNPCLootSelector|TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner' -count=1
git diff --check
```

**Step 3: Commit**

```bash
git add docs/todo.md
git commit -m "docs: record content foundation cms path"
```
