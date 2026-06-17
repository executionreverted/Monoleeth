package modules

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestSaveLoadoutValidatesStoresAndClonesAssignments(t *testing.T) {
	service, store := newLoadoutTestService(t)
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")
	laser := testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100)
	putModuleItem(t, store, laser)

	assignments := SlotAssignments{ModuleSlotOffensive1: laser.ItemInstanceID}
	loadout, err := service.SaveLoadout(SaveLoadoutInput{
		LoadoutID:       "combat-alpha",
		PlayerID:        playerID,
		ShipID:          shipID,
		Name:            "Combat Alpha",
		SlotAssignments: assignments,
		PlayerRank:      1,
		RoleLevels:      map[PilotRole]int{PilotRoleCombat: 1},
	})
	if err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}
	if loadout.CreatedAt.IsZero() || loadout.UpdatedAt.IsZero() {
		t.Fatal("SaveLoadout() returned zero timestamps")
	}

	assignments[ModuleSlotOffensive1] = "mutated-instance"
	saved, err := store.Loadout(playerID, "combat-alpha")
	if err != nil {
		t.Fatalf("stored Loadout() error = %v, want nil", err)
	}
	if got := saved.SlotAssignments[ModuleSlotOffensive1]; got != laser.ItemInstanceID {
		t.Fatalf("stored assignment = %q, want %q", got, laser.ItemInstanceID)
	}
}

