package death_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestDeathServiceProcessDeathDropsCargoCreatesLootDisablesShipRecordsRespawnAndCallsHook(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	fixture.equippedModules.SetItemIDs("module-instance-1", "module-instance-2")
	hook := &recordingDeathServiceModuleHook{
		result: death.ModuleDurabilityResult{
			StatInvalidations: []death.ModuleStatInvalidationSignal{
				{
					PlayerID:  "player-1",
					ShipID:    ships.ShipIDFighterT1,
					Reason:    death.ModuleStatInvalidationReasonDurabilityBroken,
					SourceID:  "module-instance-1",
					CreatedAt: fixture.clock.Now(),
				},
			},
		},
	}
	fixture.death.SetModuleDurabilityHook(hook)

	result, err := fixture.death.ProcessDeath(death.ProcessDeathInput{
		LethalEventID:   "lethal-event-1",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 12, Y: 34},
		KillerEntityID:  "npc-1",
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		DropOwnerPlayerID: "player-2",
		RespawnLocationID: "origin-station",
	})
	if err != nil {
		t.Fatalf("ProcessDeath() error = %v", err)
	}
	if result.Duplicate {
		t.Fatal("Duplicate = true, want false")
	}
	if result.Record.LethalEventKey != death.LethalEventKey("player_death:lethal-event-1") {
		t.Fatalf("record lethal key = %q, want player_death:lethal-event-1", result.Record.LethalEventKey)
	}
	if result.Record.ActiveShipID != ships.ShipIDFighterT1 {
		t.Fatalf("record active ship = %q, want %q", result.Record.ActiveShipID, ships.ShipIDFighterT1)
	}
	if result.Record.RespawnLocationID != death.RespawnLocationID("origin-station") {
		t.Fatalf("record respawn location = %q, want origin-station", result.Record.RespawnLocationID)
	}
	if result.Record.CargoDropPercent != 0.50 {
		t.Fatalf("record cargo drop percent = %v, want 0.50", result.Record.CargoDropPercent)
	}

	if got, want := len(result.CargoDrops), 1; got != want {
		t.Fatalf("CargoDrops len = %d, want %d", got, want)
	}
	if result.CargoDrops[0].ItemID != iron.ItemID || result.CargoDrops[0].Quantity != 5 {
		t.Fatalf("cargo drop = %+v, want 5 iron", result.CargoDrops[0])
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo quantity = %d, want 5", got)
	}
	if got, want := len(result.CargoRemovalResults), 1; got != want {
		t.Fatalf("CargoRemovalResults len = %d, want %d", got, want)
	}
	if got, want := len(result.CargoRemovalResults[0].LedgerEntries), 1; got != want {
		t.Fatalf("cargo removal ledger entries = %d, want %d", got, want)
	}
	wantReference := "death_cargo_drop:death-lethal-event-1:" + result.CargoDrops[0].SourceStackID.String()
	if got := result.CargoRemovalResults[0].LedgerEntries[0].ReferenceKey.String(); got != wantReference {
		t.Fatalf("death cargo ledger reference = %q, want %q", got, wantReference)
	}
	if got := result.CargoRemovalResults[0].LedgerEntries[0].ReferenceKey.String(); strings.HasPrefix(got, "loot_pickup:") {
		t.Fatalf("death cargo ledger reference = %q, must not use loot_pickup", got)
	}
	if got, want := len(result.LootDrops), 1; got != want {
		t.Fatalf("LootDrops len = %d, want %d", got, want)
	}
	if result.LootDrops[0].SourceType != loot.DropSourcePlayerDeath ||
		result.LootDrops[0].ItemDefinition.ItemID != iron.ItemID ||
		result.LootDrops[0].Quantity != 5 ||
		result.LootDrops[0].OwnerPlayerID != foundation.PlayerID("player-2") {
		t.Fatalf("loot drop = %+v, want player-death iron drop for player-2", result.LootDrops[0])
	}
	if got, want := len(result.ScheduledTasks), 2; got != want {
		t.Fatalf("ScheduledTasks len = %d, want %d", got, want)
	}
	if !result.ShipDisableResult.Disabled || result.ShipDisableResult.PlayerShip.State != ships.ShipStateDisabled {
		t.Fatalf("ShipDisableResult = %+v, want disabled active ship", result.ShipDisableResult)
	}
	assertDeathServiceFighterDisabled(t, fixture.ships)
	if len(hook.calls) != 1 {
		t.Fatalf("module hook calls = %d, want 1", len(hook.calls))
	}
	if got, want := len(fixture.equippedModules.calls), 1; got != want {
		t.Fatalf("equipped module provider calls = %d, want %d", got, want)
	}
	if fixture.equippedModules.calls[0].playerID != "player-1" || fixture.equippedModules.calls[0].shipID != ships.ShipIDFighterT1 {
		t.Fatalf("equipped module provider call = %+v, want player-1 fighter", fixture.equippedModules.calls[0])
	}
	if hook.calls[0].ShipID != ships.ShipIDFighterT1 ||
		len(hook.calls[0].EquippedItemIDs) != 2 ||
		hook.calls[0].EquippedItemIDs[0] != "module-instance-1" ||
		result.ModuleDurabilityResult == nil ||
		len(result.ModuleDurabilityResult.SelectedItemIDs) != 2 {
		t.Fatalf("module hook call/result = %+v / %+v, want selected equipped ids", hook.calls[0], result.ModuleDurabilityResult)
	}
	if got, want := len(result.ModuleDurabilityResult.StatInvalidations), 1; got != want {
		t.Fatalf("module stat invalidations len = %d, want %d", got, want)
	}
	moduleInvalidation := result.ModuleDurabilityResult.StatInvalidations[0]
	if moduleInvalidation.PlayerID != "player-1" ||
		moduleInvalidation.ShipID != ships.ShipIDFighterT1 ||
		moduleInvalidation.Reason != death.ModuleStatInvalidationReasonDurabilityBroken ||
		moduleInvalidation.SourceID != "module-instance-1" ||
		!moduleInvalidation.CreatedAt.Equal(fixture.clock.Now()) {
		t.Fatalf("module stat invalidation = %+v, want broken module signal for fighter", moduleInvalidation)
	}

	testutil.AssertRecordedEventTypes(t, fixture.events, death.EventPlayerDied, death.EventShipDisabled, death.EventDeathCargoDropped)
	recorded := fixture.events.Events()
	playerDied := decodeDeathServiceEventPayload[death.PlayerDiedEvent](t, recorded[0].Payload)
	if playerDied.DeathID != result.Record.DeathID ||
		playerDied.PlayerID != "player-1" ||
		playerDied.ActiveShipID != ships.ShipIDFighterT1 ||
		playerDied.RespawnLocationID != "origin-station" ||
		playerDied.CargoDropPercent != 0.50 {
		t.Fatalf("player.died payload = %+v, want death record summary", playerDied)
	}
	shipDisabled := decodeDeathServiceEventPayload[death.ShipDisabledEvent](t, recorded[1].Payload)
	if shipDisabled.DeathID != result.Record.DeathID ||
		shipDisabled.ShipID != ships.ShipIDFighterT1 ||
		shipDisabled.DisabledReason != ships.DisabledReasonDeath ||
		shipDisabled.StatInvalidation == nil ||
		shipDisabled.StatInvalidation.Reason != ships.StatInvalidationReasonActiveShipStateChanged {
		t.Fatalf("ship.disabled payload = %+v, want disabled fighter with stat invalidation", shipDisabled)
	}
	cargoDropped := decodeDeathServiceEventPayload[death.DeathCargoDroppedEvent](t, recorded[2].Payload)
	if cargoDropped.DeathID != result.Record.DeathID ||
		len(cargoDropped.Items) != 1 ||
		cargoDropped.Items[0].ItemID != iron.ItemID ||
		cargoDropped.Items[0].Quantity != 5 ||
		cargoDropped.Items[0].LootDropID != result.LootDrops[0].ID {
		t.Fatalf("death.cargo_dropped payload = %+v, want dropped cargo linked to loot", cargoDropped)
	}
}

