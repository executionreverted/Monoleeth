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
- Phase 10 records the exact missing contracts for planet claim, intel share,
  coordinate item use, building mutation, offline settlement, and route
  mutation flows. Those controls remain absent, locked, or read-only until their
  server-authoritative transaction paths are implemented.
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
  Browser route create/update/control/settle proof, durable DB/outbox, and
  durable route settlement window idempotency remain open.

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
| `planet.storage_summary` | planet id | ownership/access; client-safe capacity, visible stacks, and catalog-derived `public_map_key` |
| `route.create/update` | endpoint/config intent; update accepts only `route_id`, `destination_planet_id`, `resource_item_id`, `amount_per_hour` | endpoint visibility/access, ownership, capacity, policy; mutate route terms server-side and settle old update terms before replacement |
| `route.enable/disable` | route id | owner is resolved from the authenticated session; control accepts only `route_id`, rechecks route ownership, and returns safe route/list snapshots |
| `route.list/snapshot` | filter or empty | owner/access; reconnect-safe route state, cursors, and public source/destination map keys |
| `route.settle` | route id or empty reconcile intent | backend gateway derives owner from the authenticated session, settles one owned route or all authenticated-owner routes through owner wrappers, returns safe settlement payloads, and keeps durable idempotency key `route_settle:<route_id>:<window>` as future DB/outbox work |

Offline production and route settlement are never client-timed truth. UI requests
may ask the server to reconcile, but the server calculates eligible windows,
locks ownership/storage, applies idempotency, writes ledger/events, commits, and
then broadcasts snapshots.

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
- [ ] Add intel share and coordinate item handlers with visibility-safe
      recipient filtering.
- [x] Add read-only production summary handler for owned planets.
- [ ] Add production build/upgrade handlers.
- [ ] Add ledger-backed transaction flows for claim/build/upgrade/storage
      mutations.
- [ ] Add offline settlement reconcile path that uses server-owned windows.
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
- [ ] Intel sharing cannot reveal a coordinate the sender cannot safely expose.
- [ ] Coordinate item use consumes an owned item once.
- [x] Planet panel open rechecks visibility/ownership.
- [x] Route creation rechecks both endpoints and ownership/access.
- [x] Route update rechecks destination ownership/access and preserves
      server-owned source truth.
- [ ] Offline settlement duration is server-calculated.
- [x] Route settlement timing is server-calculated in the backend gateway.
- [ ] Durable route settlement windows are DB/outbox idempotent.
- [ ] Building and route mutations use inventory/wallet/storage ledgers.
- [ ] Storage capacity cannot be exceeded.

## Tests

- [ ] Scan rejects moving/energy-insufficient player before mutation.
- [x] Scan result does not leak seed or future candidates.
- [ ] Hidden planet detail returns safe error.
- [x] Claim consumes required item once and sets owner once.
- [x] Browser claim sends only `planet_id`, consumes the server-owned E2E X Core
      seed through Inventory, uses server-owned E2E Progression rank eligibility,
      initializes production, clears pending state, and handles `planet.claimed`
      without an unhandled-event log.
- [ ] Intel share rejects hidden/not-owned coordinate references.
- [ ] Coordinate item create/use consumes owned items once and filters results.
- [ ] Building build/upgrade debits materials/currency once.
- [ ] Production settlement is idempotent.
- [x] Server route.settle transfers storage once, returns no-op on immediate
      duplicate reconcile, rejects spoofed settlement facts and wrong-owner
      attempts without mutation/events, emits owner-scoped `route.settled`
      plus route reconciliation events, and avoids AOI diffs.
- [ ] Durable route settlement is idempotent by DB window and outbox reference.
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
- [ ] Browser route create/update/control/settle reflects server state.

## Done Criteria

- Scanner discovery and planet/route read models work through the browser.
- Scanner/planet/route UI uses real server authority for exposed operations;
  unexposed mutation controls remain locked/read-only and tracked in
  `docs/todo.md`.
- Hidden/procedural data is not leaked.
- Tests and browser smoke pass.
