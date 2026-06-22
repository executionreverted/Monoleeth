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

- Route create landed in Phase07C, enable/disable landed in Phase07D, update
  landed in Phase07E, and settle landed in Phase07F below.
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

- Route enable/disable gateway handlers, later landed in Phase07D below.
- Route update later landed in Phase07E and route settle later landed in
  Phase07F below.
- Browser route create/update/control proof and TypeScript protocol/UI work,
  later landed in Phase07G.
- Durable production/route DB rows, row locks/CAS, settlement idempotency rows,
  and outbox publishing.

## Phase07D Landed Route Control Gateway Slice

- Added authenticated `route.enable` and `route.disable` to the realtime
  registry with intent-burst posture.
- The gateway accepts only `{ "route_id": "..." }`. It rejects
  client-authored owner/player/session, map/internal/public map fields,
  enabled state, settlement facts, source/destination route facts, timestamps,
  storage, energy/risk/loss/cost/rate/resource facts, cooldown, position, and
  hidden internals before mutation, including nested payload fields.
- The owner is resolved from the authenticated command context and passed to
  `production.AutomationRouteService.EnableRouteForOwner` or
  `DisableRouteForOwner` against the runtime production store, server clock,
  and runtime route policy provider.
- Successful controls return safe `route` plus refreshed `routes` payloads and
  queue owner-scoped `route.updated`, `route.snapshot`, and `route.list` events
  without internal map/world/zone ids. When `route.disable` settles an elapsed
  route period that touches storage, the response also includes active-map
  filtered `production` and `storage` snapshots and queues owner-scoped
  `planet.production_summary` and `planet.storage_summary` events. No new
  route settlement event type was added in this slice.

Deferred after Phase07D:

- Route update later landed in Phase07E and route settle later landed in
  Phase07F below.
- Browser route create/update/control proof and TypeScript protocol/UI work,
  later landed in Phase07G.
- Durable production/route DB rows, row locks/CAS, settlement idempotency rows,
  and outbox publishing.

## Phase07E Landed Route Update Gateway Slice

- Added authenticated `route.update` to the realtime registry with intent-burst
  posture.
- The gateway accepts only `route_id`, `destination_planet_id`,
  `resource_item_id`, and `amount_per_hour`. It rejects client-authored
  owner/player/session, route/list payloads, source/source-planet/source-map
  facts, destination object/type/id/map/public-map facts, enabled state,
  settlement facts, timestamps, storage, energy/cost, risk/loss, cooldown,
  position/coordinates, and non-exact amount/rate/resource aliases before
  mutation, including nested payload fields.
- The owner is resolved from the authenticated command context, while the route
  source and existing owner are loaded from the server-owned route row.
  Destination is rebuilt with `production.NewPlanetRouteDestination`, and the
  runtime calls `AutomationRouteService.UpdateRouteForOwner` with the runtime
  production store, server clock, and map-aware route policy provider.
- Successful updates return safe `route` plus refreshed `routes` payloads and
  queue owner-scoped `route.updated`, `route.snapshot`, and `route.list` events
  without internal map/world/zone ids or AOI diffs. If the pre-update
  settlement touches storage, the response also includes active-map filtered
  production/storage snapshots and queues owner-scoped
  `planet.production_summary` and `planet.storage_summary` events. No
  `route.settled` event was added in this slice.
- Focused gateway tests cover owned route term updates, elapsed settlement
  storage reconciliation, wrong-owner rejection without mutation/events,
  spoofed server-owned payload rejection before mutation, X Core/non-routeable
  resource rejection, and realtime registry acceptance.

Deferred after Phase07E:

- Route settle gateway handler later landed in Phase07F below.
- Browser route create/update/control proof and TypeScript protocol/UI work,
  later landed in Phase07G.
- Durable production/route DB rows, row locks/CAS, settlement idempotency rows,
  and outbox publishing.

## Phase07F Landed Route Settle Gateway Slice

- Added authenticated `route.settle` to the realtime registry with
  intent-burst posture and owner-scoped `route.settled` fanout. Runtime
  post-command draining includes the operation but explicitly suppresses AOI
  diff emission.
- The gateway accepts only `{ "route_id": "..." }` to settle one owned route
  or `{}` as an owner reconcile intent. It rejects client-authored
  owner/player/session, map/internal/public map truth, route/list/result
  payloads, source/destination facts, enabled state, timestamps, settlement
  windows, storage, energy/cost, risk/loss, wanted/taken/lost/delivered/added
  amounts, amount/rate/resource aliases, cooldown, and position/coordinate
  fields before mutation, including nested payload fields. Unknown fields fail
  strict decode.
