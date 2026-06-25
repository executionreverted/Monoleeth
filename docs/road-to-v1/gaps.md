# Road To v1 — Forced Gap Register

Use this file only for gaps we deliberately leave behind because they are outside
or unsafe for the current wave slice. Keep line/file references concrete.

## Forced Deferred Gaps

### P01 Loadout Move Ledger References — TASK-0479

- Runtime now uses `contentdb.LoadoutStore` when core DB is enabled, and
  equipped rows plus moved instance item locations reload after restart through
  `LoadoutService`. Equip/unequip runtime proof remains deferred outside the
  inventory/contentdb slice; TASK-0510 closed durable `MoveItem`/`RemoveItem`
  mutation references and row-update persistence in the inventory service/store.
  Ref: `internal/game/server/runtime.go`,
  `internal/game/server/loadout_runtime_adapters.go`,
  `internal/game/economy/inventory_move.go`.

## Pre-Wave Audit Findings Not Yet Accepted As Deferrals

These came from Symphony read-only audits and must be resolved during the owning
phase or moved above with a concrete deferral reason.

### P03 Realtime Hardening Audit — TASK-0457

- Closed by TASK-0459, TASK-0460, and TASK-0463. No accepted P03 deferral.

### P04 Rate Limiting Audit — TASK-0458

- Realtime gateway limiter hook fixed by TASK-0461.
- Auth login/register backoff and duplicate-register generic response fixed by TASK-0464.
- Auth attempt backoff is process-local for this slice; durable/cross-process attempt storage remains future P16/P02-style operational hardening unless P04 later adds it. Ref: `internal/game/auth/attempts.go`, `docs/road-to-v1/04-rate-limiting-abuse.md:28`.
- Realtime bucket runtime wiring fixed by TASK-0469; `NewRuntime` and
  concrete `server.New` gateways now install the process-local limiter by
  default, with tests covering replacement/disable seams and WebSocket
  `ERR_RATE_LIMITED` without mutation. No accepted P04 runtime-wiring deferral.
- Registered realtime op bucket coverage fixed by TASK-0474; every registered op
  exhausts an enforced default limiter bucket, auth login/register routes have
  direct backoff proof, and `chat.send`/`inventory.move` remain absent from the
  realtime operation registry. Ref: `internal/game/realtime/rate_limiter_test.go`,
  `internal/game/auth/http_test.go`, `internal/game/auth/service_test.go`,
  `docs/road-to-v1/04-rate-limiting-abuse.md:27`.

### P01 Persistence Foundation Audit — TASK-0462

- Loadout contentdb migration, adapter, active-ship reader composition, durable
  inventory instance lookup, and adapter-level restart proof are covered by
  TASK-0478. Runtime loadout wiring is covered by TASK-0479. Ref:
  `internal/game/contentdb/loadout_store.go`,
  `internal/game/contentdb/loadout_postgres_smoke_test.go`,
  `internal/game/server/runtime.go`.

### P01 Inventory Instance Durability Audit — TASK-0471

- Inventory instance rows are now durable via `contentdb` load/upsert and
  `InventoryService` boot hydration/AddItem persistence. Item-ledger rows,
  `AddItem` mutation-reference rows, and durable item/ledger counters are now
  covered by TASK-0475. Move/remove durable references and item row
  updates/deletes are covered by TASK-0510. Ref:
  `internal/game/economy/inventory_service.go`,
  `internal/game/contentdb/inventory_store.go`,
  `docs/road-to-v1/01-persistence-foundation.md:19`.
