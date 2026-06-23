# Phase 04 - Runtime Loader And Catalog Assembly

## Goal

Runtime loads current published CMS snapshot and assembles existing domain
catalogs from it.

No admin editing yet. Server start/restart picks up published content.

## Runtime Flow

```text
contentdb.Open
contentdb.Migrate/Verify
contentseed.EnsurePublishedSeed
contentdb.LoadCurrentPublishedSnapshot
content.ValidateSnapshot
contentassembly.BuildRuntimeCatalogs
runtime installs catalogs/services
```

Load/assemble must happen before:

- map workers/spawners
- hangar/loadout services
- crafting service
- production settlement/building services
- NPC loot selector

## Assembly Boundary

New package:

```text
internal/game/contentassembly/
  catalogs.go
  items.go
  modules.go
  ships.go
  shop.go
  loot.go
  crafting.go
  production.go
  maps_npc.go
```

Assembly maps CMS definitions into existing domain structs:

```text
economy.ItemDefinition
modules.ModuleDefinition
ships.ShipDefinition
catalog.ContentRegistry
loot.LootTable
crafting.RecipeDefinition
production.BuildingProductionDefinition
world/maps enemy definitions
```

Domain packages keep validation ownership. Assembly must call existing
constructors/validators, not bypass them.

## Runtime Struct

Runtime should hold:

```text
ContentVersion catalog.Version
ContentSnapshot content.Snapshot
ItemCatalog map[foundation.ItemID]economy.ItemDefinition
ModuleCatalog modules.Catalog
ShipCatalog ships.Catalog
Content catalog.ContentRegistry
Recipes crafting.RecipeCatalog
lootTables map[string]loot.LootTable
ProductionCatalog production.Catalog
MapCatalog *worldmaps.Catalog
HistoricalContent content.HistoricalResolver
```

Names can adapt to current code, but content version must be visible in logs and
safe server payloads.

## Failure Policy

- DB unavailable in real mode -> boot fails.
- No published content after seed attempt -> boot fails.
- Invalid snapshot -> boot fails.
- Dev/test may use explicit fallback flag only.
- DB disabled while `GAME_CONTENT_MODE=required` -> boot fails.

No silent fallback from invalid DB content to hard-coded catalogs.

## Hardcoded Catalog Removal Rule

Normal runtime paths must not call migrated `MustMVP...` or `MVPCatalog()`
helpers after their phase completes.

Known direct-call targets to replace with injected catalogs/providers:

```text
modules.MustMVPCatalog()
crafting.MVPRecipeCatalog()
production.MustMVPCatalog()
production.MVPCatalog()
runtimeLootCatalog()
buildRuntimeContentRegistry()
worldmaps.StarterCatalog() enemy-only parts
```

Test helpers may keep MVP helpers.

## Client Safe Projection

Only expose:

- display names
- art keys
- categories
- player-visible stats
- availability/locked reason
- catalog version

Never expose:

- loot chances/weights
- hidden spawn pools
- NPC future spawn config
- procedural seeds
- map internals with `json:"-"`

## Tests

- loader uses DB-published module stat in runtime module catalog
- loader uses DB-published loot table in NPC loot selector
- invalid published content prevents runtime creation
- safe projection omits hidden fields
- DB disabled in required mode fails boot
- migrated runtime call sites use injected catalogs, not MVP helpers

Commands:

```bash
go test ./internal/game/contentassembly ./internal/game/server -run 'Content|Catalog|Loot|Runtime' -count=1
git diff --check
```

## Done

- runtime no longer directly calls old MVP catalog functions for migrated
  content in normal boot path
- DB-published content controls runtime definitions
- tests prove changed DB value affects gameplay catalog
