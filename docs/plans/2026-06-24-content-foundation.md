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
- `Starter StarterContent`
- `Shop catalog.ContentRegistry`
- `Route RouteContent`
- `Rules ProductionRulesContent`
- `Combat CombatRulesContent`

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
- per-map scanner profile refs, duplicate profile rejection, and candidate
  option validation per bounded map
- server-only starter/playtest content refs: starter ship, starter module item
  grants, scanner module, weekly X Core stock, first-NPC seed overrides, claim
  core, and route seed stored items
- shop content refs and display/category metadata through
  `catalog.ContentRegistry`
- route content refs and caps: routeable resources, route count, max distance,
  cross-map penalty, endpoint storage capacity, energy formula, and loss band
- production rule refs and caps: claim range, claim production storage/energy
  defaults, and building build/upgrade material+credit costs
- combat rule refs and caps: DarkOrbit-like demo player speed, radar range,
  loot pickup range, basic laser cost/cooldown, training NPC type, repair
  quote currency/cost, NPC kill XP, and PvP cargo-drop policy by zone
- map enemy completeness: every playable map must own spawn areas, enemy pools,
  stat templates, drop profiles, aggro profiles, and leash profiles; unreferenced
  rows and missing pool refs fail before runtime starts

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

### Task 2B: Map-Scoped Scanner Profiles

**Files:**
- Modify: `internal/game/content/scanner.go`
- Modify: `internal/game/discovery/scanner_types.go`
- Modify: `internal/game/discovery/scanner.go`
- Modify: `internal/game/discovery/scanner_helpers.go`
- Modify: `internal/game/server/runtime.go`
- Test: `internal/game/content/bundle_test.go`

**Step 1: Add profile resolver**

Add a scanner candidate-options provider that resolves options by active
server-owned map/zone. Runtime injects the content bundle as this provider, so
scan pulses use only the player's current bounded map.

**Step 2: Preserve demo canaries**

Keep current demo first-scan discoveries stable on `1-1`, `1-2`, and `1-3`.
Profile rows may vary level band and spawn budget, but density/seed choices must
not break the playable vertical-slice scanner loop until cooldown-backed rare
planet tuning exists.

**Step 3: Validate**

Run:

```bash
go test ./internal/game/discovery ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestGameplayContent|TestScannerContent|TestResolveScanPulseMaterializationAndIntelAreSeededMapScoped|TestE2EScanNoPlanetSeedReturnsNoSignalWithoutPlanetMutation|TestScanPulseUsesActiveSeededMapScope' -count=1
git diff --check
```

Expected: pass.

### Task 2C: Starter And Playtest Seed Content

**Files:**
- Create: `internal/game/content/starter.go`
- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/validation.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/economy_seed.go`
- Modify: `internal/game/server/e2e_seed.go`
- Modify: `internal/game/server/scanner_providers.go`
- Modify: `internal/game/server/runtime_players.go`
- Modify: `internal/game/server/runtime_sessions.go`
- Modify: `internal/game/server/economy_handlers.go`
- Test: `internal/game/content/bundle_test.go`

**Step 1: Add starter content catalog**

Add `StarterContent` under `GameplayContent` for values currently scattered in
runtime seed code:

- starter ship id/display
- starter wallet credits/premium
- starter module item grants and scanner module
- scanner power/radius/interval/energy cost
- weekly X Core price/stock
- first-NPC entity id overrides by map enemy pool
- playtest claim core item/quantity
- route seed storage capacity, energy, and stored starter material

**Step 2: Validate references**

Starter content validates against existing catalogs before runtime starts. It
must reject missing item/module/ship/map/pool references, duplicate starter
module grants, invalid scanner values, invalid wallet amounts, and invalid route
seed quantities.

**Step 3: Runtime wiring**

Runtime stores the validated `StarterContent` from the bundle and uses it for
server-owned seed mutations. Client payloads continue to receive only snapshots,
events, and query results; no client-authored content truth is introduced.

**Step 4: Validate**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestGameplayContent|TestScannerContent|TestPlaytestSeed|TestE2EPlanetClaimSeed|TestScanPulse|TestPremium|TestMarketStateMutationFanout|TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner' -count=1
git diff --check
```

Expected: pass.

### Task 2D: Content Repository Boundary

**Files:**
- Create: `internal/game/content/repository.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/runtime.go`

**Step 1: Add repository interface**

Add a `content.Repository` interface for loading published gameplay content.
The current implementation is `StaticRepository`, which returns the validated
static bundle.

**Step 2: Runtime wiring**

Runtime loads content through `LoadPublishedContent` instead of calling
`DefaultGameplayContent` directly. This keeps DB/CMS loading as a future
adapter without changing server-authoritative mutation paths.

