# Phase 07: Planet Claim, Production, And Routes

## Goal

Make planet claiming, production, and automation routes map-aware after bounded
scanner discovery. Claiming must require a discovered planet, active-map
proximity, rank, X Core consumption, idempotency, ledger/inventory mutation, and
durable ownership. Production and routes remain owned-planet systems, but every
contract must carry enough map identity for route rules, risk, UI grouping, and
future PvP exposure.

No client request may assert player id, map id, position, ownership, X Core
state, production output, storage contents, route loss, or settlement timing as
truth.

## Current State To Replace/Reuse

- Reuse the domain claim flow in `internal/game/discovery/claim.go:46`,
  `internal/game/discovery/claim.go:244`,
  `internal/game/discovery/claim.go:520`,
  `internal/game/discovery/claim.go:534`,
  `internal/game/discovery/claim.go:555`,
  `internal/game/discovery/claim.go:576`, and
  `internal/game/discovery/claim.go:610`: it already validates player intel,
  materialized planet state, proximity, rank, X Core consumption, owner change,
  production initialization, stale intel/listing hooks, and duplicate claim
  references.
- Replace the in-memory claim durability boundary in
  `internal/game/discovery/claim.go:244` and
  `internal/game/discovery/store.go:228`: the current MVP serializes claims in
  process and records owner changes in memory, but map-aware ownership needs a
  durable transaction/CAS plus outbox.
- Reuse the planet/intel shapes from
  `internal/game/discovery/planet.go:62`,
  `internal/game/discovery/intel.go:48`, and
  `internal/game/discovery/store.go:158`, after phase 06 adds map identity.
- Reuse production initialization and snapshots from
  `internal/game/production/store.go:15`,
  `internal/game/production/store.go:86`, and
  `internal/game/production/settlement.go:75`: owned planets already have
  production state, storage, buildings, server-timed settlement, storage caps,
  and production events.
- Reuse route service policy boundaries from
  `internal/game/production/route.go:47`,
  `internal/game/production/route.go:104`,
  `internal/game/production/route.go:120`,
  `internal/game/production/route_service.go:51`,
  `internal/game/production/route_service.go:97`, and
  `internal/game/production/route_service.go:187`: routes already validate
  owner, source, destination, policy, risk, enable/disable/update, and owner
  wrappers.
- Replace route rows that lack map identity in
  `internal/game/production/route.go:47`: source/destination map context must
  be stored or derivable without querying hidden planet data.
- Reuse route settlement mechanics from
  `internal/game/production/route_settlement.go:34` and
  `internal/game/production/route_settlement.go:56`: settlement is server-timed
  and storage-cap aware, but it must become idempotent/durable and map-risk
  aware.
- Replace read-only gateway exposure in
  `internal/game/server/discovery_production_handlers.go:204`,
  `internal/game/server/discovery_production_handlers.go:222`,
  `internal/game/server/discovery_production_handlers.go:235`,
  `internal/game/server/discovery_production_handlers.go:247`, and
  `internal/game/server/discovery_production_handlers.go:260`: mutation
  handlers for claim, build, upgrade, route create/update/enable/disable/settle
  are still absent or tracked as future work.
- Keep the open TODO contracts in `docs/todo.md` for claim, building, storage,
  route mutations, durable discovery stores, durable production stores, and
  route settlement outbox until implementation closes them.

## Phase07A Landed Backend MVP Slice

- Added the Go realtime operation `discovery.claim_planet` with intent-burst
  posture and owner-scoped `planet.claimed` client event.
- Runtime now constructs a discovery `ClaimService` using the existing
  in-memory discovery store, production store, inventory service, progression
  service, map router/catalog, and server clock.
- Claim rank is resolved from server-owned progression state. The command does
  not accept rank or player identity from the client.
- Claim proximity is resolved from the player's active map/entity state. Claims
  fail during active map transfer, across maps, or outside the conservative
  runtime claim range.
- `x_core` is registered as a stackable runtime item definition and claim
  consumption uses `Inventory.SystemRemoveItem` with
  `planet_claim:<player_id>:<planet_id>` as the domain idempotency key.
- Successful claims initialize production through
  `production.NewClaimProductionInitializer` with conservative default storage
  and energy capacity.
- The realtime handler accepts only `{ "planet_id": "..." }`, rejects trusted
  or unknown payload fields before mutation, returns refreshed safe
  known/detail/production/inventory state, and queues owner-scoped claim,
  known-planet, detail, production, and inventory events after success.
- The `planet.claimed` payload is a client-safe summary. It does not include
  internal map id, world id, zone id, owner player id, hidden coordinates,
  X Core source details, or production internals.

Deferred from Phase07A:

