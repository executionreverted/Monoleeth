package contentdb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
)

func TestPostgresLoadoutStorePersistsSavedLoadoutAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-loadout-save")
	seedPostgresLoadoutPlayerShip(t, ctx, store, playerID)
	loadoutStore, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore() error = %v, want nil", err)
	}
	loadout := modules.Loadout{
		LoadoutID: modules.LoadoutID("starter-combat"),
		PlayerID:  playerID,
		ShipID:    ships.ShipIDStarter,
		Name:      "Starter Combat",
		SlotAssignments: modules.SlotAssignments{
			modules.ModuleSlotOffensive1: foundation.ItemID("laser-alpha-instance-postgres-save"),
		},
		CreatedAt: time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 25, 15, 1, 0, 0, time.UTC),
	}
	if err := loadoutStore.SaveLoadout(loadout); err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore(reopen) error = %v, want nil", err)
	}
	stored, err := reopened.Loadout(playerID, loadout.LoadoutID)
	if err != nil {
		t.Fatalf("Loadout(reopen) error = %v, want nil", err)
	}
	if stored.PlayerID != playerID || stored.ShipID != ships.ShipIDStarter || stored.Name != loadout.Name {
		t.Fatalf("stored loadout = %+v, want player %q ship %q name %q", stored, playerID, ships.ShipIDStarter, loadout.Name)
	}
	if got := stored.SlotAssignments[modules.ModuleSlotOffensive1]; got != loadout.SlotAssignments[modules.ModuleSlotOffensive1] {
		t.Fatalf("stored offensive slot = %q, want %q", got, loadout.SlotAssignments[modules.ModuleSlotOffensive1])
	}
}

func TestPostgresLoadoutStorePersistsEquippedModuleAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-loadout-equipped")
	seedPostgresLoadoutPlayerShip(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	laser := postgresModuleInstanceItemForTest(t, playerID, foundation.ItemID("laser-alpha-instance-postgres-equipped"), foundation.ItemID("laser_alpha_t1"), economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(playerID.String()),
	}, 100)
	if err := inventoryStore.UpsertInstanceItem(ctx, laser); err != nil {
		t.Fatalf("UpsertInstanceItem(module) error = %v, want nil", err)
	}
	loadoutStore, err := contentdb.NewLoadoutStoreWithItemMover(store, postgresLoadoutItemMover{
		ctx:       ctx,
		inventory: inventoryStore,
		now:       time.Date(2026, 6, 25, 15, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewLoadoutStoreWithItemMover() error = %v, want nil", err)
	}
	equippedAt := time.Date(2026, 6, 25, 15, 4, 0, 0, time.UTC)
	if err := loadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    ships.ShipIDStarter,
		RequestID: foundation.RequestID("request-postgres-loadout-equip"),
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         ships.ShipIDStarter,
			SlotID:         modules.ModuleSlotOffensive1,
			ItemInstanceID: laser.ItemInstanceID,
			EquippedAt:     equippedAt,
		}},
	}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore(reopen) error = %v, want nil", err)
	}
	equipped, err := reopened.EquippedModules(playerID, ships.ShipIDStarter)
	if err != nil {
		t.Fatalf("EquippedModules(reopen) error = %v, want nil", err)
	}
	if len(equipped) != 1 || equipped[0].SlotID != modules.ModuleSlotOffensive1 || equipped[0].ItemInstanceID != laser.ItemInstanceID {
		t.Fatalf("equipped = %+v, want offensive laser", equipped)
	}
	item, err := reopened.ModuleItem(laser.ItemInstanceID)
	if err != nil {
		t.Fatalf("ModuleItem(reopen equipped) error = %v, want nil", err)
	}
	if item.Location != (economy.ItemLocation{Kind: economy.LocationKindShipEquipped, ID: economy.LocationID(ships.ShipIDStarter.String())}) {
		t.Fatalf("equipped item location = %s, want ship equipped starter", item.Location.String())
	}
}

