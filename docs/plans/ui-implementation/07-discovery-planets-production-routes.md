# Phase 07: Discovery, Planets, Production, And Routes UI

## Status

- State: Completed for scanner/read-model MVP plus focused browser claim proof;
  durable planet/building/route mutation contracts remain tracked in
  `docs/todo.md`
- Owner: Exploration and persistent planet network UI
- Depends on: Phase 06
- Unlocks: long-term strategy loop

## Goal

Expose scanner discovery, known intel, coordinate items, planet claiming,
production buildings/storage, offline settlement, and automation routes through
real server-backed panels and map interactions.

Current slice completed:
- Authenticated `scan.pulse` is exposed through the Go realtime gateway and the
  browser Scan action.
- Known planet, selected planet detail, production summary, storage summary,
  and route list/snapshot read models reconcile from server-owned state.
- The browser renders scanner results, known planets, minimap markers,
  production counts, and route counts without hidden procedural seeds or fake
  planet data.
- 2026-06-19 follow-up: planet detail coordinates are treated as optional
  server-owned data. If a detail response omits coordinates, the browser shows
  the coordinate as locked/empty, suppresses the world memory marker, and keeps
  Navigate disabled instead of inventing an origin `{0,0}` planet marker.
- 2026-06-19 follow-up: clicking a known planet memory marker on the world map
  opens the planet detail modal immediately and still requests fresh server
  detail for reconciliation.
- Phase 10 records the remaining missing contracts for intel share, coordinate
  item use, building mutation, durable persistence/outbox, and broader
  rollout. Unimplemented controls remain absent, locked, or read-only until
  their server-authoritative transaction paths are implemented.
- Phase07A backend follow-up: authenticated `discovery.claim_planet` now exists
  in the Go realtime gateway only. It accepts only `planet_id`, resolves
  player/rank/map/position/X Core source server-side, consumes one `x_core`
  through inventory idempotency, initializes production, and emits owner-scoped
  safe events. Browser claim UI and TypeScript protocol exposure now send only
  `planet_id`, reconcile the claim response and `planet.claimed` event, and are
  covered by a focused real-browser proof using an E2E-only Inventory X Core
  plus Progression rank seed. Durable DB/outbox, building mutations, route
  mutations, drop flow, and broader claim/drop matrix coverage remain open.
- Phase07B backend/client read-model follow-up: owned planet production,
  storage, and building payloads now include catalog-derived `public_map_key`;
  automation route rows store internal source/destination map ids for server
  policy/read grouping, while route list/detail/snapshot payloads expose only
  `from_public_map_key` and `to_public_map_key`. The browser reducer parses
  route public map keys. Route mutation handlers, building mutation handlers,
  durable DB/outbox, and route settlement idempotency remain open.
- Phase07C backend gateway follow-up: authenticated `route.create` now exists
  as the first route mutation slice. It accepts only planet-to-planet route
  intent fields, derives owner, route id, endpoint map ids, energy, and risk
  server-side, requires both endpoint planets to be owned by the authenticated
  player for this MVP, and reconciles through safe route response plus
  owner-scoped `route.updated`, `route.snapshot`, and `route.list` events.
- Phase07D backend gateway follow-up: authenticated `route.enable` and
  `route.disable` now exist for owned routes. They accept only `route_id`,
  derive owner from the authenticated command context, reject client-authored
  owner/map/enabled/settlement/source/destination/storage/risk facts before
  mutation, and reconcile through safe route response plus owner-scoped
  `route.updated`, `route.snapshot`, and `route.list` events. If
  `route.disable` settles elapsed storage transfer, it also returns and emits
  active-map filtered production/storage snapshots.
- Phase07E backend gateway follow-up: authenticated `route.update` now exists
  for owned routes. It accepts only `route_id`, `destination_planet_id`,
  `resource_item_id`, and `amount_per_hour`, derives owner from the
  authenticated command context, loads source truth from the server-owned route
  row, rejects client-authored owner/map/source/destination object/enabled/
  settlement/storage/energy/risk facts before mutation, and reconciles through
  safe route response plus owner-scoped `route.updated`, `route.snapshot`, and
  `route.list` events. If update settlement touches storage, it also returns
  and emits active-map filtered production/storage snapshots.
- Phase07F backend gateway follow-up: authenticated `route.settle` now exists
  for owned routes. It accepts only `route_id` or `{}` owner reconcile intent,
  derives owner from the authenticated command context, rejects client-authored
  owner/map/source/destination/enabled/settlement/window/storage/energy/risk/
  amount/rate/resource facts before mutation, and reconciles through safe
  settlement payloads plus owner-scoped `route.settled`, `route.updated`,
  `route.snapshot`, and one `route.list` event. If settlement touches storage,
  it also returns and emits active-map filtered production/storage snapshots.
  Durable DB/outbox and durable route settlement window idempotency remain
  open.
- Phase07G browser route proof follow-up: the browser now exposes
  `route.create`, `route.update`, `route.enable`, `route.disable`, and
  `route.settle` command builders and HUD controls. The route UI sends only
  server-safe intent fields, reconciles from route response/list/snapshot/
  updated/settled payloads, clears pending route operations, and is covered by
  `npm --cache /tmp/gameproject-npm-cache --prefix client run
  e2e:phase10-route` using a guarded `GAME_DEV_MODE=1` +
  `GAME_E2E_ROUTE_SEED=1` real-server seed. Durable production/route DB rows,
  outbox publishing, and durable settlement window idempotency remain open.
- Phase07CQ browser route settlement follow-up: route settle responses and
  `route.settled` events now persist the last safe settlement result in client
  state by `route_id` and render it in the owned-route HUD row. The browser can
  show settled, no-transfer, source-empty, destination-full, and loss-applied
  outcomes without exposing owner ids, internal map ids, storage aggregate ids,
  settlement references, or hidden route facts.
- Phase07CS browser route-list reconciliation follow-up: `route.list` responses,
  events, and snapshot payloads now flow through one client reducer path that
  updates the global route cache and the selected planet route cache from the
  same server-owned list. Durable `route.list` recovery can no longer leave the
  owned-planet panel showing stale or deleted routes after live read-model loss.
- Phase07CT browser route destination follow-up: route create and update
  controls render server-owned storage/station endpoint catalog options from
  planet detail state alongside owned planet endpoints, while keeping endpoint
  ids, map ids, and owner facts out of route rows and mutation buttons.
- Phase07H backend query follow-up: authenticated
  `planet.production_summary` and `planet.storage_summary` now reconcile
  eligible owned active-map production through
  `SettlePlanetProductionIfWholeOutputAvailable` before returning safe
  snapshots. The runtime uses one request-scoped server clock timestamp,
  rejects client-authored owner/map/time/output/storage/building facts, filters
  out other-owner and other-map planets, emits only owner-scoped
  `planet.production_summary` and `planet.storage_summary` events when a
  whole-output settlement changes production/storage state, and avoids AOI
  diffs. Immediate and near-immediate sequential duplicate queries no-op
  without advancing `last_calculated_at`; durable production DB rows/outbox and
  building build/upgrade handlers remain open.
- Phase07I durable-readiness follow-up: production and route settlement domain
  results plus outbox-safe domain event payloads now carry server-derived
  `reference_key` and deterministic colon-free `<from_unix_ms>-<to_unix_ms>`
  settlement windows using the existing `offline_settlement:*` and
  `route_settlement:*` idempotency key builders. Browser-safe realtime
  settlement payloads remain unchanged, and no-op duplicate settlements still
  emit no events and may have empty reference/window evidence. Durable DB rows,
  row locks/CAS, idempotency table enforcement, and outbox publishing remain
  open.
- Phase07J in-memory durable-boundary follow-up: the production store now
  records non-no-op production and route settlement references keyed by the
  server-derived idempotency key and mirrors appended production-domain events
  into pending in-memory outbox records under the same store lock as the state
  mutation and event append. Duplicate reuse of a recorded settlement reference
  no-ops before mutation/events/outbox, read APIs clone reference/outbox/event
  payload data, and browser-safe realtime payloads remain unchanged. Real DB
  rows, row locks/CAS across processes, DB idempotency-table enforcement,
  durable publisher/retry workers, and durable outbox persistence remain open.
- Phase07K in-memory publisher-state follow-up: pending production outbox
  records now have a process-local delivery state machine with pending,
  in-flight, published, and failed states plus claim/publish/fail/retry APIs.
  Publisher reads and mutations return cloned records, preserve append-order
  diagnostics through `OutboxRecords()`, track attempts and failure evidence,
  and keep published rows out of pending/retryable selection. Real DB rows,
  row locks/CAS across processes, DB idempotency-table enforcement, a durable
  publisher process, and durable outbox persistence remain open.
