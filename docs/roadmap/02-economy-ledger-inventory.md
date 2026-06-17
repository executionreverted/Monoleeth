# Phase 02: Economy Ledger And Inventory

## Status

- State: Complete, audited 2026-06-17
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
- [x] Implement `ReserveItems`.
- [x] Implement `ReleaseReservation`.
- [x] Implement `CommitReservation`.
- [x] Implement `CreditWallet`.
- [x] Implement `DebitWallet`.
- [x] Implement `TransferCurrency`.
- [x] Implement cargo capacity validation using server-side stat input.
- [x] Implement item trade flag validation helpers.
- [x] Implement premium bucket eligibility helper.
- [x] Implement ledger reference uniqueness for idempotent operations. Verified 2026-06-17 by operation-scoped inventory, wallet, cargo, and reservation idempotency tests.
- [x] Emit inventory, cargo, wallet, and ledger events after mutation. Verified 2026-06-17 by EventRecorder coverage for inventory add/move/remove, cargo add, wallet credit/debit/transfer, reservation-backed item moves, validation failures, and idempotent duplicate no-emission.

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
- [x] Duplicate reference ID does not duplicate currency grants.
- [x] Debit fails when balance is insufficient.
- [x] Debit writes a matching ledger entry.
- [x] Credit writes a matching ledger entry.
- [x] Transfer writes debit and credit ledger entries.
- [x] Cargo capacity blocks over-capacity add.
- [x] Concurrent cargo pickup simulation only allows capacity-safe result.
- [x] Stack merge respects max stack.
- [x] Instance item quantity cannot exceed 1.
- [x] Escrow item cannot be moved by generic player move.
- [x] Duplicate reference ID does not duplicate item removals.
- [x] RemoveItem writes one decrease ledger entry with source balance.
- [x] RemoveItem insufficient quantity fails without mutation.
- [x] Escrow, reserved, and system items cannot be removed by generic player remove.
- [x] Craft reserved item cannot be listed or equipped by policy helper.
- [x] Premium earned bucket cannot be used where paid premium is required by policy helper.
- [x] Transaction rollback does not leave ledger-only changes. Verified 2026-06-17 by release/commit failure tests plus Phase 02 rollback snapshot audit coverage for reservation mutations, ledger rows, and move references.

## Abuse And Safety Checks

- [x] Negative amount exploit blocked. Verified 2026-06-17 across wallet, cargo, inventory add/move/remove, ledger primitives, and reservation requirements.
- [x] Duplicate reward exploit blocked.
- [x] Escrow bypass blocked. Verified 2026-06-17 by generic move/remove source blockers and player trade/equip policy helper coverage for escrow, reserved, system, and equipped items.
- [x] Generic RemoveItem cannot bypass escrow, reserved, or system source locations.
- [x] Player trade/equip policy helper blocks equipped, escrow, reserved, and system locations.
- [x] Cargo capacity race blocked.
- [x] Premium laundering blocked by bucket split. Verified 2026-06-17 by paid, earned, and market-acquired premium bucket model and eligibility policy tests.
- [x] Paid-only premium policy helper rejects earned premium and handles market-acquired premium explicitly.
- [x] Currency overflow handled or rejected. Verified 2026-06-17 by Phase 02 CreditWallet and TransferCurrency overflow rejection tests.

## Done Criteria

- [x] Inventory, cargo, wallet, and ledger services exist. Verified 2026-06-17 by direct code audit of `InventoryService`, `CargoService`, `WalletService`, and ledger models/tests.
- [x] All value movements require reason and reference ID. Verified 2026-06-17 by service input validation, ledger validation, and reservation-derived release/commit references.
- [x] All value movements write ledger entries. Verified 2026-06-17 by add/move/remove, wallet credit/debit/transfer, cargo add, and reservation reserve/release/commit ledger tests.
- [x] Reservation flow is available for craft and market phases. Verified 2026-06-17 by reservation kind/location tests for craft, market, and auction plus market/auction commit behavior.
- [x] `go test ./...` passes. Verified 2026-06-17 with `GOCACHE=/private/tmp/TASK-0027-go-build go test ./...`.
- [x] `git diff --check` passes. Verified 2026-06-17.

## Resume Notes

If resuming here, first inspect whether any later system directly edits item or wallet state. If yes, stop and route it through these services before continuing.

Current Symphony wave plan:

- `docs/plans/2026-06-17-phase-02-economy-symphony-wave.md`
