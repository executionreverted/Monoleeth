# Module And Stat Aggregation

Date: 2026-06-17

## Purpose

Bu modül gemiye takılan offensive, defensive ve utility modülleri yönetir; oyuncunun final stat snapshot'ını hesaplar.

Bu oyun server-authoritative olduğu için combat, scan, movement, cargo, route ve production gibi sistemler client değerlerine değil bu modülün server-side stat snapshot'ına bakar.

## Owns

```text
ModuleService
StatAggregationService
```

## Does Not Own

- Ship unlock
- Loadout kayıt ownership
- Combat damage application
- Loot drop
- Craft recipe
- Market trade

## Module Categories

```text
offensive
defensive
utility
```

Offensive:

- Laser Gun Alpha
- Laser Gun Beta
- Rocket Launcher
- Piercing Beam

Defensive:

- Shield Generator
- Shield Battery
- Reflector
- Armor Plate

Utility:

- Scanner
- Radar
- Jammer
- Stealth Module
- Cargo Expansion
- Warp Stabilizer

## Module Definition

```text
item_id
module_type
tier
rarity
required_rank
required_role_level
slot_type
stats_json
energy_json
cooldown_json
durability_max
trade_flags
bind_rules
```

Example:

```json
{
  "item_id": "laser_alpha_t1",
  "slot_type": "offensive",
  "tier": 1,
  "required_rank": 1,
  "stats": {
    "weapon_damage_flat": 12,
    "weapon_range_flat": 650,
    "accuracy_add": 0.82,
    "laser_energy_cost_flat": 8
  },
  "cooldown": {
    "basic_attack_ms": 1200
  }
}
```

## Equipped Module State

```text
player_id
ship_id
slot_id
item_instance_id
equipped_at
```

Slot ids:

```text
offensive_1
offensive_2
defensive_1
utility_1
```

## Commands

```text
EquipModule(player_id, ship_id, slot_id, item_instance_id)
UnequipModule(player_id, ship_id, slot_id)
ValidateModuleAssignments(player_id, ship_id, assignments)
GetEffectiveStats(player_id, ship_id)
InvalidateStats(player_id, reason)
RecalculateStats(player_id, ship_id)
ApplyDurabilityDamage(item_instance_id, amount, reason)
```

`GetEffectiveStats` only receives a player/ship subject. Base ship, equipped
modules, passives, role bonuses, and temporary modifiers must be built from
server-owned records through a stat input provider. Client-provided stat payloads
are never accepted as gameplay truth.

## Equip Validation

```go
func CanEquipModule(ctx EquipContext, mod ModuleDef, item ItemInstance, slot ShipSlot) error {
	if item.OwnerPlayerID != ctx.PlayerID {
		return ErrNotOwner
	}
	if item.LocationType != "account_inventory" && item.LocationType != "ship_equipped" {
		return ErrInvalidLocation
	}
	if mod.SlotType != slot.Type {
		return ErrWrongSlotType
	}
	if ctx.PlayerRank < mod.RequiredRank {
		return ErrRankTooLow
	}
	if item.DurabilityCurrent != nil && *item.DurabilityCurrent <= 0 {
		return ErrModuleBroken
	}
	return nil
}
```

Energy budget validation:

```go
func ValidateEnergyBudget(base ShipStats, modules []ModuleDef) error {
	upkeep := 0
	for _, m := range modules {
		upkeep += m.Energy.Upkeep
	}
	if upkeep > base.EnergyRegenSoftBudget {
		return ErrEnergyBudgetExceeded
	}
	return nil
}
```

MVP'de upkeep budget yerine combat sırasında energy cost validate etmek daha basit olabilir.

## Stat Aggregation Order

Sıra:

```text
1. base ship stats
2. flat module stats
3. flat pilot passive stats
4. role level bonuses
5. percent module modifiers
6. percent passive modifiers
7. temporary buffs/debuffs
8. clamp
```

Go-ish:

