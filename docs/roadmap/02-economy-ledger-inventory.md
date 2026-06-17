# Phase 02: Economy Ledger And Inventory

## Status

- State: In progress
- Owner: Economy foundation
- Depends on: Phase 01
- Unlocks: loot, craft, market, auction, death, premium, production

## Goal

Build safe item, cargo, wallet, and ledger primitives so every value mutation is transactional, auditable, idempotent, and testable.

## Why This Comes Before Combat

Combat creates rewards. Loot pickup moves items. Death drops cargo. Craft consumes materials. Market escrow moves assets. All of these become exploit-prone if inventory and ledger primitives are weak.

## Source Specs

Read before implementation:

- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `InventoryService`
- `CargoService`
- `WalletService`
- `TransactionLedgerService`

Does not own:

- loot table rolls
- craft recipe validation
- market price rules
- death cargo drop percent
- production formula
- premium provider integration

## Package Direction

Suggested packages:

```text
internal/game/economy/
internal/game/economy/inventory/
internal/game/economy/wallet/
internal/game/economy/ledger/
```

In early MVP, use in-memory repositories with transaction-like locks or explicit test transaction objects. Later persistence can map the same service contracts to PostgreSQL.

## Core Data To Model

- item definitions
- stackable item state
- instance item state
- item locations
- cargo capacity
- wallet balances
- currency buckets
- item ledger entries
- currency ledger entries
- reservation state

Storage location types:

```text
account_inventory
ship_cargo
planet_storage
station_storage
market_escrow
auction_escrow
crafting_reserved
system_sink
world_drop
```

Currency buckets:

```text
credits
premium_paid
premium_earned
premium_market_acquired
event_token
reputation_token
```

## TODO

- [x] Define item definition catalog fields.
- [x] Define stackable item model.
- [x] Define instance item model.
- [x] Define item location model.
- [x] Define wallet balance model.
- [x] Define item ledger entry model.
- [x] Define currency ledger entry model.
- [x] Define reservation model for craft, market, and auction.
- [x] Implement `AddItem`.
- [x] Implement `MoveItem`.
- [x] Implement `RemoveItem`.
- [ ] Implement `ReserveItems`.
- [ ] Implement `ReleaseReservation`.
- [ ] Implement `CommitReservation`.
- [ ] Implement `CreditWallet`.
- [ ] Implement `DebitWallet`.
- [ ] Implement `TransferCurrency`.
- [ ] Implement cargo capacity validation using server-side stat input.
- [x] Implement item trade flag validation helpers.
- [x] Implement premium bucket eligibility helper.
- [ ] Implement ledger reference uniqueness for idempotent operations.
- [ ] Emit inventory, cargo, wallet, and ledger events after mutation.

## Transaction Rules

Every state-changing service method should follow this shape:

```text
lock
validate positive amount and ownership
validate location and capacity
mutate balances/items
write ledger entry with reason and reference_id
commit
emit event after commit
```

For in-memory MVP tests, still model the transaction boundary explicitly.

## Tests

- [x] Negative quantity is rejected.
- [x] Zero quantity is rejected.
- [x] Duplicate reference ID does not duplicate item grants.
- [ ] Duplicate reference ID does not duplicate currency grants.
- [ ] Debit fails when balance is insufficient.
- [ ] Debit writes a matching ledger entry.
- [ ] Credit writes a matching ledger entry.
- [ ] Transfer writes debit and credit ledger entries.
- [ ] Cargo capacity blocks over-capacity add.
- [ ] Concurrent cargo pickup simulation only allows capacity-safe result.
- [x] Stack merge respects max stack.
- [x] Instance item quantity cannot exceed 1.
- [x] Escrow item cannot be moved by generic player move.
- [x] Duplicate reference ID does not duplicate item removals.
- [x] RemoveItem writes one decrease ledger entry with source balance.
- [x] RemoveItem insufficient quantity fails without mutation.
- [x] Escrow, reserved, and system items cannot be removed by generic player remove.
- [x] Craft reserved item cannot be listed or equipped by policy helper.
- [x] Premium earned bucket cannot be used where paid premium is required by policy helper.
- [ ] Transaction rollback does not leave ledger-only changes.

## Abuse And Safety Checks

- [ ] Negative amount exploit blocked.
- [ ] Duplicate reward exploit blocked.
- [ ] Escrow bypass blocked.
- [x] Generic RemoveItem cannot bypass escrow, reserved, or system source locations.
- [x] Player trade/equip policy helper blocks equipped, escrow, reserved, and system locations.
- [ ] Cargo capacity race blocked.
- [ ] Premium laundering blocked by bucket split.
- [x] Paid-only premium policy helper rejects earned premium and handles market-acquired premium explicitly.
- [ ] Currency overflow handled or rejected.

## Done Criteria

- [ ] Inventory, cargo, wallet, and ledger services exist.
- [ ] All value movements require reason and reference ID.
- [ ] All value movements write ledger entries.
- [ ] Reservation flow is available for craft and market phases.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, first inspect whether any later system directly edits item or wallet state. If yes, stop and route it through these services before continuing.

Current Symphony wave plan:

- `docs/plans/2026-06-17-phase-02-economy-symphony-wave.md`
