# Content Foundation Design

Date: 2026-06-24

## Goal

Prepare gameplay content for later DB/CMS editing by loading the current static
playtest content through one server-side bundle and validating cross-catalog
references before runtime uses it.

## Current Shape

The codebase already has strong local catalogs:

- maps, portals, safe zones, PvP policy, and enemy pools in `internal/game/world/maps`
- modules in `internal/game/modules`
- ships in `internal/game/ships`
- recipes in `internal/game/crafting`
- production buildings in `internal/game/production`
- loot tables and runtime item definitions currently assembled in
  `internal/game/server/combat_loot_catalog.go`

The gap is cross-catalog ownership. A recipe can reference an item, a drop
profile can reference a loot table, or a production building can reference a
material without one canonical content boundary proving the reference is valid.

## Approved Approach

Add a server-side `GameplayContent` bundle. It is static for now, but shaped so
the same bundle can later come from a DB-backed published CMS revision.

The first slice keeps runtime behavior unchanged and adds the canonical
validation layer. Demo balancing should stay DarkOrbit-like in pacing and
structure: bounded sectors, weak starter drones, stronger border NPCs, small
salvage drops, portal progression, PvE-safe starter maps, and PvP border maps.
Use generic IDs/names for now so we can rebalance later without inheriting
proprietary labels or assets.

- item definitions are indexed once
- module definitions must have matching item definitions
- recipe item inputs and item outputs must reference known item definitions
- recipe ship unlock outputs must reference known ship definitions
- production item inputs and outputs must reference known item definitions
- map enemy drop profiles must reference known loot tables
- map enemy pools, spawn areas, portals, safe zones, bounds, PvP policy, and NPC
  stats keep using existing map catalog validation
- every playable map must own complete enemy content: spawn areas, pools, stat
  templates, drop profiles, aggro profiles, and leash profiles. Unreferenced
  NPC stat/drop/aggro/leash rows fail validation so future CMS drafts cannot
  publish dead or stale monster content.
- loot table rows must reference known item definitions and have valid weights
- scanner/planet discovery config is server-only content: static seed material,
  bounded candidate options, per-map scanner profiles, scanner radar-level
  unit, and discovery XP validate before runtime builds `ScannerService`
- scanner profiles are keyed by bounded map. The demo rows keep first-scan
  discovery stable for the current vertical slice while still letting each map
  own its level band and spawn budget. Later CMS tuning can lower density and
  add rare planet pacing once cooldowns and live balancing tools exist.
- starter/playtest seed config is server-only content: starter ship/display,
  starter wallet, starter module grants/loadout scanner, weekly X Core stock,
  fixed first-NPC entity overrides, playtest claim core quantity, and route
  seed storage. Runtime still performs all authoritative mutations; the content
  bundle only owns validated constants.
- shop/category/product registry content is built in the content package from
  static items, modules, and ships, then validated with `catalog.ContentRegistry`
  reference checks before runtime exposes shop catalog/query handlers.
- route policy content is server-only content: routeable resources, route count
  cap, max distance, cross-map distance penalty, energy formula values, loss
  band, and endpoint storage capacity are validated in the bundle, while
  runtime still derives ownership, distance, storage, and access facts.
- production rule content is server-only content: planet claim range, claim
  production storage/energy defaults, and planet-building build/upgrade costs
  are validated in the bundle, while runtime still owns proximity, ownership,
  wallet debit, and planet-storage mutation.
- combat rule content is server-only content: DarkOrbit-like demo movement,
  radar, loot pickup range, basic laser cost/cooldown, training NPC identity,
  repair quote currency/cost, NPC kill XP, and PvP cargo-drop percentages are
  validated in the bundle. Runtime still owns target visibility, PvP policy,
  cooldown checks, damage, death processing, repair mutation, and snapshots.

## Non-Goals

- No admin CMS UI.
- No DB persistence.
- No client-authored or client-trusted content.
- No gameplay balancing changes.
- No weakening of playtest, leak, or artifact gates.

## Future CMS Path

1. Keep static bundle as seed content.
2. Load runtime through a `content.Repository` boundary. The current
   implementation is `StaticRepository`; a DB-backed implementation can later
   seed an empty DB from the static bundle or load the latest published
   revision.
3. Reuse the same validator before accepting a published revision.
4. Add admin draft/publish/rollback workflows once schemas are stable.

## Verification

Use narrow tests:

- valid static playtest bundle loads
- unknown loot item fails
- enemy drop profile referencing unknown loot table fails
- recipe referencing unknown item fails
- recipe ship unlock referencing unknown ship fails
- production output referencing unknown item fails
- scanner candidate options outside `0..10000`, invalid density, or missing seed
  fail before runtime starts
- scanner map profiles must reference known maps and reject duplicate profile
  rows
- starter content must reference known ship/item/module/map/enemy-pool rows and
  reject duplicate starter modules or invalid wallet/scanner/route quantities
- published content loading must reject missing repositories and revalidate the
  loaded bundle before runtime uses it
- shop registry products must reference known items/modules/ships and keep
  display/category metadata valid before runtime serves shop payloads
- route content must reference known routeable items and reject invalid caps,
  endpoint capacity, energy formula values, or duplicate routeable resources
- production rules must reference known building material items and reject
  duplicate building-cost rows or invalid claim/default numeric values
- combat rules must reject invalid movement/radar/pickup ranges, laser
  cost/cooldown, XP, repair currency, repair fee, NPC ids, and PvP death cargo
  percentages
- map enemy validation must reject incomplete per-map enemy content,
  unreferenced NPC stat/drop/aggro/leash rows, missing pool refs, and invalid
  monster stats
- runtime uses the validated content bundle for item/loot catalogs
  and scanner/planet/starter/shop/route/production/combat-rule config
