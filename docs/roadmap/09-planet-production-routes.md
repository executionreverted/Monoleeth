# Phase 09: Planet Production And Automation Routes

## Status

- State: Not started
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

- [ ] Define planet storage model.
- [ ] Define storage capacity rules.
- [ ] Define building model.
- [ ] Define building production catalog.
- [ ] Define planet production state.
- [ ] Initialize production state on planet claim.
- [ ] Add basic extractor building.
- [ ] Add basic refinery building.
- [ ] Add energy budget fields.
- [ ] Add building active/disabled state.

## TODO: Production Settlement

- [ ] Implement `SettlePlanetProduction`.
- [ ] Lock planet production state.
- [ ] Calculate elapsed from server timestamp.
- [ ] Clamp elapsed to max offline duration.
- [ ] Handle future `last_calculated_at` safely.
- [ ] Load active buildings.
- [ ] Lock planet storage.
- [ ] Calculate output for elapsed time.
- [ ] Apply storage capacity clamp.
- [ ] Consume input materials proportionally.
- [ ] Handle input shortage.
- [ ] Handle insufficient energy.
- [ ] Update `last_calculated_at`.
- [ ] Emit production settlement events.
- [ ] Produce login/inspection summary.

## TODO: Routes

- [ ] Define route model.
- [ ] Define route risk model.
- [ ] Implement `CreateRoute`.
- [ ] Validate source planet ownership.
- [ ] Validate destination accessibility.
- [ ] Validate resource is routeable.
- [ ] Validate positive amount per hour.
- [ ] Validate rank/building requirement.
- [ ] Calculate route distance.
- [ ] Calculate route loss chance.
- [ ] Implement route enable/disable.
- [ ] Implement route update that settles old state first.
- [ ] Implement `SettleRoute`.
- [ ] Lock route.
- [ ] Lock source storage.
- [ ] Remove up to wanted amount.
- [ ] Apply partial loss.
- [ ] Lock destination storage.
- [ ] Add up to capacity.
- [ ] Update route timestamp.
- [ ] Emit route settlement events.

## Tests

- [ ] One hour production output is correct.
- [ ] Storage cap clamps output.
- [ ] Input shortage reduces output.
- [ ] Offline cap applies.
- [ ] Double settlement does not duplicate output.
- [ ] Future timestamp is handled safely.
- [ ] Energy insufficient disables or scales production.
- [ ] Login settlement summary is correct.
- [ ] Create route validates source ownership.
- [ ] Unauthorized destination fails.
- [ ] Empty source transfers zero.
- [ ] Full destination clamps delivery.
- [ ] Loss chance applies in configured range.
- [ ] Double route settlement does not duplicate transfer.
- [ ] Disable/enable preserves timestamp correctly.
- [ ] Route update settles old state first.

## Abuse And Safety Checks

- [ ] Client cannot fake offline duration.
- [ ] Duplicate settlement blocked by lock and timestamp update.
- [ ] Storage overflow blocked by capacity clamp.
- [ ] Ownership race handled by locking or MVP transfer restrictions.
- [ ] Infinite route transfer duplication blocked.
- [ ] Destination capacity bypass blocked.
- [ ] Route risk avoidance by toggling mitigated by timestamp handling.

## Done Criteria

- [ ] Claimed planet can produce resources over time.
- [ ] Offline settlement works without per-second jobs.
- [ ] Planet storage capacity limits production.
- [ ] Virtual routes move resources with loss and capacity checks.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, inspect settlement timestamp tests first. Duplicate settlement and capacity bugs are the highest-risk failures in this phase.