**Step 3: Validate**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'TestStaticRepository|TestLoadPublishedContent|TestDefaultGameplayContent|TestGameplayContent|TestPlaytestSeed|TestE2EPlanetClaimSeed|TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner' -count=1
git diff --check
```

Expected: pass.

### Task 2E: Shop Registry Ownership

**Files:**
- Create: `internal/game/content/shop.go`
- Delete: `internal/game/server/content_registry.go`
- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/validation.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/server_economy_inventory_shop_test.go`

**Step 1: Move concrete shop content**

Move static shop categories, products, display metadata, module product prices,
ship product metadata, and starter resource product out of the server package
and into the content package.

**Step 2: Store in `GameplayContent`**

Add `Shop catalog.ContentRegistry` to the content bundle. Build it during
`DefaultGameplayContent` after item/module/ship catalogs exist.

**Step 3: Validate references**

Bundle validation calls `Shop.ValidateReferences` with content-owned item,
module, ship, and premium resolvers. Invalid products fail before runtime
serves shop payloads.

**Step 4: Runtime wiring**

Runtime receives `contentBundle.Shop` directly. Shop handlers keep using
server-owned runtime state for purchases and grants.

**Step 5: Validate**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestGameplayContent|TestStaticRepository|TestLoadPublishedContent|TestShop|TestEconomyInventoryShop|TestMarketStateMutationFanout' -count=1
git diff --check
```

Expected: pass.

### Task 2F: Route Policy Content

**Files:**
- Create: `internal/game/content/route.go`
- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/validation.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/route_handlers.go`
- Modify: `internal/game/server/route_endpoints.go`

**Step 1: Add route content**

Add `RouteContent` for routeable item IDs, max routes, max distance, cross-map
distance penalty, route energy formula, loss band, and endpoint storage
capacity.

**Step 2: Validate references**

Route content validates routeable items against content item definitions and
rejects duplicate item IDs or invalid numeric caps.

**Step 3: Runtime wiring**

Runtime route creation and endpoint storage use validated `RouteContent`. The
runtime still owns source/destination ownership checks, map distance facts,
route count facts, and storage mutation.

**Step 4: Validate**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestGameplayContent|TestRoute|TestRouteCreate|TestRouteEndpoint|TestRouteSettle|TestRouteControl|TestE2ERoute|TestPlaytestSeed' -count=1
git diff --check
```

Expected: pass.

### Task 2G: Production Rule Content

**Files:**
- Create: `internal/game/content/production_rules.go`
- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/validation.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/runtime_claim_adapters.go`
- Modify: `internal/game/server/planet_claim_handlers.go`
- Modify: `internal/game/server/planet_building_handlers.go`

**Step 1: Add production rule content**

Add `ProductionRulesContent` for claim range, claim production storage/energy
defaults, and planet-building build/upgrade material+credit costs.

**Step 2: Validate references**

Production rules validate building material items against content item
definitions and reject duplicate building-cost rows or invalid claim/default
numeric values.

**Step 3: Runtime wiring**

Runtime claim proximity, claim production initialization, claim recovery, and
building cost provider read the validated content values. Runtime still owns
proximity state, ownership checks, wallet debits, and planet-storage mutations.

**Step 4: Validate**

Run:

```bash
go test ./internal/game/content ./internal/game/server -run 'TestDefaultGameplayContent|TestGameplayContent|TestPlanetClaim|TestPlaytestSeed|TestE2EPlanetClaimSeed|TestPlanetBuilding|TestClaim' -count=1
git diff --check
```

Expected: pass.

### Task 2H: Combat And Enemy Rule Hardening

**Files:**
- Create: `internal/game/content/combat_rules.go`
- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/validation.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/combat_loot_repair.go`
- Modify: `internal/game/server/combat_loot_helpers.go`
- Modify: `internal/game/server/combat_loot_death.go`
- Modify: `internal/game/server/runtime_players.go`
- Modify: `internal/game/world/maps/enemy_catalog.go`
- Modify: `internal/game/world/maps/enemy_catalog_test.go`

**Step 1: Add combat rule content**

Move DarkOrbit-like demo balance constants for speed, radar, pickup range,
basic laser cost/cooldown, training NPC type, repair quote values, NPC kill XP,
and PvP cargo-drop percentages into validated content.

**Step 2: Harden per-map enemy validation**

Require every playable map to have enemy pools plus referenced spawn/stat/drop/
aggro/leash content. Reject unreferenced NPC stat/drop/aggro/leash rows, missing
pool refs, invalid monster stat values, and incomplete enemy content before
runtime starts.

**Step 3: Validate**

Run:

```bash
go test ./internal/game/world/maps ./internal/game/content ./internal/game/server -run 'TestEnemyCatalog|TestCatalogValidation|TestDefaultGameplayContent|TestGameplayContent|TestNPCLootSelector|TestRuntimeSeedWorldInitializesStarterEnemyPoolThroughSpawner' -count=1
git diff --check
```

Expected: pass.

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
