# Phase 03 - Seed Source And Publish Bootstrap

## Goal

Use the current validated `internal/game/content.GameplayContent` bundle as
default DB seed source.

If content DB empty, write MVP content and publish first version.
If DB has content, never overwrite it silently.

## Existing Seed Source

```text
internal/game/content/bundle.go
internal/game/content/repository.go
internal/game/content/shop.go
internal/game/content/scanner.go
internal/game/content/starter.go
internal/game/content/route.go
internal/game/content/production_rules.go
internal/game/content/combat_rules.go
internal/game/world/maps/enemy_catalog.go
```

`DefaultGameplayContent(worldID)` may still call older MVP helpers internally,
but CMS seeding should consume the validated bundle shape. Do not re-copy
scattered server runtime constants into a second seed path.

## Boot Seed Flow

```text
BEGIN
lock content bootstrap key
if any content_versions exists:
  COMMIT no-op
else:
  load validated static GameplayContent
  flatten bundle into typed draft rows and snapshot
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

Each domain gets its own flattener file:

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
`contentseed` must not import `internal/game/server`. Prefer reading from
`content.DefaultGameplayContent`/`content.StaticRepository`; extract pure seed
helpers only if the bundle lacks a field.

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
- static `GameplayContent` remains available only as seed/fallback/test fixture