- The owner is resolved from the authenticated command context. Single-route
  settlement calls `AutomationRouteService.SettleRouteForOwner`; owner
  reconcile enumerates only stored routes whose owner matches the authenticated
  player and settles each through the same owner wrapper.
- Successful single-route responses include safe `route`, refreshed safe
  `routes`, and safe `settlement`. Owner reconcile responses include refreshed
  safe `routes` and one safe settlement per owned route. Settlement payloads
  expose only route id, resource item id, server settlement time, elapsed
  applied milliseconds, wanted/taken/lost/delivered/added amounts, and
  source-empty/destination-full/loss/no-op booleans.
- For each settled route the runtime queues owner-scoped `route.settled`,
  `route.updated`, and `route.snapshot` events, plus one `route.list` after all
  settlements. When any settlement touches storage, responses and events include
  active-map filtered `planet.production_summary` and
  `planet.storage_summary` snapshots once.
- Focused gateway tests cover storage transfer, route cursor advancement,
  safe response/event payloads, no other-session leak, no AOI diff, immediate
  no-op duplicate settlement, owner-wide reconcile scoping, wrong-owner
  rejection without mutation/events, spoofed server-owned field rejection,
  invalid route ids, safe error mapping, realtime registry acceptance, and
  observability command-security evidence.

Deferred after Phase07F:

- Browser route create/update/control/settle proof and route UI work landed in
  the Phase07G browser slice below.
- Durable production/route DB rows, row locks/CAS, durable settlement window
  idempotency rows, and outbox publishing.

## Phase07G Landed Browser Route Mutation Proof

- Added TypeScript protocol constants and command builders for `route.create`,
  `route.update`, `route.enable`, `route.disable`, and `route.settle`.
- Browser route command payloads contain only client intent fields:
  `source_planet_id`, `destination_planet_id`, `resource_item_id`,
  `amount_per_hour`, and/or `route_id`. They do not include owner/player,
  session, map, position, enabled, risk, energy, storage, settlement, or timing
  truth.
- Added route controls to the owned selected-planet HUD surfaces. Controls are
  disabled unless authenticated realtime state exposes an owned source planet,
  another owned known endpoint, and server-owned source storage resource state.
- Client reducer/event handling now reconciles `route.updated` and
  `route.settled`, clears pending route operations, and logs safe public route
  status without an unhandled-event entry.
- Added guarded dev-only `GAME_E2E_ROUTE_SEED`, accepted only with
  `GAME_DEV_MODE=1`, to seed two owned same-map production planets plus
  routeable source storage for the browser proof. The seed is session-time,
  idempotent, and exposes no browser setup endpoint.
- Added `client/tests/e2e/phase10-route-flow.mjs` and
  `npm --cache /tmp/gameproject-npm-cache --prefix client run
  e2e:phase10-route`, which boots the real Go server and Vite client,
  registers a real browser user, clicks the HUD to create, update, disable,
  enable, settle one route, and reconcile all routes, captures outbound
  WebSocket frames, asserts exact client-safe payload keys, verifies browser
  `state.routes` reconciliation, and scans DOM/state/WebSocket/log surfaces for
  forbidden internals.

Deferred after Phase07G:

- Durable production/route DB rows, row locks/CAS, durable settlement window
  idempotency rows, and outbox publishing.
- Broader route policy/UI matrix beyond the owned same-map MVP proof.

## Phase07H Landed Production Summary Settlement Slice

- Authenticated `planet.production_summary` and `planet.storage_summary` now
  reconcile eligible owned active-map planet production before returning
  snapshots. The handlers derive owner and active map from authenticated runtime
  state, gather only complete owned production snapshots in that active map,
  call `SettlePlanetProductionIfWholeOutputAvailable` with one request-scoped
  server timestamp, and shape fresh client-safe production/storage payloads.
- The query payloads still accept only `{}` or `{ "planet_id": "..." }`.
  Client-authored owner/player, map, time, elapsed, output, storage, building,
  procedural, and internal fields are rejected before settlement.
- If settlement changes production/storage time or contents, the runtime queues
  owner-scoped `planet.production_summary` and `planet.storage_summary` events
  to that player's sessions and suppresses AOI diffs. Immediate duplicate
  and near-immediate sequential duplicate queries no-op through the production
  store without advancing `last_calculated_at`, duplicating output, or queuing
  reconciliation events.
- Focused server tests cover active-map/owner filtering, storage reflection,
  one request-scoped settlement timestamp across multiple planets, safe
  response/event payloads, spoofed-field rejection before mutation, and
  immediate/near-immediate duplicate no-op behavior.

Deferred after Phase07H:

- Durable production DB rows, row locks, idempotency rows, and outbox
  publishing.
- Building build/upgrade/storage mutation handlers and ledger-backed
  material/wallet/storage transaction flows.