- Phase07L stale-publisher guard follow-up: each process-local production
  outbox claim now receives a deterministic claim token tied to the current
  attempt. Publish/fail mutations require the current token and in-flight
  status, so stale callbacks from older attempts return `ok=false` without
  mutating status, timestamps, error evidence, or payload bytes. Retry clears
  the old token before a later reclaim generates a new one. Real durable DB
  outbox rows, cross-process row-lock/CAS semantics, durable idempotency-table
  enforcement, and a durable publisher remain open.
- Phase07M claim-boundary follow-up: the discovery claim service now records
  successful cached claim references in process-local memory and mirrors each
  new-owner `planet.claimed` domain event into one pending process-local claim
  outbox record. Duplicate claim references return the cached result without
  duplicate reference, event, or outbox rows; initializer/stale-marker failures
  record no boundary rows before retry, and same-owner repair preserves the
  existing no-event behavior while caching an already-owned claim reference.
  Read APIs return detached records in deterministic reference or append order.
  Durable claim DB rows, row locks/CAS, DB idempotency-table enforcement,
  durable outbox rows, publisher workers, and cross-process recovery remain
  open.
- Phase07N process-local claim publisher follow-up: claim outbox rows now have
  pending, in-flight, published, and failed delivery states plus explicit
  claim/retry APIs. Each claim attempt receives a deterministic claim token,
  publish/fail callbacks require the current in-flight token, retry clears the
  stale token while preserving failure diagnostics, and published rows are
  terminal. Durable claim DB rows, cross-process row locks/CAS, idempotency
  table enforcement, durable outbox persistence, and durable publisher workers
  remain open.
- Phase07O production-domain building mutation follow-up: process-local
  `planet_building_build` and `planet_building_upgrade` idempotency keys now
  guard in-memory planet building mutations. The production domain can build an
  active catalog-backed building, upgrade an existing building to the next
  catalog level, debit required materials from `PlanetStorage` atomically with
  storage/building mutation, write production-local material ledger rows, call
  an optional wallet debit adapter before mutating store state, cache duplicate
  results without double debit, and append `planet.storage_updated` plus
  `planet.building_updated` events through the existing in-memory outbox path.
  Authenticated gateway handlers, ownership/range/requirement policy wiring,
  durable DB rows, cross-process locks/CAS, durable idempotency-table
  enforcement, and durable material/outbox persistence remain open.
- Phase07P backend gateway follow-up: authenticated `planet.building_build`
  and `planet.building_upgrade` now exist in the Go realtime gateway. Build
  accepts only `planet_id`, `building_type`, and `slot`; upgrade accepts only
  `planet_id`, `building_id`, and `target_level`/`next_level` with conflict
  rejection. The handlers derive player, active-map scope, catalog definition,
  deterministic building id, idempotency reference, material costs, wallet
  credit costs, and production snapshots server-side; wrong-owner, other-map,
  spoofed owner/map/wallet/material/storage/cost/level/catalog fields are
  rejected before mutation. Successful mutations settle owned production first,
  mutate through the production domain, and reconcile with owner-scoped
  production, storage, and wallet snapshots. Durable DB rows, cross-process
  locks/CAS, durable idempotency-table enforcement, durable material/outbox
  persistence, and broader cost/requirement balancing remain open.
- Phase07CK browser building control follow-up: planet production panels now
  expose authenticated `planet.building_build` and `planet.building_upgrade`
  intents for owned production planets. The command builder and HUD action
  router send only `planet_id`/`building_type`/`slot` or
  `planet_id`/`building_id`/`target_level`; wallet, material, production,
  level, catalog, cost, and map truth remain server-owned. Production summary
  events clear matching pending building mutations and refresh displayed
  buildings from server state.
- Phase07Q building outbox evidence follow-up: building mutation
  `planet.storage_updated` and `planet.building_updated` outbox rows now carry
  the same server-derived `planet_building_build` /
  `planet_building_upgrade` idempotency reference as the committed mutation,
  and the existing pending/in-flight/failed/published publisher state machine
  preserves that evidence. Real durable DB rows and cross-process publisher
  ownership remain open.
- Phase07R intel domain foundation follow-up: `internal/game/intel` now owns a
  small server-authoritative in-memory foundation for `intel.share`,
  `intel.coordinate_item.create`, and `intel.coordinate_item.use`. It records
  player planet memory, creates coordinate item payloads only from stored
  server intel, consumes coordinate items once, uses canonical foundation
  idempotency keys for share/create/use duplicate safety, and rejects
  mismatched reference/entity attempts before mutation. Realtime handlers,
  economy inventory item movement, daily quotas, stale market listing hooks,
  and durable DB rows remain open.
- Phase07S intel gateway follow-up: authenticated `intel.share`,
  `intel.coordinate_item.create`, and `intel.coordinate_item.use` now exist in
  the Go realtime gateway and TypeScript protocol surface. Share accepts only
  `planet_id` and `to_player_id`, derives the sender and source intel
  server-side, writes the receiver discovery read model, and queues a
  receiver-scoped known-planets refresh. Coordinate create accepts only
  `planet_id`, derives the server-owned temporary item instance from the
  request id and stored intel, and omits hidden coordinates/source metadata
  from the response. Coordinate use accepts only `item_instance_id`, checks
  ownership/consume-once through the intel domain, writes the discovery read
  model, and queues owner-scoped known-planets and planet-detail refreshes.
  Inventory-backed coordinate item mint/consume, daily quotas, market/listing
  staleness hooks, durable DB rows, and browser HUD controls remain open.
- Phase07T inventory-backed coordinate item follow-up:
  `intel.coordinate_item.create` now mints a real
  `planet_coordinate_scroll` inventory instance with the same server-authored
  item instance id as the intel record, writes an item ledger increase, returns
  a reconciled inventory snapshot, and emits `inventory.snapshot`.
  `intel.coordinate_item.use` now requires the matching owned inventory
  instance, removes it through the inventory service before marking the intel
  item used, writes an item ledger decrease, and reconciles known planets,
  planet detail, and inventory snapshots. Duplicate create/use requests replay
  through the existing idempotency paths without duplicate inventory or ledger
  rows. Phase07AG transfers coordinate item intel ownership after market
  purchase with the same market-buy idempotency key, so duplicate buy retries
  can repair the transfer and the buyer can use the bought scroll once. Daily
  quotas, durable DB rows, cross-service transaction/compensation, and browser
  HUD controls remain open.
- Phase07U outbox publisher-boundary follow-up: discovery claim outbox and
  production-domain outbox records now have small interface-backed publisher
  drain helpers. The production helper covers production settlements, route
  settlements, and building mutation rows because all three use
  `ProductionOutboxRecord`. The helpers claim pending rows, call a publisher
  callback, and mark the same claim token published or failed, so future DB
  adapters can implement the same contract with row-lock/CAS semantics. Real
  durable DB rows, durable publisher process scheduling, cross-process leases,
  and idempotency-table enforcement remain open.
- Phase07V claim idempotency evidence follow-up: process-local planet claim
  reference rows and claim outbox rows now preserve typed
  `foundation.IdempotencyKey` evidence when the claim reference is canonical
  `planet_claim:<player_id>:<planet_id>`. Legacy local test references remain
  valid without typed evidence, while the authenticated gateway path records
  DB-adapter-ready claim idempotency metadata. Durable claim DB rows,
  cross-service transaction/CAS, production-init recovery, and durable outbox
  publishing remain open.
- Phase07W claim recovery evidence follow-up: successful same-owner claim
  repairs now append process-local `ClaimRecoveryRecord` diagnostics with the
  claim reference, optional typed idempotency key, player, planet, original
  owner timestamp, recovery timestamp, and repair reason. This covers retries
  after owner mutation but before production initialization or stale-listing
  repair completed, without minting duplicate claim events/outbox rows. Durable
  recovery rows, cross-service transaction/CAS, and recovery workers remain
  open.
- Phase07CI claim production read-model repair follow-up:
  `discovery.claim_planet` now repairs missing live production/storage rows
  before building duplicate/same-owner claim response snapshots. A duplicate
  retry after process-local production read-model loss restores production from
  server claim evidence, returns production/detail snapshots for the browser,
  emits the normal production summary event, and does not consume a second
  X Core. Durable DB claim/production rows and scheduled recovery workers
  remain open.
- Phase07X outbox lease recovery follow-up: process-local claim outbox and
  production/route settlement outbox records can now release expired
  in-flight publisher leases back to pending in append order. Release clears
  stale claim tokens, preserves attempt/failure evidence, uses strict
  `claimed_at < cutoff` semantics, and lets the next claim create a fresh
  token. Durable DB rows, cross-process leases, scheduled workers, and
  row-lock/CAS enforcement remain open.
