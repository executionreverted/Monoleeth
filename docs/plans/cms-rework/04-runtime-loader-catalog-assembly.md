# Phase 04 - Runtime Loader And Catalog Assembly

## Goal

Runtime loads current published CMS snapshot through a DB-backed
`content.Repository` and installs validated `GameplayContent`.

No admin editing yet. Server start/restart picks up published content.

## Runtime Flow

```text
contentdb.Open
contentdb.Migrate/Verify
contentseed.EnsurePublishedSeed
contentdb.NewRepository
content.LoadPublishedContent(ctx, repository, worldID)
runtime installs catalogs/services from GameplayContent
```

Load/assemble must happen before:

- map workers/spawners
- hangar/loadout services
- crafting service
- production settlement/building services
- NPC loot selector

## Repository Boundary

Existing boundary:

```text
internal/game/content/repository.go
internal/game/content/bundle.go
internal/game/contentdb/
```

The DB repository maps CMS rows/snapshots into the existing bundle fields:

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

Domain packages keep validation ownership. DB mapping must call existing
constructors/validators and then `GameplayContent.Validate()`.

## Runtime Struct

Runtime should hold:

```text
ContentVersion catalog.Version
ContentRepository content.Repository
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

Normal runtime paths outside `internal/game/content.DefaultGameplayContent`
must not call migrated `MustMVP...` or `MVPCatalog()` helpers after their phase
completes.

Known direct-call targets to replace with injected catalogs/providers:

```text
modules.MustMVPCatalog()
crafting.MVPRecipeCatalog()
runtimeLootCatalog()
worldmaps.StarterCatalog() enemy-only parts
```

Test helpers may keep MVP helpers.

Production `MustMVPCatalog()`/`MVPCatalog()` calls are now allowed only in seed
fixtures, default content assembly, and tests. Normal production settlement,
route energy, and planet building mutation paths use the runtime
`production.Catalog` loaded through `GameplayContent`.

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

- DB repository uses DB-published module stat in runtime module catalog
- loader uses DB-published loot table in NPC loot selector
- invalid published content prevents runtime creation
- safe projection omits hidden fields
- DB disabled in required mode fails boot
- migrated runtime call sites use injected catalogs, not MVP helpers

Commands:

```bash
go test ./internal/game/content ./internal/game/contentdb ./internal/game/server -run 'Content|Catalog|Loot|Runtime' -count=1
git diff --check
```

## Done

- runtime content boot can load current published DB content or explicit
  static dev/test fallback
- DB-published core content controls runtime definitions
- tests prove changed DB value affects gameplay catalog
- production store, route energy, settlement, and planet building mutation paths
  use the runtime production catalog

Remaining known normal-runtime static catalog call:

```text
quests.MustMVPQuestCatalog()
```
