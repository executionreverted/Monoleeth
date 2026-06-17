package death_test

import (
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
	hook := &recordingDeathServiceModuleHook{}
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
		EquippedItemIDs:   []foundation.ItemID{"module-instance-1", "module-instance-2"},
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
	if hook.calls[0].ShipID != ships.ShipIDFighterT1 ||
		len(hook.calls[0].EquippedItemIDs) != 2 ||
		hook.calls[0].EquippedItemIDs[0] != "module-instance-1" ||
		result.ModuleDurabilityResult == nil ||
		len(result.ModuleDurabilityResult.SelectedItemIDs) != 2 {
		t.Fatalf("module hook call/result = %+v / %+v, want selected equipped ids", hook.calls[0], result.ModuleDurabilityResult)
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

func TestDeathServiceProcessDeathDuplicateLethalEventDoesNotMutateTwice(t *testing.T) {
	fixture := newDeathServiceFixture(t, nil, nil)
	iron := deathServiceItemDefinition(t, "iron_ore", economy.ItemTypeStackable, []economy.TradeFlag{economy.TradeFlagDroppable})
	cargoLocation := mustDeathServiceCargoLocation(t, ships.ShipIDFighterT1.String())
	added := fixture.addCargo(t, iron, 8, cargoLocation)
	hook := &recordingDeathServiceModuleHook{}
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
		EquippedItemIDs:   []foundation.ItemID{"module-instance-1"},
	}

	first, err := fixture.death.ProcessDeath(input)
	if err != nil {
		t.Fatalf("first ProcessDeath() error = %v", err)
	}
	if first.ShipDisableResult.PlayerShip.DisabledAt == nil {
		t.Fatal("first disabled at = nil, want timestamp")
	}
	firstDisabledAt := *first.ShipDisableResult.PlayerShip.DisabledAt
	firstDropID := first.LootDrops[0].ID
	firstLedgerCount := len(fixture.inventory.ItemLedgerEntries())

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
	assertDeathServiceFighterDisabledAt(t, fixture.ships, firstDisabledAt)
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
	clock     *testutil.FakeClock
	inventory *economy.InventoryService
	cargo     *economy.CargoService
	loot      *loot.Service
	ships     *ships.HangarService
	death     *death.DeathService
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
	deathService, err := death.NewDeathService(death.Config{
		Clock:     clock,
		RNG:       testutil.NewFakeRNG(ints, floats),
		Inventory: inventory,
		Loot:      lootService,
		Ships:     shipService,
	})
	if err != nil {
		t.Fatalf("death.NewDeathService() error = %v", err)
	}
	return deathServiceFixture{
		clock:     clock,
		inventory: inventory,
		cargo:     cargo,
		loot:      lootService,
		ships:     shipService,
		death:     deathService,
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

type recordingDeathServiceModuleHook struct {
	calls []death.ModuleDurabilityInput
}

func (hook *recordingDeathServiceModuleHook) ApplyDeathDurability(input death.ModuleDurabilityInput) (death.ModuleDurabilityResult, error) {
	hook.calls = append(hook.calls, input)
	return death.ModuleDurabilityResult{
		SelectedItemIDs: append([]foundation.ItemID(nil), input.EquippedItemIDs...),
	}, nil
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
