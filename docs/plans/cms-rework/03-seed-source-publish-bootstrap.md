# Phase 03 - Seed Source And Publish Bootstrap

## Goal

Use current code catalogs as default DB seed source.

If content DB empty, write MVP content and publish first version.
If DB has content, never overwrite it silently.

## Existing Seed Sources

```text
internal/game/modules/catalog.go
internal/game/ships/catalog.go
internal/game/crafting/catalog.go
internal/game/production/catalog.go
internal/game/server/combat_loot_catalog.go
internal/game/server/content_registry.go
internal/game/world/maps/enemy_catalog.go
```

These become seed compilers, not runtime truth.

## Boot Seed Flow

```text
BEGIN
lock content bootstrap key
if any content_versions exists:
  COMMIT no-op
else:
  compile built-in MVP content
  validate snapshot
  insert typed draft rows
  insert content_versions(status=published, is_current=true, snapshot_json=...)
  insert audit rows actor=system_seed
COMMIT
```

Use DB transaction and uniqueness to prevent double seed.
Typed rows, current version row, and audit rows must commit together. Admin
draft tables must not be empty after seed.

Implementation should expose one transaction-shaped method such as:

```text
contentdb.EnsurePublishedSeedTx(snapshot, actor=system_seed)
```

It must acquire DB-level lock/advisory lock and be safe under concurrent boot.

## Seed Version

Initial version:

```text
content_mvp_seed_v1
```

Do not reuse old per-domain versions as published CMS version. Preserve old
source versions inside migrated rows for traceability.

## Seed Compiler Rule

Each domain gets its own compiler file:

```text
internal/game/contentseed/items.go
internal/game/contentseed/modules.go
internal/game/contentseed/ships.go
internal/game/contentseed/shop.go
internal/game/contentseed/npc.go
internal/game/contentseed/loot.go
internal/game/contentseed/crafting.go
internal/game/contentseed/production.go
```

No monolithic `seed.go`.
`contentseed` must not import `internal/game/server`; server imports seed during
boot. Extract pure seed helpers out of server package where needed.

## Validation

Seed snapshot must prove:

- every recipe input/output item exists
- every module has item definition
- every ship shop grant points to real ship
- every loot row item exists
- every NPC drop profile points to real loot table
- every enemy pool points to real NPC template/drop profile/spawn area
- every spawn area is valid for map bounds/safe-zone/portal exclusions
- every aggro/leash/event spawn reference resolves
- every production input/output item exists
- every shop product grant target exists

## Safety

Seed never runs if content exists. Add explicit reset/dev command later, not
automatic overwrite.

## Tests

Small tests:

- empty store seeds once
- non-empty store does not overwrite
- seed snapshot validates
- duplicate seed calls return same published version
- concurrent seed calls create exactly one current published version and one set
  of typed rows

Commands:

```bash
go test ./internal/game/contentseed ./internal/game/contentdb -count=1
git diff --check
```

## Done

- empty DB gets first published version
- non-empty DB unchanged
- audit log records seed
- old catalogs still available but no longer only source for future phases
