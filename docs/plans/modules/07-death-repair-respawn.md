# Death, Repair, And Respawn

Date: 2026-06-17

## Purpose

Bu modül oyuncu gemisi öldüğünde ne olduğunu yönetir:

- cargo drop
- ship disabled state
- module durability loss
- respawn/checkpoint
- repair
- fallback starter ship

Ölüm anlamlı olmalı ama oyuncuyu oyundan kilitlememeli.

## Owns

```text
DeathService
RepairService
RespawnService
```

## Does Not Own

- Combat lethal damage calculation
- Loot pickup
- Cargo transfer primitive
- Ship unlock
- Module stat aggregation

## Death Flow

```text
CombatService detects HP <= 0
DeathService locks player death state
Calculate cargo drop percent
Move cargo items into world drops
Mark active ship disabled
Roll module durability loss
Choose respawn state/location
Emit events
Client shows repair/swap choices
```

## Death State Data

```text
player_deaths
- death_id
- player_id
- world_id
- position_x
- position_y
- killer_entity_id
- death_reason
- cargo_drop_percent
- active_ship_id
- respawn_location_id
- created_at
```

Ship state:

```text
player_ships.state = disabled
disabled_reason = death
disabled_at = now
```

## Cargo Drop Rule

Risky area rule:

```text
drop_percent = random 50%..100%
```

MVP global can start with:

```text
safe zone: 0%..20%
normal zone: 30%..70%
pvp/deep zone: 50%..100%
```

But design target for dangerous zones is `50%-100%`.

## Cargo Drop Algorithm

```go
func SelectCargoDrops(cargo []CargoStack, percent float64, rng RNG) []CargoDrop {
	totalUnits := SumUnits(cargo)
	targetUnits := int(float64(totalUnits) * percent)

	shuffled := rng.Shuffle(cargo)
	selected := make([]CargoDrop, 0)
	remaining := targetUnits

	for _, stack := range shuffled {
		if remaining <= 0 {
			break
		}
		if stack.NonDroppable {
			continue
		}
		units := Min(stack.Units(), remaining)
		qty := stack.QuantityForUnits(units)
		selected = append(selected, CargoDrop{ItemID: stack.ItemID, Quantity: qty})
		remaining -= units
	}
	return selected
}
```

All-or-partial:

- Stackable materials can partially drop.
- Instance items either drop or not.
- Soulbound/quest-critical items do not drop.

## Death Transaction

```text
BEGIN
  lock player combat/death state
  ensure not already dead
  lock active ship
  lock cargo
  calculate drop list
  remove dropped items from cargo
  create world drops via LootService
  mark ship disabled
  roll module durability damage
  insert death record
COMMIT
emit player.died
emit ship.disabled
emit loot.created
emit module.durability_changed
```

Death must be idempotent:

```text
combat lethal event id -> unique death processing id
```

## Respawn

Respawn location priority:

1. Last checkpoint
2. Nearest owned planet with respawn/hangar
3. Nearest safe station
4. Origin fallback

MVP:

```text
last checkpoint or origin station
```

Respawn state:

```text
player alive but active ship disabled
must repair or swap ship
```

## Repair

Repair options:

- Credits
- Premium currency convenience
- Repair kit
- Planet repair building later

Formula:

```text
repair_cost = ship_credit_value * repair_rate * location_modifier
```

Example:

```go
func RepairCost(ship ShipDef, location RepairLocation) int64 {
	base := float64(ship.CreditValue)
	rate := 0.10
	return int64(base * rate * location.Modifier)
}
```

Repair command:

```go
func RepairShip(ctx context.Context, player PlayerID, shipID string, payment PaymentMethod) error {
	return db.Tx(ctx, func(tx Tx) error {
		ship := tx.Ships().LockPlayerShip(player, shipID)
		if ship.State != "disabled" {
			return ErrShipNotDisabled
		}
		cost := repair.Cost(ship)
		if err := wallet.Debit(tx, player, payment.Currency, cost, "ship_repair", shipID); err != nil {
			return err
		}
		ship.State = "available"
		ship.DisabledReason = ""
		tx.Ships().Save(ship)
		tx.Events().Insert("ship.repaired", player, shipID)
		return nil
	})
}
```

## Module Durability Loss

Death may damage equipped durable modules:

```go
func RollModuleDamage(mod EquippedModule, zone ZoneRules, rng RNG) bool {
	if !mod.HasDurability {
		return false
	}
	chance := zone.ModuleDamageChance
	chance += mod.RiskModifier
	return rng.Float64() <= chance
}
```

Zone examples:

```text
safe: 0%
normal: 5%
deep: 10%
pvp: 15%
```

## Starter Ship Fallback

If all ships disabled:

```text
starter ship is always available or can be restored for free
```

Login safety:

```go
if !hangar.HasUsableShip(player) {
	hangar.RestoreStarterShip(player)
}
```

## Events Emitted

```text
player.died
player.respawned
ship.disabled
ship.repaired
ship.repair_failed
death.cargo_dropped
module.durability_changed
```

## Edge Cases

- Player dies twice due to duplicate lethal events.
- Player disconnects during death transaction.
- Cargo item is reserved in craft/market; reserved items should not be in cargo.
- Ship disabled while route/production UI open.
- Repair request for active usable ship.
- Repair paid but state update fails; transaction prevents partial.
- All ships disabled; starter fallback required.

## Abuse Vectors

### Death Duplication

Risk:

- Same death processed twice, cargo drops twice.

Defense:

- death state lock
- lethal event id unique
- ship already disabled check

### Cargo Hiding

Risk:

- Client tries to move cargo to safe inventory right before death.

Defense:

- Server order of events
- death/combat lock
- cargo transfer blocked while dead/in lethal transaction

### Repair Cost Bypass

Risk:

- Client sends cheaper cost.

Defense:

- server computes cost from catalog
- client sends only payment preference

### Module Damage Avoidance

Risk:

- Client unequips modules after death packet.

Defense:

- death transaction locks equipped modules as of lethal event
- no equip/unequip while dead/combat locked

## Testing Checklist

- Death processed once.
- Cargo drop percent within zone range.
- Non-droppable item stays.
- Ship becomes disabled.
- Starter fallback works.
- Repair charges correct wallet.
- Repair restores ship.
- Module durability can drop.
- Broken module invalidates stats.
- Disconnect during death leaves consistent state.

## Implementation Notes

MVP:

- death cargo drop for materials
- active ship disabled
- credit repair
- starter fallback
- simple checkpoint respawn
- module durability loss optional but prepared

