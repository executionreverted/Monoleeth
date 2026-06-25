# Phase 04 — Rate Limiting & Abuse Posture

## Status
- State: In progress
- Wave: 1
- Depends on: none
- Unlocks: safe public playtest

## Goal
Turn the metadata-only `RateLimitPosture` into enforced limits, plus login abuse
protection and broader hidden-data leak canaries.

## Why (report refs)
- Code review §10: rate limiting is documented, not enforced.
- Feature-gap §14: bots/abuse are a known DarkOrbit-class risk.

## Scope
- Enforced token-bucket limits per account/session/IP per op class.
- Login/register slowdown + lockout heuristics.
- Reject-without-mutation guarantee on throttle.

## Out Of Scope
- Full ML bot detection (later); start with telemetry + cadence checks.

## Tasks
- [x] `[P:wave1/lane-D]` Add rate-limit middleware in the gateway keyed by op `RateLimitPosture` class.
- [ ] `[P:wave1/lane-D]` Define buckets for `auth.login`, `combat.use_skill`, `loot.pickup`, `scan.pulse`, `market.search`, `chat.send` (when added), `quest.reroll`, `inventory.move`.
- [x] `[P:wave1/lane-D]` Add login/register failure backoff + temporary lockout (no user-existence leak).
- [x] `[P:wave1/lane-D]` Guarantee throttled requests perform zero mutation.
- [x] `[P:wave1/lane-E]` Expand leak canaries to admin/debug/CMS-projection/log surfaces.

## Server Ownership
- Rate limits are abuse protection, not gameplay cooldowns; never alter gameplay truth (AGENTS.md).
- TASK-0466 added process-local realtime token buckets keyed by
  session/player/op/posture for registered realtime ops. Concrete buckets exist
  for `combat.use_skill`, `loot.pickup`, `scan.pulse`, `market.search`, and
  `quest.reroll`; `inventory.move` has a predeclared bucket but remains absent
  from the realtime operation registry in this slice, so it has no active
  gateway traffic yet. `chat.send` is not registered yet, so it remains
  documented absent.
- Realtime `ERR_RATE_LIMITED` responses are retryable and are not stored in the
  request cache; retry after refill can execute the same request id.

## Smoke Tests (one assertion each)
- [x] Burst over limit on one op returns throttle error.
- [x] Throttled mutation op leaves state unchanged.
- [x] Repeated failed logins trigger backoff/lockout.
- [x] Throttle errors do not reveal whether an email exists.
- [x] Leak canary finds no hidden seed/internal id in admin/debug responses.

## Done Criteria
- [ ] Every registered op has an enforced limit.
- [ ] Throttling never partially mutates state.

## Verification
```bash
go test ./internal/game/realtime/... ./internal/game/auth/... ./internal/game/server/... -run 'RateLimit|Throttle|LoginAbuse|Leak' -count=1
go test ./... && git diff --check
```
