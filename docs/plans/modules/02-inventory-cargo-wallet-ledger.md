# Inventory, Cargo, Wallet, And Ledger

Date: 2026-06-17

## Purpose

Bu modül oyundaki item ve currency hareketlerinin güvenli temelidir.

Kapsam:

- Account inventory
- Ship cargo
- Planet storage references
- Item instances
- Stackable materials
- Wallet balances
- Paid/free premium split
- Item ledger
- Currency ledger

Bu modül sağlam değilse market, loot, craft, death, production ve premium sistemleri kırılır.

## Owns

```text
InventoryService
CargoService
WalletService
TransactionLedgerService
```

## Does Not Own

- Loot drop roll
- Craft recipe validation
- Market price rules
- Death drop percent
- Production formula
- Premium purchase provider integration

Bu modül transfer primitive'lerini sağlar.

## Storage Types

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

Her item instance veya stack bir location'da durur.

## Item Model

İki item tipi var:

```text
stackable item
instance item
```

Stackable:

- raw materials
- processed materials
- consumables
- fragments

Instance:

- modules with durability
- coordinate scrolls
- cosmetics if unique
- special quest items

Item definition:

```text
item_id
name
item_type
rarity
max_stack
weight
trade_flags
bind_rules
metadata_schema
```

Item instance:

```text
item_instance_id
item_id
owner_player_id
location_type
location_id
quantity
durability_current
bound_state
metadata_json
created_at
updated_at
```

## Currency Model

Wallet types:

```text
credits
premium_paid
premium_earned
premium_market_acquired
event_token
reputation_token
```

Premium split'in nedeni:

- paid premium tradeable olabilir
- earned premium non-tradeable olabilir
- fraud/chargeback durumunda paid kaynak takip edilir

## Commands

```text
AddItem(player_id, item_id, quantity, location, reason, reference_id)
MoveItem(player_id, item_ref, from, to, quantity, reason, reference_id)
RemoveItem(player_id, item_ref, quantity, reason, reference_id)
ReserveItems(player_id, requirements, location, reason, reference_id)
ReleaseReservation(reservation_id)
CommitReservation(reservation_id)

CreditWallet(player_id, currency, amount, reason, reference_id)
DebitWallet(player_id, currency, amount, reason, reference_id)
TransferCurrency(from_player, to_player, currency, amount, reason, reference_id)
```

## Core Rule: Ledger First-Class

Her item/currency hareketi ledger'a yazılır.

Ledger:

```text
ledger_id
player_id
asset_type
asset_id
delta
balance_after
location_type
reason
reference_id
created_at
```

Not:

- Currency için `balance_after` kolaydır.
- Item için stack/location bazlı balance karmaşık olabilir; yine de delta log şarttır.

## Transaction Pattern

Örnek wallet debit:

```go
func DebitWallet(ctx context.Context, player PlayerID, currency Currency, amount int64, reason Reason, ref string) error {
	return db.Tx(ctx, func(tx Tx) error {
		wallet := tx.Wallets().Lock(player, currency)
		if wallet.Balance < amount {
			return ErrInsufficientFunds
		}
		wallet.Balance -= amount
		tx.Wallets().Save(wallet)
		tx.Ledger().InsertCurrency(player, currency, -amount, wallet.Balance, reason, ref)
		return nil
	})
}
```

Önemli:

```text
Lock row
Validate
Mutate
Ledger
Commit
Emit event
```

Event commit'ten sonra.

## Cargo Capacity

Cargo capacity active ship statından gelir.

```go
func CanFitCargo(current CargoState, add ItemStack, stats ShipStats) bool {
	return current.UsedUnits+add.WeightUnits() <= stats.CargoCapacity
}
```

Cargo add sırasında:

- active ship alınır
- stat snapshot okunur
- item weight hesaplanır
- capacity validate edilir
- item cargo location'a eklenir

## Account Inventory Capacity

Account inventory için MVP'de capacity soft olabilir.

Ancak planet/storage sistemi için capacity gerçek olmalı.

Öneri:

- Account inventory: başlangıçta yüksek veya sınırsız
- Ship cargo: kesin limit
- Planet storage: kesin limit
- Route destination storage: kesin limit

## Reservation System

Craft, market, auction gibi sistemlerde item/currency reserve gerekir.

Örnek craft:

```text
Reserve materials -> crafting_reserved
Craft completes -> system consumes reservation, creates output
Craft fails/cancel -> release reservation
```

Market listing:

```text
Move item -> market_escrow
Sale -> buyer inventory, seller wallet
Expire/cancel -> seller inventory
```

Auction bid:

```text
Debit/escrow bid amount
Outbid -> refund previous bidder
Win -> system keeps amount, grants lot
```

## Idempotency

Ekonomi command'ları idempotent olmalı.

Unique key:

```text
player_id + operation_type + reference_id
```

Örnek:

```go
if ledger.ExistsReference(ref, "quest_reward_claim") {
	return nil
}
```

## Events Emitted

```text
inventory.item_added
inventory.item_removed
inventory.item_moved
cargo.updated
wallet.credited
wallet.debited
ledger.entry_created
storage.capacity_changed
```

## Edge Cases

- Stack merge sırasında max stack aşılmamalı.
- Instance item quantity her zaman 1 olmalı.
- Durability metadata stackable item'a yazılmamalı.
- Market escrow'daki item oyuncu tarafından equip edilememeli.
- Craft reserved item markete konamamalı.
- Cargo'da olan item account inventory gibi güvenli sayılmamalı.
- Disconnect sırasında pending transaction yarım kalmamalı.
- Negative amount hiçbir command'da kabul edilmemeli.

## Abuse Vectors

### Negative Amount Exploit

Risk:

- Client `amount = -100` gönderip debit'i credit'e çevirmeye çalışır.

Defense:

- All external amount must be positive integer.
- Use unsigned or explicit validation.
- Never multiply client amount into delta without sign control.

### Duplicate Reward

Risk:

- Retry/reconnect ile aynı reward tekrar alınır.

Defense:

- idempotency key
- ledger reference unique
- reward claimed state

### Escrow Bypass

Risk:

- Market item listed iken oyuncu aynı item'ı başka yerde kullanmaya çalışır.

Defense:

- Location-based ownership
- Only items in account/cargo allowed for specific actions
- Escrow location blocks equip/move except by owning system

### Cargo Capacity Race

Risk:

- Aynı anda iki pickup request capacity check'i geçer.

Defense:

- Lock cargo rows or player cargo aggregate
- Recompute used capacity inside transaction

### Premium Laundering

Risk:

- Free premium markette trade edilmeye çalışılır.

Defense:

- Currency bucket split
- Market validates eligible bucket
- Ledger reason/source retained

## Testing Checklist

- Negative quantity reject ediliyor mu?
- Duplicate reference id duplicate item/currency vermiyor mu?
- Cargo capacity concurrent pickup test var mı?
- Market escrow item equip edilemiyor mu?
- Auction refund ledger doğru mu?
- Premium paid/free bucket ayrımı korunuyor mu?
- Transaction rollback ledger'ı da rollback ediyor mu?
- Currency overflow testleri var mı?

## Implementation Notes

İlk sürümde basit tut:

- Stackable materials
- Instance modules
- Credits
- premium_paid / premium_earned
- Ship cargo capacity
- Market escrow
- Crafting reserved
- Ledger reference uniqueness

Bu modül bitmeden market/craft/death kodlamaya geçmemek en sağlıklısı.