func TestPostgresLoadoutStoreModuleItemReadsDurableInventoryInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-loadout-item")
	seedPostgresLoadoutPlayerShip(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	scanner := postgresModuleInstanceItemForTest(t, playerID, foundation.ItemID("scanner-instance-postgres-lookup"), foundation.ItemID("scanner_t1"), economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(playerID.String()),
	}, 75)
	if err := inventoryStore.UpsertInstanceItem(ctx, scanner); err != nil {
		t.Fatalf("UpsertInstanceItem(scanner) error = %v, want nil", err)
	}

	loadoutStore, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore() error = %v, want nil", err)
	}
	item, err := loadoutStore.ModuleItem(scanner.ItemInstanceID)
	if err != nil {
		t.Fatalf("ModuleItem() error = %v, want nil", err)
	}
	if item.ItemInstanceID != scanner.ItemInstanceID || item.ItemID != scanner.ItemID || item.OwnerPlayerID != playerID || item.DurabilityCurrent != scanner.DurabilityCurrent {
		t.Fatalf("item = %+v, want durable scanner %+v", item, scanner)
	}
}

func TestPostgresLoadoutStoreMissingAndInvalidModuleItemFailsSafe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-loadout-invalid")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	loadoutStore, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore() error = %v, want nil", err)
	}

	if _, err := loadoutStore.ModuleItem(foundation.ItemID("missing-module-instance")); !errors.Is(err, modules.ErrUnknownModuleItem) {
		t.Fatalf("missing ModuleItem error = %v, want ErrUnknownModuleItem", err)
	}
	if _, err := loadoutStore.ModuleItem(""); err == nil {
		t.Fatal("invalid ModuleItem error = nil, want validation error")
	}
	insertInvalidPostgresModuleInstanceRow(t, ctx, db, playerID, foundation.ItemID("invalid-location-module-instance"))
	if _, err := loadoutStore.ModuleItem(foundation.ItemID("invalid-location-module-instance")); !errors.Is(err, economy.ErrInvalidLocationKind) {
		t.Fatalf("invalid row ModuleItem error = %v, want ErrInvalidLocationKind", err)
	}
}

func TestPostgresLoadoutStoreReplaceRequiresMoverForLocationPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-loadout-mover-required")
	seedPostgresLoadoutPlayerShip(t, ctx, store, playerID)
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}
	laser := postgresModuleInstanceItemForTest(t, playerID, foundation.ItemID("laser-alpha-instance-postgres-mover-required"), foundation.ItemID("laser_alpha_t1"), economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(playerID.String()),
	}, 100)
	if err := inventoryStore.UpsertInstanceItem(ctx, laser); err != nil {
		t.Fatalf("UpsertInstanceItem(module) error = %v, want nil", err)
	}
	loadoutStore, err := contentdb.NewLoadoutStore(store)
	if err != nil {
		t.Fatalf("NewLoadoutStore() error = %v, want nil", err)
	}

	err = loadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    ships.ShipIDStarter,
		RequestID: foundation.RequestID("request-postgres-loadout-missing-mover"),
		Equipped: []modules.EquippedModule{{
			PlayerID:       playerID,
			ShipID:         ships.ShipIDStarter,
			SlotID:         modules.ModuleSlotOffensive1,
			ItemInstanceID: laser.ItemInstanceID,
			EquippedAt:     time.Date(2026, 6, 25, 15, 10, 0, 0, time.UTC),
		}},
	})
	if !errors.Is(err, contentdb.ErrLoadoutItemMoverRequired) {
		t.Fatalf("ReplaceEquippedModules(no mover) error = %v, want ErrLoadoutItemMoverRequired", err)
	}
	equipped, err := loadoutStore.EquippedModules(playerID, ships.ShipIDStarter)
	if err != nil {
		t.Fatalf("EquippedModules(after failed replace) error = %v, want nil", err)
	}
	if len(equipped) != 0 {
		t.Fatalf("equipped after failed replace = %+v, want none", equipped)
	}
	item, err := loadoutStore.ModuleItem(laser.ItemInstanceID)
	if err != nil {
		t.Fatalf("ModuleItem(after failed replace) error = %v, want nil", err)
	}
	if item.Location != laser.Location {
		t.Fatalf("item location after failed replace = %s, want %s", item.Location.String(), laser.Location.String())
	}
}

