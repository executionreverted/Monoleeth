# Phase 07: Discovery, Planets, Production, And Routes UI

## Status

- State: Planned
- Owner: Exploration and persistent planet network UI
- Depends on: Phase 06
- Unlocks: long-term strategy loop

## Goal

Expose scanner discovery, fog/intel, coordinate items, planet claiming,
production buildings/storage, offline settlement, and automation routes through
real server-backed panels and map interactions.

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
route.update
route.enable
route.disable
route.settle
```

## Events

```text
scan.pulse_started
scan.pulse_resolved
scanner.signal_detected
planet.discovered
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

- [ ] Register real `scan.pulse` operation in Go realtime registry.
- [ ] Add authenticated scanner command handler.
- [ ] Add scanner event mapper to safe UI payloads.
- [ ] Add planet list/summary query for known/owned planets.
- [ ] Add selected planet detail query with visibility checks.
- [ ] Add planet claim command handler.
- [ ] Add intel share and coordinate item handlers.
- [ ] Add production summary/build/upgrade handlers.
- [ ] Add offline settlement query/trigger path for UI.
- [ ] Add route create/update/enable/disable/settle handlers.
- [ ] Add client reducer state for signals, planets, production, routes.
- [ ] Add right rail planet list and selected planet panel.
- [ ] Add route UI and production/building UI.
- [ ] Add scanner action bar state and log events.

## Abuse And Safety Checklist

- [ ] Client cannot send planet candidate data as truth.
- [ ] Client cannot send scan result or procedural seed.
- [ ] Client cannot claim hidden/unowned-invalid planet.
- [ ] Client cannot fake X Core consumption.
- [ ] Planet panel open rechecks visibility/ownership.
- [ ] Route creation rechecks both endpoints and ownership/access.
- [ ] Offline settlement duration is server-calculated.
- [ ] Storage capacity cannot be exceeded.

## Tests

- [ ] Scan rejects moving/energy-insufficient player before mutation.
- [ ] Scan result does not leak seed or future candidates.
- [ ] Hidden planet detail returns safe error.
- [ ] Claim consumes required item once and sets owner once.
- [ ] Production settlement is idempotent.
- [ ] Route settlement is idempotent and respects storage capacity.
- [ ] Browser scan creates safe unknown/discovered marker.
- [ ] Browser selected planet panel uses server detail.
- [ ] Browser route create/update reflects server state.

## Done Criteria

- Exploration and planet network loop works through browser.
- Scanner/planet/route UI uses real server authority.
- Hidden/procedural data is not leaked.
- Tests and browser smoke pass.
