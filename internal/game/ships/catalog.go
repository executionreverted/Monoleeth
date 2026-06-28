package ships

import (
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// MVPShipDefinitions returns a legacy in-package fallback used by isolated
// unit tests. Runtime ship rows are loaded through content repositories and
// contentdb snapshots, not from this helper.
func MVPShipDefinitions() []ShipDefinition {
	return []ShipDefinition{
		mustMVPShipDefinition(
			ShipIDStarter,
			"Starter Skiff",
			1,
			ShipRoleSupport,
			1,
			ShipBaseStats{
				HP:            100,
				Shield:        60,
				Energy:        100,
				EnergyRegen:   10,
				Speed:         100,
				CargoCapacity: 50,
				Radar:         100,
				Signature:     100,
			},
			SlotLayout{Offensive: 1, Defensive: 1, Utility: 1},
			0,
			10_000,
		),
		mustMVPShipDefinition(
			ShipIDFighterT1,
			"Fighter T1",
			1,
			ShipRoleFighter,
			2,
			ShipBaseStats{
				HP:            160,
				Shield:        120,
				Energy:        140,
				EnergyRegen:   12,
				Speed:         110,
				CargoCapacity: 40,
				Radar:         100,
				Signature:     120,
			},
			SlotLayout{Offensive: 4, Defensive: 2, Utility: 1},
			2_000,
			12_500,
		),
		mustMVPShipDefinition(
			ShipIDScoutT1,
			"Scout T1",
			1,
			ShipRoleScout,
			2,
			ShipBaseStats{
				HP:            110,
				Shield:        80,
				Energy:        130,
				EnergyRegen:   14,
				Speed:         145,
				CargoCapacity: 35,
				Radar:         170,
				Signature:     70,
			},
			SlotLayout{Offensive: 1, Defensive: 1, Utility: 4},
			2_000,
			11_000,
		),
		mustMVPShipDefinition(
			ShipIDHaulerT1,
			"Hauler T1",
			1,
			ShipRoleHauler,
			2,
			ShipBaseStats{
				HP:            180,
				Shield:        130,
				Energy:        120,
				EnergyRegen:   9,
				Speed:         75,
				CargoCapacity: 220,
				Radar:         85,
				Signature:     150,
			},
			SlotLayout{Offensive: 1, Defensive: 3, Utility: 2},
			2_500,
			13_000,
		),
	}
}

// MVPShipCatalog returns the validated legacy fallback catalog.
func MVPShipCatalog() (Catalog, error) {
	return NewCatalog(MVPShipDefinitions())
}

func mustMVPShipDefinition(
	shipID foundation.ShipID,
	name string,
	tier int,
	role ShipRole,
	rankRequirement int,
	baseStats ShipBaseStats,
	slots SlotLayout,
	creditPrice int64,
	repairMultiplierBps int64,
) ShipDefinition {
	source, err := catalog.NewVersionedDefinitionFromStrings(shipID.String(), ShipCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	definition, err := NewShipDefinition(source, shipID, name, tier, role, rankRequirement, baseStats, slots)
	if err != nil {
		panic(err)
	}
	definition.CreditPrice = creditPrice
	definition.RepairCostMultiplierBps = repairMultiplierBps
	if err := definition.Validate(); err != nil {
		panic(err)
	}
	return definition
}
