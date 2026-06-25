# Road To v1 — Forced Gap Register

Use this file only for gaps we deliberately leave behind because they are outside
or unsafe for the current wave slice. Keep line/file references concrete.

## Forced Deferred Gaps

None yet. Current P01 work leaves unfinished P01 checklist items unchecked rather
than accepted as deferrals.

## Pre-Wave Audit Findings Not Yet Accepted As Deferrals

These came from Symphony read-only audits and must be resolved during the owning
phase or moved above with a concrete deferral reason.

### P03 Realtime Hardening Audit — TASK-0457

- Closed by TASK-0459, TASK-0460, and TASK-0463. No accepted P03 deferral.

### P04 Rate Limiting Audit — TASK-0458

- Realtime gateway limiter hook fixed by TASK-0461; remaining P04 audit items below.
- Auth login/register backoff and duplicate-register generic response fixed by TASK-0464.
- Auth attempt backoff is process-local for this slice; durable/cross-process attempt storage remains future P16/P02-style operational hardening unless P04 later adds it. Ref: `internal/game/auth/attempts.go`, `docs/road-to-v1/04-rate-limiting-abuse.md:28`.
- Realtime bucket runtime wiring fixed by TASK-0469; `NewRuntime` and
  concrete `server.New` gateways now install the process-local limiter by
  default, with tests covering replacement/disable seams and WebSocket
  `ERR_RATE_LIMITED` without mutation. No accepted P04 runtime-wiring deferral.

### P01 Persistence Foundation Audit — TASK-0462

- Loadout durability still needs DB adapters and durable instance-item rows; hangar
  `contentdb` migration/adapter exists but is not runtime-wired in this slice. Ref:
  `internal/game/ships/store.go`, `internal/game/modules/loadout_store.go`,
  `docs/road-to-v1/01-persistence-foundation.md:29`.
- Durable loadout/equipped-module restart proof depends on durable instance-item
  rows, not stackable-only inventory rows. Ref:
  `internal/game/economy/inventory_service.go`, `docs/road-to-v1/01-persistence-foundation.md:19`.

### P01 Inventory Instance Durability Audit — TASK-0471

- Inventory instance rows, item-ledger rows, mutation-reference rows, and durable ID counters remain required before loadout/equipped modules can prove restart durability. Ref: `internal/game/economy/inventory_service.go`, `internal/game/contentdb/inventory_store.go`, `docs/road-to-v1/01-persistence-foundation.md:19`.