- Phase07Y outbox lease reaper contract follow-up: claim outbox and
  production/route outbox stale-lease release now have small interface-backed
  helper contracts matching the existing publisher drain helpers. Future DB
  adapters can expose the same strict cutoff, token-clearing, retry-evidence
  preserving semantics behind row-lock/CAS updates. Durable rows, scheduler
  wiring, and cross-process idempotency enforcement remain open.
- Phase07Z store-owned claim boundary follow-up: discovery `InMemoryStore` now
  has a transaction-shaped claim boundary row with begin/complete APIs. Begin
  performs the owner CAS, marks older planet intel stale, and records a
  `pending_side_effects` claim boundary with typed idempotency evidence when
  the canonical claim key is used; complete marks production/listing side
  effects done without changing the original owner CAS evidence. Wiring
  `ClaimService` to this boundary, durable DB rows, cross-service transaction
  enforcement, recovery workers, and outbox/event completion remain open.
- Phase07AA claim-service boundary wiring follow-up: `ClaimService.ClaimPlanet`
  now routes new owner changes through the store-owned begin/complete claim
  boundary. Failed production/listing side effects leave a
  `pending_side_effects` boundary, retries repair through the same boundary
  without another X Core consume, and claim-reference conflicts are rejected
  before a second consume. Durable DB rows, cross-service transaction
  enforcement, recovery workers, and durable event/outbox completion remain
  open.
- Phase07AB store-owned claim completion artifacts follow-up: completing a
  claim boundary now records the claim reference, `planet.claimed` event, and
  pending claim outbox row under the discovery store lock. Duplicate completion
  replays the original artifacts without minting another event/outbox row, and
  repairs missing completion artifacts if an older completed boundary is
  replayed. `ClaimService` now treats a completed boundary after process cache
  loss as an original claim replay, not an already-owned repair, and its
  read/publisher APIs delegate boundary-backed claim artifacts to the
  store-owned state. Durable DB rows, X Core/owner-CAS transaction coupling,
  and cross-process publisher/recovery workers remain open.
- Phase07AC claim boundary adapter contract follow-up: `ClaimService` now uses
  a `ClaimBoundaryStore` interface for begin/complete/read claim boundary
  operations, while defaulting to the existing `InMemoryStore`. The contract
  matches the future durable DB adapter seam for row-lock/CAS and idempotent
  claim completion, and focused tests prove boundary read failures stop before
  X Core consumption while completion failures leave a retryable pending
  boundary that completes without a second X Core consume. This slice does not
  yet move X Core consumption inside the same durable begin transaction, so a
  future DB begin/row-lock failure after inventory debit still belongs to the
  open X Core/owner-CAS transaction-coupling work. Real durable rows,
  cross-service transaction coupling, and recovery workers remain open.
- Phase07AD X Core consumption evidence follow-up: `ClaimService` now records
  process-local X Core debit evidence by claim reference before owner-CAS begin.
  If a transient begin/row-lock failure happens after the debit, retrying the
  same claim reference reuses that evidence and does not call the X Core
  consumer a second time; a conflicting player/planet for the same reference is
  rejected before another consume. This is still not a durable rollback or
  atomic cross-service transaction; the DB-backed X Core debit plus owner-CAS
  coupling remains open.
- Phase07AE claim-production initialization evidence follow-up: successful
  production initialization is now recorded by claim reference before later
  claim side effects complete. If stale listing or boundary completion fails
  after production rows are initialized, retrying the same claim reference
  reuses that evidence and does not call the production initializer again;
  initializer failures still leave no evidence and are retried. Durable
  production-init recovery rows, cross-service transactions, and recovery
  workers remain open.
- Phase07AF claim-market stale listing adapter follow-up: runtime claim
  side effects now wire `ClaimListedIntelStaleMarker` to the concrete market
  and intel services. When a planet is claimed, active
  `planet_coordinate_scroll` listings whose server-owned coordinate item points
  at that planet are marked `stale` with reason `planet_claimed`, already-stale
  matching listings count consistently on retry, and stale coordinate listings
  can no longer be bought. Durable planet-to-listing DB indexes and
  cross-service transactions remain part of the broader durable persistence
  work.
- Phase07AG coordinate market-purchase transfer follow-up: market purchases of
  `planet_coordinate_scroll` instances now preflight that the matching
  server-owned intel payload exists, is unused, and belongs to the listing
  seller; after a successful or duplicate market buy, runtime transfers the
  intel coordinate item owner to the buyer using the market-buy idempotency
  key. Seller use is rejected after purchase, buyer use succeeds through the
  normal `intel.coordinate_item.use` path, and duplicate buy retries do not
  mint duplicate inventory or transfer state. Durable cross-service
  transaction/compensation remains open.
- Phase07AH scanner movement regression follow-up: server tests now prove that
  an authenticated `scan.pulse` while the player entity is moving returns a
  safe forbidden error before capacitor spend, planet creation, intel writes, or
  scanner events; this completes the movement half of the existing scanner
  mutation guard evidence alongside the insufficient-capacitor regression.
- Phase07AI intel share safety follow-up: `intel.share` now only copies
  shareable sender intel states (`fresh` and `verified`); stale,
  invalidated, missing, or colonized-by-other sender memory returns a safe
  not-found style error before receiver intel writes or receiver event fanout.
- Phase07CR intel active-map safety follow-up: `intel.share` and
  `intel.coordinate_item.create` now sync only planet intel that belongs to the
  authenticated player's active map, and `intel.coordinate_item.use` rejects a
  coordinate scroll from another map before inventory consume or discovery
  mutation. Wrong-map attempts return safe not-found style errors without
  receiver fanout, coordinate-item minting, inventory debit, or reveal events.
- Phase07AJ hidden planet detail regression follow-up: `discovery.planet_detail`
  now has server test evidence that materialized planets without player intel
  return a safe not-found response without creating intel rows or queued events.
- Phase07AK route storage capacity regression follow-up: `route.settle` now has
  gateway-level test evidence that a full destination planet storage clamps
  delivered resources, flags `destination_full`, and leaves stored units within
  server-owned capacity without accepting client-authored storage facts.
- Phase07AL building mutation rollback regression follow-up: authenticated
  `planet.building_build` gateway tests now cover insufficient planet storage
  materials and insufficient wallet credits before mutation; both paths reject
  safely without building commits, building material debits, wallet debits,
  building material ledger rows, building mutation reference rows, or queued
  owner events in the no-pending-settlement gateway path. Pre-build production
  settlement remains a separate reconciliation boundary.
- Phase07AM route storage ledger semantics follow-up: route settlements now
  append production-local route storage ledger rows under the settlement store
  lock for source debits, transfer losses, destination credits, and destination
  overflow/discarded delivery. Rows carry route id, planet/counterparty ids,
  item, quantity, item balance after, reference key, settlement window, and
  created time; duplicate/no-op settlements do not append additional rows.
- Phase07AN building mutation durable commit follow-up: successful authenticated
  building build/upgrade handlers now hand the server-derived building mutation
  idempotency reference, pending production outbox rows, and production-local
  material ledger rows to a process-local durable commit-store adapter. The
  adapter validates the row bundle against the cached mutation result, rejects
  missing references and conflicting replay attempts before mutation, supports
  exact idempotent replay/readback, and keeps duplicate gateway requests from
  appending new durable rows. Real DB rows, cross-process CAS/locks, wallet
  debit co-commit evidence, and durable publisher workers remain open.
- Phase07AN route settlement transaction-boundary follow-up:
  `ApplyRouteSettlementTransaction` now gives route settlement an explicit
  DB-adapter-ready contract: owner-scoped route validation, settlement window
  idempotency reference, route storage ledger rows, and pending production
  outbox rows are produced under one store lock and returned as newly committed
  rows. `SettleRouteForOwner` now goes through this boundary. Real DB row
  locks/CAS and durable outbox tables remain open.
- Phase07BN automation route durable-row follow-up:
  `InMemoryAutomationRouteDurableStore` now defines the DB-adapter-ready
  route-row persistence contract separately from settlement evidence. It
  records validated `AutomationRoute` snapshots with domain idempotency
  references, revision CAS, exact replay, stale revision rejection, reference
  conflict rejection, detached route/readback APIs, and deterministic owner
  route reads. Phase07BO wires runtime route create/update/enable/disable
  mutations to that durable row contract with server-derived references and
  revision advancement under the production store lock. Phase07BP wires pure
  route settlement cursor advancement to the same durable route-row contract
  using the server-derived `route_settlement:<route_id>:<window>` reference
  under the production store lock. Durable DB adapters still need to make route
  rows, settlement evidence, storage ledger rows, and outbox rows one
  row-locked/CAS commit.
