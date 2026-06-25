# Phase 02 — Transactional Economy & Outbox

## Status
- State: In progress
- Wave: 2
- Depends on: P01
- Unlocks: P07, P08, P13

## Goal
Make every value mutation transactional, idempotent, and broadcast-after-commit
using a durable outbox. Cover wallet, inventory, market, auction, premium.

## Why (report refs)
- Code review §1.4, §9, §11: multi-step settlement without durable transaction/outbox.
- Feature-gap §4.4, §6.4: in-memory economy, missing durable escrow/fanout.

## Scope
- Durable idempotency key table (domain keys, not just request ids).
- Durable outbox table + replay worker.
- Transactional market buy/cancel, auction bid/buy-now, premium claim/webhook.

## Out Of Scope
- New economy features; only durability/correctness of existing flows.

## Tasks
- [x] `[P:wave2/lane-A]` TASK-0481 foundation slice: add `idempotency_keys` + `outbox` contentdb schema skeleton and economy helper row contracts/tests.
- [x] `[P:wave2/lane-A]` TASK-0483 contentdb store adapter for economy idempotency/outbox row contracts.
- [ ] `[P:wave2/lane-A]` Add `idempotency_keys` table + helper; enforce on every mutating economy op.
- [ ] `[P:wave2/lane-A]` Add `outbox` table + after-commit publisher + replay worker.
- [ ] `[P:wave2/lane-B]` Wrap market buy/cancel in single DB transaction (escrow move + wallet + ledger).
- [ ] `[P:wave2/lane-B]` Wrap auction bid/buy-now in single transaction (refund-replace, close-once).
- [ ] `[P:wave2/lane-C]` Make premium claim + provider-event ingest idempotent and durable (replay-safe).
- [ ] `[P:wave2/lane-A]` Move loot XP reconciliation onto the durable outbox path (close `docs/todo.md` item).

## Progress Notes
- 2026-06-25 TASK-0486: contentdb outbox pending/failed due-load,
  guarded lease, publish, failure/dead transitions, and replay helper skeleton
  tests landed. Broad after-commit publisher/replay worker task remains open.
- 2026-06-25 TASK-0489: market buy now claims/completes economy idempotency
  rows, writes a market-buy outbox row, and restores in-memory wallet/inventory
  snapshots on tested mid-flow failure. Duplicate same-key buy and item-move
  failure rollback are covered. Full contentdb transaction across listing,
  wallet, inventory, idempotency, and outbox remains open; market cancel was not
  touched.
- 2026-06-25 TASK-0492: market cancel now claims/completes economy
  idempotency rows and restores the in-memory listing/inventory snapshot on the
  tested post-return failure path. Duplicate same-key cancel returns the cached
  result without moving escrow twice. Full contentdb transaction across listing,
  escrow, idempotency, and outbox remains open.
- 2026-06-25 TASK-0495: auction buy-now same-key concurrent retries now have a
  focused close-once test proving one debit, one current-bid refund, one grant,
  and one terminal lot close. Auction remains in-memory; full durable DB
  transaction plus outbox persistence is still open under the broader auction
  transaction task.

## Server Ownership
- Use canonical idempotency keys: `loot_pickup:<drop_id>`, `auction_close:<auction_id>`,
  `premium_webhook:<provider_event_id>`, etc. (AGENTS.md).
- Broadcast only after commit; on broadcast failure, clients reconcile via snapshot.

## Smoke Tests (one assertion each)
- [x] Duplicate market buy (same key) does not double-charge.
- [x] Market cancel returns escrow exactly once.
- [x] Concurrent auction buy-now closes the lot exactly once.
- [ ] Replayed premium webhook grants entitlement exactly once.
- [ ] Outbox replay re-delivers a missed economy event without duplicating state.
- [x] Failed mid-transaction leaves no partial wallet/inventory mutation.

## Done Criteria
- [ ] No economy mutation can double-apply under retry/concurrency.
- [ ] Every economy event publishes after commit via outbox.

## Verification
```bash
go test ./internal/game/economy/... ./internal/game/market/... ./internal/game/auction/... ./internal/game/premium/... -count=1 -race
go test ./... && git diff --check
```
