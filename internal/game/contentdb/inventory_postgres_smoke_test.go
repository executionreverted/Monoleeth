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