- Phase07BQ route settlement durable bundle follow-up:
  `SettlementDurableCommitPlan` now requires route settlements to carry the
  committed durable route-row snapshot alongside the settlement reference,
  pending outbox rows, and route storage ledger rows. Production settlement
  bundles still reject route rows. This makes the future DB adapter contract
  explicit without moving to a real durable DB implementation yet.
- Phase07BR route-capacity follow-up: authenticated `route.create` now enforces
  a server-owned per-player MVP route-slot cap before inserting a new route.
  The gateway rejects client-authored route count/capacity fields, counts all
  existing owned routes server-side, returns a leak-safe forbidden route
  requirements error at capacity, and emits no route events on rejection.
  `route.update` still reuses route policy for endpoint/risk/energy facts but
  does not enforce create-slot capacity, so players at cap can edit existing
  routes.
- Phase07BS route energy-upkeep follow-up: enabled automation routes now reserve
  their server-derived `energy_cost_per_hour` against the source planet
  production state's `energy_reserved_per_hour`. `route.create` and
  `route.enable` reserve upkeep before committing the enabled route,
  `route.disable` releases upkeep after successful settlement, and
  `route.update` applies only the enabled-route energy delta so same-cost edits
  still work at capacity. Production settlement already consumes reserved
  energy before buildings, so route upkeep can now reduce building throughput
  through real server state. Durable DB adapters still need to co-commit route
  row mutations and source production-state reservation changes.
- Phase07BT durable route-energy evidence follow-up:
  `AutomationRouteDurableCommitPlan` and committed route records can now carry
  the source `PlanetProductionState` row changed by route energy reservation.
  The in-memory production store applies that source state under the same lock
  as the route durable row, while standalone durable-route stores retain
  detached evidence for future DB adapters. Pure route settlement cursor
  commits still omit source production state because they do not change route
  energy reservation.
- Phase07BU route-create transaction follow-up:
  `route.create` now goes through an explicit `RouteCreateTransactionStore`
  boundary. The in-memory adapter rechecks the owner route cap against
  authoritative route rows under the route insert lock, then commits the route
  row, route-create idempotency reference, source energy reservation, and
  durable route record together. This closes the process-local stale
  `CurrentRouteCount` race class; real DB row locks/CAS and durable
  idempotency-table enforcement remain future adapter work.
- Phase07BV route settlement durable realtime follow-up:
  Route settlement durable outbox rows now have a focused runtime projection
  proof. The test settles a real owned route, clears command-queued events, then
  drains the durable settlement outbox into realtime and verifies owner-scoped
  `route.settled`, route snapshot/list, production, and storage events without
  leaking to another active session.
- Phase07BX storage destination route adapter follow-up:
  The production route domain now accepts `storage` destinations behind the
  route policy boundary and `SettleRoute` resolves those destinations to named
  storage aggregates. Storage-destination settlements use the same
  server-timed reference/window evidence, route storage ledger rows, durable
  route-row cursor snapshots, and outbox evidence as planet destinations.
  Station destinations are covered by Phase07BY; durable DB endpoint rows
  remain open.
- Phase07BY station destination route adapter follow-up:
  The same production-domain route adapter now accepts `station` destinations
  behind the route policy boundary and settles them into named station storage
  aggregates with normal route ledger, durable route-row, reference/window, and
  outbox evidence. Public route create/update handlers now accept typed
  `destination_type` + `destination_id` intent for storage/station endpoints
  whose storage aggregate already exists, while still masking non-planet
  destination ids from route response/event payloads. Durable DB endpoint rows
  remain open.
- Phase07CH route endpoint catalog follow-up:
  `planet_detail` now includes owner-scoped storage/station route endpoint
  catalog entries for owned planets. The browser route controls merge those
  server-owned endpoint ids with owned planet endpoints and send typed
  storage/station route intent without inventing endpoints locally. Route
  create/update only accept the authenticated player's catalog endpoint ids,
  idempotently ensuring the endpoint storage aggregate before mutation, and
  still mask non-planet destination ids from route response/event payloads.
- Phase07BZ non-planet route settlement gateway follow-up:
  Existing owner-scoped `storage` and `station` routes now have focused
  authenticated `route.settle` gateway proof. The browser still sends only
  `route_id`; the server settles the named endpoint storage, returns safe
  route/list/settlement/production/storage payloads with storage/station
  aggregate IDs masked, emits owner-only route events, and records durable
  settlement reference/window, route ledger, route-row, and outbox evidence.
  Public non-planet route create/update policy and durable DB endpoint rows
  remain open.
- Phase07CA durable route replay safety follow-up:
  Durable outbox realtime replay now has focused storage/station route
  settlement coverage. Replayed `route.settled`, `route.updated`,
  `route.snapshot`, and `route.list` events remain owner-only, use public map
  keys, and mask non-planet aggregate destination IDs just like the live
  `route.settle` gateway path. Durable DB route rows, endpoint rows, and
  cross-process idempotency enforcement remain open.
- Phase07CI durable owner-route reconcile follow-up:
  Empty-payload authenticated `route.settle` now builds the owner route set
  from live read-model rows plus committed durable route rows, matching
  `route.list` recovery behavior. After a live route row loss, owner-wide
  reconcile replays the durable handoff instead of silently skipping the
  route, emits no duplicate storage/outbox rows, and returns the recovered
  route in the settlement response. Route-create capacity policy uses the same
  durable-aware owner route count so live read-model loss cannot bypass the
  server-owned route-slot cap.
- Phase07CJ durable route snapshot reconcile follow-up:
  Authenticated `route.snapshot` now uses the same owner-checked durable route
  readback as route settlement when the live route read model is missing. After
  live row loss, the owner still receives a safe public-map route snapshot from
  committed durable evidence, while non-owners receive the same leak-safe
  not-found response. Durable DB route rows and cross-process enforcement remain
  open.
- Phase07CB durable outbox retry follow-up:
  Claim, settlement/route, and building durable outbox publisher contracts now
  expose an explicit retry-failed boundary. Runtime drains do not auto-retry by
  default; callers must set `RetryFailedOutboxes`. Retried rows move from
  failed back to pending, clear stale claim lease fields, preserve attempts and
  failure evidence, stamp `retried_at`, and can publish in the same drain tick.
  Durable DB row-lock/CAS implementation and retry scheduling policy remain
  open.
- Phase07CC claim production-init recovery query follow-up:
  The claim production-initialization durable-store adapter now exposes
  deterministic pending-plan readback for recovery workers. DB adapters can use
  the same shape to scan initialized-but-incomplete claim side effects, while
  completed claim init rows stay filtered out and readback remains detached.
  Durable DB rows and scheduled recovery workers remain open.
- Phase07CD settlement durable row-evidence follow-up:
  Production settlement durable commit plans now carry the committed
  `PlanetProductionState` row and changed `PlanetStorage` row evidence beside
  their settlement reference and pending outbox rows. Route settlement durable
  commit plans now carry ledger-backed changed storage rows alongside the
  committed route row, route storage ledger, settlement reference, and pending
  outbox rows; timestamp-only route settlements without storage ledger rows
  still carry no storage row evidence. Store readback and exact replay
  validation return detached rows and reject conflicting row evidence. Durable
  DB row locks/CAS, idempotency table enforcement, and scheduled durable
  publisher/recovery workers remain open.
- Phase07CE durable outbox readback follow-up:
  Claim lifecycle, settlement/route, and building mutation durable stores can
  now rebuild committed durable plans after their outbox rows move through
  in-flight, published, failed, retry, or lease-release states. Readback
  validates delivery-state consistency while normalizing a copy back to the
  original pending commit shape, then returns detached rows with the current
  publisher evidence intact. Real durable DB rows, cross-process leases, and
  scheduled publisher/recovery workers remain open.
- Phase07CF claim production-init boundary strictness follow-up:
  Claim production-initialization durable-store apply/readback now requires
  pending or complete claim-boundary evidence. Orphan production-init rows
  without claim boundary evidence are rejected before mutation and fail
  committed/pending recovery readback instead of being treated as valid claim
  recovery state. Real DB rows, cross-service row locks/CAS, an atomic
  claim/production transaction, and scheduled recovery workers remain open.
- Phase07CG settlement outbox worker readback validation follow-up:
  Settlement durable-store publisher, publish/fail callbacks, lease release,
  and failed-row retry paths now revalidate the committed settlement bundle
  before mutating outbox delivery state. Corrupt DB-style readbacks with
  mismatched outbox evidence fail instead of being claimed, published,
  released, or retried. Real DB rows, cross-process worker scheduling, and
  durable idempotency enforcement remain open.
