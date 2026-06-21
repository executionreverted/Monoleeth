# Bounded Multi-Map Rework Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` or a
> subagent-driven execution workflow before implementing these phases.

**Goal:** Convert the current single-zone/infinite-frontier world direction into
a bounded, DarkOrbit-style, multi-map space game with radar-driven visibility,
portals, safe/PvP areas, rare planet discovery, per-map NPC pools, and
server-owned map state.

**Architecture:** Keep the existing server-authoritative movement, visibility,
combat, loot, scanner, and production services where they already enforce useful
rules. Replace the current "one runtime, one worker, origin-fringe sector,
infinite scan horizon" assumptions with a server-owned map catalog, one active
map instance per map, player map membership, portal handoff, bounded coordinates,
and map-scoped transport events.

**Tech Stack:** Go backend/runtime/domain services, existing realtime gateway,
existing browser client with TypeScript state/reducer and Pixi world renderer.

---

## Canonical Decisions

- Map coordinates are bounded to `0..10000` on both axes for the current design.
- `WorldID` remains the shard/universe identifier.
- `ZoneID` can initially equal `MapID`; do not introduce unnecessary separate
  zoning until one map needs multiple simulation partitions.
- Visibility remains local: a client can only receive and act on entities in the
  player's current map and radar range.
- The fog-of-war wave concept is removed from the playable map surface.
- Radar becomes meaningful through stat/module-driven range, detection, and
  stealth/jammer counterplay.
- Each map owns its active simulation worker, safe/PvP rules, portal list,
  spawn regions, enemy pools, planet scan profile, background/theme, and risk
  modifiers.
- Portal traversal is a client intent, not client truth. The server validates
  proximity, cooldown, destination, spawn position, transfer state, and session
  ownership.
- Portal cooldown starts at `30s`, keyed by
  `player_id + source_map_id + portal_id`.
- Planet scanning remains server-owned and rare. Bounded map profiles replace
  the old infinite discovery horizon.
- Planet claiming requires discovered intel, proximity, rank, X Core item,
  idempotency, inventory/ledger mutation, and durable ownership.
- Enemy spawns are map-owned, capped, and driven by map enemy pool definitions.
- The first ECS step is data-oriented storage inside map workers/spawners, not a
  broad rewrite of every gameplay domain service.
- The client must never submit trusted map id, player id, position, speed,
  damage, hit result, cooldown, planet discovery result, loot table, drop
  contents, wallet amount, inventory amount, or ownership truth.

## Canonical Operation And Event Names

Use these names across implementation docs and code. Do not introduce synonyms
without updating this section first.

- `portal.enter`: client intent to use a visible/interactable portal in the
  player's active map.
- `map.snapshot`: optional map-only snapshot. `world.snapshot` may also carry a
  `map` object during migration.
- `map.transfer_started`: server accepted a portal handoff and entered transfer
  state.
- `map.transfer_completed`: destination map attach committed and the client
  must apply the destination snapshot.
- `map.transfer_failed`: server rejected or rolled back the transfer; player
  remains in the source map.
- `player.protection_updated`: spawn/portal protection state changed.

## Canonical IDs And Wire Fields

Use this vocabulary consistently:

- `internal_map_id`: server-only persistent map id used by catalogs, workers,
  storage, indexes, cooldown keys, scan materialization, route policy, and
  durable ownership rows.
- `public_map_key`: client-safe stable map key used in snapshots, UI labels,
  minimap grouping, and browser state. The browser may display it or use it to
  discard stale events, but commands must not trust it.
- `zone_id`: internal worker ownership id. For the first implementation,
  `zone_id == internal_map_id`.
- `portal_id`: stable portal id scoped by `source_map_id`. Do not use
  `portal_key`, `portal_entity_id`, or `portal.traverse` as alternate names in
  new code/docs.
- `map_subscription_epoch`: server-issued stream epoch for one session's active
  map subscription. Every map-scoped snapshot/event carries it so the server and
  client can drop stale pre-transfer events.

Allowed public wire fields:

- `map.public_map_key` or `map.map_key`
- `map.display_name`
- `map.bounds`
- `map.risk_band`
- `map.pvp_policy`
- client-safe portal summaries
- client-safe safe-zone/protection summaries
- `map_subscription_epoch`

Forbidden command payload fields:

- `map_id`, `internal_map_id`, `zone_id`, `worker_id`, `map_worker_id`
- `public_map_key` when used as gameplay truth
- destination map/spawn fields
- player id, position, speed, cooldown, damage, hit result, loot table, scan
  candidate, discovery result, wallet/inventory amount, ownership truth

