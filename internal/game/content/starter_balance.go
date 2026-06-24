package content

import (
	"time"

	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	DefaultStarterBalanceProfileID   = "renamed_starter_balance_v1"
	DefaultStarterBalanceProfileNote = "Local dev/test starter balance reference with renamed generic entities; no external dependency."
)

func defaultStarterBalanceModuleCatalog() (modules.Catalog, error) {
	definitions := modules.MVPModuleDefinitions()
	for index := range definitions {
		switch definitions[index].ItemID {
		case "laser_alpha_t1":
			definitions[index].Name = "Prism Lance I"
		case "shield_generator_t1":
			definitions[index].Name = "Aurora Shield Cell"
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
	definitions := ships.MVPShipDefinitions()
	for index := range definitions {
		if definitions[index].ShipID == ships.ShipIDStarter {
			definitions[index].Name = DefaultStarterShipDisplayName
		}
	}
	return ships.NewCatalog(definitions)
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
				template.LabelKey = "npc.warden_drone"
				template.HPMax = 34
				template.ShieldMax = 4
				template.EnergyMax = 6
				template.WeaponRange = 120
				template.WeaponDamage = 1
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.7
			case "training_overseer":
				template.LabelKey = "npc.warden_overseer"
				template.HPMax = 130
				template.ShieldMax = 30
				template.WeaponDamage = 5
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.85
			case "outer_ring_scout_drone":
				template.LabelKey = "npc.scout_drone"
				template.HPMax = 44
				template.ShieldMax = 8
				template.EnergyMax = 4
				template.WeaponRange = 140
				template.WeaponDamage = 2
				template.WeaponCooldown = 2 * time.Second
				template.Accuracy = 0.72
			case "border_raider_drone":
				template.LabelKey = "npc.raider_drone"
				template.HPMax = 72
				template.ShieldMax = 22
				template.EnergyMax = 8
				template.WeaponRange = 180
				template.WeaponDamage = 7
				template.WeaponCooldown = 1800 * time.Millisecond
				template.Accuracy = 0.82
			}
		}
	}
	return worldmaps.NewCatalog(definitions, worldmaps.StarterMapID, worldmaps.StarterSpawnID)
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
