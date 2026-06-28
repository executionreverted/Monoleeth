# Kalaazu DB Default Seed Design

Date: 2026-06-28

## Goal

Make the database the authoritative source for gameplay content, seeded on first
boot with default data derived from the open source Kalaazu database dumps.

## Current State

The project already has a published content snapshot path:

- `internal/game/contentseed.BuildMVPSnapshot` builds deterministic seed rows.
- `internal/game/contentseed.EnsurePublishedSeed` publishes a snapshot when the
  content database is empty.
- `internal/game/contentdb.Repository.LoadPublishedContent` maps the current
  published DB snapshot into `content.GameplayContent`.
- `internal/game/server.loadRuntimeContent` can load from a configured
  `ContentRepository` or from `ContentDB`.

The problem is that the default seed is still generated from static Go content
builders such as `content.DefaultGameplayContent`, `world/maps.StarterCatalog`,
and module/item helper catalogs. Those are fine as temporary fixtures, but they
must not be the production truth.

## Desired State

On a real server boot:

1. The server opens the content database.
2. If there is no content, it builds a Kalaazu-derived default snapshot and
   publishes it.
3. Runtime content is loaded from the current published DB snapshot.
4. If the DB is required and no valid published snapshot can be loaded, startup
   fails closed.
5. Static repositories remain available only for explicit tests.

## Source Scope

Use these Kalaazu files as seed inputs:

- `Persistence/database/maps/dump.sql`
- `Persistence/database/maps_npcs/dump.sql`
- `Persistence/database/npcs/dump.sql`
- `Persistence/database/items/dump.sql`
- `Persistence/database/ships/dump.sql`
- `Persistence/database/maps_portals/dump.sql`

Source repository and license notes live in:

```text
docs/polish/10-kalaazu-reference-content-source.md
```

## Mapping Policy

Import all source data that has an equivalent in our current content model:

- maps and public map keys
- map bounds and starter/PVP flags
- portal graph and portal coordinates
- NPC stat templates
- map NPC density rows as enemy pools
- item definitions where they can map safely to current item/module/shop shapes
- ship definitions and slot layouts from `ships/dump.sql` joined to item rows

Do not block this phase on systems we do not yet model. Rows that do not have a
current destination should be counted in an import report and left for a later
expansion phase.

## Runtime Rule

Production and normal dev runtime should not silently build gameplay content
from static Go catalogs when the content database is configured. If the database
is configured and invalid, fail closed. If a test wants static content, it must
inject `content.NewStaticRepository()` or a fake repository explicitly.

## Verification

The implementation must prove:

- empty DB seeds once with the Kalaazu-derived snapshot
- second boot does not overwrite existing published content
- DB-published edits survive runtime load
- map/NPC/item counts are present in the published snapshot
- server startup with required DB does not use static fallback
- existing combat, movement, loot, and client checks still pass