## Release Invariants

- Old infinite-world and fog-wave docs must be updated or explicitly marked
  superseded before a map-rework implementation phase is called complete.
- Any remaining old-doc contradiction must be listed in `docs/todo.md` with the
  implementation phase that owns it.
- No implementation phase may be accepted if it relies on default client
  fixtures to hide an absent server contract.

## Phase Order

1. [Map Catalog And Router](./phase-01-map-catalog-router.md)
2. [Runtime Map Instances](./phase-02-runtime-map-instances.md)
3. [Map Transport And Handoff](./phase-03-map-transport-and-handoff.md)
4. [Portals, Safe Zones, And PvP](./phase-04-portals-safe-zones-pvp.md)
5. [Radar, Stealth, And AOI](./phase-05-radar-stealth-aoi.md)
6. [Bounded Scanner And Planets](./phase-06-bounded-scanner-planets.md)
7. [Planet Claim, Production, And Routes](./phase-07-planet-claim-production-routes.md)
8. [Enemy Pools, Spawners, And ECS](./phase-08-enemy-pools-spawners-ecs.md)
9. [Client Map UI And Protocol](./phase-09-client-map-ui-protocol.md)
10. [Testing, Migration, And Rollout](./phase-10-testing-rollout.md)

## Current Code Anchors

- Live entity identity already carries `WorldID` and `ZoneID`:
  `internal/game/world/types.go`.
- The worker is already a single-owner, one-zone simulation unit:
  `internal/game/world/worker/worker.go`.
- The runtime currently owns one worker directly:
  `internal/game/server/runtime.go`.
- World commands currently route to that one worker:
  `internal/game/server/handlers.go`.
- Visibility already filters same world/zone and radar range:
  `internal/game/world/visibility/visibility.go`.
- Combat already rejects different world/zone and invisible/out-of-range targets:
  `internal/game/combat/service.go`.
- Scanner and planet candidate generation are server-owned:
  `internal/game/discovery/scanner.go`,
  `internal/game/discovery/candidate.go`.
- The current browser world renderer already has a server-driven entity surface:
  `client/src/render/world-renderer.ts`.

## Cross-Phase Invariants

- No cross-map leakage. Snapshots, AOI diffs, minimap contacts, combat events,
  loot events, scan results, and admin/debug payloads must be map-scoped.
- Broadcast after authoritative mutation. Portal transfers, combat kills, loot
  drops, scan discoveries, claims, production mutations, and route mutations
  emit after the owning service/worker accepts the change.
- Map membership is server state. Reconnect must restore the player's last
  active map and position from server-owned persistence or a safe fallback.
- Bounds are enforced server-side. Client-side clamping is only UX.
- Safe-zone and PvP policy is enforced server-side in combat/death/loot rules.
- Radar and stealth are server-side visibility inputs, never cosmetic truth.
- Per-map content definitions are catalog data, not hard-coded runtime branches.
- Generated planet candidates and NPC spawn rolls are never serialized as hidden
  seeds, future spawn candidates, loot rolls, or drop tables.

## Docs To Rewrite Or Supersede

- `docs/2026-06-17-world-system-design.md` must be rewritten to remove the
  "no hard border", "infinite coordinate plane", "zones are invisible to
  players", and "fog memory surface" assumptions.
- `docs/2026-06-17-progression-economy-systems-design.md` must align death,
  cargo drop, routes, scan progression, X Core, safe zones, and PvP risk with
  bounded maps.
- `docs/plans/ui-implementation/04-live-world-aoi-movement.md` must be amended
  after the map router and bounded movement phases land.
- `docs/plans/ui-implementation/07-discovery-planets-production-routes.md` must
  be amended after bounded scanner and map-aware planet ownership land.
- `docs/plans/modules/14-world-aoi-fog-security.md` must shift from fog wording
  toward radar, stealth, same-map membership, and client-safe map intel.

## Out Of Scope For This Rework

- Full clan/faction war systems.
- Full physical convoy combat.
- Player-built stations.
- Multi-zone partitioning inside a single 10000x10000 map.
- Region-scale procedural infinite frontier.
- Client-authored map editor/admin content.

## Verification Baseline

Before claiming any implementation phase complete, run:

```bash
go test ./...
git diff --check
cd client
npm --cache /tmp/gameproject-npm-cache run check
```

Use narrower tests while developing, but the full commands above are required
for final handoff.