func TestDeathServiceProcessDeathRejectsZonePolicyMismatch(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	policy, err := death.NewZoneCargoDropPolicy("zone-2", 0.50, 0.50)
	if err != nil {
		t.Fatalf("NewZoneCargoDropPolicy() error = %v", err)
	}

	_, err = fixture.death.ProcessDeath(death.ProcessDeathInput{
		LethalEventID:     "lethal-event-zone-mismatch",
		PlayerID:          "player-1",
		WorldID:           "world-1",
		ZoneID:            "zone-1",
		Position:          world.Vec2{X: 12, Y: 34},
		Reason:            death.DeathReasonCombat,
		CargoDropPolicy:   policy,
		RespawnLocationID: "origin-station",
	})
	if !errors.Is(err, death.ErrCargoDropPolicyZoneMismatch) {
		t.Fatalf("ProcessDeath() error = %v, want ErrCargoDropPolicyZoneMismatch", err)
	}
}

func TestDeathServiceProcessDeathRequiresEquippedModuleProviderForDurabilityHook(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	hook := &recordingDeathServiceModuleHook{}
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG(nil, nil),
		Inventory: fixture.inventory,
		Loot:      fixture.loot,
		Ships:     fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}
	deathService.SetModuleDurabilityHook(hook)

	_, err = deathService.ProcessDeath(death.ProcessDeathInput{
		LethalEventID:     "lethal-event-missing-module-provider",
		PlayerID:          "player-1",
		WorldID:           "world-1",
		ZoneID:            "zone-1",
		Position:          world.Vec2{X: 12, Y: 34},
		Reason:            death.DeathReasonCombat,
		CargoDropPolicy:   cargoPolicy(t, 0.50, 0.50),
		RespawnLocationID: "origin-station",
	})
	if !errors.Is(err, death.ErrNilEquippedModuleProvider) {
		t.Fatalf("ProcessDeath() error = %v, want ErrNilEquippedModuleProvider", err)
	}
	if len(hook.calls) != 0 {
		t.Fatalf("module hook calls = %d, want 0", len(hook.calls))
	}
	assertDeathServiceFighterActive(t, fixture.ships)
}