- Broader browser proof for production settlement UI timing beyond the
  existing production/storage read-model surfaces.

## Phase07I Landed Settlement Evidence Readiness Slice

- Production settlement results now include `reference_key` and
  `settlement_window` evidence derived server-side from the applied settlement
  cursor window. Non-no-op production settlement events
  `planet.production_settled` and `offline.settlement_completed` carry the same
  evidence in the outbox-safe domain payload.
- Route settlement results now include `reference_key` and `settlement_window`
  evidence derived server-side from the applied route cursor window. Route
  transfer domain events carry the same evidence in their outbox-safe payloads.
- Settlement windows use deterministic colon-free
  `<from_unix_ms>-<to_unix_ms>` strings. Capped offline settlements use the
  actual applied start/end window instead of the longer requested elapsed span.
- Browser-safe realtime settlement response and event payloads are unchanged;
  reference/window evidence stays in domain results and domain event payloads.
- Immediate duplicate/no-op settlements still emit no domain events and do not
  require reference/window evidence.

Deferred after Phase07I:

- Durable production/route DB rows, row locks/CAS, idempotency table
  enforcement, and outbox publishing.
- Durable retry/recovery workers that use settlement references to reconcile
  missed or duplicate outbox publications.

## Phase07J Landed In-Memory Settlement Boundary Slice

- The production store now records one in-memory settlement reference for each
  non-no-op production or route settlement, keyed by the server-derived
  `offline_settlement:*` or `route_settlement:*` idempotency key and carrying
  the settlement window, kind, planet or route id, applied time, and recorded
  time.
- The same store lock now covers settlement state mutation, settlement
  reference recording, domain event append, and creation of pending in-memory
  outbox records. Outbox records carry their own sequence/id, pending status,
  created time, the domain event envelope, cloned payload bytes, and
  reference/window metadata when the settlement payload carries it.
- If an otherwise non-no-op production or route settlement attempts to reuse an
  already-recorded settlement reference, it returns a safe no-op before storage
  or cursor mutation and does not append duplicate events or outbox records.
  Normal timestamp duplicate no-ops still emit no events/outbox and keep empty
  reference/window evidence.
- Browser-safe realtime response and event payloads are unchanged; reference
  and outbox records remain internal store read APIs for future durable
  adapters.

Deferred after Phase07J:

- Real durable production/route DB rows and outbox rows.
- Row locks/CAS and idempotency-table enforcement across processes.
- Durable publisher/retry workers that mark outbox records delivered, retry
  failed publications, and reconcile missed duplicate settlement publications.

Phase07K/Phase07L process-local publisher follow-up:

- The in-memory production outbox now has pending, in-flight, published, and
  failed delivery states plus explicit retry behavior.
- Each claim generates a deterministic per-attempt claim token. Publish/fail
  callbacks must present the current token while the record is in-flight, so
  stale callbacks from older attempts return `ok=false` without mutating a
  retried or reclaimed record.
- These guards protect the local publisher boundary only. Durable DB outbox
  rows, cross-process row-lock/CAS semantics, DB idempotency enforcement, and a
  durable publisher process remain deferred.

## Phase07M Landed Process-Local Claim Boundary Slice

- The discovery claim service now records a process-local
  `ClaimReferenceRecord` for each successful cached claim result. Records are
  keyed by the server-derived `PlanetClaimReference`, include player/planet
  identity, claim and record timestamps, already-owned repair status, and the
  claim event id when a new owner-change event exists.
- New owner-change claim success still appends exactly one `planet.claimed`
  claim event, and the same service lock now mirrors that append into one
  pending process-local `ClaimOutboxRecord` with append-order sequence/id,
  event evidence, created time, pending status, and claim reference.
- Duplicate retries with the same reference return the cached result without
  appending duplicate reference, event, or outbox records. Conflicting reuse of
  a reference still fails safely before boundary mutation.
- Initializer and stale-listing failures record no claim reference or outbox
  row before retry. If retry repairs an owner that was already set by the
  failed attempt, it preserves the current no-event repair behavior and records
  only the already-owned claim reference/cache.
- `ClaimReferences()`, `ClaimReference(ref)`, and `ClaimOutboxRecords()` are
  internal read APIs for future durable adapters. They return detached records
  in deterministic reference order or append order and do not change the
  browser-safe realtime claim payload.

Deferred after Phase07M:

- Real durable discovery/claim DB rows and outbox rows.
- Cross-process row locks/CAS and idempotency-table enforcement tying X Core
  consumption, owner transition, production initialization, stale markers,
  claim cache, and event/outbox publication together.
- Durable publisher/retry workers and recovery for crash windows between the
  current in-memory mutation steps.

## Phase07N Landed Process-Local Claim Publisher Slice

