# Phase 08 — Durable Planet, Production & Routes

## Status
- State: Not started
- Wave: 3
- Depends on: P01, P02
- Unlocks: clan outposts (post-v1), durable strategy layer

## Goal
Replace process-local claim/production/route stores and outbox scaffolding with
durable DB rows, real cross-process idempotency, and scheduled recovery workers.

## Why (report refs)
- Code review §8/§9; feature-gap §4.7, §6.3: durable DB rows/locks/windows are open.
- `docs/todo.md`: many Phase 07/08/09 in-memory durability items still open.

## Scope
- Durable claim lifecycle, production settlement, route settlement rows.
- Settlement windows enforced by DB/idempotency rows.
- Scheduled publisher/recovery workers reading committed rows.

## Out Of Scope
- Wormholes/outposts/new strategy features (post-v1).

## Tasks
- [ ] `[P:wave3/lane-D]` Add durable claim lifecycle + X Core consume coupled in one transaction/CAS.
- [ ] `[P:wave3/lane-D]` Persist production settlement windows as durable idempotent rows.
- [ ] `[P:wave3/lane-D]` Persist route settlement windows + storage ledger durably.
- [ ] `[P:wave3/lane-E]` Add scheduled outbox publisher + recovery worker (not request-driven) for claim/production/route.
- [ ] `[P:wave3/lane-E]` Replace process-local idempotency maps with DB-backed keys.

## Server Ownership
- Ownership, range, visibility, X Core consume, storage caps stay server-owned.
- Reuse canonical keys: `offline_settlement:<planet_id>:<window>`, `auction_close:*` pattern style.

## Smoke Tests (one assertion each)
- [ ] Claim consumes exactly one X Core under duplicate retry across restart.
- [ ] Production settlement for one window applies exactly once.
- [ ] Route settlement for one window transfers storage exactly once.
- [ ] Scheduled recovery worker republishes a missed claim event without duplicating ownership.
- [ ] Owned planet/production/route state survives restart.

## Done Criteria
- [ ] No claim/production/route double-apply across restart/concurrency.
- [ ] `docs/todo.md` durable claim/production/route items closed.

## Verification
```bash
go test ./internal/game/discovery/... ./internal/game/production/... ./internal/game/server/... -run 'Claim|Settle|Route|Durable|Recovery' -count=1 -race
go test ./... && git diff --check
```