func TestDeathServiceProcessDeathRetryAfterEquippedModuleProviderFailureDoesNotSkipDurability(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 8, cargoLocation)
	providerErr := errors.New("equipped module provider unavailable")
	fixture.equippedModules.err = providerErr
	hook := &recordingDeathServiceModuleHook{}
	fixture.death.SetModuleDurabilityHook(hook)
	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-module-provider-retry",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}

	_, err := fixture.death.ProcessDeath(input)
	if !errors.Is(err, providerErr) {
		t.Fatalf("first ProcessDeath() error = %v, want provider error", err)
	}
	assertDeathServiceFighterActive(t, fixture.ships)
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 8 {
		t.Fatalf("cargo after provider failure = %d, want 8", got)
	}
	if len(hook.calls) != 0 {
		t.Fatalf("module hook calls after provider failure = %d, want 0", len(hook.calls))
	}

	fixture.equippedModules.err = nil
	fixture.equippedModules.SetItemIDs("module-instance-1")
	retry, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("retry ProcessDeath() error = %v, want nil", err)
	}
	if retry.Duplicate {
		t.Fatal("retry Duplicate = true, want full death processing")
	}
	if retry.ModuleDurabilityResult == nil || len(retry.ModuleDurabilityResult.SelectedItemIDs) != 1 {
		t.Fatalf("retry module durability result = %+v, want selected equipped item", retry.ModuleDurabilityResult)
	}
	if len(hook.calls) != 1 {
		t.Fatalf("module hook calls after retry = %d, want 1", len(hook.calls))
	}
	assertDeathServiceFighterDisabled(t, fixture.ships)
}