- Durable discovery/ownership DB rows, row locks/CAS, and outbox publishing.
- Crash recovery for owner-set but production-initialization/event-cache gaps.
- Route create/update/enable/disable/settle handlers and map-aware route rows.
- Building build/upgrade/storage mutation handlers.
- Browser claim button/UI flow and TypeScript protocol exposure.
- Durable market/listed-intel stale-listing adapter.

## Phase07B Landed Read-Model Slice

- Owned planet production, storage, and building read payloads now include
  catalog-derived `public_map_key` values. The gateway derives those keys from
  the materialized planet row's map/zone and the map catalog, not from client
  input.
- Production and storage summaries still filter to the authenticated player's
  active map. Tests cover map `1-1` and `1-2` after active-map switching and
  assert browser JSON omits internal map/world/zone identifiers.
- `production.AutomationRoute` rows now store validated internal source and
  destination map ids supplied by `RouteCreatePolicy`. Create stores both map
  ids, update preserves the source map id and refreshes destination map id from
  policy, and clone/settlement/control flows preserve them.
- Route list and snapshot payloads expose only `from_public_map_key` and
  `to_public_map_key`, resolved from the route row through the map catalog.
  Owner scoping remains enforced; non-owner snapshots return safe not-found.
- The browser discovery reducer now parses route public map keys from route
  list/detail/snapshot payloads. Protocol command builders remain unchanged.

Deferred after Phase07B:

- Route update/enable/disable/settle gateway handlers.
- Building build/upgrade/storage mutation handlers.
- Durable discovery/ownership and production/route DB rows, row locks/CAS,
  settlement idempotency rows, and outbox publishing.
- Browser claim UI and browser route mutation UI.

## Phase07C Landed Route Create Gateway Slice

- Added authenticated `route.create` to the realtime registry with intent-burst
  posture and owner-scoped `route.updated` fanout.
- The gateway accepts only `source_planet_id`, `destination_planet_id`,
  `resource_item_id`, and `amount_per_hour`. It rejects client-authored
  owner/player/session, route id, map id/public map key, energy, risk, cost,
  enabled, timestamp, position, cooldown, and storage truth before mutation,
  including nested payload fields.
- Route ids are derived from the request id as `route-<request_id>` and are
  validated through the foundation route id parser. The owner is resolved from
  the authenticated command context.
- Runtime route policy derives source/destination map ids from server-owned
  discovery planet rows and the map catalog. The MVP requires both endpoint
  planets to be owned by the authenticated player and uses an explicit
  routeable resource allowlist instead of treating every stackable catalog item
  as routeable.
- Successful creation uses `production.AutomationRouteService` against the
  runtime production store and clock, returns safe `route` plus refreshed
  `routes` payloads, and queues owner-scoped `route.updated`,
  `route.snapshot`, and `route.list` events without internal map/world/zone ids.

Deferred after Phase07C:

- Route update/enable/disable/settle gateway handlers.
- Browser route create/update proof and TypeScript protocol/UI work.
- Durable production/route DB rows, row locks/CAS, settlement idempotency rows,
  and outbox publishing.

## Target Model

Planet claim is a server-owned transaction:

```text
client intent: discovery.claim_planet { planet_id }
server resolves: session -> player -> active map -> active position -> planet
server validates: personal/shared intel, planet map == active map, range,
                  rank >= planet level, unowned/CAS owner state, X Core source,
                  idempotency reference, rate limit/cooldown if configured
server mutates: reserve/consume X Core through inventory ledger, set owner,
                initialize production rows, mark stale intel/listings, outbox
server emits: planet.claimed, discovery/planet/detail snapshots, production init
```

Planet ownership is global durable truth. Coordinates remain server-approved
intel. Claiming a planet may update other players' stale intel status, but it
must not reveal hidden coordinates or other-map live state.

Production stays attached to owned planets:

- production state, storage, and buildings are keyed by `planet_id`
- snapshots and events include public map key for UI grouping and internal map
  id for server policy checks
- settlement uses server time only
- offline settlement remains capped, deterministic, storage-cap aware, and
  idempotent by settlement window
- building mutation debits materials/wallet/storage through ledger-backed
  services before publishing snapshots

Routes stay virtual for MVP, but become map-aware:

- each route stores source planet, source map, destination type/id, destination
  map when applicable, resource, amount/hour, energy/hour, risk, enabled state,
  and last settlement cursor
- route policy validates endpoint ownership/access, known intel, map route
  rules, portal graph reachability if cross-map routes are allowed, capacity,
  energy/upkeep, and resource routeability
- route risk uses map profiles, safe/PvP policy, source/destination map risk,
  distance, portal hops, and future route-security modifiers
- route events are owner-scoped by default; future physical convoy/PvP exposure
  can publish map-local safe sightings without leaking hidden route contents

