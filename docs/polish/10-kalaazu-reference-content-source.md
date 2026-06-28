# Kalaazu Reference Content Source

Date: 2026-06-28

## Purpose

Use the open source Kalaazu database dumps as a concrete reference seed for the
DarkOrbit-feel content pass, especially Phase 7 of the implementation plan.

Source repository:

- https://github.com/manulaiko/Kalaazu
- https://github.com/manulaiko/Kalaazu/tree/develop/Persistence/database

License checked on 2026-06-28:

- https://raw.githubusercontent.com/manulaiko/Kalaazu/develop/LICENSE
- License: MIT

## Relevant Dumps

Read these as source material for content ratios, starter map density, NPC stat
bands, item family shape, and portal layout:

- `Persistence/database/maps/dump.sql`
  - map ids, names, starter/PVP flags, and coordinate limits
  - observed columns: `id`, `name`, `is_pvp`, `is_starter`, `limits`
- `Persistence/database/maps_npcs/dump.sql`
  - map to NPC density rows
  - observed columns: `maps_id`, `npcs_id`, `amount`
- `Persistence/database/npcs/dump.sql`
  - NPC stat/reward source shape
  - observed columns include `health`, `shield`, `shield_absorption`, `damage`,
    `speed`, `ship_type`, `ai`
- `Persistence/database/items/dump.sql`
  - item families, categories, prices, cooldowns, slotbar ordering, buyability
- `Persistence/database/ships/dump.sql`
  - ship stats, cargo, weapon/generator slots, and item linkage
- `Persistence/database/maps_portals/dump.sql`
  - map-to-map travel graph and portal coordinates

## Safe Usage Rule

Treat the SQL dumps as usable MIT-licensed seed data, but do not blindly ship
another game's names, descriptions, ids, or exact lore identity as our final
product identity.

Allowed for this project:

- import or copy structural facts such as spawn counts, map dimensions, portal
  graph shape, NPC stat bands, and item ladder families
- use the data to set realistic density targets for `1-1`, `1-2`, and `1-3`
- map NPC archetypes into our own catalog names and internal ids
- keep source/license provenance in docs when data is derived from Kalaazu

Avoid in final player-facing content unless explicitly reviewed:

- exact branded NPC names where they are clearly inherited from DarkOrbit-like
  terminology
- exact item descriptions, lore, and trademark-sensitive labels
- exact economy prices without rebalancing against our own progression curve

The practical target is: copy the content grammar and density, then rename,
rebalance, and normalize into our server-authoritative schema.

## Mapping Target

Phase 7 should convert the source dumps into our own early-game slice:

- `1-1`: safe starter density, many passive weak contacts, enough targets that
  the player never flies through empty space for long
- `1-2`: mixed passive/farming enemies, stronger rewards, portal continuity
- `1-3`: first aggressive sector, return-fire risk, better loot and progression
  pressure
- ships: starter and early ship stats should come from `ships/dump.sql` joined
  through the corresponding item rows
- early items: starter laser, next laser, shield, cargo/radar/speed utility, and
  at least one loot/material-driven upgrade route

## Implementation Notes

Prefer a small checked-in extractor or reference fixture over a runtime
dependency on the upstream GitHub repository.

Current implementation status:

- The six source dumps are checked in under
  `internal/game/contentseed/kalaazu/testdata/` and embedded into the Go seed
  builder. Runtime never fetches GitHub.
- `contentseed.BuildDefaultSnapshot` builds the first-run default snapshot from
  the Kalaazu-derived row groups, then publishes and loads those rows through
  the content DB path.
- Runtime truth is published DB content. Static Go catalogs remain legacy seed
  helpers and explicit test adapters, not the normal runtime source of truth.
- The import report tracks source row counts, imported row counts, and
  unsupported categories so skipped data is visible.

Implemented mapping rules:

- Maps and portals: `1-1`, `1-2`, and `1-3` become `map_1_1`, `map_1_2`, and
  `map_1_3`, with Kalaazu coordinate limits and visible portal coordinates.
  Portal destinations stay server-side.
- NPC density: each `maps_npcs.amount` becomes `MapMaxAlive`; per-pool live
  count is capped at `12`; initial live count is capped at `4`; respawn cadence
  uses a short 20 second delay with jitter for early-map rhythm.
- NPC stats: Kalaazu health, shield, damage, speed, and AI feed our NPC stat,
  aggro, drop, and leash rows with safe defaults for fields the dump does not
  define.
- Items: every Kalaazu item row becomes an item definition. Duplicate `loot_id`
  values are made unique with a source-id suffix instead of being silently
  dropped.
- Ships: `ships.items_id` joins through `items.id`, producing ship rows such as
  `ship_phoenix` and `ship_goliath` with source HP, speed, cargo, laser,
  generator, and extra-slot values.
- Modules: laser rows become offensive modules with `weapon_damage`; shield
  generator rows become defensive modules with `shield_max`. Speed generators
  remain unsupported until the module/content schema can express speed stats.
- Shop: buyable Kalaazu rows become shop products and are classified as ship,
  module, or item products based on the rows imported above.

This belongs to Phase 7 of:

```text
docs/plans/2026-06-28-darkorbit-feel-implementation.md
```

The DB-default-seed implementation plan lives here:

```text
docs/plans/2026-06-28-kalaazu-db-default-seed-design.md
docs/plans/2026-06-28-kalaazu-db-default-seed-implementation.md
```
