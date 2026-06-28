# Kalaazu DB Default Seed Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Seed the content database with default gameplay data derived from Kalaazu dumps and make runtime gameplay content load from DB-published snapshots instead of static catalogs.

**Architecture:** Add a Kalaazu source fixture/parser layer that converts the relevant SQL dumps into the existing `content.Snapshot` row groups. Use `contentseed.EnsurePublishedSeed` to publish that snapshot on empty databases. Tighten runtime boot so configured DB content is the authoritative path and static content is test-only.

**Tech Stack:** Go, existing `content`, `contentseed`, `contentdb`, `world/maps`, `economy`, `modules`, `ships`, `loot`, and server runtime packages.

---

## Progress 2026-06-28

Completed and committed:

- Tasks 1-2: checked-in Kalaazu SQL dumps, provenance README, embedded fixture
  FS, and narrow SQL dump parser.
- Tasks 3-4: starter map shells, portals, NPC templates, spawn areas, enemy
  pools, drops, aggro, leash rows, and density policy for `1-1`, `1-2`, `1-3`.
- Task 5: item definitions, ship definitions, supported laser/shield modules,
  and buyable shop products derived from Kalaazu item/ship rows.
- Task 6: `kalaazu.BuildDefaultRows`, import report counts, and
  `contentseed.BuildDefaultSnapshot` as the default first-run seed path.
- Task 7 slice: runtime DB seeding now uses the Kalaazu default snapshot when
  content DB is enabled and empty; runtime still requires DB content unless a
  repository is explicitly injected.
- Task 7 hardening slice: content DB now has `content_maps` and
  `content_map_portals` draft tables, and the default starter config rewrites
  its world seed to the first Kalaazu starter enemy pool instead of the legacy
  `starter_training_drone_pool` reference.
- Task 7 browser-density hardening: Kalaazu enemy pools now use a shared
  per-map alive cap derived from total source map density, so multi-pool maps
  such as `1-3` are not capped by the smallest NPC row. The published default
  seed also projects Kalaazu Phoenix stats onto the legacy starter ship contract
  so existing starter loadout/module wiring keeps working while HP/shield/speed
  match the Kalaazu source.
- The DarkOrbit-feel canary now proves DB-seeded Origin kill/loot pickup plus
  `1-3` visible NPCs and NPC return fire through the published content DB path.
- Task 8 hardening slice: `DefaultSnapshotLegacyBridgeReport` now enumerates
  every default snapshot row that is not directly produced by Kalaazu source
  rows, with an explicit temporary reason. Tests fail if any non-Kalaazu row is
  added without a reason or if map/NPC rows regress back to legacy/static
  sources.
- Task 8 hardening slice: bridge reasons are now an explicit per-row allowlist,
  so new static item/module/ship/craft/quest/rule rows fail closed instead of
  inheriting broad type-level reasons.
- Task 8 bridge reduction slice: starter loot tables are now produced by the
  Kalaazu default row builder and reference Kalaazu resource item ids. The
  bridge report fails if loot table rows regress to local/static rows.
- Task 8 bridge reduction slice: default shop products are now replaced by
  Kalaazu buyable rows instead of appending to legacy shop rows. The bridge
  report fails if shop products regress to local/static rows.
- Task 8 bridge reduction slice: starter compatibility module ids
  `laser_alpha_t1` and `shield_generator_t1` are now projected from Kalaazu
  LF-1 and SG3N-A01 rows while preserving existing starter/loadout item ids.
- Task 8 bridge reduction slice: legacy ship contract ids `starter`,
  `fighter_t1`, `scout_t1`, and `hauler_t1` are now projected from Kalaazu
  Phoenix, Goliath, Vengeance, and BigBoy rows.
- Task 8 bridge reduction slice: starter module item ids `laser_alpha_t1` and
  `shield_generator_t1` are now projected from Kalaazu LF-1 and SG3N-A01 item
  rows as instance items.
- Task 8 bridge reduction slice: legacy material ids used by current
  recipes/quests (`prometium`, `raw_ore`, `endurium`, `iron_ore`, `terbium`,
  `prometid`, `refined_alloy`, `duranium`, `xenomit`, `carbon_shards`, and
  `promerium`) are now projected from Kalaazu resource rows.
- Task 8 bridge reduction slice: utility compatibility ids `scanner_t1`,
  `radar_t1`, and `cargo_expander_t1` plus their item rows are now projected
  from Kalaazu G-RL1, AI-R1, and G3X-CRGO-X rows while preserving existing
  starter/loadout contracts.
