# Ship, Hangar, And Loadout

Date: 2026-06-17

## Purpose

Bu modül oyuncunun sahip olduğu gemileri, aktif gemisini, hangar swap'ını ve loadout kayıtlarını yönetir.

Gemi oyunda karakter sınıfı gibi davranır:

- Fighter
- Scout
- Hauler
- Support
- Industrial

Ama gerçek güç, gemi + modül + pilot passive + stat aggregation ile oluşur.

## Owns

```text
ShipService
HangarService
LoadoutService
```

## Does Not Own

- Module stat hesaplama
- Combat damage
- Repair cost formula, Death/Repair modülü sahiplenir
- Ship craft recipe validation
- Premium shop stock
- Auction bidding

Ship unlock başka modüllerden gelebilir ama unlock state'ini bu modül yazar.

## Core Rules

- Oyuncunun tek aktif gemisi vardır.
- Aynı ship type bir kere unlock edilir.
- Starter ship her zaman erişilebilir.
- Destroyed/disabled ship repair edilene kadar aktif kullanılamaz.
- Ship swap sadece güvenli yerlerde yapılır.
- Cargo capacity hedef gemiye yetmiyorsa swap engellenir.
- Loadout equip validation server-side yapılır.

## Ship Definition

```text
ship_id
name
tier
role_tag
rank_requirement
craft_recipe_id
credit_price
premium_price
auction_buy_now_price
base_hp
base_shield
base_energy
base_energy_regen
base_speed
base_cargo
base_radar
base_signature
repair_cost_multiplier
slot_offensive
slot_defensive
slot_utility
passive_bonus_id
```

## Player Ship State

```text
player_id
ship_id
unlocked_at
state
disabled_reason
disabled_at
last_repaired_at
metadata_json
```

State:

```text
available
active
disabled
repairing
locked
```

## Commands

```text
UnlockShip(player_id, ship_id, source, reference_id)
SetActiveShip(player_id, ship_id)
SwapShip(player_id, ship_id)
SaveLoadout(player_id, ship_id, loadout)
ApplyLoadout(player_id, loadout_id)
RenameLoadout(player_id, loadout_id, name)
DeleteLoadout(player_id, loadout_id)
GetHangar(player_id)
```

## Unlock Logic

Ship unlock transaction:

```go
func UnlockShip(ctx context.Context, player PlayerID, shipID string, source Source, ref string) error {
	return db.Tx(ctx, func(tx Tx) error {
		if tx.Ships().PlayerHasShip(player, shipID) {
			return nil
		}
		def := shipCatalog.Get(shipID)
		if def == nil {
			return ErrUnknownShip
		}
		tx.Ships().InsertPlayerShip(player, shipID, "available")
		tx.Events().Insert("ship.unlocked", player, shipID, source, ref)
		return nil
	})
}
```

Not:

- CraftingService craft tamamlayınca `UnlockShip` çağırabilir.
- AuctionService lot kazanınca `UnlockShip` çağırabilir.
- ShopService payment sonrası `UnlockShip` çağırabilir.

## Starter Ship Guarantee

Her player create sonrası starter ship olmalı.

Ayrıca login sırasında safety check yapılabilir:

```go
func EnsureStarterShip(ctx context.Context, player PlayerID) error {
	if !repo.HasShip(player, "starter") {
		return repo.InsertPlayerShip(player, "starter", "available")
	}
	return nil
}
```

Starter ship:

- disabled olsa bile fallback repair ücretsiz veya çok ucuz olabilir
- asla trade edilemez
- asla kaybolmaz

## Ship Swap Validation

```go
func CanSwapShip(state PlayerState, target PlayerShip, cargo CargoState, targetStats ShipStats) error {
	if state.InCombat {
		return ErrCannotSwapInCombat
	}
	if !state.InSafeHangarArea {
		return ErrNotInHangarArea
	}
	if target.State != "available" {
		return ErrShipUnavailable
	}
	if cargo.UsedUnits > targetStats.CargoCapacity {
		return ErrCargoTooLarge
	}
	return nil
}
```

Safe places:

- Station
- Owned planet with hangar building
- Checkpoint
- Special safe zone

## Active Ship State

Aktif gemi değişince:

```text
player_active_ship update
old ship state -> available
new ship state -> active
stat cache invalidated
AOI representation updated
client receives ship_changed snapshot
```

## Loadout Model

```text
loadout_id
player_id
ship_id
name
slot_assignments_json
created_at
updated_at
```

Slot assignment:

```json
{
  "offensive_1": "item_instance_id_1",
  "offensive_2": "item_instance_id_2",
  "defensive_1": "item_instance_id_3",
  "utility_1": "item_instance_id_4"
}
```

