# Phase 08: World Discovery, Planets, And Intel

## Status

- State: Not started
- Owner: Exploration and information economy
- Depends on: Phase 03, Phase 04, Phase 05, Phase 07
- Unlocks: planet claiming, X Core sink, planet production, coordinate trading

## Goal

Implement server-owned procedural discovery: scanner pulses reveal hidden planet candidates, materialize discovered planets into persistent records, write personal intel, and validate rank + X Core planet claims.

## Source Specs

Read before implementation:

- `docs/2026-06-17-world-system-design.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `docs/plans/modules/13-intel-coordinate-trading.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- procedural cell and chunk generation for discovery
- scanner pulse resolution
- planet materialization
- player planet intel
- share service
- coordinate scroll item creation and use

Does not own:

- market sale flow
- production formulas
- route transfer
- combat damage

## Core Rules

- Static gameplay seed is server-only.
- Epoch gameplay seed is server-only.
- Client may receive only decorative visual seeds.
- Planets are global shared objects.
- Planet intel is personal unless shared.
- Discovery does not grant ownership.
- Claim requires proximity, rank, and X Core.

## TODO: Procedural World Skeleton

- [ ] Define world seed storage that never serializes to client.
- [ ] Define chunk coordinate model.
- [ ] Define scan cell coordinate model.
- [ ] Implement deterministic cell hash helper.
- [ ] Implement biome classification skeleton.
- [ ] Implement planet candidate generation.
- [ ] Filter candidates by discovery horizon.
- [ ] Filter candidates by biome and spawn budget.
- [ ] Ensure generated hidden candidates are not persisted until discovery.
- [ ] Ensure hidden candidates are never sent to client.

## TODO: Scanner Discovery

- [ ] Define scanner activation command.
- [ ] Validate scanner module equipped.
- [ ] Validate scanner cooldown.
- [ ] Validate capacitor/energy.
- [ ] Apply slow or stationary scan state.
- [ ] Schedule scan pulses server-side.
- [ ] Calculate detection chance from stat snapshot and candidate data.
- [ ] Enforce minimum radar level.
- [ ] Resolve detection roll server-side.
- [ ] Emit generic no-signal response without leaking hidden truth.
- [ ] Reveal signal on successful detection.
- [ ] Confirm planet after successful scan.

## TODO: Planet Materialization And Claim

- [ ] Define planet persistent model.
- [ ] Define planet intel model.
- [ ] Materialize planet into DB/repository on discovery.
- [ ] Create discoverer intel record.
- [ ] Emit `scan.planet_discovered`.
- [ ] Grant scout XP.
- [ ] Implement claim command.
- [ ] Validate player knows planet.
- [ ] Validate proximity.
- [ ] Validate `player_rank >= planet_level_required`.
- [ ] Validate X Core item exists.
- [ ] Consume X Core through inventory service.
- [ ] Set global planet owner.
- [ ] Emit `planet.claimed`.
- [ ] Mark stale listed intel skeleton event for future market phase.

## TODO: Intel Sharing And Items

- [ ] Implement player planet intel upsert.
- [ ] Implement freshness/confidence fields.
- [ ] Implement share quota model.
- [ ] Implement `SharePlanetIntel`.
- [ ] Prevent sharing unknown planet.
- [ ] Preserve fresher receiver intel.
- [ ] Create system mail/notification skeleton.
- [ ] Implement coordinate scroll item creation from known intel.
- [ ] Store scroll metadata server-side.
- [ ] Implement coordinate scroll use.
- [ ] Consume scroll on use in MVP.
- [ ] Add stale marking hook when planet ownership changes.

## Tests

- [ ] Same cell and seed generate same candidate.
- [ ] Gameplay seed never appears in client payload.
- [ ] Hidden planet candidate is not serialized.
- [ ] Scanner without module fails.
- [ ] Scanner cooldown blocks spam.
- [ ] Radar too low cannot discover planet.
- [ ] Successful scan materializes one planet.
- [ ] Duplicate scan does not duplicate planet record.
- [ ] Discovery writes player intel.
- [ ] Claim requires source intel.
- [ ] Claim requires proximity.
- [ ] Claim requires rank.
- [ ] Claim consumes X Core exactly once.
- [ ] Duplicate claim does not consume duplicate X Core.
- [ ] Share requires source intel.
- [ ] Share quota enforced.
- [ ] Receiver gets fog memory.
- [ ] Coordinate scroll creation uses server data.
- [ ] Client cannot create arbitrary intel payload.
- [ ] Existing fresher intel is not overwritten by stale intel.

## Abuse And Safety Checks

- [ ] Procedural seed leak blocked.
- [ ] Hidden planet probing returns generic errors.
- [ ] Scanner pulse is server-timed, not client-spammable.
- [ ] Coordinate forgery blocked by server-side item metadata.
- [ ] Share spam blocked by quota.
- [ ] Fog reveal abuse limited to specific intel points.

## Done Criteria

- [ ] Players can discover planets through server-side scanner flow.
- [ ] Planets become persistent only after discovery.
- [ ] Player intel is personal and shareable.
- [ ] Planet claiming consumes X Core and validates rank/proximity.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, first verify that no client payload contains hidden planet candidates or gameplay seeds. Discovery integrity is the whole point of this phase.