func TestDeathServiceProcessDeathDuplicateLethalEventDoesNotMutateTwice(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 8, cargoLocation)
	fixture.equippedModules.SetItemIDs("module-instance-1")
	hook := &recordingDeathServiceModuleHook{
		result: death.ModuleDurabilityResult{
			StatInvalidations: []death.ModuleStatInvalidationSignal{
				{
					PlayerID:  "player-1",
					ShipID:    ships.ShipIDFighterT1,
					Reason:    death.ModuleStatInvalidationReasonDurabilityBroken,
					SourceID:  "module-instance-1",
					CreatedAt: fixture.clock.Now(),
				},
			},
		},
	}
	fixture.death.SetModuleDurabilityHook(hook)
	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-duplicate",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}

	first, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("first ProcessDeath() error = %v", err)
	}
	if first.ShipDisableResult.PlayerShip.DisabledAt == nil {
		t.Fatal("first disabled at = nil, want timestamp")
	}
	if first.ModuleDurabilityResult == nil || len(first.ModuleDurabilityResult.StatInvalidations) != 1 {
		t.Fatalf("first module durability result = %+v, want one stat invalidation", first.ModuleDurabilityResult)
	}
	first.ModuleDurabilityResult.StatInvalidations[0].SourceID = "mutated-return-value"
	firstDisabledAt := *first.ShipDisableResult.PlayerShip.DisabledAt
	firstDropID := first.LootDrops[0].ID
	firstLedgerCount := len(fixture.inventory.ItemLedgerEntries())
	testutil.AssertRecordedEventTypes(t, fixture.events, death.EventPlayerDied, death.EventShipDisabled, death.EventDeathCargoDropped)
	fixture.events.Reset()

	fixture.clock.Advance(time.Minute)
	second, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("duplicate ProcessDeath() error = %v", err)
	}
	if !second.Duplicate {
		t.Fatal("second Duplicate = false, want true")
	}
	if second.Record.DeathID != first.Record.DeathID || second.LootDrops[0].ID != firstDropID {
		t.Fatalf("duplicate result = %+v, want original death/drop ids", second)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 4 {
		t.Fatalf("remaining cargo after duplicate = %d, want 4", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != firstLedgerCount {
		t.Fatalf("ledger entries after duplicate = %d, want %d", got, firstLedgerCount)
	}
	if second.ShipDisableResult.Disabled || !second.ShipDisableResult.Duplicate {
		t.Fatalf("duplicate ship disable result = %+v, want cached duplicate marker", second.ShipDisableResult)
	}
	if len(hook.calls) != 1 {
		t.Fatalf("module hook calls after duplicate = %d, want 1", len(hook.calls))
	}
	if got, want := len(fixture.equippedModules.calls), 1; got != want {
		t.Fatalf("equipped module provider calls after duplicate = %d, want %d", got, want)
	}
	if second.ModuleDurabilityResult == nil || !second.ModuleDurabilityResult.Duplicate {
		t.Fatalf("duplicate module durability result = %+v, want duplicate marker", second.ModuleDurabilityResult)
	}
	if got, want := len(second.ModuleDurabilityResult.StatInvalidations), 1; got != want {
		t.Fatalf("duplicate module stat invalidations len = %d, want %d", got, want)
	}
	if got := second.ModuleDurabilityResult.StatInvalidations[0].SourceID; got != "module-instance-1" {
		t.Fatalf("duplicate module stat invalidation source = %q, want cached original source", got)
	}
	if got := len(fixture.events.Events()); got != 0 {
		t.Fatalf("duplicate death emitted %d events, want 0", got)
	}
	assertDeathServiceFighterDisabledAt(t, fixture.ships, firstDisabledAt)
}

func TestDeathServiceBlocksCargoMoveWhileDeathProcessing(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	blockingLoot := newBlockingDeathServiceLoot(fixture.loot)
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG(nil, nil),
		Inventory: fixture.inventory,
		Loot:      blockingLoot,
		Ships:     fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}
	fixture.inventory.SetCargoTransferGuard(deathService)
	fixture.cargo.SetCargoTransferGuard(deathService)

	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-cargo-lock",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}

	type deathResult struct {
		result death.ProcessDeathResult
		err    error
	}
	done := make(chan deathResult, 1)
	go func() {
		result, err := deathService.ProcessDeath(input)
		done <- deathResult{result: result, err: err}
	}()

	select {
	case <-blockingLoot.entered:
	case <-time.After(time.Second):
		t.Fatal("ProcessDeath did not reach blocked loot creation")
	}

	ledgerCount := len(fixture.inventory.ItemLedgerEntries())
	_, moveErr := fixture.inventory.MoveItem(economy.MoveItemInput{
		PlayerID: "player-1",
		ItemRef: economy.MoveItemRef{
			Definition: iron,
		},
		FromLocation: cargoLocation,
		ToLocation:   mustDeathServiceLocation(t, economy.LocationKindAccountInventory, "player-1"),
		Quantity:     1,
		Reason:       economy.LedgerReason("inventory_move"),
		ReferenceKey: mustDeathServiceLootPickupKey(t, "cargo-lock-move"),
	})
	if !errors.Is(moveErr, death.ErrDeathCargoTransferBlocked) {
		t.Fatalf("MoveItem during death error = %v, want ErrDeathCargoTransferBlocked", moveErr)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("ledger entries after blocked cargo move = %d, want %d", got, ledgerCount)
	}

	close(blockingLoot.release)
	select {
	case completed := <-done:
		if completed.err != nil {
			t.Fatalf("ProcessDeath completion error = %v", completed.err)
		}
		if completed.result.Duplicate {
			t.Fatalf("ProcessDeath Duplicate = true, want false")
		}
	case <-time.After(time.Second):
		t.Fatal("ProcessDeath did not complete after releasing loot block")
	}
}

func TestDeathServiceWaitsForInFlightCargoTransferBeforeProcessing(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	blockingLoot := newBlockingDeathServiceLoot(fixture.loot)
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG(nil, nil),
		Inventory: fixture.inventory,
		Loot:      blockingLoot,
		Ships:     fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}

	lease, err := deathService.BeginCargoTransfer(economy.CargoTransferGuardInput{
		PlayerID:     "player-1",
		FromLocation: cargoLocation,
		ToLocation:   mustDeathServiceLocation(t, economy.LocationKindAccountInventory, "player-1"),
		Reason:       economy.LedgerReason("inventory_move"),
		ReferenceKey: mustDeathServiceLootPickupKey(t, "in-flight-cargo-transfer"),
	})
	if err != nil {
		t.Fatalf("BeginCargoTransfer() error = %v", err)
	}
	defer lease.Release()

	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-waits-for-cargo",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}

	type deathResult struct {
		result death.ProcessDeathResult
		err    error
	}
	done := make(chan deathResult, 1)
	go func() {
		result, err := deathService.ProcessDeath(input)
		done <- deathResult{result: result, err: err}
	}()

	waitForDeathServiceCargoBlock(t, deathService, cargoLocation)
	select {
	case <-blockingLoot.entered:
		t.Fatal("ProcessDeath reached loot creation before in-flight cargo transfer released")
	default:
	}

	lease.Release()
	select {
	case <-blockingLoot.entered:
	case <-time.After(time.Second):
		t.Fatal("ProcessDeath did not continue after in-flight cargo transfer released")
	}

	close(blockingLoot.release)
	select {
	case completed := <-done:
		if completed.err != nil {
			t.Fatalf("ProcessDeath completion error = %v", completed.err)
		}
		if completed.result.Duplicate {
			t.Fatalf("ProcessDeath Duplicate = true, want false")
		}
	case <-time.After(time.Second):
		t.Fatal("ProcessDeath did not complete after releasing loot block")
	}
}

