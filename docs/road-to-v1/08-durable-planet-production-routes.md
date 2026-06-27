# Phase 08 — Durable Planet, Production & Routes

## Status
- State: In progress (durable store adapters + migrations + smoke done; runtime wiring + recovery worker deferred)
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
- [x] `[P:wave3/lane-D]` Durable claim lifecycle Postgres adapter committed (claim_reference PK, plan JSON, idempotent replay, conflict rejection).
- [x] `[P:wave3/lane-D]` Production settlement + building mutation Postgres adapters committed (reference_key PK, plan JSON, idempotent replay, conflict rejection).
- [x] `[P:wave3/lane-D]` Route settlement covered by SettlementDurableStore (route ID/window lookup). Automation route durable Postgres adapter committed (route_id CAS + reference_key dedup, owner listing).
- [ ] `[P:wave3/lane-E]` Scheduled outbox publisher + recovery worker (not request-driven) for claim/production/route. **Deferred:** Postgres store reader interfaces are ready; the runtime tick-driven durable outbox drain wiring is runtime-level work.
- [ ] `[P:wave3/lane-E]` Replace process-local idempotency maps with DB-backed keys. **Deferred:** The DB adapters enforce dedup via reference_key uniqueness; runtime still wires in-memory stores by default.

### Completed Store Adapters (2026-06-26)
- `contentdb.ClaimDurableLifecycleStore` — `discovery.ClaimDurableLifecycleStore` + `Reader`
- `contentdb.BuildingMutationDurableStore` — `production.BuildingMutationDurableCommitStore` + `Reader`
- `contentdb.SettlementDurableStore` — `production.SettlementDurableCommitStore` + `Reader` (planet + route window lookups)
- `contentdb.AutomationRouteDurableStore` — `production.AutomationRouteDurableCommitStore` + `Reader` (CAS revision, dedup log, owner listing)
- Migrations: 0019, 0020, 0021
- Foundation fix: `Quantity.UnmarshalJSON` + `Money.UnmarshalJSON` (JSON round-trip was broken)

## Server Ownership
- Ownership, range, visibility, X Core consume, storage caps stay server-owned.
- Reuse canonical keys: `offline_settlement:<planet_id>:<window>`, `auction_close:*` pattern style.

## Smoke Tests (one assertion each)
- [x] Claim lifecycle persists + exact replay is idempotent + conflict rejected (Postgres).
- [x] Building mutation persists + exact replay is idempotent + conflict rejected (Postgres).
- [x] Settlement persists + exact replay is idempotent + conflict rejected (Postgres).
- [x] Automation route persists + duplicate reference idempotent + revision CAS enforced + owner listing (Postgres).
- [ ] Owned planet/production/route state survives restart via runtime wiring. **Deferred:** stores are ready; runtime still uses in-memory default.

## Done Criteria
- [ ] No claim/production/route double-apply across restart/concurrency.
- [ ] `docs/todo.md` durable claim/production/route items closed.

## Verification
```bash
go test ./internal/game/discovery/... ./internal/game/production/... ./internal/game/server/... -run 'Claim|Settle|Route|Durable|Recovery' -count=1 -race
go test ./... && git diff --check
```