- Phase07CH claim wrong-owner regression follow-up:
  Authenticated `discovery.claim_planet` now has gateway-level coverage proving
  another player cannot claim an already-owned planet even when they know the
  planet and hold an X Core. The rejection leaves the original owner intact,
  does not consume the second player's X Core, does not commit a lifecycle row,
  and queues no failed-claim events.
- Phase07AO production settlement transaction-boundary follow-up:
  `ApplyProductionSettlementTransaction` now gives offline planet production
  settlement the matching DB-adapter-ready contract: planet validation,
  settlement window idempotency reference, and pending production outbox rows
  are produced under one store lock and returned as newly committed rows.
  `SettlePlanetProduction` and `SettlePlanetProductionIfWholeOutputAvailable`
  now go through this boundary. Real DB row locks/CAS and durable outbox tables
  remain open.
- Phase07AP claim X Core/owner boundary follow-up:
  `ClaimService.ClaimPlanet` now begins new owner claims through
  `ClaimXCoreOwnerBoundary`, a DB-adapter-ready contract that couples the
  server-derived X Core debit input with the unowned-planet owner-CAS begin
  input. The default in-memory adapter composes the existing inventory consumer
  and claim boundary while returning debit evidence even when owner begin fails,
  preserving retry-without-second-debit behavior. A real durable adapter still
  needs DB row locks/CAS, idempotency rows, ledger mutation, and claim boundary
  writes in one transaction or recoverable state machine.
- Phase07AQ settlement outbox dispatch contract follow-up:
  `NewSettlementOutboxDispatchPlan` now validates the after-commit handoff from
  production/route settlement transactions to a durable publisher scheduler.
  It requires a committed settlement reference for non-empty dispatches, pending
  outbox rows with payloads, matching reference/window evidence when a row
  carries settlement evidence, and at least one settlement-evidenced outbox row.
  This is still a contract/helper layer; durable DB rows, scheduler wiring, and
  cross-process publisher ownership remain open.
- Phase07AR durable settlement commit-plan follow-up:
  `NewSettlementDurableCommitPlan` now validates the row bundle a future
  production DB transaction must commit for one settlement window: the
  idempotency reference, after-commit outbox dispatch plan, and matching route
  storage ledger rows when route storage moves. It rejects route ledger rows
  with mismatched reference/window/route evidence and rejects production
  settlement commits that accidentally include route ledger rows. Concrete DB
  adapters, unique constraints, row locks/CAS, and publisher workers remain
  open.
- Phase07AS transaction result durable-plan helper follow-up:
  `ProductionSettlementTransactionResult` and `RouteSettlementTransactionResult`
  now expose `DurableCommitPlan()` so future DB adapters and publisher workers
  consume a single validated row bundle directly from the transaction result,
  without callers manually matching references, outbox rows, and route storage
  ledger rows. Concrete durable stores and workers remain open.
- Phase07BH settlement durable window readback/publisher follow-up:
  the settlement durable commit-store adapter can now rebuild production and
  route commit/dispatch plans by server-derived planet-or-route settlement
  window, not only by a precomputed idempotency key. The same durable adapter
  also implements the production-domain publisher and lease-reaper contracts,
  so committed settlement outbox rows can be claimed, published, failed, or
  released with claim-token guards while preserving settlement reference/window
  evidence and route storage ledger rows. Real DB rows, row locks/CAS,
  cross-process publisher workers, and scheduled reapers remain open.
- Phase07BI claim durable lifecycle publisher follow-up:
  the claim durable lifecycle-store adapter now implements the claim outbox
  publisher and lease-reaper contracts for committed `planet.claimed` rows.
  Pending committed claim outbox rows can be claimed, published, failed, or
  released with claim-token guards while preserving claim reference, event, X
  Core debit, and production-init evidence. Real DB rows, row locks/CAS,
  cross-process publisher workers, and scheduled reapers remain open.
- Phase07AT claim durable commit-plan follow-up:
  `NewClaimDurableCommitPlan` and
  `CompletePlanetClaimBoundaryResult.DurableCommitPlan()` now validate the
  completed planet-claim row bundle a future durable DB adapter must commit:
  owner-CAS boundary row, idempotency reference, `planet.claimed` event, pending
  claim outbox row, and optional X Core debit evidence. This is still a
  contract/helper layer; durable DB rows, cross-process locks/CAS, and durable
  publisher scheduling remain open.
- Phase07AU claim durable begin-plan follow-up:
  `NewClaimDurableBeginPlan` and
  `BeginPlanetClaimWithXCoreResult.DurableBeginPlan()` now validate the claim
  begin bundle: X Core debit evidence, pending owner-CAS boundary row, owned
  planet snapshot, and stale-intel evidence when owner-CAS succeeds. Debit-only
  begin failures produce X Core recovery evidence without pretending the owner
  transition committed. Real DB idempotency rows, row locks/CAS, and recovery
  workers remain open.
- Phase07AV claim production-init durable-plan follow-up:
  `NewClaimProductionInitializationDurablePlan` now validates production-init
  recovery evidence for claimed planets, including canonical claim reference
  keys, positive planet level, init timing, mutually exclusive created/already
  initialized outcomes, and optional pending/complete claim-boundary evidence.
  Real durable claim/production transaction rows and recovery workers remain
  open.
- Phase07BE claim production-init durable-store follow-up:
  `ClaimProductionInitializationDurablePlan` can now be handed to a
  process-local durable-store adapter before the full claim lifecycle is
  complete. The adapter validates pending or complete boundary evidence,
  records one production-init row per claim reference, exact-replays duplicate
  attempts, rejects conflicting reference reuse before mutation, and exposes
  defensive committed-plan readback for future recovery workers. Real DB rows,
  cross-service row locks/CAS, and production-init recovery workers remain
  open.
- Phase07BF runtime claim production-init durable-store follow-up:
  successful authenticated `discovery.claim_planet` commands now apply the
  validated production-init plan to the runtime production-init durable adapter
  before committing the completed claim lifecycle bundle. Duplicate claim
  retries exact-replay through the adapter without appending new rows. Failed
  pre-initialization claim commands leave both lifecycle and production-init
  durable stores empty. Real DB rows, cross-service row locks/CAS,
  pending-side-effect recovery workers, and an atomic claim/production
  transaction remain open.
- Phase07BG runtime pending claim production-init recovery follow-up:
  `ClaimService` now exposes a validated production-init durable-plan query for
  pending or complete claim boundaries, and the runtime claim handler applies
  that plan even when `discovery.claim_planet` returns an error after
  production initialization but before later side effects. The production-init
  durable-store adapter can advance the same claim reference from pending
  boundary evidence to complete boundary evidence on a successful retry without
  appending a second row. Real DB rows, cross-service row locks/CAS, scheduled
  recovery workers, and an atomic claim/production transaction remain open.
- Phase07BW claim production-init recovery readback follow-up:
  Runtime and durable-store tests now prove that a production-init row first
  committed as pending after a later side-effect failure advances to completed
  evidence on retry, and the committed claim lifecycle readback embeds the same
  completed production-init evidence instead of only relying on the sidecar row.
- Phase07AW claim durable lifecycle-plan follow-up:
  `NewClaimDurableLifecyclePlan` now validates that a completed claim lifecycle
  is one coherent row bundle across begin, optional production-init, and commit
  plans. It rejects debit-only begin evidence, mismatched claim identity,
  stale-intel counts, completion timestamps before claim time, wrong production
  planet levels, production boundary drift, and mixed X Core debit evidence.
  This is still a contract/helper layer; durable claim/storage DB rows,
  cross-service transaction ownership, and recovery workers remain open.
- Phase07AX claim X Core storage evidence follow-up:
  `ClaimXCoreConsumeResult` now carries the inventory `RemoveItemResult` from
  the runtime adapter instead of discarding it, and
  `NewClaimXCoreStorageMutationPlan` validates that the canonical claim
  idempotency key, X Core consumption record, inventory decrease ledger row,
  touched item rows, and optional owner-boundary evidence describe the same
  debit. This directly narrows the claim/storage coupling gap; real durable DB
  rows, cross-service row locks/CAS, and recovery workers remain open.
- Phase07AY durable begin storage-coupling follow-up:
  `BeginPlanetClaimWithXCoreResult.DurableBeginPlan()` now builds its begin
  plan from the validated X Core storage-mutation plan, and
  `NewClaimDurableBeginPlan` rejects non-empty begin evidence that lacks the
  inventory remove ledger/touched-row bundle. Full owner-begin plans require a
  storage plan bound to the same pending boundary, while debit-only recovery
  plans keep storage evidence without pretending owner-CAS committed. Real
  durable DB rows, cross-service row locks/CAS, and recovery workers remain
  open.