func TestDeathServiceProcessDeathAlreadyDisabledActiveShipNewLethalEventDoesNotDropAgain(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-first-disable",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}
	first, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("first ProcessDeath() error = %v", err)
	}
	if got := len(first.LootDrops); got != 1 {
		t.Fatalf("first loot drops = %d, want 1", got)
	}
	ledgerCount := len(fixture.inventory.ItemLedgerEntries())
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo after first death = %d, want 5", got)
	}

	input.LethalEventID = "lethal-event-after-disabled"
	second, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("new lethal ProcessDeath() error = %v", err)
	}
	if !second.Duplicate || !second.ShipDisableResult.Duplicate {
		t.Fatalf("new lethal result = %+v, want duplicate disabled no-op", second)
	}
	if len(second.CargoDrops) != 0 || len(second.CargoRemovalResults) != 0 || len(second.LootDrops) != 0 {
		t.Fatalf("new lethal mutated death outputs = %+v, want no cargo removal or loot", second)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo after new lethal = %d, want 5", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("ledger entries after new lethal = %d, want %d", got, ledgerCount)
	}

	retry, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("same lethal retry ProcessDeath() error = %v", err)
	}
	if !retry.Duplicate || !retry.ShipDisableResult.Duplicate {
		t.Fatalf("same lethal retry result = %+v, want cached duplicate", retry)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("ledger entries after same lethal retry = %d, want %d", got, ledgerCount)
	}
}

