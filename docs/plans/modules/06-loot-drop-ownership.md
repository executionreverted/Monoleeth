# Loot And Drop Ownership

Date: 2026-06-17

## Purpose

Bu modül world drop entity'lerini, drop ownership window'larını, pickup validation'ı ve loot XP hook'larını yönetir.

Loot sistemi crafting ekonomisini bozmamalı. Normal drop çoğunlukla hammadde ve fragment üretmeli.

## Owns

```text
LootService
DropOwnershipService
```

## Does Not Own

- Combat kill validation
- Item inventory primitive
- Cargo capacity primitive
- XP table
- Market value

## Drop Philosophy

Drop:

- Herkes tarafından görülebilir, eğer AOI/fog içinde ise.
- İlk X saniye owner-locked olur.
- Sonra public olur.
- Süresi dolunca silinir.

State:

```text
owner_locked -> public -> expired
```

## Drop Data

```text
drop_id
world_id
zone_id
position_x
position_y
item_id
quantity
owner_player_id
owner_lock_until
public_until
expires_at
source_type
source_id
created_at
```

source_type:

```text
npc_kill
player_death
gather_node
event_cache
system_spawn
admin
```

## Drop Creation

NPC death:

```text
CombatService emits combat.npc_killed
LootService rolls loot table
LootService creates world_loot_drops
AOI broadcasts drop_created to visible players
```

Player death:

```text
DeathService calculates cargo drop items
LootService creates drops at death position
owner may be nil or killer depending PvP rules
```

## Loot Table v0

MVP drop types:

- Common raw material
- Uncommon raw material
- Rare raw material
- X Core Fragment, very rare
- Consumable, rare
- Coordinate/intel item, very rare

Avoid:

- Frequent finished modules
- Frequent finished ships
- Direct premium currency except special quest/event

## Roll Logic

Server-only roll:

```go
func RollLoot(rng RNG, table LootTable, context LootContext) []LootItem {
	items := make([]LootItem, 0)
	for _, row := range table.Rows {
		chance := row.Chance
		chance *= context.ZoneDropMultiplier
		chance *= context.EventMultiplier
		if rng.Float64() <= chance {
			qty := rng.Int(row.MinQty, row.MaxQty)
			items = append(items, LootItem{ItemID: row.ItemID, Quantity: qty})
		}
	}
	return items
}
```

RNG:

- server-side
- source event id ile trace edilebilir
- client seed gönderemez

## Ownership Window

Example tuning:

```text
owner_lock_duration = 60s
public_duration = 120s
total_lifetime = 180s
```

Pickup permission:

```go
func CanPickupDrop(now time.Time, player PlayerID, drop Drop) error {
	if now.After(drop.ExpiresAt) {
		return ErrDropExpired
	}
	if now.Before(drop.OwnerLockUntil) && drop.OwnerPlayerID != player {
		return ErrDropOwnerLocked
	}
	return nil
}
```

Party rules later:

```text
owner_player_id can become owner_party_id
```

## Pickup Validation

```go
func PickupDrop(ctx context.Context, player PlayerID, dropID string) error {
	return db.Tx(ctx, func(tx Tx) error {
		drop := tx.Drops().Lock(dropID)
		if drop == nil {
			return ErrDropNotFound
		}
		if err := CanPickupDrop(time.Now(), player, *drop); err != nil {
			return err
		}
		if !world.DistanceOK(player, drop.Position, PickupRange) {
			return ErrTooFar
		}
		if !visibility.CanSeeDrop(player, dropID) {
			return ErrNotVisible
		}
		if err := inventory.AddToCargo(tx, player, drop.ItemID, drop.Quantity, "loot_pickup", dropID); err != nil {
			return err
		}
		tx.Drops().Delete(dropID)
		tx.Events().Insert("loot.picked_up", player, dropID)
		return nil
	})
}
```

Important:

- lock drop row
- capacity check inside transaction
- delete after successful inventory add
- no client amount

## Loot XP

Only server-generated loot can grant loot XP.

```text
if drop.source_type in [npc_kill, gather_node, event_cache]:
  grant small loot XP
```

Player death loot may not grant XP to avoid abuse.

## Drop Visibility

AOI/Fog rule:

```text
Drop exists in world.
Client receives it only if visible.
Pickup requires visibility and distance.
```

This prevents:

- hidden drop scanners
- packet-based pickup outside sight

## Events Emitted

```text
loot.created
loot.owner_lock_expired
loot.picked_up
loot.expired
loot.pickup_failed
```

`loot.picked_up` is emitted after the pickup claim and cargo mutation succeed.
The internal payload carries the claiming player, item id, quantity, and drop id
so quest progress can consume pickup results without trusting client payloads.

## Edge Cases

- Two players pickup same public drop at same time.
- Owner disconnects before lock expires.
- Drop expires while pickup request in flight.
- Cargo capacity changes while pickup request in flight.
- Drop stack partially fits; choose all-or-nothing for MVP.
- Drop created near zone boundary; owning worker must be clear.
- NPC death processed twice; drop should not duplicate.

## Abuse Vectors

### Vacuum Loot

Risk:

- Client sends pickup for far-away drop id.

Defense:

- server distance check
- visibility check
- pickup rate limit

### Duplicate Pickup Race

Risk:

- Two pickup requests both succeed.

Defense:

- lock drop row
- delete/drop claimed in transaction
- unique terminal state

### Owner Lock Bypass

Risk:

- Player modifies client and picks owner-locked drop.

Defense:

- ownership validation server-side
- owner lock time server-side

### Loot Table Spoof

Risk:

- Client claims rare loot dropped.

Defense:

- client never sends loot contents
- loot roll only server side

### Player Death Loot XP Abuse

Risk:

- Two players kill each other to farm loot pickup XP.

Defense:

- player_death source gives no loot XP or strict caps

## Testing Checklist

- Owner can pickup during lock.
- Non-owner cannot pickup during lock.
- Anyone can pickup after lock.
- No one can pickup expired drop.
- Far pickup fails.
- Hidden pickup fails.
- Concurrent pickup only one succeeds.
- Cargo full blocks pickup and drop remains.
- NPC duplicate death does not duplicate drops.

## Implementation Notes

MVP:

- all-or-nothing pickup
- owner lock 60s
- expire 180s
- raw material loot tables
- no party loot yet
