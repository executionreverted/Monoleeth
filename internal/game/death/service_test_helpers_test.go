package death_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
)

type deathServiceFixture struct {
	clock           *testutil.FakeClock
	inventory       *economy.InventoryService
	cargo           *economy.CargoService
	loot            *loot.Service
	ships           *ships.HangarService
	equippedModules *recordingDeathServiceEquippedModules
	death           *death.DeathService
	events          *testutil.EventRecorder
}

func newDeathServiceFixture(t *testing.T, ints []int, floats []float64) deathServiceFixture {
	t.Helper()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	lootService, err := loot.NewService(loot.Config{
		Clock: clock,
		RNG:   testutil.NewFakeRNG(nil, nil),
		Cargo: cargo,
	})
	if err != nil {
		t.Fatalf("loot.NewService() error = %v", err)
	}
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog() error = %v", err)
	}
	shipService, err := ships.NewHangarService(
		shipCatalog,
		ships.NewInMemoryHangarStore(),
		ships.StaticPlayerRankProvider{"player-1": 2},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		t.Fatalf("ships.NewHangarService() error = %v", err)
	}
	ensureDeathServiceActiveFighter(t, shipService)
	eventRecorder := testutil.NewEventRecorder()
	equippedModules := &recordingDeathServiceEquippedModules{}
	deathService, err := death.NewDeathService(death.Config{
		Clock:           clock,
		RNG:             testutil.NewFakeRNG(ints, floats),
		Inventory:       inventory,
		Loot:            lootService,
		Ships:           shipService,
		EquippedModules: equippedModules,
		EventEmitter:    eventRecorder,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}
	return deathServiceFixture{
		clock:           clock,
		inventory:       inventory,
		cargo:           cargo,
		loot:            lootService,
		ships:           shipService,
		equippedModules: equippedModules,
		death:           deathService,
		events:          eventRecorder,
	}
}

func (fixture deathServiceFixture) addCargo(t *testing.T, definition economy.ItemDefinition, quantity int64, location economy.ItemLocation) economy.AddItemResult {
	t.Helper()
	result, err := fixture.cargo.AddItem(economy.CargoAddItemInput{
		PlayerID:           "player-1",
		ActiveCargo:        location,
		ItemDefinition:     definition,
		Quantity:           quantity,
		CargoCapacityUnits: 1000,
		Reason:             economy.LedgerReason("test_seed_cargo"),
		ReferenceKey:       mustDeathServiceLootPickupKey(t, "seed-"+definition.ItemID.String()),
	})
	if err != nil {
		t.Fatalf("CargoService.AddItem(%s) error = %v", definition.ItemID, err)
	}
	return result
}

func deathServiceItemDefinition(t *testing.T, itemID foundation.ItemID, itemType economy.ItemType, tradeFlags []economy.TradeFlag) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), "v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	maxStack := int64(999)
	if itemType == economy.ItemTypeInstance {
		maxStack = 1
	}
	definition, err := economy.NewItemDefinition(
		source,
		itemID,
		itemID.String(),
		itemType,
		economy.ItemRarityCommon,
		deathServiceQuantity(t, maxStack),
		deathServiceQuantity(t, 1),
		tradeFlags,
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v", err)
	}
	return definition
}

func cargoStackFromDeathServiceStackable(t *testing.T, item economy.StackableItem, definition economy.ItemDefinition) death.CargoStack {
	t.Helper()
	return death.CargoStack{
		ItemInstanceID: item.ItemInstanceID,
		Definition: death.CargoItemDefinition{
			Source:            definition.Source,
			ItemID:            definition.ItemID,
			Type:              definition.Type,
			TradeFlags:        append([]economy.TradeFlag(nil), definition.TradeFlags...),
			BindRules:         append([]economy.BindRule(nil), definition.BindRules...),
			CargoUnitsPerItem: definition.WeightUnits.Int64(),
		},
		EconomyDefinition: definition,
		OwnerPlayerID:     item.OwnerPlayerID,
		Location:          item.Location,
		Quantity:          item.Quantity.Int64(),
		BoundState:        economy.BoundStateUnbound,
	}
}

