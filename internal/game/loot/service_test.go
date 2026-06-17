package loot_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestCreateDropsForNPCKillRollsServerSideAndIsIdempotent(t *testing.T) {
	service, _, _, _ := newLootService(t, []int{2}, []float64{0})
	event := npcKilledEvent()
	table := lootTable(t, 2, 4, 1)

	result, err := service.CreateDropsForNPCKill(event, table)
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	if len(result.Drops) != 1 {
		t.Fatalf("drops len = %d, want 1", len(result.Drops))
	}
	if result.Drops[0].Quantity != 4 {
		t.Fatalf("drop quantity = %d, want server rng quantity 4", result.Drops[0].Quantity)
	}
	if result.Drops[0].OwnerPlayerID != event.OwnerPlayerID {
		t.Fatalf("owner = %q, want %q", result.Drops[0].OwnerPlayerID, event.OwnerPlayerID)
	}

	duplicate, err := service.CreateDropsForNPCKill(event, table)
	if err != nil {
		t.Fatalf("duplicate CreateDropsForNPCKill() error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("Duplicate = false, want true")
	}
	if len(duplicate.Drops) != 1 || duplicate.Drops[0].ID != result.Drops[0].ID {
		t.Fatalf("duplicate drops = %+v, want original drop id %q", duplicate.Drops, result.Drops[0].ID)
	}
}

func TestNewServiceRejectsInconsistentDurations(t *testing.T) {
	_, err := loot.NewService(loot.Config{
		Clock:             testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)),
		Cargo:             fakeCargoAdder{},
		OwnerLockDuration: 2 * time.Minute,
		PublicDuration:    2 * time.Minute,
		TotalLifetime:     3 * time.Minute,
	})

	if !errors.Is(err, loot.ErrInvalidLootDurations) {
		t.Fatalf("NewService() error = %v, want ErrInvalidLootDurations", err)
	}
}

