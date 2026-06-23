# CMS Rework Plan Index

Date: 2026-06-24

## Purpose

Move dynamic game content out of hard-coded runtime catalogs into Postgres.

Runtime still stays server-authoritative. CMS owns definitions, not player
state.

## Non-Negotiable Outcome

- Items, modules/equipment, ships, shop products, NPC templates, enemy pools,
  loot tables, craft recipes, and production/building definitions load from DB.
- Code catalogs become seed/fallback/test fixture sources only.
- Server boot seeds default MVP content only when content DB is empty.
- Runtime reads current published content snapshot, validates it, then builds
  existing domain catalogs from it.
- Client receives only safe projections. Hidden drop tables, spawn internals,
  procedural seeds, and server-only map fields never leak.
- Admin edits draft content. Gameplay reads published content.

## Phase List

1. [Postgres And Migration Foundation](./01-postgres-migration-foundation.md)
2. [Content Snapshot Schema](./02-content-snapshot-schema.md)
3. [Seed Source And Publish Bootstrap](./03-seed-source-publish-bootstrap.md)
4. [Runtime Loader And Catalog Assembly](./04-runtime-loader-catalog-assembly.md)
5. [Items Modules Ships And Shop Content](./05-items-modules-ships-shop.md)
6. [NPC Enemy Pools And Loot Content](./06-npc-enemy-pools-loot.md)
7. [Crafting Production And Building Content](./07-crafting-production-buildings.md)
8. [Admin Publish Rollback And Audit API](./08-admin-publish-rollback-audit-api.md)
9. [Admin Content UI And Safe Projections](./09-admin-content-ui-safe-projections.md)
10. [Rollout Versioning And Balancing Hardening](./10-rollout-versioning-balancing.md)
11. [Quest Board And Reward Content](./11-quest-board-reward-content.md)

Implementation plans live under:

```text
docs/plans/cms-rework/implementation/
```

## Dependency Map

```text
01 Postgres/Migrations
  -> 02 Content Snapshot Schema
  -> 03 Seed/Publish Bootstrap
  -> 04 Runtime Loader
  -> 05 Items/Modules/Ships/Shop
  -> 06 NPC/Pools/Loot
  -> 07 Craft/Production/Buildings
  -> 11 Quest Board/Rewards
  -> 08 Admin API
  -> 09 Admin UI
  -> 10 Rollout/Versioning
```

Phases 05, 06, and 07 can be split into separate Symphony tasks after Phase 04.
Each worker task must touch one domain slice only.

## Existing Code Anchors

```text
internal/game/catalog/
internal/game/server/runtime.go
internal/game/server/content_registry.go
internal/game/server/combat_loot_catalog.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
internal/game/crafting/catalog.go
internal/game/quests/catalog.go
internal/game/production/catalog.go
internal/game/world/maps/catalog.go
internal/game/world/maps/enemy_catalog.go
internal/game/admin/
```

## Content Vs Player State

CMS content:

- definition IDs
- display metadata
- item/module/ship stats
- NPC stats
- spawn pool settings
- loot rows
- recipe inputs/outputs
- production rates/costs
- shop products
- quest templates/reward tables in Phase 11

Not CMS content:

- player inventory rows
- cargo contents
- wallet balances
- active loadout
- current HP/shield/energy
- active craft jobs
- planet ownership
- market listings and auction bids
- quest progress

Player state must keep ledger/idempotency/service boundaries from `AGENTS.md`.

## Stable ID Policy

IDs are durable references. Admin may edit values, not casually rename IDs.

Examples:

```text
item.laser.lc1
item.module.radar_mk1
item.core.x_core
npc.starter.drone
loot_table.starter_drone
recipe.lc1_upgrade
ship.starter.skiff
shop.product.lc1
production.iron_extractor_l1
```

Existing IDs may be kept during migration:

```text
laser_alpha_t1
radar_t1
x_core
raw_ore
training_drone_salvage
refined_alloy_batch
```

Rename path must be explicit alias/migration, not silent replacement.

## DB Shape Preference

Use typed tables for editable rows plus immutable version snapshots.

Typed tables help admin lists, filters, validation, and diff. Snapshot rows help
runtime load one published version deterministically.

Postgres features:

- transactions for publish
- unique/check constraints for hard invariants
- `jsonb` for nested stat/row payloads where typed columns would explode
- indexes on content IDs, status, version, and updated fields
- audit table for old/new values and actor metadata
- one deterministic current published version, enforced by DB

## Content Source Policy

Real gameplay mode must require DB content.

Allowed modes:

```text
required     DB required; boot fails if DB/content invalid
dev_fallback DB optional; old code catalogs allowed only for local tests/dev
off          tests only; not valid for real server run
```

Missing DB URL in non-dev real mode must fail boot. No silent fallback.

## Runtime Boot Contract

```text
connect DB
run/verify migrations
if no content rows -> seed built-in MVP content
load current published content version
validate full snapshot
assemble domain catalogs
install runtime catalogs
serve safe projections only
```

Boot must fail closed if published content is invalid.

## Version Policy Summary

Some durable player state stores `definition_id + content_version`. Runtime must
either retain old snapshots needed by live state or block publish until old state
can safely finish/migrate.

Minimum policy:

- inventory/cargo items: old item definitions remain resolvable for weight and
  display until no stored item references that version
- equipped modules/loadout: publish must not silently change existing module
  slot compatibility; stat-change behavior must be explicit
- craft jobs: old recipe version must complete, or publish must be blocked
  while jobs using changed recipe are active
- planet buildings/routes: old production definition version must settle, or
  publish must block/migrate
- loot drops: already-created drops keep concrete item/quantity; new rolls use
  current version
- shop products: purchase uses current published product at command time

## Symphony Rules

Main project-manager session creates Symphony tasks. Worker sessions:

- follow `docs/symphony-worker-rules.md`
- do not follow `AGENTS.md`
- do not spawn subagents
- do not manage queue
- do not commit
- use `$caveman` in task prompt
- keep one phase/slice per task

Task prompts should include:

```text
Use $caveman. Read docs/symphony-worker-rules.md.
Work only on docs/plans/cms-rework/<phase>. Do not expand scope.
Run phase-specific tests plus git diff --check.
```

## Context7 Rule

Use Context7 before implementing or changing docs-dependent usage of:

- Docker Compose
- PostgreSQL
- Go DB driver
- migration tool
- any new admin UI framework/library API

Current design already checked Docker Compose and PostgreSQL docs.

## Test Shape Rule

No giant test files.

Prefer:

- one test file per package/domain slice
- table tests for validators
- integration test for boot loader
- small fixture helpers under domain package
- no broad exact-count catalog tests except seed bootstrap smoke

Full handoff still requires:

```bash
go test ./...
git diff --check
```
