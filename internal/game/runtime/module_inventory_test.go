package runtime

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/testutil"
)

func TestModuleInventoryLedgerAdapterMovesItemsDuringLoadoutApply(t *testing.T) {
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")
	moduleCatalog := modules.MustMVPCatalog()
	inventory := economy.NewInventoryService(testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)))
	adapter, err := NewModuleInventoryLedgerAdapter(inventory, moduleCatalog)
	if err != nil {
		t.Fatalf("NewModuleInventoryLedgerAdapter() error = %v, want nil", err)
	}
	store := modules.NewInMemoryLoadoutStoreWithItemMover(adapter)
	service := newRuntimeLoadoutService(t, store, moduleCatalog)

	laser := addRuntimeModuleItem(t, inventory, store, moduleCatalog, "laser_alpha_t1", playerID, "seed-laser")
	shield := addRuntimeModuleItem(t, inventory, store, moduleCatalog, "shield_generator_t1", playerID, "seed-shield")
	scanner := addRuntimeModuleItem(t, inventory, store, moduleCatalog, "scanner_t1", playerID, "seed-scanner")
	if err := store.SetActiveShip(playerID, shipID); err != nil {
		t.Fatalf("SetActiveShip() error = %v, want nil", err)
	}
	if err := store.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    shipID,
		RequestID: "seed-current-loadout",
		Equipped: []modules.EquippedModule{
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotOffensive1,
				ItemInstanceID: laser.ItemInstanceID,
				EquippedAt:     time.Date(2026, 6, 18, 12, 1, 0, 0, time.UTC),
			},
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotDefensive1,
				ItemInstanceID: shield.ItemInstanceID,
				EquippedAt:     time.Date(2026, 6, 18, 12, 1, 0, 0, time.UTC),
			},
		},
	}); err != nil {
		t.Fatalf("seed ReplaceEquippedModules() error = %v, want nil", err)
	}

	if _, err := service.SaveLoadout(modules.SaveLoadoutInput{
		LoadoutID: "scan-fit",
		PlayerID:  playerID,
		ShipID:    shipID,
		Name:      "Scan Fit",
		SlotAssignments: modules.SlotAssignments{
			modules.ModuleSlotOffensive1: laser.ItemInstanceID,
			modules.ModuleSlotUtility1:   scanner.ItemInstanceID,
		},
	}); err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}

	beforeLedgerCount := len(inventory.ItemLedgerEntries())
	result, err := service.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  playerID,
		LoadoutID: "scan-fit",
		RequestID: "apply-scan-fit-1",
	})
	if err != nil {
		t.Fatalf("ApplyLoadout() error = %v, want nil", err)
	}
	if result.Noop {
		t.Fatal("ApplyLoadout() Noop = true, want false")
	}
	if got, want := len(result.Equipped), 1; got != want {
		t.Fatalf("Equipped len = %d, want %d", got, want)
	}
	if result.Equipped[0].ItemInstanceID != scanner.ItemInstanceID {
		t.Fatalf("Equipped item = %q, want %q", result.Equipped[0].ItemInstanceID, scanner.ItemInstanceID)
	}
	if got, want := len(result.Unequipped), 1; got != want {
		t.Fatalf("Unequipped len = %d, want %d", got, want)
	}
	if result.Unequipped[0].ItemInstanceID != shield.ItemInstanceID {
		t.Fatalf("Unequipped item = %q, want %q", result.Unequipped[0].ItemInstanceID, shield.ItemInstanceID)
	}

	assertRuntimeInventoryQuantity(t, inventory, playerID, shield.ItemID, runtimeAccountLocation(playerID), 1)
	assertRuntimeInventoryQuantity(t, inventory, playerID, shield.ItemID, runtimeEquippedLocation(shipID), 0)
	assertRuntimeInventoryQuantity(t, inventory, playerID, scanner.ItemID, runtimeAccountLocation(playerID), 0)
	assertRuntimeInventoryQuantity(t, inventory, playerID, scanner.ItemID, runtimeEquippedLocation(shipID), 1)
	assertRuntimeInventoryQuantity(t, inventory, playerID, laser.ItemID, runtimeEquippedLocation(shipID), 1)

	entries := inventory.ItemLedgerEntries()[beforeLedgerCount:]
	if got, want := len(entries), 4; got != want {
		t.Fatalf("loadout ledger entries = %d, want %d", got, want)
	}
	wantUnequipRef, err := foundation.ModuleUnequipIdempotencyKey(playerID, shipID, shield.ItemInstanceID, "apply-scan-fit-1")
	if err != nil {
		t.Fatalf("ModuleUnequipIdempotencyKey() error = %v, want nil", err)
	}
	wantEquipRef, err := foundation.ModuleEquipIdempotencyKey(playerID, shipID, scanner.ItemInstanceID, "apply-scan-fit-1")
	if err != nil {
		t.Fatalf("ModuleEquipIdempotencyKey() error = %v, want nil", err)
	}
	for index, entry := range entries[:2] {
		if entry.Reason != modules.LedgerReasonModuleUnequip || entry.ReferenceKey != wantUnequipRef {
			t.Fatalf("unequip ledger[%d] = reason %q ref %q, want %q %q", index, entry.Reason, entry.ReferenceKey, modules.LedgerReasonModuleUnequip, wantUnequipRef)
		}
	}
	for index, entry := range entries[2:] {
		if entry.Reason != modules.LedgerReasonModuleEquip || entry.ReferenceKey != wantEquipRef {
			t.Fatalf("equip ledger[%d] = reason %q ref %q, want %q %q", index, entry.Reason, entry.ReferenceKey, modules.LedgerReasonModuleEquip, wantEquipRef)
		}
	}

	retry, err := service.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  playerID,
		LoadoutID: "scan-fit",
		RequestID: "apply-scan-fit-1",
	})
	if err != nil {
		t.Fatalf("retry ApplyLoadout() error = %v, want nil", err)
	}
	if !retry.Noop {
		t.Fatal("retry Noop = false, want true")
	}
	if got, want := len(inventory.ItemLedgerEntries()), beforeLedgerCount+4; got != want {
		t.Fatalf("ledger count after retry = %d, want %d", got, want)
	}
}