func TestPickupDropOwnerLockPublicAndExpiredWindows(t *testing.T) {
	service, clock, inventory, progressionService := newLootService(t, []int{0, 0}, []float64{0, 0})
	drop := createOneDrop(t, service)
	cargoLocation := mustCargoLocation(t, "ship_1")

	_, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           "player_2",
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, loot.ErrDropOwnerLocked) {
		t.Fatalf("non-owner locked pickup error = %v, want ErrDropOwnerLocked", err)
	}

	clock.Advance(loot.DefaultOwnerLockDuration)
	publicResult, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           "player_2",
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if err != nil {
		t.Fatalf("public PickupDrop() error = %v", err)
	}
	if publicResult.Drop.ClaimedBy != foundation.PlayerID("player_2") {
		t.Fatalf("claimed by = %q, want player_2", publicResult.Drop.ClaimedBy)
	}
	if inventory.TotalItemQuantity("player_2", rawOreDefinition(t).ItemID, cargoLocation) != drop.Quantity {
		t.Fatalf("cargo quantity not added")
	}
	if publicResult.XPResult == nil || publicResult.XPResult.Duplicate {
		t.Fatalf("XPResult = %+v, want first loot XP grant", publicResult.XPResult)
	}
	if _, err := progressionService.GrantXP(progression.GrantXPInput{
		PlayerID:       "player_2",
		Amount:         999,
		SourceType:     progression.XPSourceTypeLoot,
		SourceID:       progression.XPSourceID(drop.ID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("loot_pickup:" + drop.ID.String()),
	}); err != nil {
		t.Fatalf("manual duplicate GrantXP() error = %v", err)
	}

	expiredDrop := createOneDropWithEvent(t, service, "npc_2")
	clock.Advance(loot.DefaultTotalLifetime)
	_, err = service.PickupDrop(loot.PickupInput{
		PlayerID:           expiredDrop.OwnerPlayerID,
		DropID:             expiredDrop.ID,
		Viewer:             viewerAt(expiredDrop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, loot.ErrDropExpired) {
		t.Fatalf("expired PickupDrop() error = %v, want ErrDropExpired", err)
	}
}

func TestPickupDropReportsXPFailureWithoutUndoingClaimOrCargo(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
		Cargo:       cargo,
		Progression: failingXPGranter{},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	drop := createOneDrop(t, service)
	cargoLocation := mustCargoLocation(t, "ship_1")

	result, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           drop.OwnerPlayerID,
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if err != nil {
		t.Fatalf("PickupDrop() error = %v, want nil with XPError in result", err)
	}
	if result.XPError == nil {
		t.Fatal("XPError = nil, want progression failure recorded")
	}
	if result.Drop.ClaimedAt == nil || result.Drop.ClaimedBy != drop.OwnerPlayerID {
		t.Fatalf("drop claim = %+v, want claimed by owner", result.Drop)
	}
	if inventory.TotalItemQuantity(drop.OwnerPlayerID, rawOreDefinition(t).ItemID, cargoLocation) != drop.Quantity {
		t.Fatal("cargo item was not added despite successful pickup")
	}
}

func TestPickupDropRejectsFarHiddenAndCargoFullWithoutClaim(t *testing.T) {
	service, _, _, _ := newLootService(t, []int{0, 0, 0}, []float64{0, 0, 0})
	cargoLocation := mustCargoLocation(t, "ship_1")

	farDrop := createOneDropWithEvent(t, service, "npc_far")
	_, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           farDrop.OwnerPlayerID,
		DropID:             farDrop.ID,
		Viewer:             viewerAt(world.Vec2{X: 500, Y: 0}),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, loot.ErrPickupOutOfRange) {
		t.Fatalf("far PickupDrop() error = %v, want ErrPickupOutOfRange", err)
	}

	hiddenDrop := createOneDropWithEvent(t, service, "npc_hidden")
	_, err = service.PickupDrop(loot.PickupInput{
		PlayerID:           hiddenDrop.OwnerPlayerID,
		DropID:             hiddenDrop.ID,
		Viewer:             viewerWithRadar(world.Vec2{X: hiddenDrop.Position.X + 10, Y: hiddenDrop.Position.Y}, 1),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, loot.ErrPickupNotVisible) {
		t.Fatalf("hidden PickupDrop() error = %v, want ErrPickupNotVisible", err)
	}

	fullDrop := createOneDropWithEvent(t, service, "npc_full")
	_, err = service.PickupDrop(loot.PickupInput{
		PlayerID:           fullDrop.OwnerPlayerID,
		DropID:             fullDrop.ID,
		Viewer:             viewerAt(fullDrop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 1,
	})
	if !errors.Is(err, economy.ErrCargoCapacityExceeded) {
		t.Fatalf("cargo full PickupDrop() error = %v, want ErrCargoCapacityExceeded", err)
	}
	after, ok := service.Drop(fullDrop.ID)
	if !ok || after.ClaimedAt != nil {
		t.Fatalf("drop after cargo full = %+v, ok %t; want unclaimed", after, ok)
	}
}

func TestConcurrentPickupOnlyOneSucceeds(t *testing.T) {
	service, clock, inventory, _ := newLootService(t, []int{0}, []float64{0})
	drop := createOneDrop(t, service)
	clock.Advance(loot.DefaultOwnerLockDuration)
	cargoLocation := mustCargoLocation(t, "ship_1")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for index := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = service.PickupDrop(loot.PickupInput{
				PlayerID:           foundation.PlayerID("player_1"),
				DropID:             drop.ID,
				Viewer:             viewerAt(drop.Position),
				ActiveCargo:        cargoLocation,
				CargoCapacityUnits: 100,
			})
		}(index)
	}
	wg.Wait()

	successes := 0
	claimed := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, loot.ErrDropClaimed) {
			claimed++
		}
	}
	if successes != 1 || claimed != 1 {
		t.Fatalf("pickup results = %v, want one success and one ErrDropClaimed", errs)
	}
	if inventory.TotalItemQuantity("player_1", rawOreDefinition(t).ItemID, cargoLocation) != drop.Quantity {
		t.Fatalf("cargo quantity duplicated or missing")
	}
}

func TestVisibleDropsFiltersByVisibilityAndOmitsClaimedOrExpired(t *testing.T) {
	service, clock, _, _ := newLootService(t, []int{0, 0}, []float64{0, 0})
	visible := createOneDropWithEvent(t, service, "npc_visible")
	createOneDropWithEvent(t, service, "npc_hidden")

	payloads := service.VisibleDrops(viewerAt(visible.Position))
	if len(payloads) != 2 {
		t.Fatalf("visible drops len = %d, want 2 while both are in radar", len(payloads))
	}

	if _, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           visible.OwnerPlayerID,
		DropID:             visible.ID,
		Viewer:             viewerAt(visible.Position),
		ActiveCargo:        mustCargoLocation(t, "ship_1"),
		CargoCapacityUnits: 100,
	}); err != nil {
		t.Fatalf("PickupDrop() error = %v", err)
	}
	clock.Advance(loot.DefaultTotalLifetime)

	payloads = service.VisibleDrops(viewerAt(visible.Position))
	if len(payloads) != 0 {
		t.Fatalf("visible drops after claimed/expired len = %d, want 0", len(payloads))
	}
}

