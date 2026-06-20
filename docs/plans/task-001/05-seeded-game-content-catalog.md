# Phase 05 - Seeded Game Content Catalog

## Goal

Create a broad, server-owned playtest content catalog inspired by DarkOrbit
equipment categories but using original names, values, and assets. This gives
Shop, Inventory, Hangar, Combat, Loot, and Planet systems enough real data to
test with.

## Problems Covered

- Current content is too thin for playtesting.
- Shop exposes raw ore because the server seed only has a narrow market fixture.
- Equipment categories are incomplete from a player perspective.
- Lootable things, NPCs, ships, and modules need testable numbers.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/todo.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/04-module-stat-aggregation.md
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/06-loot-drop-ownership.md
docs/plans/modules/08-crafting-recipes-materials.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/12-automation-routes.md
docs/plans/modules/13-intel-coordinate-trading.md
docs/2026-06-17-progression-economy-systems-design.md
internal/game/server/combat_loot_catalog.go
internal/game/server/economy_seed.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
```

## Content Rules

- Use original names and stats.
- Use DarkOrbit only as category inspiration.
- Everything visible in real mode must come from server definitions/snapshots.
- Demo/test-only fixtures must be behind explicit dev/test switches.
- Every item/currency mutation still goes through inventory/wallet/ledger
  services.
- Phase 05 is a prerequisite for phases 06, 07, 08, and 09. Those UI phases
  should not rebuild layouts against raw/thin fixtures.
- Content must be defined through a canonical server-owned registry or an
  explicit cross-catalog validator covering items, modules, ships, NPCs, loot,
  shop products, auctions, crafting recipes, production outputs, routes, and
  quests.

## Catalog Registry Contract

Phase 05 must introduce an explicit server-owned content model, not only a loose
seed validator.

Required concepts:

- `ContentRegistry` and `CatalogVersion` for deterministic playtest content.
- Stable internal ids plus player-facing `DisplayName`, `Description`,
  `Category`, `Subcategory`, `ArtKey`, rarity/tier, and sort order.
- `ShopProductDefinition` records for system shop products, separate from
  player market listings.
- `AvailabilityRule`, `PricePolicy`, `StockPolicy`, `GrantTarget`, and backing
  refs for item, module, ship, premium, quest, planet, or unlock grants.
- Cross-catalog validation for products, NPC drops, loot boxes, crafting
  recipes, production outputs, route resources, quest rewards, auction
  fixtures, starter inventory, and starter loadout.
- Server payload helpers so UI surfaces can render display metadata without
  printing raw ids or inventing client-only labels.

## Subagent Review Additions - 2026-06-20

- Make the canonical registry an explicit model, not only a vague validator.
  Preferred shape is a new `internal/game/catalog` or `internal/game/content`
  package with `ContentRegistry`, `CatalogVersion`, `Category`, `ArtKey`,
  `DisplayName`, `Description`, and reference records for item/module/ship/
  recipe/production/quest definitions.
- Add `ShopProductDefinition` or equivalent server-owned product records in
  this phase so Phase 07 does not have to infer shop categories from player
  market listings.
- Validate all cross-catalog references at seed time: shop products, NPC drops,
  loot boxes, crafting recipes, production outputs, route resources, quest
  rewards, auction fixtures, and starter inventory/loadout.
- Tests should assert catalog invariants rather than old exact MVP counts. Raw
  ids and snake_case names are allowed as internal ids only, not as primary
  display names.

## Second Subagent Review Additions - 2026-06-20

- Existing code has only partial catalog/version pieces and old `raw_ore`
  economy seed paths. Treat replacement of raw market fixtures with registry
  products as part of this phase, not a Phase 07-only UI cleanup.
- Add a seed-time invariant that every player-visible display name differs from
  raw snake_case ids unless explicitly whitelisted as a deliberate lore name.
- Add product grant-target validation before UI work consumes the catalog:
  ship unlocks, module/items, premium weekly stock, auction grants, crafting
  outputs, planet/building requirements, and quest rewards must all resolve to
  owning services or named blockers.

## Third Subagent Review Additions - 2026-06-20

- Treat Phase 05 as a root blocker for downstream UI. The current code has a
  generic definition/version shape, but not a canonical `ContentRegistry` with
  typed refs, display metadata, art keys, categories, and cross-catalog
  validation.
- Replace raw market fixture truth. `raw_ore` and a single system seller
  listing must not be the real shop/catalog seed; system shop products should
  be registry-backed and separated from player market listings.
- Add an NPC archetype catalog and seed visible playtest NPCs matching quest and
  combat types, not only the training drone. Quest kill targets must align with
  spawned hostile archetypes.
- Replace hard-coded one-row loot with NPC/lootable-box tables covering common,
  uncommon, rare, X-Core/intel, and resource drops with display metadata.
- Fix crafting/runtime item reference mismatches. Every recipe input/output
  such as lens/coil/batch items must resolve to a runtime item definition or be
  renamed to an existing id.
- Extend the seed validator so no recipe, quest, production route, auction,
  premium grant, starter inventory, starter loadout, or shop product references
  missing content.

## Fourth Subagent Review Additions - 2026-06-20

- Add a concrete startup validation gate such as
  `ContentRegistry.ValidateReferences()` that resolves quest recipe ids, quest
  NPC target archetypes, recipe inputs/outputs, ship acquisition refs,
  shop/auction/premium grant targets, production resources, and route cargo
  against the same registry.
- Fix or explicitly block the current quest/crafting mismatch where the quest
  template references `energy_cell_batch` but the crafting catalog does not
  define a matching output.
- Crafting recipe payloads must include display metadata or catalog refs for
  inputs and outputs. UI surfaces such as `Next` must not print raw recipe,
  item, or module ids.
- Loot drop payloads need server-owned `display_name`, `category`, and
  `art_key` fields or catalog refs so target panels and pickup logs do not
  prettify raw ids client-side.
- Every non-starter ship should have at least one testable acquisition path
  through shop, craft, auction, quest, premium, or an explicit named blocker.
- Browser smoke should ban raw ids in crafting, loot target, production, route,
  shop, and inventory visible text and relevant attributes, not only cargo rows.

## Seed Categories

Initial original playtest catalog:

- Ships: `Ember Skiff`, `Vesper Dart`, `Helion Lance`, `Aegis Courier`,
  `Nomad Bulwark`, `Warden Relay`, `Rift Prospector`.
- Weapons: `Prism Lance I`, `Ion Scatterer`, `Needle Laser`, `Grav Cutter`.
- Launchers/ammo: `Rift Torpedo Rack`, `Shard Rockets`, `Pulse Rockets`.
- Generators: `Aurora Shield Cell`, `Vector Thruster`, `Pulse Reactor`,
  `Bastion Plating`.
- Extras/modules: `Horizon Scanner`, `Nebula Lens`, `Cargo Spine`,
  `Veil Projector`, `Repair Relay`, `Salvage Magnet`.
- Resources/loot: `Ferrite Ore`, `Carbon Lattice`, `Ion Residue`,
  `Lens Shard`, `X-Core Shard`, `Scanner Coil`, `Alloy Plate`.
- NPCs: `Rust Drone`, `Marauder Kite`, `Salvage Warden`, `Null Frigate`.
- Loot boxes: salvage cache, cargo crate, signal cache, encrypted wreckage.
- Planet/production: planet archetypes, claim costs, building definitions,
  production outputs, storage capacities, and route fixture resources.
- Quests: starter offer board, active quest fixture, claimable quest fixture,
  reward tables, reroll/cooldown state.

Names can change during balancing, but raw ids must not be the player display
names.

## Implementation Plan

1. Expand server catalogs.
   - Create a canonical content registry or validator that can verify
     cross-catalog references before runtime seed/test fixtures are accepted.
   - Add ship definitions with role, stats, rank requirement, slot layout, and
     prices/unlock requirements.
   - Add module definitions with slot type, tier, stats, cooldown/activation,
     durability, tradeability, and bind rules.
   - Add stackable item definitions with display names, categories, cargo
     behavior, and trade flags.
   - Add NPC/loot archetypes with stats, display names, drop tables, XP, and
     balance knobs instead of hard-coded training NPC/drop helpers.
   - Add planet/building/production/route/quest seed definitions needed by
     phases 08 and 09.
   - Extend module/stat definitions only where gameplay systems consume them;
     do not add cosmetic stat keys that have no server effect.

2. Seed deterministic playtest data.
   - Starter inventory gets a small but useful set.
   - System shop/market has category coverage.
   - NPC/loot tables have visible test drops.
   - Auctions/premium get deterministic playtest fixtures if needed.
   - Crafting recipe inputs/outputs all reference valid item/module/ship
     definitions.
   - Market, auction, premium, planet storage, route, cargo, loot, and quest
     payloads include display names/categories so the client does not print raw
     ids.

3. Connect UI.
   - Shop categories use catalog categories.
   - Inventory filters use item/module categories.
   - Hangar list uses owned ships and catalog metadata.
   - Combat/loot names are presentable.

4. Add tests and balancing notes.
   - Validate IDs/display names.
   - Validate every seeded product references a real item/module/ship.
   - Validate no raw id appears as display name unless intentionally equal.
   - Validate crafting recipe references, NPC/loot references, shop/listing
     references, production/route resource references, and quest reward
     references.
   - Update tests that currently lock in old MVP counts or raw-ore fixtures to
     assert invariants instead of exact old catalog sizes.

## Likely Files

```text
internal/game/server/combat_loot_catalog.go
internal/game/server/economy_seed.go
internal/game/crafting/catalog.go
internal/game/crafting/validation.go
internal/game/modules/catalog.go
internal/game/modules/definitions.go
internal/game/ships/catalog.go
internal/game/production/
internal/game/quest/
internal/game/server/progression_inventory_handlers.go
internal/game/server/server_test.go
client/src/state/types.ts
client/src/ui/hud.ts
docs/plans/task-001/05-seeded-game-content-catalog.md
```

## Acceptance Criteria

- [ ] Server catalog includes multiple ships, weapons, generators, extras,
      scanner/radar, stealth, cargo/support modules, resources, NPCs, and loot.
- [ ] A canonical registry or cross-catalog validator verifies all seeded
      item/module/ship/NPC/loot/shop/auction/craft/production/route/quest
      references.
- [ ] Shop and inventory can display category coverage without fake client data.
- [x] `Raw Ore` is removed from primary shop playtest presentation or renamed
      into an intentional resource with game context.
- [ ] Raw ids/snake_case are not used as player display names.
- [ ] Seeded products/listings reference valid server definitions.
- [x] Server-owned shop product definitions exist for system shop categories,
      separate from player market listing fixtures.
- [ ] Loot and NPC names are visible from server-owned data.
- [ ] Crafting recipes reference real item/module/ship definitions.
- [ ] Planet, production, route, and quest fixtures exist for downstream UI
      phases.
- [ ] Server payloads include display metadata needed to avoid raw id rendering.
- [x] Tests protect catalog reference integrity.
- [ ] Runtime seeded NPC archetypes match quest/combat target types.
- [ ] Crafting recipe inputs/outputs all resolve to runtime item definitions.
- [x] Real-mode system shop seed no longer depends on `listing-raw-ore-1` as
      primary product truth.
- [ ] Startup catalog validation resolves quest recipe ids, quest NPC targets,
      recipe inputs/outputs, ship acquisition refs, shop/auction/premium grants,
      production resources, and route cargo.
- [ ] The `energy_cell_batch` quest/crafting mismatch is fixed or blocked by
      registry validation.
- [ ] Crafting recipe and loot drop payloads expose display metadata or catalog
      refs, and normal UI does not render their raw ids.
- [ ] Every non-starter ship has an acquisition path or named blocker.

## Verification

```bash
go test ./internal/game/modules ./internal/game/ships -count=1
go test ./internal/game/server -run 'Test.*(Catalog|Seed|Market|Loot|NPC|Inventory|Hangar)' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run smoke
```