func TestDeathServiceProcessDeathRetryAfterLootFailureDoesNotRemoveCargoTwice(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	flakyLoot := &failOnceDeathServiceLoot{
		delegate: fixture.loot,
		err:      errors.New("temporary loot outage"),
	}
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG(nil, nil),
		Inventory: fixture.inventory,
		Loot:      flakyLoot,
		Ships:     fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}

	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-loot-retry",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}
	if _, err := deathService.ProcessDeath(input); !errors.Is(err, flakyLoot.err) {
		t.Fatalf("first ProcessDeath() error = %v, want %v", err, flakyLoot.err)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo after failed loot = %d, want 5", got)
	}
	ledgerCount := len(fixture.inventory.ItemLedgerEntries())

	result, err := deathService.ProcessDeath(input)
	if err != nil {
		t.Fatalf("retry ProcessDeath() error = %v", err)
	}
	if result.Duplicate {
		t.Fatal("retry result Duplicate = true, want false because first attempt failed before death commit")
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo after retry = %d, want 5", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
		t.Fatalf("ledger entries after retry = %d, want %d", got, ledgerCount)
	}
	if got := len(result.CargoRemovalResults); got != 1 || !result.CargoRemovalResults[0].Duplicate {
		t.Fatalf("retry removal results = %+v, want one duplicate inventory result", result.CargoRemovalResults)
	}
	if got := len(result.LootDrops); got != 1 {
		t.Fatalf("retry loot drops = %d, want 1", got)
	}
}