## Loadout Apply Logic

Server validate eder:

- Loadout owner doğru mu?
- Loadout ship active/target ship ile uyumlu mu?
- Item instances oyuncuya ait mi?
- Item location account inventory/equipped valid mi?
- Module slot type uyuyor mu?
- Module rank requirement karşılanıyor mu?
- Module durability > 0 mı?
- Aynı item iki slota takılmıyor mu?
- Energy budget aşılmıyor mu? Bu validation `ModuleService` ile yapılabilir.

Pseudo:

```go
func ApplyLoadout(ctx context.Context, player PlayerID, loadoutID string, requestID RequestID) error {
	return db.Tx(ctx, func(tx Tx) error {
		loadout := tx.Loadouts().Lock(loadoutID)
		activeShip := tx.Ships().ActiveShip(player)
		if loadout.ShipID != activeShip.ShipID {
			return ErrLoadoutShipMismatch
		}
		if err := moduleSvc.ValidateAssignments(ctx, tx, player, activeShip.ShipID, loadout.Assignments); err != nil {
			return err
		}
		tx.Inventory().MoveModuleItemsForLoadout(player, activeShip.ShipID, loadout.Assignments, requestID)
		tx.Modules().EquipAssignments(player, activeShip.ShipID, loadout.Assignments)
		tx.Events().Insert("ship.loadout_applied", player, loadoutID)
		return nil
	})
}
```

Module equip/unequip changes item locations between `account_inventory` and
`ship_equipped`. The loadout/module boundary validates slot, ownership, rank,
role, durability, and duplicate use; the actual item movement must go through
`InventoryService` and item ledger primitives with domain idempotency references
such as `module_equip:<player_id>:<ship_id>:<item_instance_id>:<request_id>` and
`module_unequip:<player_id>:<ship_id>:<item_instance_id>:<request_id>`.

## Ship Archetype Examples

Fighter:

```text
offensive 4
defensive 2
utility 1
cargo low
speed medium
```

Scout:

```text
offensive 1
defensive 1
utility 4
radar high
signature low
```

Hauler:

```text
offensive 1
defensive 3
utility 2
cargo high
speed low
```

Support:

```text
offensive 2
defensive 2
utility 3
party aura passive
```

## Events Emitted

```text
ship.unlocked
ship.active_changed
ship.swap_failed
ship.loadout_saved
ship.loadout_applied
ship.loadout_deleted
player.stats_invalidated
```

## Edge Cases

- Oyuncu aktif gemisini disable ettikten sonra login olursa fallback seçimi gösterilmeli.
- Target ship cargo kapasitesi düşükse swap blocked olmalı.
- Loadout'taki modül market escrow'a taşındıysa loadout apply fail olmalı.
- Module durability 0'a düştüyse loadout apply fail olmalı.
- Ship definition slot sayısı değişirse eski loadout migration gerekir.
- Aynı item instance iki slota atanamaz.
- Ship swap sırasında combat flag yeni geldiyse transaction içinde tekrar kontrol gerekir.

## Abuse Vectors

### Combat Swap Abuse

Risk:

- Oyuncu savaş ortasında tank gemiye geçmeye çalışır.

Defense:

- Server combat flag
- Safe area requirement
- Swap cast time optional
- Recent damage cooldown

### Cargo Capacity Bypass

Risk:

- Oyuncu yüksek cargo gemiyle doldurup düşük cargo hızlı gemiye geçmeye çalışır.

Defense:

- Target cargo capacity transaction içinde validate edilir.
- Cargo overflow state'e izin verilmez.

### Locked Ship Activation

Risk:

- Client unlock olmayan ship_id ile active request yollar.

Defense:

- Player ship table validate
- ship state validate
- rank/effective requirement validate

### Duplicate Ship Unlock

Risk:

- Auction/craft retry aynı gemiyi tekrar verir.

Defense:

- unique(player_id, ship_id)
- idempotent unlock
- no duplicate ship items

## Testing Checklist

- Starter ship guarantee test edildi mi?
- Duplicate unlock no-op mu?
- Swap in combat fail mi?
- Swap outside hangar fail mi?
- Cargo overflow swap fail mi?
- Destroyed ship active yapılamıyor mu?
- Loadout module ownership validate mi?
- Loadout duplicate module fail mi?
- Loadout stat invalidation event atıyor mu?

## Implementation Notes

İlk sürüm:

- `starter`, `fighter_t1`, `scout_t1`, `hauler_t1` yeterli.
- Ship effective scaling sonraya bırakılabilir.
- Loadout slot sayısı başlangıçta 1 olabilir.
- Premium loadout slot sonra eklenir.