- Process-local claim outbox records now use pending, in-flight, published, and
  failed delivery states plus claim, publish, fail, and explicit retry APIs.
- Claiming a pending row increments attempts and assigns a deterministic
  per-attempt claim token. Publish and fail callbacks require the current
  non-empty token while the row is in-flight, so wrong, missing, stale,
  retried, or already-published callbacks return `ok=false` without mutation.
- Retry moves failed rows back to pending in append order, clears the claim
  token and claim timestamp, preserves failure diagnostics, and records retry
  evidence for process-local publisher adapters.
- `ClaimOutboxRecords()` remains an append-order diagnostic API, while pending,
  claim, publish, fail, and retry APIs all return detached records and keep
  `planet.claimed` payloads browser-safe and unchanged.

Deferred after Phase07N:

- Real durable discovery/claim DB rows and outbox rows.
- Cross-process row locks/CAS and idempotency-table enforcement tying X Core
  consumption, owner transition, production initialization, stale markers,
  claim cache, and event/outbox publication together.
- Durable publisher/retry workers, durable outbox persistence, and recovery for
  crash windows between the current in-memory mutation steps.

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
   Phase07M added a process-local claim reference/outbox boundary around
   successful cached claim results, and Phase07N added process-local claim
   outbox delivery state plus claim-token guards. Durable DB rows,
   cross-process recovery, durable outbox persistence, and idempotency-table
   enforcement remain open.
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
    id. `route.create`, `route.update`, `route.enable`, `route.disable`, and
    `route.settle` have landed for the owned-route backend MVP.
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
- Claim success records one process-local claim reference and one pending claim
  outbox row; duplicate retries, conflicting reference reuse, and failed
  initializer/stale-marker attempts do not append duplicate boundary rows.
- Concurrent claims for the same unowned planet produce one owner and one
  durable claim event.
- Claim initializes production rows once and recovers missing initialization on
  retry.
- Claim marks stale intel/listings without revealing hidden coordinates.
- Building build/upgrade handlers reject unowned planets, wrong-map access, bad
  requirements, insufficient materials/wallet, capacity overflow, and duplicate
  references.
- Production summary/storage queries settle eligible owned active-map
  production with server time, cap storage, no-op immediate and near-immediate
  sequential duplicates, reject spoofed owner/map/time/storage/output/building
  fields before mutation, and emit only owner-scoped safe production/storage
  reconciliation events.
- Production settlement domain results/events now carry deterministic
  server-derived settlement reference/window evidence and the in-memory store
  now records settlement references plus pending outbox rows for applied
  settlements. Durable DB rows, row locks/CAS, idempotency table enforcement,
  and durable publisher/retry workers remain open.
- Route create accepts only intent fields, derives owner/route id/map ids
  server-side, reconciles safe route list/snapshot payloads, and rejects
  unowned source, inaccessible destination, hidden endpoint, bad resource,
  insufficient capacity, and forged owner/map/energy/risk payloads.
- Route update settles old terms before replacing terms and preserves owner/map
  rules.
- Route enable/disable gateway controls accept only `route_id`, derive owner
  server-side, reject wrong-owner and spoofed server-owned fields without
  mutation/events, emit only owner-scoped safe route events, and reconcile
  active-map production/storage snapshots when disable settlement touches
  storage.
- Route settle gateway accepts only `route_id` or `{}` owner reconcile intent,
  derives owner server-side, rejects wrong-owner and spoofed server-owned
  settlement fields without mutation/events, emits owner-scoped safe
  `route.settled` plus route reconciliation events, avoids AOI diffs, and
  reconciles active-map production/storage snapshots when storage changes.
- Route settlement domain results/events now carry deterministic
  server-derived settlement reference/window evidence and the in-memory store
  now records settlement references plus pending outbox rows for applied route
  settlements. Durable route settlement idempotency table enforcement must
  still respect source storage, destination capacity, loss rolls, and map-risk
  policy.
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
  claim-production recovery, building mutation contracts, and durable
  settlement idempotency are implemented.
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
- Production summary/storage queries reconcile eligible owned active-map
  production with server time, remain capped and storage-cap aware, and prove
  immediate/sequential duplicate no-op behavior in the current in-memory
  gateway. Domain results/events now carry server-derived settlement
  reference/window evidence and process-local store-owned reference/outbox
  records for applied settlements. Building mutation outbox rows now carry the
  committed build/upgrade idempotency reference through the process-local
  publisher state machine. Durable concurrent DB rows, row locks/CAS,
  idempotency table enforcement, and durable outbox publishing remain open.
- Route rows carry source/destination map identity and use map policy for
  endpoint access and risk.
- Route mutations and settlements use server-resolved ownership and do not
  accept client-authored owner/map truth.
- Events and socket fanout do not leak other-map or hidden route/planet data.
