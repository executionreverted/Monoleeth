# Phase 06 — Movement, Combat & Death Correctness

## Status
- State: In progress
- Wave: 2
- Depends on: none (integrates better after P01)
- Unlocks: P11

## Goal
Close the remaining MVP correctness gaps in movement settling, death/disabled
events, and repair, so the combat loop is durable and UX-complete.

## Why (report refs)
- Code review §5: movement stop/interact can use stale positions.
- Feature-gap §4.3: open Phase 05 items (death event mapper, repair quote/debit tests).

## Scope
- Server settle movement to "now" on stop/interact/disconnect.
- Death/disabled ship event mapper to client-safe payloads.
- Repair: stale-quote rejection, wallet debit via ledger, re-enable.

## Out Of Scope
- Ammo/rockets/new weapons (P12).

## Tasks
- [ ] `[P:wave2/lane-E]` Add worker helper to settle position to current server time on stop/detach/interact.
- [ ] `[P:wave2/lane-E]` Add `death.ship_disabled` event mapper (server-safe fields only).
- [ ] `[P:wave2/lane-E]` Make repair quote server-time bound; reject stale/tampered quotes.
- [ ] `[P:wave2/lane-E]` Debit repair via wallet ledger; re-enable ship after commit.
- [ ] `[P:wave2/lane-E]` Disable combat buttons from server death state in client.

## Server Ownership
- Position, hit, damage, cooldown, repair price, ship state are server-owned.

## Smoke Tests (one assertion each)
- [x] Stop during movement settles to server-computed position, not last click target.
- [x] Disconnect during movement settles position (no stale active route on reconnect).
- [ ] NPC/PvP death emits a client-safe disabled event.
- [ ] Stale/tampered repair quote is rejected.
- [ ] Repair with insufficient wallet leaves ship disabled.
- [ ] Successful repair debits wallet once and re-enables the ship.

## Done Criteria
- [ ] Phase 05 open death/repair checklist items closed.
- [ ] No stale-position desync on stop/disconnect.

## Verification
```bash
go test ./internal/game/death/... ./internal/game/world/... ./internal/game/server/... -run 'Death|Repair|Move|Settle' -count=1
go test ./... && git diff --check
```
