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

Suggested flow:

1. Add a local reference document or fixture with only the rows needed for the
   first three maps and the first item/NPC bands.
2. Write content tests that encode density, map-risk, NPC stat, and upgrade
   ladder expectations.
3. Update our content catalogs and seed files to satisfy those tests.
4. Document which source tables informed each content family.
5. Run backend content/server tests and final project verification.

This belongs to Phase 7 of:

```text
docs/plans/2026-06-28-darkorbit-feel-implementation.md
```

The DB-default-seed implementation plan lives here:

```text
docs/plans/2026-06-28-kalaazu-db-default-seed-design.md
docs/plans/2026-06-28-kalaazu-db-default-seed-implementation.md
```
