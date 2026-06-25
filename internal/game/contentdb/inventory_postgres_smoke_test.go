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

func TestPostgresInventoryStorePersistsAddItemReferenceAcrossServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-add-ref-smoke")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	service, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, inventoryStore)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	referenceKey, err := foundation.ShopPurchaseIdempotencyKey(playerID, foundation.RequestID("request-postgres-add-ref"))
	if err != nil {
		t.Fatalf("ShopPurchaseIdempotencyKey() error = %v, want nil", err)
	}
	input := economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: postgresInstanceDefinitionForTest(t),
		ItemInstanceID: foundation.ItemID("coordinate_scroll_t1-instance-postgres-ref"),
		Quantity:       1,
		Location:       economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")},
		Reason:         economy.LedgerReason("postgres_inventory_add_ref_smoke"),
		ReferenceKey:   referenceKey,
	}
	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem(instance) error = %v, want nil", err)
	}

	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reopen) error = %v, want nil", err)
	}
	second, err := reloaded.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem(instance) after reload error = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate AddItem after reload Duplicate = false, want true")
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate ledger = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
	items := reloaded.InstanceItems()
	if len(items) != 1 {
		t.Fatalf("InstanceItems() len = %d, want 1", len(items))
	}
}

func TestPostgresInventoryStorePersistsMoveItemReferenceAcrossServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-move-ref")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	service, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, inventoryStore)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	definition := postgresStackableDefinitionForTest(t)
	account := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")}
	station := economy.ItemLocation{Kind: economy.LocationKindStationStorage, ID: economy.LocationID("station-postgres-move")}
	addInput := economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       75,
		Location:       account,
		Reason:         economy.LedgerReason("postgres_inventory_move_seed"),
		ReferenceKey:   postgresInventoryReferenceKey(t, "loot_pickup:postgres-move-seed"),
	}
	if _, err := service.AddItem(addInput); err != nil {
		t.Fatalf("AddItem(stackable seed) error = %v, want nil", err)
	}
	moveInput := economy.MoveItemInput{
		PlayerID: playerID,
		ItemRef: economy.MoveItemRef{
			Definition: definition,
		},
		FromLocation: account,
		ToLocation:   station,
		Quantity:     30,
		Reason:       economy.LedgerReason("postgres_inventory_move_ref"),
		ReferenceKey: postgresInventoryReferenceKey(t, "loot_pickup:postgres-move-ref"),
	}
	first, err := service.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("first MoveItem() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reopen) error = %v, want nil", err)
	}
	duplicate, err := reloaded.MoveItem(moveInput)
	if err != nil {
		t.Fatalf("duplicate MoveItem() after reload error = %v, want nil", err)
	}

	if !duplicate.Duplicate {
		t.Fatal("duplicate MoveItem after reload Duplicate = false, want true")
	}
	if duplicate.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID ||
		duplicate.LedgerEntries[1].LedgerID != first.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate move ledgers = %+v, want %+v", duplicate.LedgerEntries, first.LedgerEntries)
	}
	if got := reloaded.TotalItemQuantity(playerID, definition.ItemID, account); got != 45 {
		t.Fatalf("account TotalItemQuantity() after duplicate reload = %d, want 45", got)
	}
	if got := reloaded.TotalItemQuantity(playerID, definition.ItemID, station); got != 30 {
		t.Fatalf("station TotalItemQuantity() after duplicate reload = %d, want 30", got)
	}
	if got := len(reloaded.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ItemLedgerEntries() len after duplicate reload = %d, want 3", got)
	}
}

