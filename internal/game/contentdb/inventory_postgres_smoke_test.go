package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestPostgresInventoryStorePersistsStackableAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	seedPostgresWalletPlayer(t, ctx, store, "player-postgres-inventory-smoke")
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	item := postgresStackableItemForTest(t)
	if err := inventoryStore.UpsertStackableItem(ctx, item); err != nil {
		t.Fatalf("UpsertStackableItem() error = %v, want nil", err)
	}
	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	items, err := reopened.LoadStackableItems(ctx)
	if err != nil {
		t.Fatalf("LoadStackableItems() error = %v, want nil", err)
	}
	if len(items) != 1 || items[0].OwnerPlayerID != item.OwnerPlayerID || items[0].ItemID != item.ItemID || items[0].Quantity != item.Quantity {
		t.Fatalf("items = %+v, want persisted item %+v", items, item)
	}
}

func TestPostgresInventoryStorePersistsInstanceAddItemAcrossServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-instance-smoke")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	service, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, inventoryStore)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	referenceKey, err := foundation.ShopPurchaseIdempotencyKey(playerID, foundation.RequestID("request-postgres-instance"))
	if err != nil {
		t.Fatalf("ShopPurchaseIdempotencyKey() error = %v, want nil", err)
	}
	input := economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: postgresInstanceDefinitionForTest(t),
		ItemInstanceID: foundation.ItemID("coordinate_scroll_t1-instance-postgres"),
		Quantity:       1,
		Location:       economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")},
		Reason:         economy.LedgerReason("postgres_inventory_instance_smoke"),
		ReferenceKey:   referenceKey,
	}
	if _, err := service.AddItem(input); err != nil {
		t.Fatalf("AddItem(instance) error = %v, want nil", err)
	}

	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reopen) error = %v, want nil", err)
	}

	items := reloaded.InstanceItems()
	if len(items) != 1 || items[0].ItemInstanceID != input.ItemInstanceID || items[0].OwnerPlayerID != playerID || items[0].ItemID != input.ItemDefinition.ItemID {
		t.Fatalf("InstanceItems() = %+v, want persisted instance %s", items, input.ItemInstanceID)
	}
}

func postgresStackableItemForTest(t *testing.T) economy.StackableItem {
	t.Helper()
	quantity, err := foundation.NewQuantity(12)
	if err != nil {
		t.Fatalf("NewQuantity() error = %v, want nil", err)
	}
	item, err := economy.NewStackableItem(
		catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID("raw_ore"), Version: catalog.Version("test-v1")},
		foundation.ItemID("raw_ore-stack-postgres"),
		foundation.ItemID("raw_ore"),
		foundation.PlayerID("player-postgres-inventory-smoke"),
		economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")},
		quantity,
	)
	if err != nil {
		t.Fatalf("NewStackableItem() error = %v, want nil", err)
	}
	item.CreatedAt = time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)
	item.UpdatedAt = time.Date(2026, 6, 25, 14, 1, 0, 0, time.UTC)
	return item
}

func postgresInstanceDefinitionForTest(t *testing.T) economy.ItemDefinition {
	t.Helper()
	one, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	weight, err := foundation.NewQuantity(3)
	if err != nil {
		t.Fatalf("NewQuantity(3) error = %v, want nil", err)
	}
	definition, err := economy.NewItemDefinition(
		catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID("coordinate_scroll_t1"), Version: catalog.Version("test-v1")},
		foundation.ItemID("coordinate_scroll_t1"),
		"Coordinate Scroll T1",
		economy.ItemTypeInstance,
		economy.ItemRarityRare,
		one,
		weight,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition(instance) error = %v, want nil", err)
	}
	return definition
}
