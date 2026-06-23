package server

import (
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestRuntimeClaimXCoreConsumerCopiesStorageMutationEvidence(t *testing.T) {
	inventory := economy.NewInventoryService(nil)
	playerID := foundation.PlayerID("player-runtime-xcore")
	planetID := foundation.PlanetID("planet-runtime-xcore")
	definition := runtimeClaimXCoreDefinitionForTest(t)
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("NewItemLocation: %v", err)
	}
	if _, err := inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       2,
		Location:       location,
		Reason:         "test_seed",
		ReferenceKey:   "loot_pickup:add-runtime-xcore",
	}); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	key, err := foundation.PlanetClaimIdempotencyKey(playerID, planetID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey: %v", err)
	}
	input := discovery.ClaimXCoreConsumeInput{
		PlayerID: playerID,
		PlanetID: planetID,
		ItemRef: economy.RemoveItemRef{
			Definition: definition,
		},
		SourceLocation: location,
		Quantity:       1,
		Reason:         economy.LedgerReason("planet_claim"),
		Reference:      discovery.PlanetClaimReference(key.String()),
	}
	consumer := runtimeClaimXCoreConsumer{inventory: inventory}

	result, err := consumer.ConsumeClaimXCore(input)
	if err != nil {
		t.Fatalf("ConsumeClaimXCore() error = %v, want nil", err)
	}
	if result.Duplicate || result.StorageMutation.Duplicate {
		t.Fatalf("first consume duplicate = %v/%v, want false", result.Duplicate, result.StorageMutation.Duplicate)
	}
	if len(result.StorageMutation.LedgerEntries) != 1 {
		t.Fatalf("ledger entries = %+v, want one", result.StorageMutation.LedgerEntries)
	}
	entry := result.StorageMutation.LedgerEntries[0]
	if entry.Action != economy.LedgerActionDecrease ||
		entry.PlayerID != playerID ||
		entry.ItemID != definition.ItemID ||
		entry.Quantity.Int64() != 1 ||
		entry.ReferenceKey != key ||
		entry.Location != location {
		t.Fatalf("ledger entry = %+v, want X Core decrease evidence for claim", entry)
	}
	if len(result.StorageMutation.StackableItems) != 1 ||
		result.StorageMutation.StackableItems[0].Quantity.Int64() != 1 {
		t.Fatalf("stackable evidence = %+v, want remaining X Core row", result.StorageMutation.StackableItems)
	}

	duplicate, err := consumer.ConsumeClaimXCore(input)
	if err != nil {
		t.Fatalf("duplicate ConsumeClaimXCore() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.StorageMutation.Duplicate {
		t.Fatalf("duplicate flags = %v/%v, want true", duplicate.Duplicate, duplicate.StorageMutation.Duplicate)
	}
	if len(duplicate.StorageMutation.LedgerEntries) != 1 ||
		duplicate.StorageMutation.LedgerEntries[0].LedgerID != entry.LedgerID {
		t.Fatalf("duplicate ledger evidence = %+v, want same ledger id %q", duplicate.StorageMutation.LedgerEntries, entry.LedgerID)
	}
}

func runtimeClaimXCoreDefinitionForTest(t *testing.T) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("x_core", "test")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings: %v", err)
	}
	maxStack, err := foundation.NewQuantity(99)
	if err != nil {
		t.Fatalf("NewQuantity(max stack): %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight): %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		"x_core",
		"X Core",
		economy.ItemTypeStackable,
		economy.ItemRarityRare,
		maxStack,
		weight,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition: %v", err)
	}
	return definition
}