MVP route scope should be conservative. Same-map planet-to-planet routes are the
safest first slice. Cross-map virtual routes can be enabled only when the map
catalog has explicit route rules or portal graph paths and the policy provider
can calculate risk without leaking hidden destination data.

### Production To Crafting Future Contract

Planet production is the strategic resource source for later crafting. This
phase does not need to build every crafting UI, but it must preserve these
contracts:

- produced materials are real inventory/storage entries, not UI counters
- crafting authorizers can require materials to exist in a specific owned
  planet storage or an allowed storage network
- building upgrades and routes can consume production outputs through
  ledger-backed storage mutations
- production snapshots expose only client-safe item ids/amounts/capacity
- crafting jobs never trust client-supplied production rates, storage contents,
  route deliveries, or planet ownership
- route settlement and production settlement must write enough event/ledger
  references for later craft-cost audits

## Data Structures/Contracts To Add Or Change

- Claim command contract:
  - Client sends only `planet_id` and request id.
  - Gateway derives `player_id`, active internal map id, ship/position, and
    stable claim reference, for example
    `planet_claim:<player_id>:<planet_id>`.
  - Domain input may include server-derived `ActiveMapID` or use a proximity
    provider that receives active map context.
- `ClaimProximityInput` must validate active map, planet map, range, and
  bounded map-local coordinates. `WorldID` plus `ZoneID` is acceptable if
  `ZoneID == MapID` for MVP.
- Claim idempotency must be durable. Duplicate retries with the same reference
  return the original result. Reuse with conflicting player/planet fails.
- X Core consumption contract:
  - source location is server-selected
  - quantity is exactly one unless future tier rules change it
  - ledger reason remains claim-specific
  - reference is the claim idempotency key
  - duplicate consumption is safe
- Planet ownership storage:
  - `planet_id`
  - `world_id`
  - `internal_map_id`
  - `public_map_key` cached or derivable for read models
  - map-local coordinates
  - owner player id
  - owner changed timestamp
  - claim reference/source event
  - unique unowned-to-owned CAS or equivalent row lock
- Production rows and snapshots:
  - include public map key in payloads and internal map id in event metadata
  - production state may derive map id from the planet row, but API payloads
    should not require clients to join hidden data
  - building build/upgrade inputs remain client intent only:
    `planet_id`, building/slot/type/upgrade target, request id
- Route rows:
  - add `source_map_id`
  - add `destination_map_id` for planet destinations
  - keep `destination_type` and `destination_id`
  - keep risk fields, but calculate them from map policy instead of old
    infinite-region/deep-space assumptions
  - add durable settlement cursor/window id for idempotency
- Route command contracts:
  - `route.create`
  - `route.update`
  - `route.enable`
  - `route.disable`
  - `route.settle`
  - server derives owner from session; client never submits trusted owner
- Events/read models include internal map id for services and public map key
  for browser payloads:
  - `planet.claimed`
  - `planet.production_updated`
  - `planet.storage_updated`
  - `planet.building_updated`
  - `route.updated`
  - `route.settled`
  - `route.source_empty`
  - `route.destination_full`
  - `route.transfer_lost`

## Implementation Tasks In Order

1. Depend on phase 06 map-aware planet materialization and personal intel.
2. Add a claim gateway handler for `discovery.claim_planet` that rejects trusted
   payload fields and resolves player, active map, active position, and stable
   claim reference from server context.
3. Update claim proximity validation so the player must be in the same active
   map as the planet and inside claim range. Cross-map claim must fail even if
   the player knows the coordinates.
4. Move claim owner changes, X Core consumption, production initialization,
   stale intel/listing markers, and claim event/outbox writes into a durable
   transaction or explicit recoverable state machine:
   `lock -> validate -> mutate ledger/inventory -> set owner -> init production -> outbox -> commit`.
5. Add recovery for production initialization after claim. A retry must repair
   missing production rows without consuming a second X Core or changing owner
   twice.
6. Add public map key to production snapshots and storage/building read
   payloads, while keeping internal map id in server-side policy/event metadata.
7. Add `planet.building_build` and `planet.building_upgrade` handlers only when
   they can validate owner, map-aware access policy, requirements, material and
   wallet costs, storage capacity, idempotency, and outbox publication.
8. Add map-aware route endpoint policy. Start with same-map owned
   planet-to-planet routes unless the catalog/router already supplies explicit
   cross-map route reachability and risk.
9. Add `source_map_id` and destination map identity to `AutomationRoute` and its
   safe payloads. Existing routes in tests/local data should backfill from the
   source/destination planet rows.
10. Update route risk calculation to use bounded map policy:
    safe/PvP zone, source/destination map risk, route distance within map or
    portal-hop distance, player bonuses, and route security modifiers.