- Phase07BB runtime claim lifecycle handoff follow-up:
  `ClaimService` now keeps the validated claim-begin durable plan for completed
  owner changes, exposes a `ClaimDurableLifecyclePlan` readback for completed
  claim references, and the `discovery.claim_planet` handler applies that
  server-owned begin/production-init/commit bundle through the runtime claim
  lifecycle-store adapter. Duplicate claim retries exact-replay through the
  adapter without appending another lifecycle row, and failed claims do not
  record lifecycle evidence. Real durable DB rows, cross-service row locks/CAS,
  durable outbox persistence, and recovery workers remain open.
- Phase07BC claim outbox dispatch-readback follow-up:
  `NewClaimOutboxDispatchPlan` now validates the after-commit handoff from a
  completed planet claim lifecycle to a durable publisher scheduler: committed
  claim reference evidence, pending `planet.claimed` outbox row, matching
  event/reference identity, and empty delivery lease state. The claim lifecycle
  durable store can now rebuild this dispatch plan by claim reference after a
  restart. Real durable DB rows, cross-process publisher leases, and scheduled
  publisher/recovery workers remain open.
- Phase07BD building outbox dispatch-readback follow-up:
  `NewBuildingMutationOutboxDispatchPlan` now validates the after-commit
  handoff from a committed planet building mutation to a durable publisher
  scheduler: committed building mutation reference evidence, pending
  storage/building outbox rows, matching building mutation reference keys,
  empty settlement windows, empty delivery lease state, payload presence, and
  at least one `planet.building_updated` event. The building mutation durable
  store can rebuild this dispatch plan by mutation reference after a restart.
  Real durable DB rows, wallet debit co-commit evidence, cross-process
  publisher leases, and scheduled publisher/recovery workers remain open.
- Phase07BJ building durable publisher follow-up:
  the building mutation durable commit-store adapter now implements the
  production outbox publisher and lease-reaper contracts for committed
  `planet.storage_updated` / `planet.building_updated` rows. Pending committed
  building outbox rows can be claimed, published, failed, or released with
  claim-token guards while preserving building mutation reference evidence and
  material ledger rows. Real durable DB rows, wallet debit co-commit evidence,
  cross-process publisher workers, and scheduled reapers remain open.
- Phase07BK runtime durable outbox drain follow-up:
  `Runtime.DrainDurableOutboxes` now provides a single server-owned handoff for
  committed claim lifecycle, production/route settlement, and building mutation
  durable outbox rows. It optionally releases expired leases first, then drains
  each durable store through the existing claim/production publisher contracts
  with per-store limits and caller-provided publish callbacks. Real durable DB
  rows, cross-process row locks/CAS, and scheduled workers remain open;
  client-safe projection wiring is covered by Phase07BL.
- Phase07BL runtime durable realtime projection follow-up:
  `Runtime.DrainDurableOutboxesToRealtime` now wires committed claim,
  settlement, route, and building outbox rows into existing server-owned
  client-safe realtime projections instead of forwarding raw durable payloads.
  Runtime ticks with an event sink release expired leases, drain durable rows,
  queue owner-scoped safe snapshots, and flush those queued events through the
  sink in the same tick. Offline/no-active-session owners are treated as a
  safe no-op publish because reconnect snapshots reconcile current state.
  Real durable DB rows, cross-process row locks/CAS, external publisher
  workers, durable delivery acknowledgements, and scheduled worker ownership
  remain open.
- Phase07BM durable realtime drain-collect follow-up:
  `Runtime.DrainDurableOutboxesToRealtimeAndCollectEvents` now couples durable
  outbox draining with the filtered per-session realtime events queued by the
  safe projections, so runtime sink delivery does not leave committed outbox
  projections stranded in the command-event queue. `StartWithEventSink` uses
  this collect result and merges it with the same tick's AOI events before
  writing to sessions. Real DB row locks/CAS, external durable publisher
  processes, and delivery acknowledgements remain open.

## Source Specs

Read before implementation:
- `docs/plans/modules/11-planet-production-offline-settlement.md`
- `docs/plans/modules/12-automation-routes.md`
- `docs/plans/modules/13-intel-coordinate-trading.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/2026-06-17-world-system-design.md`
- `internal/game/discovery`
- `internal/game/production`
- `internal/game/world/visibility`

## Server Features To Expose

- scan pulse command
- scanner cooldown/energy/stationary validation
- scan pulse started/resolved events
- safe unknown signal markers
- discovered planet summary
- planet detail query with visibility/ownership checks
- planet claim command
- intel share/coordinate item commands
- planet production state query
- building build/upgrade/enable commands
- storage summary
- offline settlement trigger/query
- route create/update/enable/disable/settle commands

## Commands And Queries

```text
scan.pulse
discovery.known_planets
discovery.planet_detail
discovery.claim_planet
intel.share
intel.coordinate_item.create
intel.coordinate_item.use
planet.production_summary
planet.building_build
planet.building_upgrade
planet.storage_summary
route.create
route.list
route.snapshot
route.update
route.enable
route.disable
route.settle
```

## Operation Contracts

| Operation | Client Intent | Server Authority / Mutation |
| --- | --- | --- |
| `scan.pulse` | request scan | server position, stationary state, energy, cooldown, active-map visibility; emits safe signal/discovery results |
| `discovery.known_planets` | list/filter | player visibility/intel/ownership; returns only known safe summaries |
| `discovery.planet_detail` | planet id | recheck visibility/ownership; omit hidden/procedural fields |
| `discovery.claim_planet` | planet id | validate visibility, range/policy, required item/currency; lock/mutate/ledger/event/commit |
| `intel.share` | recipient, intel/planet/coordinate reference | sender visibility, recipient eligibility, client-safe filtering; never share hidden coordinates |
| `intel.coordinate_item.create` | known coordinate reference | owned/visible coordinate; consume/move item through inventory ledger once |
| `intel.coordinate_item.use` | owned coordinate item id | ownership, visibility rules, item consumption idempotency; reveal only safe result |
| `planet.production_summary` | planet id | ownership/access; active-map filtered read payload includes catalog-derived `public_map_key`; settle/reconcile server-owned windows as needed |
| `planet.building_build` | planet id, building type/slot | ownership, requirements, storage/wallet/materials; lock/mutate/ledger/event/commit |
| `planet.building_upgrade` | building id | ownership, level requirements, storage/wallet/materials; lock/mutate/ledger/event/commit |
| `planet.storage_summary` | planet id | ownership/access; reconciles eligible owned active-map production first; client-safe capacity, visible stacks, and catalog-derived `public_map_key` |
| `route.create/update` | endpoint/config intent; update accepts only `route_id`, `destination_planet_id`, `resource_item_id`, `amount_per_hour` | endpoint visibility/access, ownership, capacity, policy; mutate route terms server-side and settle old update terms before replacement |
| `route.enable/disable` | route id | owner is resolved from the authenticated session; control accepts only `route_id`, rechecks route ownership, and returns safe route/list snapshots |
| `route.list/snapshot` | filter or empty | owner/access; reconnect-safe route state, cursors, and public source/destination map keys |
| `route.settle` | route id or empty reconcile intent | backend gateway derives owner from the authenticated session, settles one owned route or all authenticated-owner routes through owner wrappers, returns safe settlement payloads, and uses durable idempotency key `route_settlement:<route_id>:<window>` for server-owned settlement evidence |

Offline production and route settlement are never client-timed truth. UI requests
may ask the server to reconcile, but the server calculates eligible windows,
locks ownership/storage, applies in-memory duplicate/no-op guards where
implemented, and then broadcasts snapshots after state changes. Durable DB
idempotency rows, ledger/outbox references, and commit boundaries remain
follow-up work where the code has not implemented them yet.

## Events

```text
scan.pulse_started
scan.pulse_resolved
scan.planet_discovered
discovery.known_planets
discovery.planet_detail
planet.claimed
intel.updated
coordinate_item.created
planet.production_updated
planet.storage_updated
planet.production_summary
planet.storage_summary
planet.building_updated
route.updated
route.settled
```

## UI Surfaces

Mockup areas covered:
- center unknown signal markers
- center friendly planet/outpost markers
- right planet list
- selected planet panel
- planet buttons: Build, Upgrade, Route, Auto
- topbar energy production
- sector map unknown/friendly/outpost markers
- log scan and production events

## TODO

- [x] Register real `scan.pulse` operation in Go realtime registry.
- [x] Add authenticated scanner command handler.
- [x] Add scanner event mapper to safe UI payloads.
- [x] Add planet list/summary query for known/owned planets.
- [x] Add selected planet detail query with visibility checks.
- [x] Add planet claim command handler.
- [x] Add browser planet claim protocol, HUD action, reducer handling, and
      focused real-browser proof.