```go
func AggregateStats(input StatInput) EffectiveStats {
	stats := input.Ship.BaseStats

	for _, mod := range input.Modules {
		stats.ApplyFlat(mod.FlatStats)
	}
	for _, passive := range input.Passives {
		stats.ApplyFlat(passive.FlatStats)
	}
	for _, role := range input.RoleBonuses {
		stats.ApplyFlat(role.FlatStats)
	}
	for _, mod := range input.Modules {
		stats.ApplyPercent(mod.PercentStats)
	}
	for _, passive := range input.Passives {
		stats.ApplyPercent(passive.PercentStats)
	}
	for _, buff := range input.Buffs {
		stats.ApplyModifier(buff.Modifier)
	}

	stats.Clamp()
	return stats
}
```

## Effective Stat Snapshot

Snapshot:

```text
player_id
ship_id
version
stats_json
created_at
invalidated_at
```

Active session cache:

```text
player_id -> EffectiveStats
version
```

Combat/scan uses cached snapshot version.

## Invalidation Triggers

```text
ship.active_changed
module.equipped
module.unequipped
module.durability_changed_to_broken
pilot.skill_unlocked
pilot.skills_respecced
player.role_level_up
player.rank_up
buff.applied
buff.expired
debuff.applied
debuff.expired
```

## Important Stats

Core:

```text
hp_max
shield_max
shield_regen
energy_max
energy_regen
speed
cargo_capacity
```

Combat:

```text
weapon_damage
weapon_range
weapon_cooldown
accuracy
tracking
evasion
penetration
crit_chance
crit_multiplier
resist_laser
resist_explosive
resist_kinetic
```

Exploration:

```text
radar_range
scan_power
scan_radius
scan_interval
signature_radius
stealth_strength
jammer_strength
```

Economy:

```text
craft_speed
route_loss_reduction
production_bonus
market_fee_reduction
```

## Durability

Durability damage:

```go
func DamageModuleDurability(item ItemInstance, amount int) ItemInstance {
	if item.DurabilityCurrent == nil {
		return item
	}
	next := *item.DurabilityCurrent - amount
	if next < 0 {
		next = 0
	}
	item.DurabilityCurrent = &next
	return item
}
```

When module reaches 0:

```text
emit module.broken
emit player.stats_invalidated
if equipped, effects no longer apply
```

## Events Emitted

```text
module.equipped
module.unequipped
module.durability_changed
module.broken
player.stats_invalidated
player.stats_recalculated
```

## Edge Cases

- Aynı module instance iki slotta takılı olamaz.
- Broken module stat vermemeli.
- Marketplace escrow'daki module equip edilemez.
- Equipped module markete konamaz veya önce unequip ister.
- Ship slot layout değişirse equipped modules migration gerekir.
- Stat percent stacking order net olmalı.
- Negative modifier statı sıfır altına düşürmemeli.
- Buff expire olurken snapshot invalid edilmeli.

## Abuse Vectors

### Client Stat Injection

Risk:

- Client "speed 999" veya "damage 999" gibi değer yollar.

Defense:

- Client stat göndermez.
- Server stat snapshot kullanır.
- Movement/combat validation snapshot'tan yapılır.

### Duplicate Equip Race

Risk:

- Aynı item iki request ile iki slota takılmaya çalışılır.

Defense:

- transaction lock
- unique(item_instance_id) in equipped table
- assignment validation inside tx

### Broken Module Still Active

Risk:

- Durability 0 sonrası stat cache invalid edilmezse oyuncu bonus almaya devam eder.

Defense:

- durability update emits invalidation
- combat reads stat version
- tests for break while equipped

### Catalog Tampering

Risk:

- Client module tier/stat metadata gönderir.

Defense:

- Server catalog only
- Client only sends item_instance_id/slot_id intent

## Testing Checklist

- Wrong slot type fail mi?
- Rank too low fail mi?
- Broken module equip fail mi?
- Duplicate module equip fail mi?
- Stat aggregation deterministic mi?
- Invalidation triggerları çalışıyor mu?
- Broken equipped module stattan düşüyor mu?
- Cargo capacity statı module ile artıp swap validation'a yansıyor mu?
- Combat snapshot client değerinden bağımsız mı?

## Implementation Notes

İlk sürümde:

- Flat stats ağırlıklı başla.
- Percent modifierları az tut.
- Rarity affix sistemini ertele.
- Energy budget'i combat use sırasında validate et.
- Stat snapshot'ı memory cache + DB optional yap.
- Stat snapshot recalculation inputu server-side provider'dan gelsin; API
  consumer'ı doğrudan damage/speed/cargo payload gönderemesin.
