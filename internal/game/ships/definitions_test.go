package ships

import (
	"errors"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

func TestMVPShipCatalogRowsValidateAndUseExpectedIDs(t *testing.T) {
	catalogRows, err := MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog() error = %v, want nil", err)
	}

	definitions := catalogRows.All()
	if got, want := len(definitions), 4; got != want {
		t.Fatalf("len(All()) = %d, want %d", got, want)
	}

	tests := []struct {
		shipID foundation.ShipID
		role   ShipRole
		rank   int
		slots  SlotLayout
	}{
		{ShipIDStarter, ShipRoleSupport, 1, SlotLayout{Offensive: 1, Defensive: 1, Utility: 1}},
		{ShipIDFighterT1, ShipRoleFighter, 2, SlotLayout{Offensive: 4, Defensive: 2, Utility: 1}},
		{ShipIDScoutT1, ShipRoleScout, 2, SlotLayout{Offensive: 1, Defensive: 1, Utility: 4}},
		{ShipIDHaulerT1, ShipRoleHauler, 2, SlotLayout{Offensive: 1, Defensive: 3, Utility: 2}},
	}
	for _, test := range tests {
		definition, ok := catalogRows.Get(test.shipID)
		if !ok {
			t.Fatalf("catalog missing ship %q", test.shipID)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("%s Validate() = %v, want nil", test.shipID, err)
		}
		if definition.Role != test.role {
			t.Fatalf("%s Role = %q, want %q", test.shipID, definition.Role, test.role)
		}
		if definition.RankRequirement != test.rank {
			t.Fatalf("%s RankRequirement = %d, want %d", test.shipID, definition.RankRequirement, test.rank)
		}
		if definition.Slots != test.slots {
			t.Fatalf("%s Slots = %+v, want %+v", test.shipID, definition.Slots, test.slots)
		}
	}
}

func TestShipDefinitionRejectsInvalidCatalogFields(t *testing.T) {
	valid := validShipDefinition(t)

	definition := valid
	definition.ShipID = ""
	if err := definition.Validate(); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank ship id error = %v, want foundation.ErrEmptyID", err)
	}

	definition = valid
	definition.Name = " "
	if err := definition.Validate(); !errors.Is(err, ErrEmptyShipName) {
		t.Fatalf("blank name error = %v, want ErrEmptyShipName", err)
	}

	definition = valid
	definition.Tier = 0
	if err := definition.Validate(); !errors.Is(err, ErrInvalidShipTier) {
		t.Fatalf("zero tier error = %v, want ErrInvalidShipTier", err)
	}

	definition = valid
	definition.Role = ShipRole("miner")
	if err := definition.Validate(); !errors.Is(err, ErrInvalidShipRole) {
		t.Fatalf("invalid role error = %v, want ErrInvalidShipRole", err)
	}

	definition = valid
	definition.RankRequirement = 0
	if err := definition.Validate(); !errors.Is(err, ErrInvalidRankRequirement) {
		t.Fatalf("zero rank requirement error = %v, want ErrInvalidRankRequirement", err)
	}

	definition = valid
	definition.Source = validShipSource(t, "other_ship")
	if err := definition.Validate(); !errors.Is(err, ErrShipSourceMismatch) {
		t.Fatalf("source mismatch error = %v, want ErrShipSourceMismatch", err)
	}
}

func TestShipDefinitionRejectsInvalidStatsSlotsAndPrices(t *testing.T) {
	valid := validShipDefinition(t)

	definition := valid
	definition.BaseStats.HP = 0
	if err := definition.Validate(); !errors.Is(err, ErrInvalidShipBaseStat) {
		t.Fatalf("zero hp error = %v, want ErrInvalidShipBaseStat", err)
	}

	definition = valid
	definition.BaseStats.CargoCapacity = 0
	if err := definition.Validate(); !errors.Is(err, ErrInvalidCargoCapacity) {
		t.Fatalf("zero cargo capacity error = %v, want ErrInvalidCargoCapacity", err)
	}

	definition = valid
	definition.Slots.Offensive = -1
	if err := definition.Validate(); !errors.Is(err, ErrInvalidSlotCount) {
		t.Fatalf("negative slot error = %v, want ErrInvalidSlotCount", err)
	}

	definition = valid
	definition.Slots = SlotLayout{}
	if err := definition.Validate(); !errors.Is(err, ErrEmptySlotLayout) {
		t.Fatalf("empty slot layout error = %v, want ErrEmptySlotLayout", err)
	}

	definition = valid
	definition.CreditPrice = -1
	if err := definition.Validate(); !errors.Is(err, ErrNegativeShipPrice) {
		t.Fatalf("negative price error = %v, want ErrNegativeShipPrice", err)
	}

	definition = valid
	definition.RepairCostMultiplierBps = 0
	if err := definition.Validate(); !errors.Is(err, ErrInvalidRepairCostMultiplier) {
		t.Fatalf("zero repair multiplier error = %v, want ErrInvalidRepairCostMultiplier", err)
	}
}

func TestShipCatalogRejectsDuplicateAndUnknownDefinitions(t *testing.T) {
	definition := validShipDefinition(t)

	if _, err := NewCatalog(nil); !errors.Is(err, ErrEmptyShipCatalog) {
		t.Fatalf("empty catalog error = %v, want ErrEmptyShipCatalog", err)
	}
	if _, err := NewCatalog([]ShipDefinition{definition, definition}); !errors.Is(err, ErrDuplicateShipDefinition) {
		t.Fatalf("duplicate catalog error = %v, want ErrDuplicateShipDefinition", err)
	}

	catalogRows, err := NewCatalog([]ShipDefinition{definition})
	if err != nil {
		t.Fatalf("NewCatalog(valid) error = %v, want nil", err)
	}
	if _, err := catalogRows.MustGet("missing_ship"); !errors.Is(err, ErrUnknownShipDefinition) {
		t.Fatalf("unknown ship error = %v, want ErrUnknownShipDefinition", err)
	}
}

func TestSlotAndRoleStringBehaviorIsStable(t *testing.T) {
	if got := ShipRoleFighter.String(); got != "fighter" {
		t.Fatalf("ShipRoleFighter.String() = %q, want fighter", got)
	}
	if got := SlotTypeUtility.String(); got != "utility" {
		t.Fatalf("SlotTypeUtility.String() = %q, want utility", got)
	}

	layout := SlotLayout{Offensive: 1, Defensive: 2, Utility: 3}
	if got := layout.Total(); got != 6 {
		t.Fatalf("layout.Total() = %d, want 6", got)
	}
	count, err := layout.Count(SlotTypeDefensive)
	if err != nil {
		t.Fatalf("layout.Count(defensive) error = %v, want nil", err)
	}
	if count != 2 {
		t.Fatalf("layout.Count(defensive) = %d, want 2", count)
	}
	if _, err := layout.Count(SlotType("hangar")); !errors.Is(err, ErrInvalidSlotType) {
		t.Fatalf("invalid slot type error = %v, want ErrInvalidSlotType", err)
	}
}

func validShipDefinition(t *testing.T) ShipDefinition {
	t.Helper()

	definition, err := NewShipDefinition(
		validShipSource(t, "starter"),
		"starter",
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
	)
	if err != nil {
		t.Fatalf("NewShipDefinition(valid) error = %v", err)
	}
	return definition
}

func validShipSource(t *testing.T, shipID string) catalog.VersionedDefinition {
	t.Helper()

	source, err := catalog.NewVersionedDefinitionFromStrings(shipID, ShipCatalogVersion.String())
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings(%q) error = %v", shipID, err)
	}
	return source
}
