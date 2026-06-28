package content

import (
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	DefaultStarterBalanceProfileID   = "old_darkorbit_2009_balance_v1"
	DefaultStarterBalanceProfileNote = "Local dev/test seed derived from Old DarkOrbit 2009 balance values; names are temporary and may be renamed later."
)

func defaultStarterBalanceModuleCatalog() (modules.Catalog, error) {
	definitions := modules.MVPModuleDefinitions()
	for index := range definitions {
		switch definitions[index].ItemID {
		case "laser_alpha_t1":
			definitions[index].Name = "LF-1"
			setStarterBalanceModuleStat(&definitions[index], modules.StatWeaponDamage, 40)
			setStarterBalanceModuleStat(&definitions[index], modules.StatWeaponRange, 650)
			setStarterBalanceModuleStat(&definitions[index], modules.StatAccuracy, 8_200)
		case "shield_generator_t1":
			definitions[index].Name = "SG3N-A01"
			setStarterBalanceModuleStat(&definitions[index], modules.StatShieldMax, 1_000)
			setStarterBalanceModuleStat(&definitions[index], modules.StatShieldRegen, 4)
		case "scanner_t1":
			definitions[index].Name = "Horizon Scanner"
		case "radar_t1":
			definitions[index].Name = "Warden Radar I"
		case "cargo_expander_t1":
			definitions[index].Name = "Cargo Spine I"
		}
	}
	return modules.NewCatalog(definitions)
}

func defaultStarterBalanceShipCatalog() (ships.Catalog, error) {
	definitions := make([]ships.ShipDefinition, 0, 4)
	for _, seed := range []struct {
		shipID    foundation.ShipID
		name      string
		role      ships.ShipRole
		rank      int
		stats     ships.ShipBaseStats
		slots     ships.SlotLayout
		price     int64
		repairBps int64
	}{
		{
			shipID: ships.ShipIDStarter,
			name:   "Phoenix",
			role:   ships.ShipRoleSupport,
			rank:   1,
			stats: ships.ShipBaseStats{
				HP: 4_000, Shield: 1_000, Energy: 2_000, EnergyRegen: 10,
				Speed: 320, CargoCapacity: 100, Radar: 100, Signature: 100,
			},
			slots:     ships.SlotLayout{Offensive: 1, Defensive: 1, Utility: 1},
			price:     0,
			repairBps: 10_000,
		},
		{
			shipID: ships.ShipIDFighterT1,
			name:   "Goliath K2",
			role:   ships.ShipRoleFighter,
			rank:   2,
			stats: ships.ShipBaseStats{
				HP: 256_000, Shield: 15_000, Energy: 32_000, EnergyRegen: 12,
				Speed: 300, CargoCapacity: 1_500, Radar: 100, Signature: 120,
			},
			slots:     ships.SlotLayout{Offensive: 4, Defensive: 3, Utility: 3},
			price:     80_000,
			repairBps: 12_500,
		},
		{
			shipID: ships.ShipIDScoutT1,
			name:   "Vengeance",
			role:   ships.ShipRoleScout,
			rank:   2,
			stats: ships.ShipBaseStats{
				HP: 180_000, Shield: 10_000, Energy: 16_000, EnergyRegen: 14,
				Speed: 360, CargoCapacity: 1_000, Radar: 170, Signature: 70,
			},
			slots:     ships.SlotLayout{Offensive: 4, Defensive: 3, Utility: 2},
			price:     40_000,
			repairBps: 11_000,
		},
		{
			shipID: ships.ShipIDHaulerT1,
			name:   "Bigboy",
			role:   ships.ShipRoleHauler,
			rank:   2,
			stats: ships.ShipBaseStats{
				HP: 128_000, Shield: 15_000, Energy: 18_000, EnergyRegen: 9,
				Speed: 260, CargoCapacity: 700, Radar: 85, Signature: 150,
			},
			slots:     ships.SlotLayout{Offensive: 4, Defensive: 3, Utility: 3},
			price:     20_000,
			repairBps: 13_000,
		},
	} {
		definition, err := starterBalanceShipDefinition(seed.shipID, seed.name, seed.role, seed.rank, seed.stats, seed.slots, seed.price, seed.repairBps)
		if err != nil {
			return ships.Catalog{}, err
		}
		definitions = append(definitions, definition)
	}
	return ships.NewCatalog(definitions)
}