- [x] Add process-local claim reference records and pending claim outbox
      records for successful discovery claim owner changes.
- [x] Add process-local claim outbox delivery state and claim-token guards for
      publisher-worker behavior.
- [x] Add intel share and coordinate item handlers with visibility-safe
      recipient filtering.
- [x] Add read-only production summary handler for owned planets.
- [x] Add production build/upgrade handlers.
- [x] Add process-local production-domain build/upgrade material debit ledger,
      idempotency, and in-memory event/outbox foundation.
- [x] Add authenticated gateway transaction flows for build/upgrade mutations.
- [x] Add explicit claim X Core debit plus owner-CAS begin boundary contract
      for future durable claim/storage adapters.
- [x] Add X Core claim consume storage-mutation evidence and durable-plan
      validation tying inventory remove ledger rows to claim debit evidence.
- [x] Add X Core exact-stack deletion evidence so durable storage adapters can
      recover both updated and deleted inventory rows for claim debits.
- [x] Add claim durable begin-plan validation tying X Core debit evidence,
      storage mutation evidence, pending owner-CAS boundary rows, owned planet
      snapshots, and stale-intel evidence together.
- [x] Add claim durable commit-plan validation tying completed owner-CAS
      boundary rows, claim references, events, pending outbox rows, and optional
      X Core debit evidence together.
- [x] Add claim production-initialization durable-plan validation tying
      production recovery evidence to pending/complete claim boundaries.
- [x] Add claim production-initialization durable-store adapter contract with
      exact replay, conflict rejection, and committed-plan readback.
- [x] Add claim durable lifecycle-plan validation tying begin, optional
      production-init, and completion/outbox evidence together.
- [x] Re-validate claim lifecycle begin plans so missing or forged X Core
      storage-mutation evidence cannot satisfy durable claim/storage coupling.
- [x] Add claim durable lifecycle-store adapter contract with idempotent exact
      replay, conflict rejection, and claim-reference readback.
- [x] Add claim durable lifecycle plan handoff helper that revalidates nested
      begin/commit/production-init rows and applies completed claim bundles
      through the lifecycle-store adapter.
- [x] Apply completed claim durable lifecycle bundles through the runtime
      claim lifecycle-store adapter after authenticated
      `discovery.claim_planet` success.
- [x] Apply pending production-init durable evidence when an authenticated
      claim command fails after production initialization, then advance the
      same durable row to complete evidence on successful retry.
- [x] Add pending production-init durable readback for recovery workers so
      initialized-but-incomplete claim side effects can be scanned without
      replaying completed rows.
- [x] Require pending or complete claim-boundary evidence when applying or
      reading claim production-initialization durable rows so orphan init rows
      cannot satisfy recovery contracts.
- [x] Add claim outbox dispatch-plan validation and committed lifecycle-store
      readback for durable `planet.claimed` publisher scheduling.
- [x] Let the claim durable lifecycle-store adapter satisfy the claim outbox
      publisher and lease-reaper contracts for committed `planet.claimed`
      rows.
- [x] Add a runtime durable outbox drain handoff for committed claim,
      settlement, and building durable stores using the existing publisher and
      lease-reaper contracts.
- [ ] Add durable authenticated transaction flows for claim/storage mutation
      coupling once DB/CAS storage boundaries replace process-local stores.
- [x] Add offline settlement reconcile path that uses server-owned windows for
      production/storage summary queries.
- [x] Add production/route settlement domain result and outbox payload evidence
      with server-derived reference keys and deterministic settlement windows.
- [x] Add settlement outbox dispatch-plan validation for after-commit
      production/route publisher scheduling.
- [x] Add durable settlement commit-plan validation tying idempotency
      reference, outbox dispatch rows, and route storage ledger rows together.
- [x] Add transaction-result helpers that expose validated durable commit plans
      for production and route settlement callers.
- [x] Add durable settlement commit-store adapter contract that atomically
      records settlement references, pending outbox rows, and route storage
      ledger rows with idempotent exact replay and conflict rejection.
- [x] Add production/route settlement transaction-result helpers that hand
      validated durable commit plans to the durable commit-store adapter.
- [x] Add production/route settlement durable commit-plan handoff helper that
      applies validated reference/outbox/route-ledger bundles through the
      durable commit-store adapter.
- [x] Add durable settlement commit readback helpers that recover committed
      durable commit and outbox dispatch plans by settlement reference key.
- [x] Add durable settlement window readback helpers that recover production
      and route commit/dispatch plans by server-derived planet-or-route window.
- [x] Let the settlement durable commit-store adapter satisfy the production
      outbox publisher and lease-reaper contracts for committed settlement
      outbox rows.
- [x] Add process-local store-owned settlement reference records and pending
      outbox records for production and route settlements.
- [x] Add process-local production outbox claim/publish/fail/retry delivery
      state for publisher-worker behavior.
- [x] Add process-local production outbox claim-token guards so publish/fail
      callbacks cannot mutate a retried or reclaimed attempt with a stale
      token.
- [x] Add route.create handler for owned planet-to-planet MVP.
- [x] Add route.update handler for owned routes.
- [x] Add route.enable and route.disable handlers for owned routes.
- [x] Add route.settle handler.
- [x] Add route list/snapshot handlers for reconnect.
- [x] Add client reducer state for signals, planets, production, routes.
- [x] Add right rail known planet list.
- [x] Add selected planet action panel.
- [x] Add route UI and production/building UI.
- [x] Add scanner action state and log events.

## Abuse And Safety Checklist

- [x] Client cannot send planet candidate data as truth.
- [x] Client cannot send scan result or procedural seed.
- [x] Client cannot claim hidden/unowned-invalid planet.
- [x] Client cannot fake X Core consumption.
- [x] Intel sharing cannot reveal a coordinate the sender cannot safely expose.
- [x] Coordinate item use consumes an owned item once.
- [x] Planet panel open rechecks visibility/ownership.
- [x] Route creation rechecks both endpoints and ownership/access.
- [x] Route update rechecks destination ownership/access and preserves
      server-owned source truth.
- [x] Offline settlement duration is server-calculated.
- [x] Route settlement timing is server-calculated in the backend gateway.
- [x] Route settlement handlers hand server-owned route settlement transaction
      rows to the runtime durable commit-store adapter.
- [ ] Durable route settlement windows are enforced by DB/idempotency rows and
      published through the durable outbox.
- [x] Building mutations use production-local material ledger rows and wallet
      debit ledger rows.
- [x] Authenticated building mutation handlers hand idempotency reference,
      material ledger rows, and pending outbox rows to the runtime durable
      commit-store adapter.
- [x] Building mutation durable commits can rebuild validated pending outbox
      dispatch plans for future publisher scheduling.
- [x] Building mutation durable commits can claim, publish, fail, and
      lease-release committed storage/building outbox rows through the
      production outbox publisher contracts.
- [x] Define and enforce route storage ledger semantics for route mutations.
- [x] Storage capacity cannot be exceeded.

## Tests

- [x] Scan rejects moving/energy-insufficient player before mutation.
- [x] Scan result does not leak seed or future candidates.
- [x] Hidden planet detail returns safe error.
- [x] Claim consumes required item once and sets owner once.
- [x] Browser claim sends only `planet_id`, consumes the server-owned E2E X Core
      seed through Inventory, uses server-owned E2E Progression rank eligibility,
      initializes production, clears pending state, and handles `planet.claimed`
      without an unhandled-event log.
- [x] Claim success stores one process-local claim reference plus one pending
      claim outbox record; duplicate references and repair retries do not
      duplicate events/outbox rows, and read APIs return detached records.
- [x] Claim outbox records can be filtered, claimed, marked published or failed
      with the current claim token, and explicitly retried in append order
      without exposing mutable event aliases or letting stale publisher
      callbacks mutate later attempts.
- [x] Claim X Core debit and owner-CAS begin use a single boundary contract
      that returns debit evidence on owner-begin failure so retries do not call
      the X Core consumer a second time.
- [x] Claim handler applies completed claim lifecycle evidence through the
      runtime claim lifecycle-store adapter; duplicate retries exact-replay
      without another lifecycle row, and pre-initialization failed claims record
      none.
- [x] Claim handler applies pending production-init durable evidence for
      post-initialization side-effect failures; retry completion advances that
      row to complete evidence without another X Core debit or extra init row.
- [x] Claim handler rejects another player's already-owned planet without
      consuming X Core, committing claim lifecycle rows, changing ownership, or
      queuing failed-claim events.