- Task 8 bridge reduction slice: `starter_config` is now emitted by the
  Kalaazu default row builder, with the first Kalaazu starter enemy pool and
  Phoenix starter display projected into existing account/session contracts.
- Task 8 bridge reduction slice: `scanner_config` is now emitted by the
  Kalaazu default row builder, using a Kalaazu scanner seed and starter-map
  profiles aligned to the seeded `1-1`, `1-2`, and `1-3` rows.
- Task 8 bridge reduction slice: `route_policy` is now emitted by the Kalaazu
  default row builder and routeable resources reference the Kalaazu-projected
  `refined_alloy` item.
- Task 8 bridge reduction slice: remaining local item contract ids
  (`laser_lens`, `energy_cell`, `scanner_circuit`, `warp_coil`,
  `helium_dust`, `planet_coordinate_scroll`, and `x_core`) are now emitted by
  the Kalaazu default row builder as explicit Kalaazu/default projections, so
  the bridge report fails if any item row regresses to local/static ownership.
- Task 8 bridge reduction slice: starter craft recipe rows
  `refined_alloy_batch`, `laser_alpha_t1`, and `scout_t1_unlock` are now
  emitted by the Kalaazu default row builder over Kalaazu-projected item/ship
  contract rows, so the bridge report fails if craft recipes regress to
  local/static ownership.
- Task 8 bridge reduction slice: production building rows
  `iron_extractor_l1`, `iron_extractor_l2`, and `alloy_foundry_l1` are now
  emitted by the Kalaazu default row builder over Kalaazu-projected resource
  rows, so the bridge report fails if production buildings regress to
  local/static ownership.
- Task 8 bridge reduction slice: `production_rules` is now emitted by the
  Kalaazu default row builder over Kalaazu/default production building rows, so
  the bridge report fails if production rules regress to local/static
  ownership.

Remaining before this plan is complete:

- Continue Task 8 hardening by replacing the now-explicit bridge rows with
  domain-specific Kalaazu/default rows where source data exists or new content
  design is approved.
- Run Task 10 full verification, including `go test ./...`, client check, and
  `git diff --check`.
- Speed generator rows are now mapped through `modules.StatSpeed`; remaining
  unsupported equipment rows are still counted in the import report.

---

## Source Material

Use these upstream files:

- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/maps/dump.sql`
- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/maps_npcs/dump.sql`
- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/npcs/dump.sql`
- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/items/dump.sql`
- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/ships/dump.sql`
- `https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/maps_portals/dump.sql`

License/provenance note:

- `docs/polish/10-kalaazu-reference-content-source.md`

## Task 1: Add Checked-In Kalaazu Seed Inputs

**Files:**

- Create: `internal/game/contentseed/kalaazu/testdata/maps.sql`
- Create: `internal/game/contentseed/kalaazu/testdata/maps_npcs.sql`
- Create: `internal/game/contentseed/kalaazu/testdata/npcs.sql`
- Create: `internal/game/contentseed/kalaazu/testdata/items.sql`
- Create: `internal/game/contentseed/kalaazu/testdata/ships.sql`
- Create: `internal/game/contentseed/kalaazu/testdata/maps_portals.sql`
- Create: `internal/game/contentseed/kalaazu/README.md`

**Step 1: Copy source dumps**

Copy the six upstream SQL dumps into `internal/game/contentseed/kalaazu/testdata/`.

Keep the files as source seed input. Do not make runtime fetch GitHub.

**Step 2: Write provenance README**

Document:

- upstream repository URL
- MIT license URL
- copy date
- source file paths
- rule that runtime consumes generated snapshot rows, not these raw SQL files

**Step 3: Run diff check**

Run:

```bash
git diff --check
```

Expected: pass.

## Task 2: Build a Minimal Kalaazu SQL Dump Parser

**Files:**

- Create: `internal/game/contentseed/kalaazu/parser.go`
- Create: `internal/game/contentseed/kalaazu/parser_test.go`

**Step 1: Write failing parser tests**

Tests:

- parses `INSERT INTO` column list
- parses multi-row `VALUES`
- handles quoted strings, escaped quotes, booleans, integers, and `NULL`
- returns table rows for all six checked-in dumps
- rejects unsupported or malformed SQL with a clear error

Run:

```bash
go test ./internal/game/contentseed/kalaazu -run 'Parser' -count=1
```

Expected: fail until parser exists.

**Step 2: Implement parser**

Implement a narrow parser for the dump shape we actually checked in:

- no general SQL engine
- parse one or more `INSERT INTO table (columns...) VALUES (...), ...;`
- preserve values as strings plus typed helpers
- expose `LoadDumpRows(fs.FS, path string) ([]DumpRow, error)`

**Step 3: Run tests**

Run:

```bash
go test ./internal/game/contentseed/kalaazu -run 'Parser' -count=1
```

Expected: pass.

## Task 3: Map Kalaazu Maps and Portals to Our Map Snapshot Rows

**Files:**

- Create: `internal/game/contentseed/kalaazu/maps.go`
- Create: `internal/game/contentseed/kalaazu/maps_test.go`
- Modify: `internal/game/contentdb/map_maps_npc.go`
- Modify: `internal/game/contentseed/snapshot.go`

**Step 1: Write failing map mapping tests**

Tests:

- maps `1-1`, `1-2`, and `1-3` from Kalaazu into internal ids `map_1_1`, `map_1_2`, `map_1_3`
- converts Kalaazu limits like `0,0|20800,12800` into map bounds
- maps starter and PVP flags into risk/PVP policy
- maps portal rows into visible portal definitions
- preserves portal destination server-side only

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentdb ./internal/game/world/maps -run 'Kalaazu|Map|Portal' -count=1
```