func TestDeathServiceProcessDeathRetryAfterFailureReusesOriginalCargoSelection(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	carbon := deathServiceItemDefinition(t, "carbon_shards", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	ironAdded := fixture.addCargo(t, iron, 10, cargoLocation)
	carbonAdded := fixture.addCargo(t, carbon, 10, cargoLocation)
	flakyLoot := &failOnceDeathServiceLoot{
		delegate: fixture.loot,
		err:      errors.New("temporary loot outage"),
	}
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG([]int{1, 0}, []float64{0.50, 0.50}),
		Inventory: fixture.inventory,
		Loot:      flakyLoot,
		Ships:     fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}

	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-reuse-selection",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, ironAdded.StackableItems[0], iron),
			cargoStackFromDeathServiceStackable(t, carbonAdded.StackableItems[0], carbon),
		},
		RespawnLocationID: "origin-station",
	}
	if _, err := deathService.ProcessDeath(input); !errors.Is(err, flakyLoot.err) {
		t.Fatalf("first ProcessDeath() error = %v, want %v", err, flakyLoot.err)
	}

	result, err := deathService.ProcessDeath(input)
	if err != nil {
		t.Fatalf("retry ProcessDeath() error = %v", err)
	}
	if got := len(result.CargoDrops); got != 1 {
		t.Fatalf("retry CargoDrops len = %d, want 1", got)
	}
	if result.CargoDrops[0].ItemID != iron.ItemID {
		t.Fatalf("retry cargo drop item = %q, want original iron selection", result.CargoDrops[0].ItemID)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 0 {
		t.Fatalf("iron after retry = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", carbon.ItemID, cargoLocation); got != 10 {
		t.Fatalf("carbon after retry = %d, want 10", got)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != 3 {
		t.Fatalf("ledger entries after retry = %d, want 2 seeds + 1 death remove", got)
	}
}

func TestDeathServiceProcessDeathShipDisableFailureLeavesCargoAndLootUntouched(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 10, cargoLocation)
	recordingLoot := &recordingDeathServiceLoot{delegate: fixture.loot}
	flakyShips := &failOnceDeathServiceShips{
		delegate: fixture.ships,
		err:      errors.New("temporary ship store outage"),
	}
	deathService, err := death.NewDeathService(death.Config{
		Clock:     fixture.clock,
		RNG:       testutil.NewFakeRNG(nil, nil),
		Inventory: fixture.inventory,
		Loot:      recordingLoot,
		Ships:     flakyShips,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}

	input := death.ProcessDeathInput{
		LethalEventID:   "lethal-event-ship-retry",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 1, Y: 2},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 0.50, 0.50),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron),
		},
		RespawnLocationID: "origin-station",
	}
	if _, err := deathService.ProcessDeath(input); !errors.Is(err, flakyShips.err) {
		t.Fatalf("first ProcessDeath() error = %v, want %v", err, flakyShips.err)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 10 {
		t.Fatalf("remaining cargo after failed ship disable = %d, want 10", got)
	}
	if recordingLoot.calls != 0 {
		t.Fatalf("loot calls after failed ship disable = %d, want 0", recordingLoot.calls)
	}
	assertDeathServiceFighterActive(t, fixture.ships)
	ledgerCount := len(fixture.inventory.ItemLedgerEntries())

	result, err := deathService.ProcessDeath(input)
	if err != nil {
		t.Fatalf("retry ProcessDeath() error = %v", err)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 5 {
		t.Fatalf("remaining cargo after retry = %d, want 5", got)
	}
	if got, want := len(fixture.inventory.ItemLedgerEntries()), ledgerCount+1; got != want {
		t.Fatalf("ledger entries after retry = %d, want %d", got, want)
	}
	if got := len(result.CargoRemovalResults); got != 1 || result.CargoRemovalResults[0].Duplicate {
		t.Fatalf("retry removal results = %+v, want one non-duplicate inventory result", result.CargoRemovalResults)
	}
	if got := len(result.LootDrops); got != 1 {
		t.Fatalf("retry loot drops = %d, want 1", got)
	}
	if recordingLoot.calls != 1 {
		t.Fatalf("loot calls after retry = %d, want 1", recordingLoot.calls)
	}
	assertDeathServiceFighterDisabled(t, fixture.ships)
}

func TestDeathServiceProcessDeathRejectsCargoOutsidePlayerActiveShipBeforeMutation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*testing.T, *death.CargoStack)
		wantErr error
	}{
		{
			name: "another player owner",
			mutate: func(t *testing.T, stack *death.CargoStack) {
				stack.OwnerPlayerID = "player-2"
			},
			wantErr: death.ErrDeathCargoOwnerMismatch,
		},
		{
			name: "another ship cargo",
			mutate: func(t *testing.T, stack *death.CargoStack) {
				stack.Location = mustDeathServiceCargoLocation(t, ships.ShipIDStarter.String())
			},
			wantErr: death.ErrDeathCargoLocationMismatch,
		},
		{
			name: "non ship cargo location",
			mutate: func(t *testing.T, stack *death.CargoStack) {
				stack.Location = mustDeathServiceLocation(t, economy.LocationKindAccountInventory, "player-1")
			},
			wantErr: death.ErrDeathCargoLocationMismatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newDeathServiceFixture(t, nil, nil)
			iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
			cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
			added := fixture.addCargo(t, iron, 10, cargoLocation)
			stack := cargoStackFromDeathServiceStackable(t, added.StackableItems[0], iron)
			tc.mutate(t, &stack)
			ledgerCount := len(fixture.inventory.ItemLedgerEntries())

			_, err := fixture.death.ProcessDeath(death.ProcessDeathInput{
				LethalEventID:     "lethal-event-invalid-cargo",
				PlayerID:          "player-1",
				WorldID:           "world-1",
				ZoneID:            "zone-1",
				Position:          world.Vec2{X: 1, Y: 2},
				Reason:            death.DeathReasonCombat,
				CargoDropPolicy:   cargoPolicy(t, 0.50, 0.50),
				Cargo:             []death.CargoStack{stack},
				RespawnLocationID: "origin-station",
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ProcessDeath() error = %v, want %v", err, tc.wantErr)
			}
			if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 10 {
				t.Fatalf("remaining cargo after reject = %d, want 10", got)
			}
			if got := len(fixture.inventory.ItemLedgerEntries()); got != ledgerCount {
				t.Fatalf("ledger entries after reject = %d, want %d", got, ledgerCount)
			}
			assertDeathServiceFighterActive(t, fixture.ships)
		})
	}
}