func TestModuleInventoryLedgerAdapterFailureLeavesLoadoutStoreUnchanged(t *testing.T) {
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")
	moduleCatalog := modules.MustMVPCatalog()
	inventory := economy.NewInventoryService(testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)))
	adapter, err := NewModuleInventoryLedgerAdapter(inventory, moduleCatalog)
	if err != nil {
		t.Fatalf("NewModuleInventoryLedgerAdapter() error = %v, want nil", err)
	}
	store := modules.NewInMemoryLoadoutStoreWithItemMover(adapter)
	service := newRuntimeLoadoutService(t, store, moduleCatalog)

	laser := addRuntimeModuleItem(t, inventory, store, moduleCatalog, "laser_alpha_t1", playerID, "seed-laser")
	shield := addRuntimeModuleItem(t, inventory, store, moduleCatalog, "shield_generator_t1", playerID, "seed-shield")
	missingScanner := runtimeModuleItemSnapshot(t, moduleCatalog, "scanner_t1", "scanner_t1-instance-missing", playerID, runtimeAccountLocation(playerID))
	if err := store.PutModuleItem(missingScanner); err != nil {
		t.Fatalf("PutModuleItem(missing scanner) error = %v, want nil", err)
	}
	if err := store.SetActiveShip(playerID, shipID); err != nil {
		t.Fatalf("SetActiveShip() error = %v, want nil", err)
	}
	if err := store.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    shipID,
		RequestID: "seed-current-loadout",
		Equipped: []modules.EquippedModule{
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotOffensive1,
				ItemInstanceID: laser.ItemInstanceID,
				EquippedAt:     time.Date(2026, 6, 18, 12, 1, 0, 0, time.UTC),
			},
			{
				PlayerID:       playerID,
				ShipID:         shipID,
				SlotID:         modules.ModuleSlotDefensive1,
				ItemInstanceID: shield.ItemInstanceID,
				EquippedAt:     time.Date(2026, 6, 18, 12, 1, 0, 0, time.UTC),
			},
		},
	}); err != nil {
		t.Fatalf("seed ReplaceEquippedModules() error = %v, want nil", err)
	}

	if _, err := service.SaveLoadout(modules.SaveLoadoutInput{
		LoadoutID: "broken-scan-fit",
		PlayerID:  playerID,
		ShipID:    shipID,
		Name:      "Broken Scan Fit",
		SlotAssignments: modules.SlotAssignments{
			modules.ModuleSlotOffensive1: laser.ItemInstanceID,
			modules.ModuleSlotUtility1:   missingScanner.ItemInstanceID,
		},
	}); err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}

	beforeLedgerCount := len(inventory.ItemLedgerEntries())
	_, err = service.ApplyLoadout(modules.ApplyLoadoutInput{
		PlayerID:  playerID,
		LoadoutID: "broken-scan-fit",
		RequestID: "apply-broken-scan-fit-1",
	})
	if !errors.Is(err, economy.ErrItemNotOwned) {
		t.Fatalf("ApplyLoadout() error = %v, want ErrItemNotOwned", err)
	}
	if got, want := len(inventory.ItemLedgerEntries()), beforeLedgerCount; got != want {
		t.Fatalf("ledger count after failed apply = %d, want %d", got, want)
	}
	equipped, err := store.EquippedModules(playerID, shipID)
	if err != nil {
		t.Fatalf("EquippedModules() error = %v, want nil", err)
	}
	if got, want := len(equipped), 2; got != want {
		t.Fatalf("equipped len after failed apply = %d, want %d", got, want)
	}
	equippedItems := map[foundation.ItemID]struct{}{
		equipped[0].ItemInstanceID: {},
		equipped[1].ItemInstanceID: {},
	}
	if _, ok := equippedItems[laser.ItemInstanceID]; !ok {
		t.Fatalf("laser missing from equipped after failed apply: %+v", equipped)
	}
	if _, ok := equippedItems[shield.ItemInstanceID]; !ok {
		t.Fatalf("shield missing from equipped after failed apply: %+v", equipped)
	}
	assertRuntimeInventoryQuantity(t, inventory, playerID, shield.ItemID, runtimeEquippedLocation(shipID), 1)
	assertRuntimeInventoryQuantity(t, inventory, playerID, shield.ItemID, runtimeAccountLocation(playerID), 0)
	storedScanner, err := store.ModuleItem(missingScanner.ItemInstanceID)
	if err != nil {
		t.Fatalf("ModuleItem(scanner) error = %v, want nil", err)
	}
	if storedScanner.Location != runtimeAccountLocation(playerID) {
		t.Fatalf("scanner location after failed apply = %s, want %s", storedScanner.Location.String(), runtimeAccountLocation(playerID).String())
	}
}

