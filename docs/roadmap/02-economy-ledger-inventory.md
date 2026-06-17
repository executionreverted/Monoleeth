# Phase 02: Economy Ledger And Inventory

## Status

- State: Not started
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

- [ ] Define item definition catalog fields.
- [ ] Define stackable item model.
- [ ] Define instance item model.
- [ ] Define item location model.
- [ ] Define wallet balance model.
- [ ] Define item ledger entry model.
- [ ] Define currency ledger entry model.
- [ ] Define reservation model for craft, market, and auction.
- [ ] Implement `AddItem`.
- [ ] Implement `MoveItem`.
- [ ] Implement `RemoveItem`.
- [ ] Implement `ReserveItems`.
- [ ] Implement `ReleaseReservation`.
- [ ] Implement `CommitReservation`.
- [ ] Implement `CreditWallet`.
- [ ] Implement `DebitWallet`.
- [ ] Implement `TransferCurrency`.
- [ ] Implement cargo capacity validation using server-side stat input.
- [ ] Implement item trade flag validation helpers.
- [ ] Implement premium bucket eligibility helper.
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

- [ ] Negative quantity is rejected.
- [ ] Zero quantity is rejected.
- [ ] Duplicate reference ID does not duplicate item grants.
- [ ] Duplicate reference ID does not duplicate currency grants.
- [ ] Debit fails when balance is insufficient.
- [ ] Debit writes a matching ledger entry.
- [ ] Credit writes a matching ledger entry.
- [ ] Transfer writes debit and credit ledger entries.
- [ ] Cargo capacity blocks over-capacity add.
- [ ] Concurrent cargo pickup simulation only allows capacity-safe result.
- [ ] Stack merge respects max stack.
- [ ] Instance item quantity cannot exceed 1.
- [ ] Escrow item cannot be moved by generic player move.
- [ ] Craft reserved item cannot be listed or equipped.
- [ ] Premium earned bucket cannot be used where paid premium is required.
- [ ] Transaction rollback does not leave ledger-only changes.

## Abuse And Safety Checks

- [ ] Negative amount exploit blocked.
- [ ] Duplicate reward exploit blocked.
- [ ] Escrow bypass blocked.
- [ ] Cargo capacity race blocked.
- [ ] Premium laundering blocked by bucket split.
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