Expected: fail until map rows are supported.

**Step 2: Add map row content support if needed**

Current content DB map mapper builds shell definitions from static
`mapShellDefinitions`. Replace or extend this so map shells can be read from
snapshot rows.

Preferred shape:

- add a `Maps []SnapshotRow` group only if no existing content type can hold map
  shells cleanly
- add a `MapPortals []SnapshotRow` group only if portals cannot live in map rows
  cleanly
- keep destination internals server-only

**Step 3: Implement mapper**

Implement Kalaazu-to-project mapping:

- Kalaazu map id `1` and `2` order can be normalized by map name/public key
- public key remains `1-1`, `1-2`, `1-3`, etc.
- internal id is `map_` + public key with dashes converted to underscores
- use source bounds unless validation currently requires `0..10000`; if so,
  update map validation and movement tests to accept catalog bounds instead of
  hard-coded exact playable bounds

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentdb ./internal/game/world/maps -run 'Kalaazu|Map|Portal|Catalog|Bounds' -count=1
```

Expected: pass.

## Task 4: Map NPCs and Map Density to NPC Templates and Enemy Pools

**Files:**

- Create: `internal/game/contentseed/kalaazu/npcs.go`
- Create: `internal/game/contentseed/kalaazu/npcs_test.go`
- Modify: `internal/game/contentseed/snapshot.go`
- Modify: `internal/game/world/maps/enemy_catalog_test.go`
- Modify: `internal/game/server/server_enemy_spawner_test.go`

**Step 1: Write failing NPC mapping tests**

Tests:

- each Kalaazu `npcs` row used by `maps_npcs` creates an NPC stat template
- each `maps_npcs` row creates an enemy pool for the matching map
- `amount` drives `MapMaxAlive`, `PoolMaxAlive`, and `InitialAlive` through a documented scale factor
- passive/aggro behavior maps from Kalaazu `ai`
- `1-1`, `1-2`, and `1-3` have dense spawnable pools
- no hidden spawn internals are serialized to player payloads

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/content ./internal/game/server -run 'Kalaazu|NPC|Enemy|Spawner|Density' -count=1
```

Expected: fail until mapping exists.

**Step 2: Implement NPC stat mapping**

Map:

- `name` to internal label/display metadata
- `health` to `HPMax`
- `shield` to `ShieldMax`
- `damage` to `WeaponDamage`
- `speed` to `Speed`
- `ai` to passive/cautious/aggressive aggro profile

Use safe defaults for fields Kalaazu does not provide:

- `WeaponRange`
- `WeaponCooldown`
- `Accuracy`
- `RadarSignature`
- `EnergyMax`
- `XPValue`
- leash profile

**Step 3: Implement density scaling**

Kalaazu `amount` is source density, but our worker cannot necessarily spawn all
rows immediately. Define a scale policy, for example:

- `MapMaxAlive = min(amount, per-map cap)`
- `PoolMaxAlive = max(1, scaled amount for that pool)`
- `InitialAlive = min(PoolMaxAlive, first-session density cap)`
- `SpawnInterval` and `KillRespawnDelay` short enough for MMO rhythm

