# Phase 08: World Discovery, Planets, And Intel

## Status

- State: Complete, verified 2026-06-18 - runtime/gateway follow-ups tracked
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

- [x] Define world seed storage that never serializes to client.
- [x] Define chunk coordinate model.
- [x] Define scan cell coordinate model.
- [x] Implement deterministic cell hash helper.
- [x] Implement biome classification skeleton.
- [x] Implement planet candidate generation.
- [x] Filter candidates by discovery horizon.
- [x] Filter candidates by biome and spawn budget.
- [x] Ensure generated hidden candidates are not persisted until discovery.
- [x] Ensure hidden candidates are never sent to client.

## TODO: Scanner Discovery

- [x] Define scanner activation command.
- [x] Validate scanner module equipped.
- [x] Validate scanner cooldown.
- [x] Validate capacitor/energy. Verified 2026-06-18 by
  `TestStartScanPulseEnergyUnavailableFailsBeforeCooldownAndMutation`.
- [x] Validate stationary scan state before cooldown and pulse creation.
  Verified 2026-06-18 by
  `TestStartScanPulseMovingShipFailsBeforeCooldownAndMutation`; live slow-state
  lease application remains tracked in `docs/todo.md`.
- [x] Schedule scan pulses server-side.
- [x] Calculate detection chance from stat snapshot and candidate data.
- [x] Enforce minimum radar level.
- [x] Resolve detection roll server-side.
- [x] Emit generic no-signal response without leaking hidden truth.
- [x] Reveal signal on successful detection.
- [x] Confirm planet after successful scan.

## TODO: Planet Materialization And Claim

- [x] Define planet persistent model.
- [x] Define planet intel model.
- [x] Materialize planet into DB/repository on discovery.
- [x] Create discoverer intel record.
- [x] Emit `scan.planet_discovered`.
- [x] Grant scout XP.
- [x] Implement claim command.
- [x] Validate player knows planet.
- [x] Validate proximity.
- [x] Validate `player_rank >= planet_level_required`.
- [x] Validate X Core item exists.
- [x] Consume X Core through inventory service.
- [x] Set global planet owner.
- [x] Emit `planet.claimed`.
- [x] Mark older personal intel stale on planet ownership change.

## TODO: Intel Sharing And Items

- [x] Implement player planet intel upsert.
- [x] Implement freshness/confidence fields.
- [x] Implement share quota model.
- [x] Implement `SharePlanetIntel`.
- [x] Prevent sharing unknown planet.
- [x] Preserve fresher receiver intel.
- [x] Create system mail/notification skeleton.
- [x] Implement coordinate scroll item creation from known intel.
- [x] Store scroll metadata server-side.
- [x] Implement coordinate scroll use.
- [x] Consume scroll on use in MVP.
- [x] Add personal-intel stale marking hook when planet ownership changes.

## Tests

- [x] Same cell and seed generate same candidate.
- [x] Gameplay seed never appears in client payload.
- [x] Hidden planet candidate is not serialized.
- [x] Scanner without module fails.
- [x] Scanner cooldown blocks spam.
- [x] Scanner energy unavailable fails before cooldown or scanner mutation.
- [x] Moving ship cannot start scanner pulse.
- [x] Radar too low cannot discover planet.
- [x] Successful scan materializes one planet.
- [x] Duplicate scan does not duplicate planet record.
- [x] Discovery writes player intel.
- [x] Claim requires source intel.
- [x] Claim requires proximity.
- [x] Claim requires rank.
- [x] Claim consumes X Core exactly once.
- [x] Duplicate claim does not consume duplicate X Core.
- [x] Share requires source intel.
- [x] Share quota enforced.
- [x] Receiver gets fog memory.
- [x] Coordinate scroll creation uses server data.
- [x] Client cannot create arbitrary intel payload.
- [x] Existing fresher intel is not overwritten by stale intel.

## Abuse And Safety Checks

- [x] Procedural seed leak blocked.
- [x] Hidden planet probing returns generic errors.
- [x] Scanner pulse is server-timed, not client-spammable.
- [x] Coordinate forgery blocked by server-side item metadata.
- [x] Share spam blocked by quota.
- [x] Fog reveal abuse limited to specific intel points.

## Done Criteria

- [x] Players can discover planets through server-side scanner flow.
- [x] Planets become persistent only after discovery.
- [x] Player intel is personal and shareable.
- [x] Planet claiming consumes X Core and validates rank/proximity.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Implementation Notes

- Scanner gate implementation plan:
  `docs/plans/2026-06-18-phase08-scanner-gates.md`.
- Phase 08 currently lands as a backend in-memory domain MVP under
  `internal/game/discovery`; no realtime, REST, DB persistence, market listing,
  production, or route integration is exposed yet.
- Scanner, claim, share, and coordinate-scroll paths use provider interfaces for
  runtime stat/cooldown, XP, proximity, rank, inventory, quota, and item-consume
  boundaries.
- Scanner start now requires a server-owned stationary movement state and
  server-owned scanner energy availability before cooldown, pulse, or event
  mutation.
- Symphony security, performance, and code-quality review found boundary
  hardening work; actionable Phase 08 MVP fixes landed, and larger
  multi-process/runtime risks are tracked in `docs/todo.md`.

## Resume Notes

If resuming here, first verify that no client payload contains hidden planet candidates or gameplay seeds. Discovery integrity is the whole point of this phase.
