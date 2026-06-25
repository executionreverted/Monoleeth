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
- WebSocket write path writes inline under `client.mu`, so slow clients can stall
  event delivery until write timeout. Ref: `internal/game/server/transport.go`,
  `docs/road-to-v1/03-realtime-hardening.md:26`.

### P04 Rate Limiting Audit — TASK-0458

- `RateLimitPosture` is metadata only; gateway has no limiter hook before command
  handler execution. Ref: `internal/game/realtime/envelope.go`,
  `internal/game/realtime/gateway.go`, `docs/road-to-v1/04-rate-limiting-abuse.md:26`.
- Auth login/register route specs expose rate-limit metadata but no throttle,
  backoff, or lockout. Ref: `internal/game/auth/http.go`,
  `docs/road-to-v1/04-rate-limiting-abuse.md:28`.
- Register duplicate-email response remains an existence leak candidate. Ref:
  `internal/game/auth/service.go`, `docs/road-to-v1/04-rate-limiting-abuse.md:39`.
