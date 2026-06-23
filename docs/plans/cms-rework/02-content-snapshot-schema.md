# Phase 02 - Content Snapshot Schema

## Goal

Define DB schema for draft content rows and immutable published snapshots.

This phase designs data ownership. Runtime still may use old catalogs until
Phase 04.

## Core Tables

```text
content_versions
content_audit_log
content_items
content_modules
content_ships
content_shop_products
content_npc_templates
content_spawn_areas
content_enemy_pools
content_npc_drop_profiles
content_npc_aggro_profiles
content_npc_leash_profiles
content_npc_event_spawns
content_loot_tables
content_craft_recipes
content_production_buildings
content_quest_templates
content_quest_reward_tables
```

Maps/portals/safe zones stay careful. They can enter schema later as separate
tables after enemy pools prove safe projection rules.

## Version Table

`content_versions`:

```text
id uuid primary key
version text unique
status draft|published|archived|rolled_back
is_current boolean not null default false
idempotency_key text unique nullable
snapshot_json jsonb not null
validation_report_json jsonb not null
notes text
balance_tag text
created_by account id nullable for seed
created_at timestamptz
published_by account id nullable
published_at timestamptz nullable
rolled_back_from uuid nullable
```

Constraints:

```text
status check in ('draft','published','archived','rolled_back')
published_at not null when status='published'
partial unique index: one row where is_current = true
is_current true only allowed for status='published'
```

Runtime reads only `is_current=true`. It must not guess from timestamp order.

## Draft Row Pattern

Each content table has:

```text
content_id text
draft_version uuid nullable
enabled bool
display_json jsonb
data_json jsonb
created_at
updated_at
updated_by
```

Use check constraints for hard simple bounds:

```text
content_id <> ''
enabled is not null
data_json is not null
```

Use Go validators for cross-table rules.

## Snapshot Shape

One snapshot contains all runtime definition groups:

```json
{
  "version": "content_2026_06_24_001",
  "items": [],
  "modules": [],
  "ships": [],
  "shop_products": [],
  "npc_templates": [],
  "spawn_areas": [],
  "enemy_pools": [],
  "npc_drop_profiles": [],
  "npc_aggro_profiles": [],
  "npc_leash_profiles": [],
  "npc_event_spawns": [],
  "loot_tables": [],
  "craft_recipes": [],
  "production_buildings": [],
  "quest_templates": [],
  "quest_reward_tables": []
}
```

Snapshot is immutable after publish.

## ID Rules

- IDs unique within type.
- Cross-type references use explicit ref kind.
- Long-lived state stores `definition_id + content_version`.
- Deleting content means `enabled=false` unless no published version ever used
  the ID.

## Validation Rules

Phase 02 adds validator skeleton only:

- required IDs
- duplicate IDs
- positive amounts/durations
- drop chance `0..1`
- stat bounds finite/non-negative
- known enum values
- JSON parseable
- duplicate stats/cooldowns/inputs/outputs rejected
- no arbitrary expression DSL in content rows

Cross-table validation deepens in domain phases.

## Migration Runner Rules

`schema_migrations` must store:

```text
version text primary key
checksum text not null
applied_at timestamptz not null
```

Checksum mismatch must fail. Dirty/partial migration state must fail closed.

## Code Shape

New package:

```text
internal/game/content/
  snapshot.go
  ids.go
  validation.go
  projection.go
```

Keep DB code in `contentdb`. Keep gameplay catalog assembly outside DB package.

## Validation

```bash
go test ./internal/game/content ./internal/game/contentdb -count=1
git diff --check
```

## Done

- schema exists
- snapshot model validates basics
- content store can read/write draft rows and version rows
- no runtime catalog replacement yet