func ensureDeathServiceActiveFighter(t *testing.T, service *ships.HangarService) {
	t.Helper()
	if _, err := service.EnsureStarterShip("player-1"); err != nil {
		t.Fatalf("EnsureStarterShip() error = %v", err)
	}
	if _, err := service.UnlockShip(ships.UnlockShipInput{PlayerID: "player-1", ShipID: ships.ShipIDFighterT1}); err != nil {
		t.Fatalf("UnlockShip() error = %v", err)
	}
	if _, err := service.SetActiveShip(ships.SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ships.ShipIDFighterT1,
		Context: ships.ShipSwapContext{
			InSafeHangarArea: true,
		},
	}); err != nil {
		t.Fatalf("SetActiveShip() error = %v", err)
	}
}

func assertDeathServiceFighterDisabled(t *testing.T, service *ships.HangarService) {
	t.Helper()
	snapshot, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar() error = %v", err)
	}
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID == ships.ShipIDFighterT1 {
			if playerShip.State != ships.ShipStateDisabled || playerShip.DisabledReason != ships.DisabledReasonDeath {
				t.Fatalf("fighter state = %+v, want disabled by death", playerShip)
			}
			return
		}
	}
	t.Fatalf("fighter_t1 missing from hangar snapshot %+v", snapshot)
}

func assertDeathServiceFighterDisabledAt(t *testing.T, service *ships.HangarService, disabledAt time.Time) {
	t.Helper()
	snapshot, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar() error = %v", err)
	}
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID == ships.ShipIDFighterT1 {
			if playerShip.DisabledAt == nil || !playerShip.DisabledAt.Equal(disabledAt) {
				t.Fatalf("fighter DisabledAt = %v, want %s", playerShip.DisabledAt, disabledAt)
			}
			return
		}
	}
	t.Fatalf("fighter_t1 missing from hangar snapshot %+v", snapshot)
}

func assertDeathServiceFighterActive(t *testing.T, service *ships.HangarService) {
	t.Helper()
	snapshot, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar() error = %v", err)
	}
	if !snapshot.HasActiveShip || snapshot.ActiveShip.ShipID != ships.ShipIDFighterT1 {
		t.Fatalf("active ship = %+v has=%v, want fighter_t1", snapshot.ActiveShip, snapshot.HasActiveShip)
	}
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID == ships.ShipIDFighterT1 {
			if playerShip.State != ships.ShipStateActive {
				t.Fatalf("fighter state = %+v, want active", playerShip)
			}
			return
		}
	}
	t.Fatalf("fighter_t1 missing from hangar snapshot %+v", snapshot)
}

func mustDeathServiceCargoLocation(t *testing.T, id string) economy.ItemLocation {
	t.Helper()
	return mustDeathServiceLocation(t, economy.LocationKindShipCargo, id)
}

func mustDeathServiceLocation(t *testing.T, kind economy.LocationKind, id string) economy.ItemLocation {
	t.Helper()
	location, err := economy.NewItemLocation(kind, id)
	if err != nil {
		t.Fatalf("NewItemLocation() error = %v", err)
	}
	return location
}

func deathServiceQuantity(t *testing.T, amount int64) foundation.Quantity {
	t.Helper()
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		t.Fatalf("NewQuantity(%d) error = %v", amount, err)
	}
	return quantity
}

func mustDeathServiceLootPickupKey(t *testing.T, id string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.LootPickupIdempotencyKey(id)
	if err != nil {
		t.Fatalf("LootPickupIdempotencyKey(%q) error = %v", id, err)
	}
	return key
}

func decodeDeathServiceEventPayload[T any](t *testing.T, payload []byte) T {
	t.Helper()
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode death event payload %s: %v", string(payload), err)
	}
	return decoded
}

type deathServiceEquippedModuleCall struct {
	playerID foundation.PlayerID
	shipID   foundation.ShipID
}

type recordingDeathServiceEquippedModules struct {
	calls   []deathServiceEquippedModuleCall
	itemIDs []foundation.ItemID
	err     error
}

func (provider *recordingDeathServiceEquippedModules) SetItemIDs(itemIDs ...foundation.ItemID) {
	provider.itemIDs = append([]foundation.ItemID(nil), itemIDs...)
}