func TestSaveLoadoutRejectsInvalidModuleAssignments(t *testing.T) {
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")

	cases := []struct {
		name        string
		item        economy.InstanceItem
		assignments SlotAssignments
		playerRank  int
		roleLevels  map[PilotRole]int
		setup       func(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem)
		wantErr     error
	}{
		{
			name:        "wrong slot type",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{ModuleSlotUtility1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrWrongModuleSlotType,
		},
		{
			name:        "rank too low",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  0,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrPlayerRankTooLow,
		},
		{
			name:        "role too low",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 0},
			wantErr:     ErrPlayerRoleLevelTooLow,
		},
		{
			name:        "broken module",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 0),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrModuleBroken,
		},
		{
			name: "duplicate module instance",
			item: testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{
				ModuleSlotOffensive1: "laser-instance-1",
				ModuleSlotOffensive2: "laser-instance-1",
			},
			playerRank: 1,
			roleLevels: map[PilotRole]int{
				PilotRoleCombat: 1,
			},
			wantErr: ErrDuplicateModuleAssignment,
		},
		{
			name:        "not owner",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", "other-player", economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrModuleItemNotOwned,
		},
		{
			name:        "ship cargo location",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindShipCargo, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrInvalidModuleItemLocation,
		},
		{
			name:        "market escrow location",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindMarketEscrow, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrBlockedModuleItemLocation,
		},
		{
			name:        "crafting reserved location",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindCraftingReserved, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			wantErr:     ErrBlockedModuleItemLocation,
		},
		{
			name:        "already equipped on another ship",
			item:        testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100),
			assignments: SlotAssignments{ModuleSlotOffensive1: "laser-instance-1"},
			playerRank:  1,
			roleLevels:  map[PilotRole]int{PilotRoleCombat: 1},
			setup: func(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem) {
				t.Helper()
				err := store.ReplaceEquippedModules(playerID, "ship-2", []EquippedModule{{
					PlayerID:       playerID,
					ShipID:         "ship-2",
					SlotID:         ModuleSlotOffensive1,
					ItemInstanceID: item.ItemInstanceID,
					EquippedAt:     time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
				}})
				if err != nil {
					t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
				}
			},
			wantErr: ErrModuleItemAlreadyEquipped,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, store := newLoadoutTestService(t)
			putModuleItem(t, store, tc.item)
			if tc.setup != nil {
				tc.setup(t, store, tc.item)
			}

			_, err := service.SaveLoadout(SaveLoadoutInput{
				LoadoutID:       "combat-alpha",
				PlayerID:        playerID,
				ShipID:          shipID,
				Name:            "Combat Alpha",
				SlotAssignments: tc.assignments,
				PlayerRank:      tc.playerRank,
				RoleLevels:      tc.roleLevels,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("SaveLoadout() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestApplyLoadoutReplacesEquippedModulesAndReturnsInvalidations(t *testing.T) {
	service, store := newLoadoutTestService(t)
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")
	laser := testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100)
	shield := testModuleItem(t, "shield-instance-1", "shield_generator_t1", playerID, economy.LocationKindAccountInventory, 100)
	scanner := testModuleItem(t, "scanner-instance-1", "scanner_t1", playerID, economy.LocationKindAccountInventory, 100)
	putModuleItem(t, store, laser)
	putModuleItem(t, store, shield)
	putModuleItem(t, store, scanner)
	if err := store.SetActiveShip(playerID, shipID); err != nil {
		t.Fatalf("SetActiveShip() error = %v, want nil", err)
	}

	originalEquippedAt := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	err := store.ReplaceEquippedModules(playerID, shipID, []EquippedModule{
		{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         ModuleSlotOffensive1,
			ItemInstanceID: laser.ItemInstanceID,
			EquippedAt:     originalEquippedAt,
		},
		{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         ModuleSlotDefensive1,
			ItemInstanceID: shield.ItemInstanceID,
			EquippedAt:     originalEquippedAt,
		},
	})
	if err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}

	_, err = service.SaveLoadout(SaveLoadoutInput{
		LoadoutID: "scout-alpha",
		PlayerID:  playerID,
		ShipID:    shipID,
		Name:      "Scout Alpha",
		SlotAssignments: SlotAssignments{
			ModuleSlotOffensive1: laser.ItemInstanceID,
			ModuleSlotUtility1:   scanner.ItemInstanceID,
		},
		PlayerRank: 1,
		RoleLevels: map[PilotRole]int{
			PilotRoleCombat: 1,
			PilotRoleScout:  1,
		},
	})
	if err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}

	result, err := service.ApplyLoadout(ApplyLoadoutInput{
		PlayerID:   playerID,
		LoadoutID:  "scout-alpha",
		PlayerRank: 1,
		RoleLevels: map[PilotRole]int{
			PilotRoleCombat: 1,
			PilotRoleScout:  1,
		},
	})
	if err != nil {
		t.Fatalf("ApplyLoadout() error = %v, want nil", err)
	}
	if got, want := len(result.Current), 2; got != want {
		t.Fatalf("Current len = %d, want %d", got, want)
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
	if got, want := result.Current[0].EquippedAt, originalEquippedAt; !got.Equal(want) {
		t.Fatalf("kept module EquippedAt = %v, want %v", got, want)
	}

	wantReasons := []StatInvalidationReason{
		StatInvalidationReasonModuleUnequipped,
		StatInvalidationReasonModuleEquipped,
		StatInvalidationReasonLoadoutApplied,
	}
	if got, want := len(result.StatInvalidations), len(wantReasons); got != want {
		t.Fatalf("StatInvalidations len = %d, want %d", got, want)
	}
	for i, want := range wantReasons {
		if got := result.StatInvalidations[i].Reason; got != want {
			t.Fatalf("StatInvalidations[%d].Reason = %q, want %q", i, got, want)
		}
	}

	stored, err := store.EquippedModules(playerID, shipID)
	if err != nil {
		t.Fatalf("EquippedModules() error = %v, want nil", err)
	}
	if got, want := len(stored), 2; got != want {
		t.Fatalf("stored equipped len = %d, want %d", got, want)
	}
	if stored[1].ItemInstanceID != scanner.ItemInstanceID {
		t.Fatalf("stored utility item = %q, want %q", stored[1].ItemInstanceID, scanner.ItemInstanceID)
	}
}

func TestBreakEquippedModuleMarksBrokenAndReturnsOneInvalidation(t *testing.T) {
	service, store := newLoadoutTestService(t)
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")
	laser := testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 9)
	putModuleItem(t, store, laser)
	equipModuleForTest(t, store, playerID, shipID, ModuleSlotOffensive1, laser.ItemInstanceID)

	result, err := service.BreakEquippedModule(BreakEquippedModuleInput{
		PlayerID:       playerID,
		ShipID:         shipID,
		ItemInstanceID: laser.ItemInstanceID,
	})
	if err != nil {
		t.Fatalf("BreakEquippedModule() error = %v, want nil", err)
	}
	if result.Broken.ItemInstanceID != laser.ItemInstanceID {
		t.Fatalf("Broken item = %q, want %q", result.Broken.ItemInstanceID, laser.ItemInstanceID)
	}
	if got, want := len(result.StatInvalidations), 1; got != want {
		t.Fatalf("StatInvalidations len = %d, want %d", got, want)
	}
	signal := result.StatInvalidations[0]
	if signal.PlayerID != playerID || signal.ShipID != shipID {
		t.Fatalf("signal subject = (%q, %q), want (%q, %q)", signal.PlayerID, signal.ShipID, playerID, shipID)
	}
	if signal.Reason != StatInvalidationReasonModuleDurabilityBroken {
		t.Fatalf("signal Reason = %q, want %q", signal.Reason, StatInvalidationReasonModuleDurabilityBroken)
	}
	if signal.SourceID != laser.ItemInstanceID.String() {
		t.Fatalf("signal SourceID = %q, want %q", signal.SourceID, laser.ItemInstanceID)
	}
	if want := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC); !signal.CreatedAt.Equal(want) {
		t.Fatalf("signal CreatedAt = %v, want %v", signal.CreatedAt, want)
	}

	stored, err := store.ModuleItem(laser.ItemInstanceID)
	if err != nil {
		t.Fatalf("ModuleItem() error = %v, want nil", err)
	}
	if got, want := stored.DurabilityCurrent, int64(0); got != want {
		t.Fatalf("stored DurabilityCurrent = %d, want %d", got, want)
	}

	retry, err := service.BreakEquippedModule(BreakEquippedModuleInput{
		PlayerID:       playerID,
		ShipID:         shipID,
		ItemInstanceID: laser.ItemInstanceID,
	})
	if err != nil {
		t.Fatalf("second BreakEquippedModule() error = %v, want nil", err)
	}
	if got, want := len(retry.StatInvalidations), 0; got != want {
		t.Fatalf("second StatInvalidations len = %d, want %d", got, want)
	}
}