func TestDeathServiceProcessDeathPreservesProtectedCargo(t *testing.T) {
	fixture := newDeathServiceFixture(t, []int{0}, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	questItem := deathServiceItemDefinition(t, "quest_token", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagTradeable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	ironAdded := fixture.addCargo(t, iron, 4, cargoLocation)
	questAdded := fixture.addCargo(t, questItem, 3, cargoLocation)

	result, err := fixture.death.ProcessDeath(death.ProcessDeathInput{
		LethalEventID:   "lethal-event-protected",
		PlayerID:        "player-1",
		WorldID:         "world-1",
		ZoneID:          "zone-1",
		Position:        world.Vec2{X: 3, Y: 4},
		Reason:          death.DeathReasonCombat,
		CargoDropPolicy: cargoPolicy(t, 1, 1),
		Cargo: []death.CargoStack{
			cargoStackFromDeathServiceStackable(t, ironAdded.StackableItems[0], iron),
			cargoStackFromDeathServiceStackable(t, questAdded.StackableItems[0], questItem),
		},
		RespawnLocationID: "origin-station",
	})
	if err != nil {
		t.Fatalf("ProcessDeath() error = %v", err)
	}
	if got, want := len(result.CargoDrops), 1; got != want {
		t.Fatalf("CargoDrops len = %d, want %d", got, want)
	}
	if result.CargoDrops[0].ItemID != iron.ItemID {
		t.Fatalf("dropped item = %q, want iron only", result.CargoDrops[0].ItemID)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", iron.ItemID, cargoLocation); got != 0 {
		t.Fatalf("iron remaining = %d, want 0", got)
	}
	if got := fixture.inventory.TotalItemQuantity("player-1", questItem.ItemID, cargoLocation); got != 3 {
		t.Fatalf("protected quest item remaining = %d, want 3", got)
	}
	if got, want := len(result.CargoSelection.Preserved), 1; got != want {
		t.Fatalf("preserved len = %d, want %d", got, want)
	}
	if result.CargoSelection.Preserved[0].Reason != death.CargoPreserveNotDroppable {
		t.Fatalf("preserved reason = %q, want not_droppable", result.CargoSelection.Preserved[0].Reason)
	}
	assertDeathServiceFighterDisabled(t, fixture.ships)
}

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