11. Add route mutation handlers using owner wrappers and server-derived player
    id. `route.create` has landed for owned planet-to-planet MVP; update,
    enable, disable, and settle remain open.
12. Make route settlement durable and idempotent by route/window key, for
    example `route_settle:<route_id>:<window_start>:<window_end>`.
13. Add owner-scoped fanout after commit for claim, production, storage,
    building, and route snapshots. Map-local public events may be added later
    only with client-safe data and visibility checks.
14. Update read models so planet and route lists can be grouped by map, while
    actions remain locked unless active-map and range/access policies pass.
15. Update `docs/todo.md` and UI implementation docs only after the matching
    implementation and verification are complete.

## Tests To Add/Update

- Claim rejects unknown, hidden, unshared, invalidated, or stale-ineligible
  planet intel.
- Claim rejects when the planet is in a different map from the player's active
  map.
- Claim rejects when the player is in the same map but outside claim range.
- Claim rejects rank below planet level.
- Claim consumes exactly one X Core once for the claim reference.
- Duplicate claim retry returns the original result and does not consume a
  second X Core.
- Conflicting claim reference reuse fails safely.
- Concurrent claims for the same unowned planet produce one owner and one
  durable claim event.
- Claim initializes production rows once and recovers missing initialization on
  retry.
- Claim marks stale intel/listings without revealing hidden coordinates.
- Building build/upgrade handlers reject unowned planets, wrong-map access, bad
  requirements, insufficient materials/wallet, capacity overflow, and duplicate
  references.
- Production settlement is capped, server-timed, storage-cap aware, idempotent,
  and emits map-tagged events after commit.
- Route create accepts only intent fields, derives owner/route id/map ids
  server-side, reconciles safe route list/snapshot payloads, and rejects
  unowned source, inaccessible destination, hidden endpoint, bad resource,
  insufficient capacity, and forged owner/map/energy/risk payloads.
- Route update settles old terms before replacing terms and preserves owner/map
  rules.
- Route enable/disable/settle owner wrappers reject wrong-owner attempts.
- Route settlement is idempotent by durable window and respects source storage,
  destination capacity, loss rolls, and map-risk policy.
- Realtime/event tests prove claim, production, and route events do not leak to
  other maps or unrelated sessions.
- Browser/API tests prove mutation controls reconcile from server snapshots and
  never present fake owned planets, storage, production, routes, or X Core
  state.

## Migration/Doc Updates

- Update the world design docs to describe bounded maps, map-local planet
  ownership, same-map claim rules, and no fog-wave gameplay.
- Update progression/economy docs to align X Core, production, storage, route
  risk, death/PvP risk, and route exposure with bounded maps.
- Update `docs/plans/ui-implementation/07-discovery-planets-production-routes.md`
  when mutation handlers exist, including command payloads, map ids, empty
  states, locked states, and abuse controls.
- Keep `docs/todo.md` entries open until durable repositories/outbox,
  claim-production recovery, route mutation contracts, and settlement
  idempotency are implemented.
- Durable migration path:
  - existing materialized planets get the starter map id and validated
    map-local coordinates
  - existing production rows derive map id from planet rows
  - existing routes derive source/destination map ids from endpoint planets
  - any row outside bounds is quarantined for manual repair, not silently
    clamped
- Add operational docs for route risk tuning per map, X Core source policy, and
  claim abuse/rate-limit posture.

## Risks And Acceptance Criteria

Risks:

- Claim is an economy mutation and ownership mutation in one flow. Without a
  durable transaction or state machine, X Core consumption and owner state can
  diverge.
- Production initialization after claim can fail after owner mutation. Recovery
  must be explicit and duplicate-safe.
- Route rows without map identity make future PvP exposure and UI grouping
  ambiguous.
- Cross-map routes can leak hidden destination knowledge if endpoint policy is
  not strict.
- Owner-only route events can later conflict with physical convoy visibility if
  event scopes are not named carefully.

Acceptance criteria:

- `discovery.claim_planet` succeeds only for an authenticated player with
  server-approved planet intel, same active map, proximity, rank, and one
  consumable X Core.
- Claim idempotency, X Core ledger mutation, owner CAS, production
  initialization, stale intel/listing updates, and events are durable or
  recoverable.
- Claimed planets carry map identity through ownership, production, storage,
  building, and route read models.
- Production settlement remains server-timed, capped, idempotent, and
  storage-cap aware.
- Route rows carry source/destination map identity and use map policy for
  endpoint access and risk.
- Route mutations and settlements use server-resolved ownership and do not
  accept client-authored owner/map truth.
- Events and socket fanout do not leak other-map or hidden route/planet data.
