# Phase 07: Discovery, Planets, Production, And Routes UI

## Status

- State: Completed for scanner/read-model MVP; planet/building/route mutation
  contracts remain tracked in `docs/todo.md`
- Owner: Exploration and persistent planet network UI
- Depends on: Phase 06
- Unlocks: long-term strategy loop

## Goal

Expose scanner discovery, fog/intel, coordinate items, planet claiming,
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
- Phase 10 records the exact missing contracts for planet claim, intel share,
  coordinate item use, building mutation, offline settlement, and route
  mutation flows. Those controls remain absent, locked, or read-only until their
  server-authoritative transaction paths are implemented.

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
| `scan.pulse` | request scan | server position, stationary state, energy, cooldown, fog; emits safe signal/discovery results |
| `discovery.known_planets` | list/filter | player visibility/intel/ownership; returns only known safe summaries |
| `discovery.planet_detail` | planet id | recheck visibility/ownership; omit hidden/procedural fields |
| `discovery.claim_planet` | planet id | validate visibility, range/policy, required item/currency; lock/mutate/ledger/event/commit |
| `intel.share` | recipient, intel/planet/coordinate reference | sender visibility, recipient eligibility, client-safe filtering; never share hidden coordinates |
| `intel.coordinate_item.create` | known coordinate reference | owned/visible coordinate; consume/move item through inventory ledger once |
| `intel.coordinate_item.use` | owned coordinate item id | ownership, visibility rules, item consumption idempotency; reveal only safe result |
| `planet.production_summary` | planet id | ownership/access; settle/reconcile server-owned windows as needed |
| `planet.building_build` | planet id, building type/slot | ownership, requirements, storage/wallet/materials; lock/mutate/ledger/event/commit |
| `planet.building_upgrade` | building id | ownership, level requirements, storage/wallet/materials; lock/mutate/ledger/event/commit |
| `planet.storage_summary` | planet id | ownership/access; client-safe capacity and visible stacks |
| `route.create/update/enable/disable` | endpoint/config intent | endpoint visibility/access, ownership, capacity, policy; mutate route state server-side |
| `route.list/snapshot` | filter or empty | owner/access; reconnect-safe route state and cursors |
| `route.settle` | route id or empty reconcile intent | server computes eligible windows under lock; idempotency key `route_settle:<route_id>:<window>` |

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
- [ ] Add planet claim command handler.
- [ ] Add intel share and coordinate item handlers with visibility-safe
      recipient filtering.
- [x] Add read-only production summary handler for owned planets.
- [ ] Add production build/upgrade handlers.
- [ ] Add ledger-backed transaction flows for claim/build/upgrade/storage
      mutations.
- [ ] Add offline settlement reconcile path that uses server-owned windows.
- [ ] Add route create/update/enable/disable/settle handlers.
- [x] Add route list/snapshot handlers for reconnect.
- [x] Add client reducer state for signals, planets, production, routes.
- [x] Add right rail known planet list.
- [x] Add selected planet action panel.
- [x] Add route UI and production/building UI.
- [x] Add scanner action state and log events.

## Abuse And Safety Checklist

- [x] Client cannot send planet candidate data as truth.
- [x] Client cannot send scan result or procedural seed.
- [ ] Client cannot claim hidden/unowned-invalid planet.
- [ ] Client cannot fake X Core consumption.
- [ ] Intel sharing cannot reveal a coordinate the sender cannot safely expose.
- [ ] Coordinate item use consumes an owned item once.
- [x] Planet panel open rechecks visibility/ownership.
- [ ] Route creation rechecks both endpoints and ownership/access.
- [ ] Offline settlement duration is server-calculated.
- [ ] Route settlement windows are server-calculated and idempotent.
- [ ] Building and route mutations use inventory/wallet/storage ledgers.
- [ ] Storage capacity cannot be exceeded.

## Tests

- [ ] Scan rejects moving/energy-insufficient player before mutation.
- [x] Scan result does not leak seed or future candidates.
- [ ] Hidden planet detail returns safe error.
- [ ] Claim consumes required item once and sets owner once.
- [ ] Intel share rejects hidden/not-owned coordinate references.
- [ ] Coordinate item create/use consumes owned items once and filters results.
- [ ] Building build/upgrade debits materials/currency once.
- [ ] Production settlement is idempotent.
- [ ] Route settlement is idempotent and respects storage capacity.
- [x] Route list/snapshot restores route read model after reconnect.
- [x] Browser scan creates safe discovered intel.
- [x] Browser selected planet panel uses server detail.
- [ ] Browser route create/update reflects server state.

## Done Criteria

- Scanner discovery and planet/route read models work through the browser.
- Scanner/planet/route UI uses real server authority for exposed operations;
  unexposed mutation controls remain locked/read-only and tracked in
  `docs/todo.md`.
- Hidden/procedural data is not leaked.
- Tests and browser smoke pass.