- [x] Claim lifecycle durable readback rebuilds a validated pending outbox
      dispatch plan for `planet.claimed` publisher scheduling.
- [x] Claim lifecycle durable outbox rows can be claimed, published, failed, or
      lease-released through the claim outbox publisher contracts.
- [x] Intel share rejects hidden/not-owned coordinate references.
- [x] Coordinate item create/use consumes owned active-map items once and
      filters results.
- [x] Market-bought coordinate scrolls transfer server-owned intel item
      authority to the buyer and can be used once by that buyer.
- [x] Planet claim marks active coordinate-scroll market listings for the
      claimed planet stale and stale coordinate listings cannot be bought.
- [x] Building build/upgrade debits materials/currency once.
- [x] Production summary/storage duplicate and sub-unit polls no-op without
      advancing production time or queuing duplicate events.
- [x] Production settlement stores an in-memory settlement reference and pending
      outbox records for settlement events; duplicate reference reuse no-ops
      without mutation, duplicate events, or duplicate outbox records.
- [x] Production settlement outbox records can be filtered, claimed, marked
      published or failed with the current claim token, and explicitly retried
      in append order without exposing mutable event payload aliases or letting
      stale publisher callbacks mutate later attempts.
- [x] Production settlement uses an explicit transaction boundary that returns
      the committed production idempotency reference and pending outbox rows
      from the same store lock.
- [x] Production settlement transaction outbox rows validate into an
      after-commit dispatch plan before durable publisher scheduling.
- [x] Production settlement transaction rows validate as one durable commit
      plan with no route storage ledger rows.
- [x] Production settlement transaction result exposes its durable commit plan
      directly for future DB/publisher adapters.
- [x] Production settlement durable commit plans carry committed production
      state and changed storage row evidence for future DB CAS writes.
- [x] Production settlement durable commit plans apply through the durable
      commit-store adapter with exact replay and invalid-row rejection.
- [x] Production/storage summary handlers hand server-owned production
      settlement transaction rows to the runtime durable commit-store adapter.
- [x] Durable production settlement outbox rows can be claimed and published
      through the production outbox publisher contract with claim-token guards.
- [ ] Durable production settlement is enforced by DB/idempotency rows and
      published through the durable outbox.
- [x] Server route.settle transfers storage once, returns no-op on immediate
      duplicate reconcile, rejects spoofed settlement facts and wrong-owner
      attempts without mutation/events, emits owner-scoped `route.settled`
      plus route reconciliation events, and avoids AOI diffs.
- [x] Route settlement records production-local storage ledger rows for source
      debit, transfer loss, destination credit, and destination overflow
      partitions, and duplicate/no-op settlements do not append new ledger rows.
- [x] Route settlement owner command path uses an explicit transaction boundary
      that returns the committed route idempotency reference, route storage
      ledger rows, and pending outbox rows from the same store lock.
- [x] Route settlement stores an in-memory settlement reference and pending
      outbox records for route transfer events; duplicate reference reuse
      no-ops without transfer, duplicate events, or duplicate outbox records.
- [x] Route settlement outbox records share the process-local delivery state
      machine for pending, in-flight, published, failed, and explicit retry
      behavior, including claim-token publish/fail guards for retried or
      reclaimed attempts.
- [x] Route settlement transaction outbox rows validate into an after-commit
      dispatch plan before durable publisher scheduling.
- [x] Route settlement transaction rows validate as one durable commit plan
      tying route storage ledger rows to the settlement reference/window.
- [x] Route settlement transaction result exposes its durable commit plan
      directly for future DB/publisher adapters.
- [x] Route settlement durable commit plans apply through the durable
      commit-store adapter with exact replay and invalid-row rejection.
- [x] Route settlement handlers apply committed route references, pending
      outbox rows, and route storage ledger rows through the runtime durable
      commit-store adapter.
- [x] Durable route settlement outbox rows can be claimed, published, failed,
      and lease-released through the production outbox publisher contracts
      without mutating committed route storage ledger rows.
- [x] Durable settlement/route, building mutation, and claim lifecycle
      readback rebuilds committed plans after publisher delivery-state changes
      while preserving current outbox evidence.
- [x] Building mutation durable readback rebuilds a validated pending outbox
      dispatch plan for `planet.building_updated` publisher scheduling.
- [x] Building mutation durable outbox rows can be claimed, published, failed,
      or lease-released through the production outbox publisher contracts
      without mutating committed material ledger rows.
- [x] Runtime durable outbox drain publishes and lease-releases committed
      claim, settlement, and building rows through server-owned callbacks
      without reading from process-local non-durable outbox queues.
- [x] Runtime durable outbox realtime projection recomputes client-safe
      claim, production/storage, route, inventory, and wallet snapshots from
      server-owned read models and flushes queued owner events on tick.
- [x] Runtime durable outbox drain-collect hands safe projected events to the
      active sink delivery path without leaving them queued after publish.
- [x] Automation route durable-row contract records route snapshots with
      idempotency references, revision CAS, detached readback, and owner route
      recovery queries.
- [x] Runtime route create/update/enable/disable writes durable route-row
      snapshots with server-derived idempotency references and revision
      advancement.
- [x] Runtime route.create uses a transaction boundary that rechecks owner
      route-slot capacity under the insert lock before committing route rows.
- [x] Pure route settlement writes durable route-row cursor snapshots with the
      server-derived route settlement idempotency reference.
- [x] Route settlement durable commit bundles include the committed route-row
      snapshot with settlement reference, outbox, and route ledger rows.
- [x] Route settlement durable commit bundles include ledger-backed changed
      storage row evidence alongside route-row and route-ledger evidence.
- [x] Route settlement durable outbox realtime projection publishes owner-scoped
      route settled/snapshot/list plus production/storage reconciliation events.
- [x] Empty-payload route.settle owner reconcile includes committed durable
      owner route rows after live read-model loss and does not duplicate route
      storage/outbox rows.
- [x] Route.create capacity policy counts committed durable owner routes after
      live read-model loss, preserving the server-owned route-slot cap.
- [x] Route.snapshot falls back to committed durable route rows after live
      read-model loss while preserving owner checks and public map-key payloads.
- [x] Browser route.list responses/events/snapshots reconcile the selected
      planet route cache from the same server-owned route list as the global
      route cache.
- [x] Route settlement supports named storage/station-destination aggregates with the
      same server-owned window/reference, ledger, durable route-row, and outbox
      evidence as planet destinations.
- [x] Existing owner-scoped storage/station routes can settle through the
      authenticated gateway with safe payloads/events, masked aggregate IDs,
      and durable settlement evidence while public create/update remains
      planet-only.
- [x] Durable outbox replay of storage/station route settlements preserves
      owner-only safe route events and masks non-planet aggregate destination
      IDs.
- [x] Durable claim, settlement/route, and building outbox stores expose an
      explicit failed-row retry boundary that preserves failure evidence before
      republishing.
- [x] Settlement durable outbox worker paths revalidate committed settlement
      bundles before claim, publish/fail, lease release, or retry mutation.
- [ ] Durable route settlement is enforced by DB/idempotency rows and published
      through the durable outbox.
- [x] Route list/snapshot restores route read model after reconnect.
- [x] Server route.create creates an owned planet route with server-derived
      owner, route id, map ids, safe response/events, and route list/snapshot
      reconciliation.
- [x] Server route.enable/disable toggles an owned route with server-derived
      owner, rejects spoofed server-owned fields and wrong-owner attempts, and
      emits owner-scoped safe route events plus active-map production/storage
      snapshots when disable settlement touches storage.
- [x] Server route.update changes an owned route with server-derived owner,
      rejects spoofed server-owned fields, wrong-owner attempts, and
      X Core/non-routeable resources without mutation/events, emits
      owner-scoped safe route events, and emits active-map production/storage
      snapshots when update settlement touches storage.
- [x] Browser scan creates safe discovered intel.
- [x] Browser selected planet panel uses server detail.
- [x] Browser claim reflects server state.
- [x] Browser owned-planet building build/upgrade controls send server-authoritative
      intent payloads and reconcile from production summary events.
- [x] Browser route create/update/control/settle reflects server state,
      including last safe settlement outcome flags in route rows.
- [x] Browser selected planet route list cannot remain stale after server
      route.list recovery or route removal events.
- [x] Browser route create/update destination controls include server-owned
      storage/station endpoint catalog options without exposing owner or map
      internals.

## Done Criteria

- Scanner discovery and planet/route read models work through the browser.
- Scanner/planet/route UI uses real server authority for exposed operations;
  unexposed mutation controls remain locked/read-only and tracked in
  `docs/todo.md`.
- Hidden/procedural data is not leaked.
- Tests and browser smoke pass.
