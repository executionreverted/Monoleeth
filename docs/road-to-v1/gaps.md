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

- Request replay mismatch fixed by TASK-0459; remaining P03 audit items below.
- Reconnect has cursor evidence but no bounded replay ring yet. Ref:
  `internal/game/server/runtime_sessions.go`, `docs/road-to-v1/03-realtime-hardening.md:28`.
- WebSocket writer queue fixed by TASK-0460; remaining P03 audit item below.

### P04 Rate Limiting Audit — TASK-0458

- Realtime gateway limiter hook fixed by TASK-0461; remaining P04 audit items below.
- Auth login/register route specs expose rate-limit metadata but no throttle,
  backoff, or lockout. Ref: `internal/game/auth/http.go`,
  `docs/road-to-v1/04-rate-limiting-abuse.md:28`.
- Register duplicate-email response remains an existence leak candidate. Ref:
  `internal/game/auth/service.go`, `docs/road-to-v1/04-rate-limiting-abuse.md:39`.

### P01 Persistence Foundation Audit — TASK-0462

- Hangar/loadout durability still needs repository interfaces and DB adapters. Ref:
  `internal/game/ships/store.go`, `internal/game/modules/loadout_store.go`,
  `docs/road-to-v1/01-persistence-foundation.md:29`.
- Durable loadout/equipped-module restart proof depends on durable instance-item
  rows, not stackable-only inventory rows. Ref:
  `internal/game/economy/inventory_service.go`, `docs/road-to-v1/01-persistence-foundation.md:19`.
