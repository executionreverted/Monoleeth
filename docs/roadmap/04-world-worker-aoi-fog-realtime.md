# Phase 04: World Worker, AOI, Fog, And Realtime Contracts

## Status

- State: Complete, verified 2026-06-18 - WebSocket adapter follow-up tracked
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
- [x] Define command mailbox interface.
- [x] Implement single worker tick loop.
- [x] Implement fixed tick delta.
- [x] Implement command drain order.
- [x] Implement delayed task scheduler skeleton.
- [x] Implement entity insert/remove/update.
- [x] Implement player session attachment model.
- [x] Implement server-owned movement target state.
- [x] Implement movement from server stat speed.
- [x] Reject client final position as truth.

## TODO: Spatial Hash And AOI

- [x] Implement spatial hash cell coordinate calculation.
- [x] Implement entity membership updates.
- [x] Implement radius query with exact distance check.
- [x] Implement AOI candidate query.
- [x] Implement visible entity diff: entered, updated, left.
- [x] Implement snapshot payload that includes visible entities only.

## TODO: Visibility And Fog

- [x] Implement `CanSendEntityToClient`.
- [x] Implement `CanInteract`.
- [x] Add radar range input from effective stats.
- [x] Add entity signature field.
- [x] Add hidden entity flag.
- [x] Ensure hidden entities never serialize.
- [x] Add fog memory model skeleton for discovered planets.
- [x] Add scanner bridge event skeleton.
- [x] Add generic hidden/not-found error behavior.

## TODO: Realtime Contracts

- [x] Define JSON operation registry.
- [x] Validate request envelope fields.
- [x] Resolve player/session server-side.
- [x] Add request ID cache skeleton for retry safety.
- [x] Add per-op rate limit placeholders.
- [x] Define common client events: `player.snapshot`, `aoi.entity_entered`, `aoi.entity_left`, `position.corrected`.
- [x] Ensure internal event payload can differ from filtered client event payload.
- [x] Add commit-then-broadcast convention to package docs.

## Tests

- [x] Movement intent updates position by server speed.
- [x] Client-supplied impossible position is ignored.
- [x] Spatial hash returns nearby entities.
- [x] Spatial hash does not return far entities after exact distance check.
- [x] Hidden entity is not serialized.
- [x] Entity leaving AOI emits left/despawn.
- [x] Fog memory does not grant live interaction permission.
- [x] Interaction with hidden entity fails.
- [x] Duplicate request ID returns safe retry behavior.
- [x] Invalid payload is rejected.
- [x] Error messages for hidden entities do not leak hidden truth.
- [x] AOI stress test with many entities stays deterministic.

## Abuse And Safety Checks

- [x] Packet sniffing hidden data is impossible because hidden data is not serialized.
- [x] Entity ID memory attack fails because interaction rechecks visibility.
- [x] Radar spoof fails because radar comes from stat snapshot.
- [x] Procedural gameplay seed is not present in payloads.
- [x] Operation flood has at least a placeholder rate-limit path.

## Done Criteria

- [x] A test worker can spawn a player and move them server-authoritatively.
- [x] Visible snapshots contain only allowed entities.
- [x] Hidden entities cannot be interacted with.
- [x] Realtime envelope types exist.
- [x] Combat phase can call visibility/range helpers.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Resume Notes

If resuming here, first run or inspect tests that prove hidden entities never serialize. Do not start combat until visibility and position authority are working.

Verified slices:

- World, zone, entity, position, movement target, and movement intent primitives are implemented in `internal/game/world`.
- `AdvanceMovement` moves toward a target by server-provided speed and tick delta, stops without overshoot, and exposes no client final-position input.
- Spatial hash cell coordinates, entity insert/update/remove membership, deterministic radius queries, and exact distance filtering are implemented in `internal/game/world/spatial`.
- Realtime JSON request/response/error/event envelopes, Phase 04 operation registry, client event constants, request ID cache skeleton, and rate-limit posture metadata are implemented in `internal/game/realtime`.
- Transport-agnostic realtime gateway request handling is implemented in `internal/game/realtime`: `Gateway` decodes request envelopes, resolves authenticated session/player/world/zone identity through a server-side `SessionResolver`, ignores client payload identity, executes registered operation handlers with `ObservedCommandExecutor`, and caches completed responses by session/request id.
- A single-zone in-process worker harness with FIFO command mailbox, fixed tick delta, deterministic command drain, delayed task scheduler skeleton, entity lifecycle, player session attachment, and server-speed movement is implemented in `internal/game/world/worker`.
- Worker scheduled-task dispatch runs registered in-process handlers during ticks, records handler errors without stopping later due tasks, and can requeue handler-declared early tasks for a later retry.
- Visibility filtering, generic hidden/not-visible interaction errors, server-stat radar range input, entity signature/hidden flags, fog memory summaries, and scanner bridge event shells are implemented in `internal/game/world/visibility`.
- Client-safe AOI snapshots and deterministic entered/updated/left diffs are implemented in `internal/game/world/aoi`; snapshot payloads omit hidden/internal metadata, seeds, movement internals, world/zone ids, and future spawn data.
- Final verification passed with `go test ./...`, `git diff --check`, and `go test -race ./internal/game/world/... ./internal/game/realtime`.

Remaining follow-up:

- A concrete WebSocket network adapter is still future Phase 11 work; Phase 04 now has the in-process gateway boundary that resolves sessions server-side before operation handlers run.
- Scheduled-task dispatch is an in-memory worker facility, not a durable outbox, distributed queue, or realtime gateway.