func starterBalanceShipDefinition(
	shipID foundation.ShipID,
	name string,
	role ships.ShipRole,
	rankRequirement int,
	baseStats ships.ShipBaseStats,
	slots ships.SlotLayout,
	creditPrice int64,
	repairMultiplierBps int64,
) (ships.ShipDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings(shipID.String(), ships.ShipCatalogVersion.String())
	if err != nil {
		return ships.ShipDefinition{}, err
	}
	definition, err := ships.NewShipDefinition(source, shipID, name, 1, role, rankRequirement, baseStats, slots)
	if err != nil {
		return ships.ShipDefinition{}, err
	}
	definition.CreditPrice = creditPrice
	definition.RepairCostMultiplierBps = repairMultiplierBps
	if err := definition.Validate(); err != nil {
		return ships.ShipDefinition{}, err
	}
	return definition, nil
}

func defaultStarterBalanceRecipeCatalog() (crafting.RecipeCatalog, error) {
	definitions := crafting.MVPRecipeDefinitions()
	for index := range definitions {
		switch definitions[index].RecipeID {
		case crafting.RecipeIDLaserAlphaT1:
			definitions[index].Inputs = []crafting.RecipeInput{
				mustStarterBalanceRecipeInput("refined_alloy", 18),
				mustStarterBalanceRecipeInput("laser_lens", 3),
				mustStarterBalanceRecipeInput("energy_cell", 2),
			}
			definitions[index].RequiredCredits = mustStarterBalanceMoney(650)
			definitions[index].CraftDuration = 20 * time.Minute
		case crafting.RecipeIDScoutT1:
			definitions[index].Inputs = []crafting.RecipeInput{
				mustStarterBalanceRecipeInput("refined_alloy", 80),
				mustStarterBalanceRecipeInput("scanner_circuit", 12),
				mustStarterBalanceRecipeInput("warp_coil", 4),
			}
			definitions[index].RequiredCredits = mustStarterBalanceMoney(2_200)
			definitions[index].CraftDuration = 90 * time.Minute
		}
	}
	return crafting.NewRecipeCatalog(definitions)
}

func defaultStarterBalanceMapCatalog(worldID world.WorldID) (*worldmaps.Catalog, error) {
	mapCatalog, err := worldmaps.StarterCatalog(worldID)
	if err != nil {
		return nil, err
	}
	definitions := mapCatalog.Definitions()
	for definitionIndex := range definitions {
		for templateIndex := range definitions[definitionIndex].NPCStatTemplates {
			template := &definitions[definitionIndex].NPCStatTemplates[templateIndex]
			switch template.NPCType {
			case "training_drone":
				template.LabelKey = "npc.lordakia"
				template.HPMax = 2000
				template.ShieldMax = 2000
				template.EnergyMax = 20
				template.WeaponRange = 120
				template.WeaponDamage = 80
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.7
				template.Speed = 320
				template.XPValue = 800
			case "training_overseer":
				template.LabelKey = "npc.mordon"
				template.HPMax = 20000
				template.ShieldMax = 10000
				template.WeaponDamage = 300
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.85
				template.Speed = 125
				template.XPValue = 3200
			case "outer_ring_scout_drone":
				template.LabelKey = "npc.streuner"
				template.HPMax = 800
				template.ShieldMax = 400
				template.EnergyMax = 10
				template.WeaponRange = 140
				template.WeaponDamage = 20
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.72
				template.Speed = 280
				template.XPValue = 400
			case "border_raider_drone":
				template.LabelKey = "npc.saimon"
				template.HPMax = 6000
				template.ShieldMax = 3000
				template.EnergyMax = 40
				template.WeaponRange = 180
				template.WeaponDamage = 200
				template.WeaponCooldown = 1800 * time.Millisecond
				template.Accuracy = 0.82
				template.Speed = 320
				template.XPValue = 1600
			}
		}
	}
	return worldmaps.NewCatalog(definitions, worldmaps.StarterMapID, worldmaps.StarterSpawnID)
}

func setStarterBalanceModuleStat(definition *modules.ModuleDefinition, stat modules.StatKey, value int64) {
	for index := range definition.StatModifiers {
		if definition.StatModifiers[index].Stat == stat && definition.StatModifiers[index].Kind == modules.StatModifierFlat {
			definition.StatModifiers[index].Value = value
			return
		}
	}
	definition.StatModifiers = append(definition.StatModifiers, modules.StatModifier{
		Stat:  stat,
		Kind:  modules.StatModifierFlat,
		Value: value,
	})
}

func mustStarterBalanceRecipeInput(itemID foundation.ItemID, amount int64) crafting.RecipeInput {
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		panic(err)
	}
	return crafting.RecipeInput{ItemID: itemID, Quantity: quantity}
}

func mustStarterBalanceMoney(amount int64) foundation.Money {
	money, err := foundation.NewMoney(amount)
	if err != nil {
		panic(err)
	}
	return money
}