func newLootService(t *testing.T, ints []int, floats []float64) (*loot.Service, *testutil.FakeClock, *economy.InventoryService, *progression.ProgressionService) {
	t.Helper()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	progressionService := progression.NewProgressionService(clock, nil)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		RNG:         testutil.NewFakeRNG(ints, floats),
		Cargo:       cargo,
		Progression: progressionService,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service, clock, inventory, progressionService
}

func createOneDrop(t *testing.T, service *loot.Service) loot.Drop {
	t.Helper()
	return createOneDropWithEvent(t, service, "npc_1")
}

func createOneDropWithEvent(t *testing.T, service *loot.Service, npcID world.EntityID) loot.Drop {
	t.Helper()
	event := npcKilledEvent()
	event.NPCEntityID = npcID
	event.SourceID = npcID
	result, err := service.CreateDropsForNPCKill(event, lootTable(t, 3, 3, 1))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill(%s) error = %v", npcID, err)
	}
	if len(result.Drops) != 1 {
		t.Fatalf("drops len = %d, want 1", len(result.Drops))
	}
	return result.Drops[0]
}

func npcKilledEvent() combat.NPCKilledEvent {
	return combat.NPCKilledEvent{
		SourceID:      "npc_1",
		NPCEntityID:   "npc_1",
		WorldID:       "world_1",
		ZoneID:        "zone_1",
		Position:      world.Vec2{X: 10, Y: 0},
		OwnerPlayerID: "player_1",
		KilledAt:      time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
}

func lootTable(t *testing.T, minQuantity int64, maxQuantity int64, chance float64) loot.LootTable {
	t.Helper()
	source, err := catalog.NewLootTableSource("loot_table_v0", "v1")
	if err != nil {
		t.Fatalf("NewLootTableSource() error = %v", err)
	}
	return loot.LootTable{
		Source: source,
		Rows: []loot.LootRow{
			{
				ItemDefinition: rawOreDefinition(t),
				MinQuantity:    minQuantity,
				MaxQuantity:    maxQuantity,
				Chance:         chance,
			},
		},
	}
}

func rawOreDefinition(t *testing.T) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings("raw_ore", "v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		"raw_ore",
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		mustQuantity(t, 999),
		mustQuantity(t, 2),
		[]economy.TradeFlag{economy.TradeFlagDroppable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v", err)
	}
	return definition
}

func mustQuantity(t *testing.T, value int64) foundation.Quantity {
	t.Helper()
	quantity, err := foundation.NewQuantity(value)
	if err != nil {
		t.Fatalf("NewQuantity(%d) error = %v", value, err)
	}
	return quantity
}

func mustCargoLocation(t *testing.T, id string) economy.ItemLocation {
	t.Helper()
	location, err := economy.NewItemLocation(economy.LocationKindShipCargo, id)
	if err != nil {
		t.Fatalf("NewItemLocation() error = %v", err)
	}
	return location
}

func viewerAt(position world.Vec2) loot.Viewer {
	return viewerWithRadar(position, 500)
}

func viewerWithRadar(position world.Vec2, radar float64) loot.Viewer {
	snapshot := stats.NewStatSnapshot(
		"player_1",
		"ship_1",
		1,
		stats.EffectiveStats{
			Exploration: stats.ExplorationStats{
				RadarRange: radar,
			},
		},
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	)
	return loot.Viewer{
		WorldID:    "world_1",
		ZoneID:     "zone_1",
		Position:   position,
		RadarRange: visibility.RadarRangeFromStatSnapshot(snapshot),
	}
}

type fakeCargoAdder struct{}

func (fakeCargoAdder) AddItem(economy.CargoAddItemInput) (economy.AddItemResult, error) {
	return economy.AddItemResult{}, nil
}

type failingXPGranter struct{}

func (failingXPGranter) GrantXP(progression.GrantXPInput) (progression.GrantXPResult, error) {
	return progression.GrantXPResult{}, fmt.Errorf("xp store unavailable")
}
