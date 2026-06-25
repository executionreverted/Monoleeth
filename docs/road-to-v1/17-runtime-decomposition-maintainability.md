# Phase 17 — Runtime Decomposition & Maintainability

## Status
- State: Not started
- Wave: 6 (incremental / continuous)
- Depends on: P02, P05 (do not refactor before behavior is durable)
- Unlocks: future multi-service split, lower change risk

## Goal
Split the monolithic `Runtime` into narrower coordinators so changes are safer and
broad locking is discouraged. Do this incrementally, not as one big rewrite.

## Why (report refs)
- Code review §15: `Runtime` owns everything; encourages broad locks and risky changes.
- AGENTS.md: small files, clear ownership, no monolith.

## Scope
- Extract coordinators with clear ownership; top-level `Runtime` composes them.
- Keep behavior identical per extraction step (refactor, not feature change).

## Out Of Scope
- Multi-process services (post-v1); behavior changes (other phases own those).

## Tasks (each is an independent, behavior-preserving extraction)
- [ ] `[P:wave6/lane-A]` Extract `SessionRuntime` (session attach/detach/resolve).
- [ ] `[P:wave6/lane-B]` Extract `WorldRuntime`/`MapRuntime` (map instances, tick, AOI handoff).
- [ ] `[P:wave6/lane-C]` Extract `EconomyRuntime` (wallet/inventory/market/auction/premium coordination).
- [ ] `[P:wave6/lane-D]` Extract `DiscoveryRuntime` + `ProductionRuntime`.
- [ ] `[P:wave6/lane-E]` Extract `AdminContentRuntime` + `RealtimeEventBus`.
- [ ] `[P:wave6/lane-*]` Split oversized files toward the 300-500 line soft rule.

## Server Ownership
- Authority/ownership rules unchanged; this phase only moves code, not truth.

## Smoke Tests (one assertion each)
- [ ] Existing runtime tests pass unchanged after `SessionRuntime` extraction.
- [ ] Existing world/AOI tests pass unchanged after `WorldRuntime` extraction.
- [ ] Existing economy tests pass unchanged after `EconomyRuntime` extraction.
- [ ] No public protocol/op behavior changes (golden response test).

## Done Criteria
- [ ] `Runtime` composes coordinators instead of owning all mutation logic.
- [ ] No behavior regressions; files trend toward the size soft rule.

## Verification
```bash
go test ./internal/game/server/... -count=1
go test ./... -race && git diff --check
```
