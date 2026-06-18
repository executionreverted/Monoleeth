# Phase 09: Planet Production And Automation Routes

## Status

- State: In review
- Owner: Strategy and persistence layer
- Depends on: Phase 02, Phase 08
- Unlocks: long-term resource economy, production chains, logistics gameplay

## Goal

Turn claimed planets into productive network nodes with storage, buildings, offline settlement, and virtual automation routes between accessible locations.

## Source Specs

Read before implementation:

- `docs/plans/modules/11-planet-production-offline-settlement.md`
- `docs/plans/modules/12-automation-routes.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/2026-06-17-world-system-design.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `PlanetProductionService`
- `OfflineSettlementService`
- `AutomationRouteService`
- `RouteSettlementService`

Does not own:

- planet discovery and claim
- storage primitive internals
- physical convoy combat
- market sale flow

## MVP Scope

Production:

- one timestamp per planet
- simple building production
- storage capacity clamp
- input-based production
- energy budget rule
- settlement on login or inspection

Routes:

- virtual transfer, no physical convoy
- partial loss
- source and destination storage locks
- route summary API

## TODO: Planet Storage And Buildings

- [x] Define planet storage model.
- [x] Define storage capacity rules.
- [x] Define building model.
- [x] Define building production catalog.
- [x] Define planet production state.
- [x] Initialize production state on planet claim.
- [x] Add basic extractor building.
- [x] Add basic refinery building.
- [x] Add energy budget fields.
- [x] Add building active/disabled state.

## TODO: Production Settlement

- [x] Implement `SettlePlanetProduction`.
- [x] Lock planet production state.
- [x] Calculate elapsed from server timestamp.
- [x] Clamp elapsed to max offline duration.
- [x] Handle future `last_calculated_at` safely.
- [x] Load active buildings.
- [x] Lock planet storage.
- [x] Calculate output for elapsed time.
- [x] Apply storage capacity clamp.
- [x] Consume input materials proportionally.
- [x] Handle input shortage.
- [x] Handle insufficient energy.
- [x] Update `last_calculated_at`.
- [ ] Emit production settlement events.
- [x] Produce login/inspection summary.

## TODO: Routes

- [x] Define route model.
- [x] Define route risk model.
- [x] Implement `CreateRoute`.
- [x] Validate source planet ownership.
- [x] Validate destination accessibility.
- [x] Validate resource is routeable.
- [x] Validate positive amount per hour.
- [x] Validate rank/building requirement.
- [x] Calculate route distance.
- [x] Calculate route loss chance.
- [x] Implement route enable/disable.
- [x] Implement route update that settles old state first.
- [x] Implement `SettleRoute`.
- [x] Lock route.
- [x] Lock source storage.
- [x] Remove up to wanted amount.
- [x] Apply partial loss.
- [x] Lock destination storage.
- [x] Add up to capacity.
- [x] Update route timestamp.
- [ ] Emit route settlement events.

## Tests

- [x] One hour production output is correct.
- [x] Storage cap clamps output.
- [x] Input shortage reduces output.
- [x] Offline cap applies.
- [x] Double settlement does not duplicate output.
- [x] Future timestamp is handled safely.
- [x] Energy insufficient disables or scales production.
- [x] Login settlement summary is correct.
- [x] Create route validates source ownership.
- [x] Unauthorized destination fails.
- [x] Empty source transfers zero.
- [x] Full destination clamps delivery.
- [x] Loss chance applies in configured range.
- [x] Double route settlement does not duplicate transfer.
- [x] Disable/enable preserves timestamp correctly.
- [x] Route update settles old state first.

## Abuse And Safety Checks

- [x] Client cannot fake offline duration.
- [x] Duplicate settlement blocked by lock and timestamp update.
- [x] Storage overflow blocked by capacity clamp.
- [x] Ownership race handled by locking or MVP transfer restrictions.
- [x] Infinite route transfer duplication blocked.
- [x] Destination capacity bypass blocked.
- [x] Route risk avoidance by toggling mitigated by timestamp handling.

## Done Criteria

- [x] Claimed planet can produce resources over time.
- [x] Offline settlement works without per-second jobs.
- [x] Planet storage capacity limits production.
- [x] Virtual routes move resources with loss and capacity checks.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Implementation Notes

- Production and route settlement are in-memory MVP services under
  `internal/game/production`.
- Planet claim initializes production through an optional discovery-facing
  adapter; discovery does not import concrete production services.
- Route creation validates ownership, accessibility, resource routing, distance,
  rank/building requirements, energy cost, and risk through server-owned policy
  provider facts.
- Route settlement currently supports planet-to-planet storage. Generic
  `storage` and `station` destination settlement adapters are deferred.
- Settlement methods return summaries, but durable event/outbox emission remains
  deferred.

## Resume Notes

If resuming here, inspect settlement timestamp tests first. Duplicate settlement and capacity bugs are the highest-risk failures in this phase.