func TestBreakEquippedModuleRejectsSpoofedInvalidations(t *testing.T) {
	playerID := foundation.PlayerID("player-1")
	shipID := foundation.ShipID("ship-1")

	cases := []struct {
		name    string
		item    economy.InstanceItem
		setup   func(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem)
		wantErr error
	}{
		{
			name:    "non-equipped item",
			item:    testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 9),
			wantErr: ErrModuleItemNotEquipped,
		},
		{
			name: "wrong owner item",
			item: testModuleItem(t, "laser-instance-1", "laser_alpha_t1", "other-player", economy.LocationKindAccountInventory, 9),
			setup: func(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem) {
				t.Helper()
				equipModuleForTest(t, store, item.OwnerPlayerID, shipID, ModuleSlotOffensive1, item.ItemInstanceID)
			},
			wantErr: ErrModuleItemNotOwned,
		},
		{
			name: "wrong ship item",
			item: testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 9),
			setup: func(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem) {
				t.Helper()
				equipModuleForTest(t, store, playerID, "ship-2", ModuleSlotOffensive1, item.ItemInstanceID)
			},
			wantErr: ErrModuleItemNotEquipped,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, store := newLoadoutTestService(t)
			putModuleItem(t, store, tc.item)
			if tc.setup != nil {
				tc.setup(t, store, tc.item)
			}

			result, err := service.BreakEquippedModule(BreakEquippedModuleInput{
				PlayerID:       playerID,
				ShipID:         shipID,
				ItemInstanceID: tc.item.ItemInstanceID,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("BreakEquippedModule() error = %v, want %v", err, tc.wantErr)
			}
			if got, want := len(result.StatInvalidations), 0; got != want {
				t.Fatalf("StatInvalidations len = %d, want %d", got, want)
			}

			stored, err := store.ModuleItem(tc.item.ItemInstanceID)
			if err != nil {
				t.Fatalf("ModuleItem() error = %v, want nil", err)
			}
			if got, want := stored.DurabilityCurrent, tc.item.DurabilityCurrent; got != want {
				t.Fatalf("stored DurabilityCurrent = %d, want %d", got, want)
			}
		})
	}
}

