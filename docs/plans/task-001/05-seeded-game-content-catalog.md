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
- Definitions exposed to UI phases need player-facing display metadata:
  display name, category, art/icon key where available, safe requirement/lock
  reason codes, and backing refs for item/module/ship/product relationships.

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
   - Add UI-facing art/icon keys and safe lock/requirement reason codes for
     shop, inventory, loadout, hangar, planet, and quest surfaces.
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
   - System shop products include stable product ids, categories, art keys,
     backing refs, availability rules, and quote policy metadata.

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
- [ ] `Raw Ore` is removed from primary shop playtest presentation or renamed
      into an intentional resource with game context.
- [ ] Raw ids/snake_case are not used as player display names.
- [ ] Seeded products/listings reference valid server definitions.
- [ ] System shop products include stable product ids, categories, art/icon
      keys, backing refs, availability rules, and quote policy metadata.
- [ ] Loot and NPC names are visible from server-owned data.
- [ ] Crafting recipes reference real item/module/ship definitions.
- [ ] Planet, production, route, and quest fixtures exist for downstream UI
      phases.
- [ ] Server payloads include display metadata needed to avoid raw id rendering.
- [ ] Server payloads include safe requirement/lock reason codes instead of
      internal validation strings for normal player UI.
- [ ] Tests protect catalog reference integrity.

## Verification

```bash
go test ./internal/game/modules ./internal/game/ships -count=1
go test ./internal/game/server -run 'Test.*(Catalog|Seed|Market|Loot|NPC|Inventory|Hangar)' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run smoke
```