func TestPostgresInventoryStorePersistsRemoveItemReferenceAcrossServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-remove-ref")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	service, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, inventoryStore)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	definition := postgresStackableDefinitionForTest(t)
	account := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")}
	addInput := economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       75,
		Location:       account,
		Reason:         economy.LedgerReason("postgres_inventory_remove_seed"),
		ReferenceKey:   postgresInventoryReferenceKey(t, "loot_pickup:postgres-remove-seed"),
	}
	if _, err := service.AddItem(addInput); err != nil {
		t.Fatalf("AddItem(stackable seed) error = %v, want nil", err)
	}
	removeInput := economy.RemoveItemInput{
		PlayerID: playerID,
		ItemRef: economy.RemoveItemRef{
			Definition: definition,
		},
		SourceLocation: account,
		Quantity:       30,
		Reason:         economy.LedgerReason("postgres_inventory_remove_ref"),
		ReferenceKey:   postgresInventoryReferenceKey(t, "loot_pickup:postgres-remove-ref"),
	}
	first, err := service.RemoveItem(removeInput)
	if err != nil {
		t.Fatalf("first RemoveItem() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reopen) error = %v, want nil", err)
	}
	duplicate, err := reloaded.RemoveItem(removeInput)
	if err != nil {
		t.Fatalf("duplicate RemoveItem() after reload error = %v, want nil", err)
	}

	if !duplicate.Duplicate {
		t.Fatal("duplicate RemoveItem after reload Duplicate = false, want true")
	}
	if duplicate.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate remove ledger = %q, want %q", duplicate.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if got := reloaded.TotalItemQuantity(playerID, definition.ItemID, account); got != 45 {
		t.Fatalf("account TotalItemQuantity() after duplicate reload = %d, want 45", got)
	}
	if got := len(reloaded.ItemLedgerEntries()); got != 2 {
		t.Fatalf("ItemLedgerEntries() len after duplicate reload = %d, want 2", got)
	}
}

func TestPostgresInventoryStoreGeneratedInstanceIDDoesNotCollideAfterServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-instance-counter")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	service, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, inventoryStore)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	firstReference, err := foundation.ShopPurchaseIdempotencyKey(playerID, foundation.RequestID("request-postgres-counter-1"))
	if err != nil {
		t.Fatalf("ShopPurchaseIdempotencyKey(first) error = %v, want nil", err)
	}
	firstInput := economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: postgresInstanceDefinitionForTest(t),
		Quantity:       1,
		Location:       economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")},
		Reason:         economy.LedgerReason("postgres_inventory_instance_counter"),
		ReferenceKey:   firstReference,
	}
	first, err := service.AddItem(firstInput)
	if err != nil {
		t.Fatalf("first AddItem(instance) error = %v, want nil", err)
	}

	reopened, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewInventoryServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reopen) error = %v, want nil", err)
	}
	secondReference, err := foundation.ShopPurchaseIdempotencyKey(playerID, foundation.RequestID("request-postgres-counter-2"))
	if err != nil {
		t.Fatalf("ShopPurchaseIdempotencyKey(second) error = %v, want nil", err)
	}
	secondInput := firstInput
	secondInput.ReferenceKey = secondReference
	second, err := reloaded.AddItem(secondInput)
	if err != nil {
		t.Fatalf("second AddItem(instance) error = %v, want nil", err)
	}

	if first.InstanceItems[0].ItemInstanceID == second.InstanceItems[0].ItemInstanceID {
		t.Fatalf("generated instance ID after reload = %q, want different ID", second.InstanceItems[0].ItemInstanceID)
	}
	if got := len(reloaded.InstanceItems()); got != 2 {
		t.Fatalf("InstanceItems() len after second add = %d, want 2", got)
	}
}

func postgresStackableDefinitionForTest(t *testing.T) economy.ItemDefinition {
	t.Helper()
	maxStack, err := foundation.NewQuantity(100)
	if err != nil {
		t.Fatalf("NewQuantity(100) error = %v, want nil", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	definition, err := economy.NewItemDefinition(
		catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID("raw_ore"), Version: catalog.Version("test-v1")},
		foundation.ItemID("raw_ore"),
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition(stackable) error = %v, want nil", err)
	}
	return definition
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

func postgresInventoryReferenceKey(t *testing.T, value string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.ParseIdempotencyKey(value)
	if err != nil {
		t.Fatalf("ParseIdempotencyKey(%q) error = %v, want nil", value, err)
	}
	return key
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