Document this in `docs/polish/10-kalaazu-reference-content-source.md`.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/content ./internal/game/server -run 'Kalaazu|NPC|Enemy|Spawner|Density' -count=1
```

Expected: pass.

## Task 5: Map Kalaazu Items and Ships into Items, Modules, Ships, and Shop Products

**Files:**

- Create: `internal/game/contentseed/kalaazu/items.go`
- Create: `internal/game/contentseed/kalaazu/items_test.go`
- Create: `internal/game/contentseed/kalaazu/ships.go`
- Create: `internal/game/contentseed/kalaazu/ships_test.go`
- Modify: `internal/game/contentseed/snapshot.go`
- Modify: `internal/game/contentdb/map_items.go`
- Modify: `internal/game/contentdb/map_modules.go`
- Modify: `internal/game/contentdb/map_ships.go`
- Modify: `internal/game/contentdb/map_shop.go`
- Modify: `internal/game/content/bundle_test.go`

**Step 1: Write failing item mapping tests**

Tests:

- currencies/resource rows become item definitions where supported
- laser/shield/generator/ammo-like rows become module or item definitions where
  our schema supports them
- ship rows join through `ships.items_id -> items.id` and create ship
  definitions with HP, speed, cargo, weapon slots, generator slots, and extras
- buyable rows produce shop products
- unsupported item categories are counted in an import report, not silently lost
- existing starter grants reference items/modules that exist in the DB snapshot

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentdb ./internal/game/content -run 'Kalaazu|Item|Ship|Module|Shop|Starter' -count=1
```

Expected: fail until mapping exists.

**Step 2: Implement item category mapping**

Create an explicit mapping table from Kalaazu `category`/`type`/`loot_id` to our
content model.

Map only supported categories in this phase:

- currencies as item/currency metadata if current wallet flow needs a content row
- resources/materials as stackable items
- laser and shield equipment as modules plus matching item definitions
- buyable items as shop products

Count unsupported rows by category in `KalaazuImportReport`.

**Step 3: Implement ship mapping**

Create ship definitions from `ships.sql`:

- join `ships.items_id` to `items.id`
- use item `loot_id` or normalized item name as the ship id
- map `health`, `speed`, and `cargo` to base ship stats
- map `lasers`, `generators`, and `extras` into slot layout
- preserve source ids in importer metadata/report

**Step 4: Preserve server authority**

Do not let item rows create wallet balances, inventory stacks, or player-owned
equipment directly. The seed only creates definitions. Starter grants still go
through existing server-owned starter config and inventory/loadout services.

**Step 5: Run tests**

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentdb ./internal/game/content -run 'Kalaazu|Item|Ship|Module|Shop|Starter' -count=1
```

Expected: pass.

## Task 6: Build the Kalaazu Default Snapshot

**Files:**

- Create: `internal/game/contentseed/kalaazu/snapshot.go`
- Create: `internal/game/contentseed/kalaazu/snapshot_test.go`
- Modify: `internal/game/contentseed/snapshot.go`
- Modify: `internal/game/contentseed/bootstrap_test.go`
- Modify: `internal/game/contentdb/repository_test.go`

**Step 1: Write failing snapshot tests**

Tests:

- `BuildDefaultSnapshot(worldID)` returns a valid `content.Snapshot`
- snapshot version is a new stable version such as `content_kalaazu_seed_v1`
- snapshot contains rows from maps, portals, NPCs, enemy pools, items, modules,
  shop products, loot tables, server rules, starter config, quests, crafting,
  and production
- `contentdb.ValidateSnapshot(snapshot, worldID)` passes
- `content.LoadPublishedContent` can load the snapshot through `contentdb.Repository`

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentseed ./internal/game/contentdb -run 'Kalaazu|DefaultSnapshot|Repository' -count=1
```

Expected: fail until snapshot builder exists.

**Step 2: Implement snapshot builder**

Implement:

```go
package kalaazu

func BuildDefaultSnapshot(worldID world.WorldID) (content.Snapshot, ImportReport, error)
```

The builder should:

- load checked-in dumps from embedded `testdata`
- parse source rows
- map supported source rows into content snapshot rows
- merge required non-Kalaazu server rows for systems Kalaazu does not define yet
- validate the final snapshot
- return an import report with source row counts, imported counts, skipped counts,
  and unsupported categories

**Step 3: Switch default seed builder**

Change `contentseed.BuildMVPSnapshot` or introduce a new
`contentseed.BuildDefaultSnapshot` so the default first-run seed comes from the
Kalaazu builder.

