# Phase 04: World Worker, AOI, Fog, And Realtime Contracts

## Status

- State: Not started
- Owner: Realtime world simulation
- Depends on: Phase 01, Phase 03
- Unlocks: combat validation, scanner discovery, loot visibility, client prototype

## Goal

Build the first authoritative map or zone worker with movement intents, spatial hash, AOI, visibility filtering, fog memory hooks, JSON realtime envelopes, and commit-then-broadcast discipline.

## Why This Comes Before Combat

Combat needs authoritative position, range, and visibility. Loot pickup needs visibility and distance. Scanner discovery must not leak hidden procedural data.

## Source Specs

Read before implementation:

- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `docs/2026-06-17-world-system-design.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`

## Module Ownership

Owns:

- `AOIService`
- `FogOfWarService`
- `VisibilityService`
- `ScannerVisibilityBridge` skeleton
- realtime protocol contracts
- map worker command mailbox

Does not own:

- combat damage
- loot table rolls
- planet generation truth
- market intel sales

## Package Direction

Suggested packages:

```text
internal/game/world/
internal/game/world/worker/
internal/game/world/spatial/
internal/game/world/visibility/
internal/game/realtime/
```

## MVP Worker Scope

Start with one world and one zone.

Entities:

- players
- NPC placeholder
- loot placeholder
- planet signal placeholder

Commands:

- `move_to`
- `stop`
- `debug_spawn_npc`
- `debug_snapshot`

Do not build full gateway scaling yet. Keep a direct in-process worker test harness first.

## TODO: Worker Core

- [x] Define world, zone, entity, and position types.
- [ ] Define command mailbox interface.
- [ ] Implement single worker tick loop.
- [ ] Implement fixed tick delta.
- [ ] Implement command drain order.
- [ ] Implement delayed task scheduler skeleton.
- [ ] Implement entity insert/remove/update.
- [ ] Implement player session attachment model.
- [x] Implement server-owned movement target state.
- [x] Implement movement from server stat speed.
- [x] Reject client final position as truth.

## TODO: Spatial Hash And AOI

- [ ] Implement spatial hash cell coordinate calculation.
- [ ] Implement entity membership updates.
- [ ] Implement radius query with exact distance check.
- [ ] Implement AOI candidate query.
- [ ] Implement visible entity diff: entered, updated, left.
- [ ] Implement snapshot payload that includes visible entities only.

## TODO: Visibility And Fog

- [ ] Implement `CanSendEntityToClient`.
- [ ] Implement `CanInteract`.
- [ ] Add radar range input from effective stats.
- [ ] Add entity signature field.
- [ ] Add hidden entity flag.
- [ ] Ensure hidden entities never serialize.
- [ ] Add fog memory model skeleton for discovered planets.
- [ ] Add scanner bridge event skeleton.
- [ ] Add generic hidden/not-found error behavior.

## TODO: Realtime Contracts

- [ ] Define JSON operation registry.
- [ ] Validate request envelope fields.
- [ ] Resolve player/session server-side.
- [ ] Add request ID cache skeleton for retry safety.
- [ ] Add per-op rate limit placeholders.
- [ ] Define common client events: `player.snapshot`, `aoi.entity_entered`, `aoi.entity_left`, `position.corrected`.
- [ ] Ensure internal event payload can differ from filtered client event payload.
- [ ] Add commit-then-broadcast convention to package docs.

## Tests

- [x] Movement intent updates position by server speed.
- [x] Client-supplied impossible position is ignored.
- [ ] Spatial hash returns nearby entities.
- [ ] Spatial hash does not return far entities after exact distance check.
- [ ] Hidden entity is not serialized.
- [ ] Entity leaving AOI emits left/despawn.
- [ ] Fog memory does not grant live interaction permission.
- [ ] Interaction with hidden entity fails.
- [ ] Duplicate request ID returns safe retry behavior.
- [ ] Invalid payload is rejected.
- [ ] Error messages for hidden entities do not leak hidden truth.
- [ ] AOI stress test with many entities stays deterministic.

## Abuse And Safety Checks

- [ ] Packet sniffing hidden data is impossible because hidden data is not serialized.
- [ ] Entity ID memory attack fails because interaction rechecks visibility.
- [ ] Radar spoof fails because radar comes from stat snapshot.
- [ ] Procedural gameplay seed is not present in payloads.
- [ ] Operation flood has at least a placeholder rate-limit path.

## Done Criteria

- [ ] A test worker can spawn a player and move them server-authoritatively.
- [ ] Visible snapshots contain only allowed entities.
- [ ] Hidden entities cannot be interacted with.
- [ ] Realtime envelope types exist.
- [ ] Combat phase can call visibility/range helpers.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, first run or inspect tests that prove hidden entities never serialize. Do not start combat until visibility and position authority are working.

Verified slices:

- World, zone, entity, position, movement target, and movement intent primitives are implemented in `internal/game/world`.
- `AdvanceMovement` moves toward a target by server-provided speed and tick delta, stops without overshoot, and exposes no client final-position input.
