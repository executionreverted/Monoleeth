# Phase 05 — Map Worker Ownership & Concurrency

## Status
- State: In progress
- Wave: 2
- Depends on: P01
- Unlocks: P11, scale

## Goal
Reduce the single global `Runtime.mu` bottleneck by giving each map worker
ownership of its live entities/AOI, and add race tests for concurrent command + tick.

## Why (report refs)
- Code review §2 (High): one global mutex serializes ticks, commands, AOI, economy.
- Architecture docs: actor-style map workers behind gateway/router.

## Scope
- Per-map command queue owns movement/entities/AOI for that map.
- Runtime lock narrowed to session map + global routing metadata.
- Race tests proving no data race without relying on the global lock.

## Runtime Mutex Contract
- `Runtime.mu` is the runtime-coordinator lock, not the per-map command/tick
  gate. Map workers own per-map live entity mutation.
- `Runtime.mu` protects runtime-owned cross-worker metadata: session/player and
  session/map routing, session map epochs, replay/queued-event maps, player
  runtime bookkeeping, per-instance `ActiveSessions`/`LastAOI` cursors, and
  transitional command guards/cooldowns still held on `Runtime`.
- `TestConcurrentAttachDetachSerializesSessionMapsAndWorkerAttachment` proves
  concurrent session attach/detach serializes `sessions`, `sessionLocations`,
  `sessionEpochs`, per-map session cursors, and worker session attachment.
- Broader P05 narrowing remains open while non-session transient guards still
  live on `Runtime`.

## Out Of Scope
- Multi-process workers / cross-host handoff (post-v1).

## Tasks
- [ ] `[P:wave2/lane-D]` Move per-map live state mutation behind each worker's command queue.
- [ ] `[P:wave2/lane-D]` Narrow `Runtime.mu` to session/routing only; document what it still protects.
- [ ] `[P:wave2/lane-D]` Use immutable snapshots/copy-on-write for read projections (AOI, minimap).
- [x] `[P:wave2/lane-D]` Add `-race` tests: concurrent commands + tick on the same and on different maps.

## Server Ownership
- Each map worker is the single owner of its zone's authoritative live state.
- Progress: loot pickup drop removal now submits `worker.RemoveEntityCommand`
  through the owning worker queue instead of calling `Worker.RemoveEntity`
  directly. Covered by `TestLootPickupRemovesDropThroughWorkerCommandQueue`.
- Progress: loot drop creation now submits `worker.InsertEntityCommand` through
  the owning worker queue instead of calling `Worker.InsertEntity` directly.
  Covered by `TestLootDropInsertUsesWorkerCommandQueue`.
- Progress: portal transfer source-player removal now submits
  `worker.RemoveEntityCommand` through the source worker queue instead of
  calling `Worker.RemoveEntity` directly. Covered by
  `TestPortalEnterRemovesSourcePlayerThroughWorkerCommandQueue`.
- Progress: failed portal transfer source restore now submits
  `worker.UpdateEntityCommand` through the source worker queue instead of
  calling `Worker.UpdateEntity` directly. Covered by
  `TestPortalEnterFailedTransferRestoresSourcePlayerThroughWorkerCommandQueue`.
- Progress: runtime movement refresh now submits
  `worker.RefreshPlayerMovementPositionCommand` through the owning worker queue
  and flushes queued commands without running a full simulation tick. Covered by
  `TestRefreshPlayerMovementPositionUsesCommandQueueWithoutStoppingRoute`.
- Progress: login/portal/respawn inactive map cleanup now submits
  `worker.RemoveEntityCommand` through each inactive map's owning worker queue
  and flushes queued commands without running a full simulation tick. Covered by
  `TestInactiveCleanupRemovesPlayerThroughWorkerCommandQueue`.

## Smoke Tests (one assertion each)
- [x] Command on map A does not block a command on map B (timing assertion).
- [x] Concurrent move + tick on one map passes `-race`.
- [x] Map B worker tick collection reaches the worker while `Runtime.mu` is held by unrelated runtime activity (`TestRuntimeTickCollectionReachesOtherMapWhileRuntimeMutexHeld`).
- [x] AOI read projection never observes a torn entity state.
- [x] Narrowed lock still serializes session attach/detach safely
  (`TestConcurrentAttachDetachSerializesSessionMapsAndWorkerAttachment`).

## Done Criteria
- [ ] Map A activity no longer serializes behind map B.
- [x] `-race` tests green for concurrent command/tick.

## Verification
```bash
go test ./internal/game/world/... ./internal/game/server/... -run 'Worker|Concurren|Race|Ownership' -count=1 -race
go test ./... && git diff --check
```