Keep the old MVP/static builder only as a test helper or explicitly named
legacy builder.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentseed ./internal/game/contentdb -run 'Kalaazu|DefaultSnapshot|Repository|Bootstrap' -count=1
```

Expected: pass.

## Task 7: Make Runtime DB-Only for Configured Servers

**Files:**

- Modify: `internal/game/server/runtime.go`
- Modify: `internal/game/server/config.go`
- Modify: `internal/game/server/server_content_runtime_test.go`
- Modify: `internal/game/server/config_test.go`
- Modify: `internal/game/content/repository.go`

**Step 1: Write failing runtime tests**

Tests:

- when `ContentDB` is enabled and empty, runtime seeds with the Kalaazu default
  snapshot then loads from DB
- second runtime boot does not reseed or overwrite a published snapshot
- when `GAME_CONTENT_MODE=required` and no DB URL exists, startup fails
- when DB is configured but published snapshot is invalid, startup fails closed
- when DB is not configured, runtime only uses static content if an explicit
  repository was injected by the test

Run:

```bash
go test ./internal/game/server -run 'ContentRuntime|RuntimeContent|Config|Kalaazu|DBOnly' -count=1
```

Expected: fail until runtime is tightened.

**Step 2: Tighten runtime content loading**

Keep this rule:

- `config.ContentRepository != nil`: use injected repository. This is test/admin
  embedding only.
- `ContentDB.Enabled()`: migrate, seed if empty, then load from DB.
- otherwise: return `contentdb.ErrContentDatabaseDisabled`.

Do not add a silent static fallback.

**Step 3: Update config defaults if needed**

If local dev currently starts with no DB, decide one explicit local path:

- documented local DB URL required; or
- explicit `GAME_CONTENT_MODE=dev_fallback` only for local development and never
  production.

Production must remain DB required.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/server -run 'ContentRuntime|RuntimeContent|Config|Kalaazu|DBOnly' -count=1
```

Expected: pass.

## Task 8: Remove Production Dependence on Static Catalog Builders

**Files:**

- Modify: `internal/game/content/bundle.go`
- Modify: `internal/game/content/starter_balance.go`
- Modify: `internal/game/world/maps/catalog.go`
- Modify: `internal/game/world/maps/enemy_catalog.go`
- Modify: `internal/game/contentseed/snapshot.go`
- Modify tests that call static builders directly

**Step 1: Write failing guard tests**

Tests:

- production runtime path does not call `content.DefaultGameplayContent`
- static repository is used only when explicitly injected
- default content snapshot rows can be rebuilt without static `StarterCatalog`
  as the authoritative source

Run:

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/server -run 'Static|DefaultGameplayContent|DBOnly|Kalaazu' -count=1
```

Expected: fail until static use is isolated.

**Step 2: Reclassify static builders**

Rename or document static builders as test/legacy helpers:

- `DefaultGameplayContent` can remain temporarily for tests if too much code
  depends on it.
- new production seed builder should be Kalaazu-derived.
- runtime boot must not call static builders unless an explicit test repository
  does so.

**Step 3: Update tests**

Move tests that assert static old values into legacy/helper tests, or rewrite
them to assert Kalaazu DB seed values.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/server -run 'Static|DefaultGameplayContent|DBOnly|Kalaazu' -count=1
```

Expected: pass.

## Task 9: Update Docs and Polish Plan

**Files:**

- Modify: `docs/polish/10-kalaazu-reference-content-source.md`
- Modify: `docs/plans/2026-06-28-darkorbit-feel-implementation.md`
- Modify: `docs/todo.md`
- Modify: `AGENTS.md` only if workflow commands or default content rules change

**Step 1: Document default content source**

Update docs to say:

- default DB seed source is Kalaazu-derived
- runtime truth is published DB content
- static Go catalogs are not production truth
- unsupported Kalaazu rows are tracked in import report

**Step 2: Update Phase 7**

Mark Phase 7 as depending on the DB default seed work instead of manually adding
small static content rows.

**Step 3: Run diff check**

Run:

```bash
git diff --check
```

Expected: pass.

## Task 10: Full Verification

**Files:**

- No planned edits.

**Step 1: Run focused backend verification**

Run:

```bash
go test ./internal/game/contentseed/kalaazu ./internal/game/contentseed ./internal/game/contentdb ./internal/game/content ./internal/game/world/maps ./internal/game/server -count=1
```

Expected: pass.

**Step 2: Run full backend verification**

Run:

```bash
go test ./...
```

Expected: pass.

**Step 3: Run client verification**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run check
```

Expected: pass.

**Step 4: Run whitespace verification**

Run:

```bash
git diff --check
```

Expected: pass.

## Non-Goals

- Do not implement every Kalaazu table in this phase.
- Do not create player inventory/wallet balances directly from item definitions.
- Do not fetch GitHub at runtime.
- Do not expose hidden NPC pools, loot tables, destination internals, or spawn
  future state to clients.
- Do not remove explicit test repositories until affected tests have a DB-backed
  replacement.