func newRuntimeLoadoutService(
	t *testing.T,
	store *modules.InMemoryLoadoutStore,
	moduleCatalog modules.Catalog,
) modules.LoadoutService {
	t.Helper()
	service, err := modules.NewLoadoutService(
		moduleCatalog,
		store,
		modules.StaticShipSlotLayoutProvider{
			"ship-1": {Offensive: 2, Defensive: 1, Utility: 1},
		},
		modules.StaticPilotProgressionProvider{
			"player-1": {
				Rank: 1,
				RoleLevels: map[modules.PilotRole]int{
					modules.PilotRoleCombat: 1,
					modules.PilotRoleScout:  1,
				},
			},
		},
		testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 2, 0, 0, time.UTC)),
	)
	if err != nil {
		t.Fatalf("NewLoadoutService() error = %v, want nil", err)
	}
	return service
}

func addRuntimeModuleItem(
	t *testing.T,
	inventory *economy.InventoryService,
	store *modules.InMemoryLoadoutStore,
	moduleCatalog modules.Catalog,
	itemID foundation.ItemID,
	playerID foundation.PlayerID,
	seedRef string,
) economy.InstanceItem {
	t.Helper()
	definition, ok := moduleCatalog.Lookup(itemID)
	if !ok {
		t.Fatalf("module catalog missing %q", itemID)
	}
	itemDefinition, err := moduleItemDefinition(definition)
	if err != nil {
		t.Fatalf("moduleItemDefinition() error = %v, want nil", err)
	}
	reference, err := foundation.AdminCompensationIdempotencyKey(itemID.String(), seedRef)
	if err != nil {
		t.Fatalf("AdminCompensationIdempotencyKey() error = %v, want nil", err)
	}
	result, err := inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: itemDefinition,
		Quantity:       1,
		Location:       runtimeAccountLocation(playerID),
		Reason:         economy.LedgerReason("test_seed_module"),
		ReferenceKey:   reference,
	})
	if err != nil {
		t.Fatalf("AddItem(%q) error = %v, want nil", itemID, err)
	}
	if got, want := len(result.InstanceItems), 1; got != want {
		t.Fatalf("AddItem(%q) instances = %d, want %d", itemID, got, want)
	}
	item := result.InstanceItems[0]
	item.DurabilityCurrent = definition.Durability.Max
	if err := store.PutModuleItem(item); err != nil {
		t.Fatalf("PutModuleItem(%q) error = %v, want nil", item.ItemInstanceID, err)
	}
	return item
}

func runtimeModuleItemSnapshot(
	t *testing.T,
	moduleCatalog modules.Catalog,
	itemID foundation.ItemID,
	itemInstanceID foundation.ItemID,
	playerID foundation.PlayerID,
	location economy.ItemLocation,
) economy.InstanceItem {
	t.Helper()
	definition, ok := moduleCatalog.Lookup(itemID)
	if !ok {
		t.Fatalf("module catalog missing %q", itemID)
	}
	one, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	item := economy.InstanceItem{
		Source:            definition.Source,
		ItemInstanceID:    itemInstanceID,
		ItemID:            itemID,
		OwnerPlayerID:     playerID,
		Location:          location,
		Quantity:          one,
		DurabilityCurrent: definition.Durability.Max,
		BoundState:        economy.BoundStateUnbound,
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("runtime module item Validate() error = %v, want nil", err)
	}
	return item
}

func assertRuntimeInventoryQuantity(
	t *testing.T,
	inventory *economy.InventoryService,
	playerID foundation.PlayerID,
	itemID foundation.ItemID,
	location economy.ItemLocation,
	want int64,
) {
	t.Helper()
	if got := inventory.TotalItemQuantity(playerID, itemID, location); got != want {
		t.Fatalf("TotalItemQuantity(%q, %q, %s) = %d, want %d", playerID, itemID, location.String(), got, want)
	}
}

func runtimeAccountLocation(playerID foundation.PlayerID) economy.ItemLocation {
	return economy.ItemLocation{
		Kind: economy.LocationKindAccountInventory,
		ID:   economy.LocationID(playerID.String()),
	}
}

func runtimeEquippedLocation(shipID foundation.ShipID) economy.ItemLocation {
	return economy.ItemLocation{
		Kind: economy.LocationKindShipEquipped,
		ID:   economy.LocationID(shipID.String()),
	}
}
