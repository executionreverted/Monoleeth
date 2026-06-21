package economy

import "testing"

func validMoveItemInput(t *testing.T) MoveItemInput {
	t.Helper()

	return MoveItemInput{
		PlayerID: "player-1",
		ItemRef: MoveItemRef{
			Definition: validStackableDefinition(t),
		},
		FromLocation: validLocation(t),
		ToLocation:   validStationStorageLocation(t),
		Quantity:     1,
		Reason:       "inventory_move",
		ReferenceKey: validReferenceKey(t, "loot_pickup:move-1"),
	}
}

func validStationStorageLocation(t *testing.T) ItemLocation {
	t.Helper()

	return validLocationKind(t, LocationKindStationStorage, "station-1")
}

func addStackableItems(
	t *testing.T,
	service *InventoryService,
	definition ItemDefinition,
	quantity int64,
	location ItemLocation,
	reference string,
) AddItemResult {
	t.Helper()

	input := validAddItemInput(t)
	input.ItemDefinition = definition
	input.Quantity = quantity
	input.Location = location
	input.ReferenceKey = validReferenceKey(t, reference)
	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem stackable setup: %v", err)
	}
	return result
}

func addInstanceItems(
	t *testing.T,
	service *InventoryService,
	definition ItemDefinition,
	quantity int64,
	location ItemLocation,
	reference string,
) AddItemResult {
	t.Helper()

	input := validAddItemInput(t)
	input.ItemDefinition = definition
	input.Quantity = quantity
	input.Location = location
	input.ReferenceKey = validReferenceKey(t, reference)
	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem instance setup: %v", err)
	}
	return result
}

func seedStackableItem(
	t *testing.T,
	service *InventoryService,
	definition ItemDefinition,
	quantity int64,
	location ItemLocation,
) {
	t.Helper()

	item, err := NewStackableItem(
		definition.Source,
		"seed-stackable-1",
		definition.ItemID,
		"player-1",
		location,
		validQuantity(t, quantity),
	)
	if err != nil {
		t.Fatalf("NewStackableItem seed: %v", err)
	}
	service.stackableItems = append(service.stackableItems, item)
}

func validInstanceDefinition(t *testing.T) ItemDefinition {
	t.Helper()

	source := validItemSource(t, "laser_module")
	maxStack := validQuantity(t, 1)
	weight := validQuantity(t, 3)
	definition, err := NewItemDefinition(
		source,
		"laser_module",
		"Laser Module",
		ItemTypeInstance,
		ItemRarityRare,
		maxStack,
		weight,
		[]TradeFlag{TradeFlagTradeable},
		[]BindRule{BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition valid instance: %v", err)
	}
	return definition
}

func validShipCargoLocation(t *testing.T) ItemLocation {
	t.Helper()

	return validLocationKind(t, LocationKindShipCargo, "ship-1")
}

func validShipEquippedLocation(t *testing.T) ItemLocation {
	t.Helper()

	return validLocationKind(t, LocationKindShipEquipped, "ship-1")
}

func validLocationKind(t *testing.T, kind LocationKind, id string) ItemLocation {
	t.Helper()

	location, err := NewItemLocation(kind, id)
	if err != nil {
		t.Fatalf("NewItemLocation(%q, %q): %v", kind, id, err)
	}
	return location
}