type postgresLoadoutItemMover struct {
	ctx       context.Context
	inventory *contentdb.InventoryStore
	now       time.Time
}

func (mover postgresLoadoutItemMover) MoveModuleItemLocations(moves []modules.ModuleItemLocationMove) ([]modules.ModuleItemLocationMoveResult, error) {
	results := make([]modules.ModuleItemLocationMoveResult, 0, len(moves))
	for _, move := range moves {
		item, ok, err := mover.inventory.LoadInstanceItem(mover.ctx, move.ItemInstanceID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, modules.ErrUnknownModuleItem
		}
		if item.Location != move.FromLocation {
			return nil, modules.ErrInvalidModuleItemLocation
		}
		item.Location = move.ToLocation
		item.UpdatedAt = mover.now
		if err := mover.inventory.UpsertInstanceItem(mover.ctx, item); err != nil {
			return nil, err
		}
		results = append(results, modules.ModuleItemLocationMoveResult{})
	}
	return results, nil
}

func seedPostgresLoadoutPlayerShip(t *testing.T, ctx context.Context, store *contentdb.Store, playerID foundation.PlayerID) {
	t.Helper()
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	service := newPostgresHangarService(t, playerID, mustPostgresHangarStore(t, store))
	if _, err := service.EnsureStarterShip(playerID); err != nil {
		t.Fatalf("EnsureStarterShip(loadout seed) error = %v, want nil", err)
	}
}

func mustPostgresHangarStore(t *testing.T, store *contentdb.Store) *contentdb.HangarStore {
	t.Helper()
	hangarStore, err := contentdb.NewHangarStore(store)
	if err != nil {
		t.Fatalf("NewHangarStore(loadout seed) error = %v, want nil", err)
	}
	return hangarStore
}

func insertInvalidPostgresModuleInstanceRow(t *testing.T, ctx context.Context, db *sql.DB, playerID foundation.PlayerID, itemInstanceID foundation.ItemID) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO player_inventory_instance_items(item_instance_id, player_id, item_id, location, location_kind, location_id, quantity, durability_current, bound_state, source_definition_id, source_version, created_at, updated_at)
		VALUES ($1, $2, 'laser_alpha_t1', 'bad_location:bad', 'bad_location', 'bad', 1, 100, 'unbound', 'laser_alpha_t1', 'test-v1', $3, $3)
	`, itemInstanceID.String(), playerID.String(), time.Date(2026, 6, 25, 15, 6, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("insert invalid module instance row error = %v, want nil", err)
	}
}

func postgresModuleInstanceItemForTest(
	t *testing.T,
	playerID foundation.PlayerID,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	location economy.ItemLocation,
	durability int64,
) economy.InstanceItem {
	t.Helper()
	definition, ok := modules.MustMVPCatalog().Lookup(itemID)
	if !ok {
		t.Fatalf("module %q missing from MVP catalog", itemID)
	}
	quantity, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	item, err := economy.NewInstanceItem(definition.Source, itemInstanceID, itemID, playerID, location, quantity)
	if err != nil {
		t.Fatalf("NewInstanceItem(module) error = %v, want nil", err)
	}
	item.DurabilityCurrent = durability
	item.CreatedAt = time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	item.UpdatedAt = item.CreatedAt
	if err := item.Validate(); err != nil {
		t.Fatalf("module item Validate() error = %v, want nil", err)
	}
	return item
}
