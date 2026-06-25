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

## Out Of Scope
- Multi-process workers / cross-host handoff (post-v1).

## Tasks
- [ ] `[P:wave2/lane-D]` Move per-map live state mutation behind each worker's command queue.
- [ ] `[P:wave2/lane-D]` Narrow `Runtime.mu` to session/routing only; document what it still protects.
- [ ] `[P:wave2/lane-D]` Use immutable snapshots/copy-on-write for read projections (AOI, minimap).
- [x] `[P:wave2/lane-D]` Add `-race` tests: concurrent commands + tick on the same and on different maps.

## Server Ownership
- Each map worker is the single owner of its zone's authoritative live state.

## Smoke Tests (one assertion each)
- [x] Command on map A does not block a command on map B (timing assertion).
- [x] Concurrent move + tick on one map passes `-race`.
- [x] Map B worker tick collection reaches the worker while `Runtime.mu` is held by unrelated runtime activity (`TestRuntimeTickCollectionReachesOtherMapWhileRuntimeMutexHeld`).
- [x] AOI read projection never observes a torn entity state.
- [x] Narrowed lock still serializes session attach/detach safely.

## Done Criteria
- [ ] Map A activity no longer serializes behind map B.
- [x] `-race` tests green for concurrent command/tick.

## Verification
```bash
go test ./internal/game/world/... ./internal/game/server/... -run 'Worker|Concurren|Race|Ownership' -count=1 -race
go test ./... && git diff --check
```