func (provider *recordingDeathServiceEquippedModules) EquippedItemIDs(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
) ([]foundation.ItemID, error) {
	provider.calls = append(provider.calls, deathServiceEquippedModuleCall{
		playerID: playerID,
		shipID:   shipID,
	})
	if provider.err != nil {
		return nil, provider.err
	}
	return append([]foundation.ItemID(nil), provider.itemIDs...), nil
}

type recordingDeathServiceModuleHook struct {
	calls  []death.ModuleDurabilityInput
	result death.ModuleDurabilityResult
}

func (hook *recordingDeathServiceModuleHook) ApplyDeathDurability(input death.ModuleDurabilityInput) (death.ModuleDurabilityResult, error) {
	hook.calls = append(hook.calls, input)
	result := hook.result
	if len(result.SelectedItemIDs) == 0 {
		result.SelectedItemIDs = append([]foundation.ItemID(nil), input.EquippedItemIDs...)
	} else {
		result.SelectedItemIDs = append([]foundation.ItemID(nil), result.SelectedItemIDs...)
	}
	result.StatInvalidations = append([]death.ModuleStatInvalidationSignal(nil), result.StatInvalidations...)
	return result, nil
}

type failOnceDeathServiceLoot struct {
	delegate death.PlayerDeathDropCreator
	err      error
	calls    int
}

func (service *failOnceDeathServiceLoot) CreateDropsForPlayerDeath(input loot.CreatePlayerDeathDropsInput) (loot.CreateDropsResult, error) {
	service.calls++
	if service.calls == 1 {
		return loot.CreateDropsResult{}, service.err
	}
	return service.delegate.CreateDropsForPlayerDeath(input)
}

type blockingDeathServiceLoot struct {
	delegate death.PlayerDeathDropCreator
	entered  chan struct{}
	release  chan struct{}
}

func newBlockingDeathServiceLoot(delegate death.PlayerDeathDropCreator) *blockingDeathServiceLoot {
	return &blockingDeathServiceLoot{
		delegate: delegate,
		entered:  make(chan struct{}, 1),
		release:  make(chan struct{}),
	}
}

func (service *blockingDeathServiceLoot) CreateDropsForPlayerDeath(input loot.CreatePlayerDeathDropsInput) (loot.CreateDropsResult, error) {
	select {
	case service.entered <- struct{}{}:
	default:
	}
	<-service.release
	return service.delegate.CreateDropsForPlayerDeath(input)
}

func waitForDeathServiceCargoBlock(t *testing.T, service *death.DeathService, cargoLocation economy.ItemLocation) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		lease, err := service.BeginCargoTransfer(economy.CargoTransferGuardInput{
			PlayerID:     "player-1",
			FromLocation: cargoLocation,
			ToLocation:   mustDeathServiceLocation(t, economy.LocationKindAccountInventory, "player-1"),
			Reason:       economy.LedgerReason("inventory_move"),
			ReferenceKey: mustDeathServiceLootPickupKey(t, "death-block-probe"),
		})
		if errors.Is(err, death.ErrDeathCargoTransferBlocked) {
			return
		}
		if err != nil {
			t.Fatalf("BeginCargoTransfer probe error = %v", err)
		}
		if lease != nil {
			lease.Release()
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("death cargo transfer block did not become active")
}

type recordingDeathServiceLoot struct {
	delegate death.PlayerDeathDropCreator
	calls    int
}

func (service *recordingDeathServiceLoot) CreateDropsForPlayerDeath(input loot.CreatePlayerDeathDropsInput) (loot.CreateDropsResult, error) {
	service.calls++
	return service.delegate.CreateDropsForPlayerDeath(input)
}

type failOnceDeathServiceShips struct {
	delegate death.ActiveShipDisabler
	err      error
	calls    int
}

func (service *failOnceDeathServiceShips) GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error) {
	return service.delegate.GetHangar(playerID)
}

func (service *failOnceDeathServiceShips) DisableActiveShipForDeath(input ships.DisableActiveShipForDeathInput) (ships.DisableActiveShipForDeathResult, error) {
	service.calls++
	if service.calls == 1 {
		return ships.DisableActiveShipForDeathResult{}, service.err
	}
	return service.delegate.DisableActiveShipForDeath(input)
}