func TestApplyLoadoutRejectsActiveShipMismatch(t *testing.T) {
	service, store := newLoadoutTestService(t)
	playerID := foundation.PlayerID("player-1")
	laser := testModuleItem(t, "laser-instance-1", "laser_alpha_t1", playerID, economy.LocationKindAccountInventory, 100)
	putModuleItem(t, store, laser)

	_, err := service.SaveLoadout(SaveLoadoutInput{
		LoadoutID:       "combat-alpha",
		PlayerID:        playerID,
		ShipID:          "ship-1",
		Name:            "Combat Alpha",
		SlotAssignments: SlotAssignments{ModuleSlotOffensive1: laser.ItemInstanceID},
		PlayerRank:      1,
		RoleLevels:      map[PilotRole]int{PilotRoleCombat: 1},
	})
	if err != nil {
		t.Fatalf("SaveLoadout() error = %v, want nil", err)
	}
	if err := store.SetActiveShip(playerID, "ship-2"); err != nil {
		t.Fatalf("SetActiveShip() error = %v, want nil", err)
	}

	_, err = service.ApplyLoadout(ApplyLoadoutInput{
		PlayerID:   playerID,
		LoadoutID:  "combat-alpha",
		PlayerRank: 1,
		RoleLevels: map[PilotRole]int{
			PilotRoleCombat: 1,
		},
	})
	if !errors.Is(err, ErrLoadoutShipMismatch) {
		t.Fatalf("ApplyLoadout() error = %v, want ErrLoadoutShipMismatch", err)
	}
}

type fixedLoadoutClock struct {
	now time.Time
}

func (clock fixedLoadoutClock) Now() time.Time {
	return clock.now
}

func newLoadoutTestService(t *testing.T) (LoadoutService, *InMemoryLoadoutStore) {
	t.Helper()
	store := NewInMemoryLoadoutStore()
	service, err := NewLoadoutService(
		MustMVPCatalog(),
		store,
		fixedLoadoutClock{now: time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatalf("NewLoadoutService() error = %v, want nil", err)
	}
	return service, store
}

func testModuleItem(
	t *testing.T,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	owner foundation.PlayerID,
	locationKind economy.LocationKind,
	durability int64,
) economy.InstanceItem {
	t.Helper()
	quantity, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	item := economy.InstanceItem{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(itemID),
			Version:      ModuleCatalogVersion,
		},
		ItemInstanceID:    itemInstanceID,
		ItemID:            itemID,
		OwnerPlayerID:     owner,
		Location:          economy.ItemLocation{Kind: locationKind, ID: economy.LocationID(owner.String())},
		Quantity:          quantity,
		DurabilityCurrent: durability,
		BoundState:        economy.BoundStateUnbound,
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("test item Validate() error = %v, want nil", err)
	}
	return item
}

func putModuleItem(t *testing.T, store *InMemoryLoadoutStore, item economy.InstanceItem) {
	t.Helper()
	if err := store.PutModuleItem(item); err != nil {
		t.Fatalf("PutModuleItem() error = %v, want nil", err)
	}
}

func equipModuleForTest(
	t *testing.T,
	store *InMemoryLoadoutStore,
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	slotID ModuleSlotID,
	itemInstanceID foundation.ItemID,
) {
	t.Helper()
	err := store.ReplaceEquippedModules(playerID, shipID, []EquippedModule{{
		PlayerID:       playerID,
		ShipID:         shipID,
		SlotID:         slotID,
		ItemInstanceID: itemInstanceID,
		EquippedAt:     time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}
}
