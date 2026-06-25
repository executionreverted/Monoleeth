# Phase 16 — Production Config & Operational Hardening

## Status
- State: Not started
- Wave: 2
- Depends on: P01 (env/mode), can start early
- Unlocks: safe production deploy

## Goal
Fail closed on unsafe production config, gate debug ops out of the production
protocol, make telemetry failures visible, and add structured operational logs
for critical state transitions.

## Why (report refs)
- Code review MD-02: secure cookie can be misconfigured silently.
- Code review MD-04: debug ops registered in main protocol.
- Code review MD-05 + §12: telemetry failures invisible; no structured state-transition logs.
- Code review §15.2: in-memory services could deploy as if durable.

## Scope
- `GAME_ENV=production` guard: require secure cookies + durable stores.
- Debug ops registered only in dev; startup warning/metric when dev mode on.
- Telemetry-error counters/logs.
- Structured logs for auth, transfer, settlement, CMS, death/repair, premium.

## Out Of Scope
- External dashboards/alerts (P13).

## Tasks
- [ ] `[P:wave2/lane-F]` Add `GAME_ENV` mode; production requires `CookieSecure=true` or fail startup.
- [ ] `[P:wave2/lane-F]` Production mode requires durable auth/economy/progression/world stores or fail startup.
- [ ] `[P:wave2/lane-G]` Register debug ops only in dev config; log/metric a warning when dev mode is enabled.
- [ ] `[P:wave2/lane-H]` Add telemetry-error counters: metric write errors, event encode errors, queue drops, slow-client disconnects, tick overruns.
- [ ] `[P:wave2/lane-H]` Add structured logs with `player_id/session_id/request_id/op/idempotency_key/ref_ids/result/error_code/duration_ms` for critical transitions (no secrets/tokens).

## Server Ownership
- Never log passwords, hashes, tokens, cookies, reset secrets (AGENTS.md).

## Smoke Tests (one assertion each)
- [ ] Production mode with insecure cookie fails startup.
- [ ] Production mode with an in-memory core store fails startup.
- [ ] Debug op is not registered in production protocol.
- [ ] A simulated telemetry write failure increments the telemetry-error counter.
- [ ] A market settlement emits one structured log with an idempotency key and no secrets.

## Done Criteria
- [ ] Unsafe production config cannot boot.
- [ ] Critical transitions are traceable; code review MD-02/MD-04/MD-05/§15.2 closed.

## Verification
```bash
go test ./internal/game/server/... ./internal/game/auth/... ./internal/game/observability/... -run 'Env|Cookie|Debug|Telemetry|StructuredLog|ProductionGuard' -count=1
go test ./... && git diff --check
```
