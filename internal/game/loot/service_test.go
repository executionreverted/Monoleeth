package loot_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/observability"
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
	if got, want := len(result.ScheduledTasks), 2; got != want {
		t.Fatalf("scheduled tasks len = %d, want %d", got, want)
	}
	if result.ScheduledTasks[0].Kind != loot.ScheduledDropTaskOwnerLockExpired ||
		result.ScheduledTasks[0].DropID != result.Drops[0].ID ||
		!result.ScheduledTasks[0].DueAt.Equal(result.Drops[0].OwnerLockUntil) {
		t.Fatalf("owner-lock scheduled task = %+v, want drop owner-lock expiry", result.ScheduledTasks[0])
	}
	if result.ScheduledTasks[1].Kind != loot.ScheduledDropTaskDespawn ||
		result.ScheduledTasks[1].DropID != result.Drops[0].ID ||
		!result.ScheduledTasks[1].DueAt.Equal(result.Drops[0].ExpiresAt) {
		t.Fatalf("despawn scheduled task = %+v, want drop expiry", result.ScheduledTasks[1])
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
	if len(duplicate.ScheduledTasks) != 2 ||
		duplicate.ScheduledTasks[0].DropID != result.Drops[0].ID ||
		duplicate.ScheduledTasks[1].DropID != result.Drops[0].ID {
		t.Fatalf("duplicate scheduled tasks = %+v, want retry-safe tasks for original drop %q", duplicate.ScheduledTasks, result.Drops[0].ID)
	}
}

func TestLootMetricsRecordCreatedAndPickedQuantities(t *testing.T) {
	service, clock, _, _ := newLootService(t, []int{0}, []float64{0})
	recorder := observability.NewMetricRecorder()
	service.SetMetricRecorder(recorder)
	drop := createOneDrop(t, service)

	created := requireLootMetricCounter(t, recorder.Snapshot(), observability.MetricLootCreatedPerSecond)
	if created.Value != drop.Quantity {
		t.Fatalf("loot created metric = %d, want %d", created.Value, drop.Quantity)
	}
	assertLootMetricLabels(t, created.Labels, []observability.Label{
		{Name: "item_id", Value: rawOreDefinition(t).ItemID.String()},
		{Name: "source_type", Value: loot.DropSourceNPCDeath.String()},
	})

	clock.Advance(loot.DefaultOwnerLockDuration)
	if _, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           "player_2",
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        mustCargoLocation(t, "ship_1"),
		CargoCapacityUnits: 100,
	}); err != nil {
		t.Fatalf("PickupDrop() error = %v", err)
	}

	picked := requireLootMetricCounter(t, recorder.Snapshot(), observability.MetricLootPickedPerSecond)
	if picked.Value != drop.Quantity {
		t.Fatalf("loot picked metric = %d, want %d", picked.Value, drop.Quantity)
	}
	assertLootMetricLabels(t, picked.Labels, []observability.Label{
		{Name: "item_id", Value: rawOreDefinition(t).ItemID.String()},
		{Name: "source_type", Value: loot.DropSourceNPCDeath.String()},
	})
}

func TestLootCreatedMetricDoesNotDoubleCountDuplicateSource(t *testing.T) {
	service, _, _, _ := newLootService(t, []int{0}, []float64{0})
	recorder := observability.NewMetricRecorder()
	service.SetMetricRecorder(recorder)
	event := npcKilledEvent()
	table := lootTable(t, 3, 3, 1)

	first, err := service.CreateDropsForNPCKill(event, table)
	if err != nil {
		t.Fatalf("first CreateDropsForNPCKill() error = %v", err)
	}
	duplicate, err := service.CreateDropsForNPCKill(event, table)
	if err != nil {
		t.Fatalf("duplicate CreateDropsForNPCKill() error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate result Duplicate = false, want true")
	}

	created := requireLootMetricCounter(t, recorder.Snapshot(), observability.MetricLootCreatedPerSecond)
	if created.Value != first.Drops[0].Quantity {
		t.Fatalf("loot created metric = %d, want only first quantity %d", created.Value, first.Drops[0].Quantity)
	}
}

func TestLootMetricFailureDoesNotBlockCreateOrPickup(t *testing.T) {
	service, clock, _, _ := newLootService(t, []int{0}, []float64{0})
	service.SetMetricRecorder(failingLootMetrics{})

	drop := createOneDrop(t, service)
	clock.Advance(loot.DefaultOwnerLockDuration)
	if _, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           "player_2",
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        mustCargoLocation(t, "ship_1"),
		CargoCapacityUnits: 100,
	}); err != nil {
		t.Fatalf("PickupDrop() error = %v, want success despite metric failure", err)
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
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)
	drop := createOneDrop(t, service)
	recorder.Reset()
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
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootPickedUp)
	var pickedUpPayload loot.PickedUpPayload
	if err := json.Unmarshal(recorder.Events()[0].Payload, &pickedUpPayload); err != nil {
		t.Fatalf("unmarshal loot picked payload: %v", err)
	}
	if pickedUpPayload.PlayerID != foundation.PlayerID("player_2") ||
		pickedUpPayload.ItemID != drop.ItemDefinition.ItemID ||
		pickedUpPayload.Quantity != drop.Quantity ||
		pickedUpPayload.State != loot.DropStateClaimed {
		t.Fatalf("picked payload = %+v, want player_2 %q x%d claimed", pickedUpPayload, drop.ItemDefinition.ItemID, drop.Quantity)
	}
	if publicResult.XPResult == nil || publicResult.XPResult.Duplicate {
		t.Fatalf("XPResult = %+v, want first loot XP grant", publicResult.XPResult)
	}
	assertLootXPReconciliation(t, publicResult.Drop, loot.LootXPReconciliationGranted, "")
	storedPublicDrop, ok := service.Drop(publicResult.Drop.ID)
	if !ok {
		t.Fatalf("Drop(%q) ok = false, want true", publicResult.Drop.ID)
	}
	assertLootXPReconciliation(t, storedPublicDrop, loot.LootXPReconciliationGranted, "")
	if _, err := progressionService.GrantXP(progression.GrantXPInput{
		PlayerID:       "player_2",
		Amount:         999,
		SourceType:     progression.XPSourceTypeLoot,
		SourceID:       progression.XPSourceID(drop.ID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("loot_pickup:" + drop.ID.String()),
		Authority:      progression.XPGrantAuthorityLootService,
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
	assertLootXPReconciliation(t, result.Drop, loot.LootXPReconciliationFailed, "xp store unavailable")
	storedDrop, ok := service.Drop(result.Drop.ID)
	if !ok {
		t.Fatalf("Drop(%q) ok = false, want true", result.Drop.ID)
	}
	assertLootXPReconciliation(t, storedDrop, loot.LootXPReconciliationFailed, "xp store unavailable")
	if inventory.TotalItemQuantity(drop.OwnerPlayerID, rawOreDefinition(t).ItemID, cargoLocation) != drop.Quantity {
		t.Fatal("cargo item was not added despite successful pickup")
	}
}

func TestPickupDropRecordsDuplicateLootXPReconciliationWithoutUndoingClaimOrCargo(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
		Cargo:       cargo,
		Progression: duplicateXPGranter{},
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
		t.Fatalf("PickupDrop() error = %v", err)
	}
	if result.XPError != nil {
		t.Fatalf("XPError = %v, want nil duplicate result", result.XPError)
	}
	if result.XPResult == nil || !result.XPResult.Duplicate {
		t.Fatalf("XPResult = %+v, want duplicate loot XP result", result.XPResult)
	}
	if result.Drop.ClaimedAt == nil || result.Drop.ClaimedBy != drop.OwnerPlayerID {
		t.Fatalf("drop claim = %+v, want claimed by owner", result.Drop)
	}
	if inventory.TotalItemQuantity(drop.OwnerPlayerID, rawOreDefinition(t).ItemID, cargoLocation) != drop.Quantity {
		t.Fatal("cargo item was not added despite duplicate loot XP")
	}
	assertLootXPReconciliation(t, result.Drop, loot.LootXPReconciliationDuplicate, "")

	storedDrop, ok := service.Drop(result.Drop.ID)
	if !ok {
		t.Fatalf("Drop(%q) ok = false, want true", result.Drop.ID)
	}
	assertLootXPReconciliation(t, storedDrop, loot.LootXPReconciliationDuplicate, "")
}

func TestLootXPOutboxReplayDuplicateDoesNotGrantXPAgain(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 25, 21, 0, 0, 0, time.UTC))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	progressionService := progression.NewProgressionService(clock, nil)
	outboxStore := newLootXPOutboxStore(t)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
		Cargo:       cargo,
		Progression: progressionService,
		XPOutbox:    outboxStore,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	drop := createOneDrop(t, service)
	cargoLocation := mustCargoLocation(t, "ship_1")

	pickup, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           drop.OwnerPlayerID,
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if err != nil {
		t.Fatalf("PickupDrop() error = %v", err)
	}
	if pickup.XPResult == nil || pickup.XPResult.Duplicate || pickup.XPResult.Snapshot.Player.MainXP != 5 {
		t.Fatalf("pickup XPResult = %+v, want first 5 XP grant", pickup.XPResult)
	}

	outboxID := "loot_xp:loot_pickup:" + drop.ID.String()
	row, ok, err := outboxStore.LoadOutboxRow(context.Background(), outboxID)
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow(%q) = ok %v err %v, want true nil", outboxID, ok, err)
	}
	if row.Status != economy.OutboxStatusPending || row.EventType != loot.EventLootXPReconciliationRequested {
		t.Fatalf("outbox row = %+v, want pending loot XP reconciliation row", row)
	}

	publisher, err := loot.NewLootXPOutboxPublisher(progressionService)
	if err != nil {
		t.Fatalf("NewLootXPOutboxPublisher() error = %v, want nil", err)
	}
	worker := economy.OutboxReplayWorker{
		Store:         outboxStore,
		Publisher:     publisher,
		LeaseOwner:    "loot-xp-test-worker",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		RetryDelay:    time.Second,
		Now:           clock.Now,
	}

	replayed, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil", err)
	}
	if replayed != (economy.OutboxReplayResult{Loaded: 1, Leased: 1, Published: 1}) {
		t.Fatalf("RunOnce() result = %+v, want one published duplicate replay", replayed)
	}
	snapshot, err := progressionService.GetProgressionSnapshot(drop.OwnerPlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.MainXP != 5 {
		t.Fatalf("main XP after outbox replay = %d, want 5", snapshot.Player.MainXP)
	}
	stored, ok, err := outboxStore.LoadOutboxRow(context.Background(), outboxID)
	if err != nil || !ok {
		t.Fatalf("LoadOutboxRow(published) = ok %v err %v, want true nil", ok, err)
	}
	if stored.Status != economy.OutboxStatusPublished || stored.AttemptCount != 1 || stored.PublishedAt.IsZero() {
		t.Fatalf("published row = %+v, want published attempt 1", stored)
	}

	again, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce(second) error = %v, want nil", err)
	}
	if again != (economy.OutboxReplayResult{}) {
		t.Fatalf("RunOnce(second) result = %+v, want no due rows", again)
	}
	snapshot, err = progressionService.GetProgressionSnapshot(drop.OwnerPlayerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot(second) error = %v, want nil", err)
	}
	if snapshot.Player.MainXP != 5 {
		t.Fatalf("main XP after second replay = %d, want 5", snapshot.Player.MainXP)
	}
}

func TestPlayerDeathDropPickupDoesNotGrantLootXP(t *testing.T) {
	service, _, inventory, progressionService := newLootService(t, nil, nil)
	result, err := service.CreateDropsForPlayerDeath(loot.CreatePlayerDeathDropsInput{
		SourceID:      "death_event_1",
		DeadPlayerID:  "player_dead",
		OwnerPlayerID: "player_2",
		WorldID:       "world_1",
		ZoneID:        "zone_1",
		Position:      world.Vec2{X: 10, Y: 0},
		Items: []loot.DropItem{
			{ItemDefinition: rawOreDefinition(t), Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateDropsForPlayerDeath() error = %v", err)
	}
	if len(result.Drops) != 1 {
		t.Fatalf("player-death drops len = %d, want 1", len(result.Drops))
	}
	if result.Drops[0].SourceType != loot.DropSourcePlayerDeath {
		t.Fatalf("SourceType = %q, want player_death", result.Drops[0].SourceType)
	}
	duplicate, err := service.CreateDropsForPlayerDeath(loot.CreatePlayerDeathDropsInput{
		SourceID:      "death_event_1",
		DeadPlayerID:  "player_dead",
		OwnerPlayerID: "player_2",
		WorldID:       "world_1",
		ZoneID:        "zone_1",
		Position:      world.Vec2{X: 10, Y: 0},
		Items: []loot.DropItem{
			{ItemDefinition: rawOreDefinition(t), Quantity: 99},
		},
	})
	if err != nil {
		t.Fatalf("duplicate CreateDropsForPlayerDeath() error = %v", err)
	}
	if !duplicate.Duplicate || len(duplicate.Drops) != 1 || duplicate.Drops[0].Quantity != 2 {
		t.Fatalf("duplicate player-death result = %+v, want original drop without new quantity", duplicate)
	}
	if len(duplicate.ScheduledTasks) != 2 ||
		duplicate.ScheduledTasks[0].DropID != result.Drops[0].ID ||
		duplicate.ScheduledTasks[1].DropID != result.Drops[0].ID {
		t.Fatalf("duplicate player-death scheduled tasks = %+v, want retry-safe tasks for original drop %q", duplicate.ScheduledTasks, result.Drops[0].ID)
	}

	cargoLocation := mustCargoLocation(t, "ship_1")
	pickup, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           "player_2",
		DropID:             result.Drops[0].ID,
		Viewer:             viewerAt(result.Drops[0].Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if err != nil {
		t.Fatalf("PickupDrop(player death) error = %v", err)
	}
	if pickup.XPResult != nil || pickup.XPError != nil {
		t.Fatalf("player-death pickup XPResult = %+v XPError = %v, want no XP attempt", pickup.XPResult, pickup.XPError)
	}
	assertLootXPReconciliation(t, pickup.Drop, loot.LootXPReconciliationNotEligible, "")
	if inventory.TotalItemQuantity("player_2", rawOreDefinition(t).ItemID, cargoLocation) != 2 {
		t.Fatalf("cargo quantity from player-death drop mismatch")
	}
	snapshot, err := progressionService.GetProgressionSnapshot("player_2")
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v", err)
	}
	if snapshot.Player.MainXP != 0 {
		t.Fatalf("player_2 main XP after player-death pickup = %d, want 0", snapshot.Player.MainXP)
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

func TestPickupDropRejectsCrossMapViewerWithoutClaim(t *testing.T) {
	service, _, inventory, _ := newLootService(t, []int{0}, []float64{0})
	event := npcKilledEvent()
	event.NPCEntityID = "npc_map_two"
	event.SourceID = event.NPCEntityID
	event.ZoneID = "map_1_2"
	result, err := service.CreateDropsForNPCKill(event, lootTable(t, 3, 3, 1))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill(map_1_2) error = %v", err)
	}
	if len(result.Drops) != 1 {
		t.Fatalf("drops len = %d, want 1", len(result.Drops))
	}
	drop := result.Drops[0]
	cargoLocation := mustCargoLocation(t, "ship_1")
	viewer := viewerAt(drop.Position)
	viewer.ZoneID = "map_1_1"

	_, err = service.PickupDrop(loot.PickupInput{
		PlayerID:           drop.OwnerPlayerID,
		DropID:             drop.ID,
		Viewer:             viewer,
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
	})
	if !errors.Is(err, loot.ErrPickupNotVisible) {
		t.Fatalf("cross-map PickupDrop() error = %v, want ErrPickupNotVisible", err)
	}
	after, ok := service.Drop(drop.ID)
	if !ok || after.ClaimedAt != nil || after.ClaimedBy != "" {
		t.Fatalf("drop after cross-map pickup = %+v, ok %t; want unclaimed", after, ok)
	}
	if got := inventory.TotalItemQuantity(drop.OwnerPlayerID, rawOreDefinition(t).ItemID, cargoLocation); got != 0 {
		t.Fatalf("cargo quantity after rejected cross-map pickup = %d, want 0", got)
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

func TestHandleScheduledDropTaskEmitsOwnerLockAndExpiryIdempotently(t *testing.T) {
	service, clock, _, _ := newLootService(t, []int{0}, []float64{0})
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)
	result, err := service.CreateDropsForNPCKill(npcKilledEvent(), lootTable(t, 3, 3, 1))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootCreated)
	recorder.Reset()

	ownerTask := result.ScheduledTasks[0]
	early, err := service.HandleScheduledDropTask(ownerTask)
	if err != nil {
		t.Fatalf("early HandleScheduledDropTask(owner) error = %v", err)
	}
	if early.Handled {
		t.Fatal("early owner-lock task Handled = true, want false")
	}
	if !early.RetryAt.Equal(ownerTask.DueAt) {
		t.Fatalf("early owner-lock task RetryAt = %s, want %s", early.RetryAt, ownerTask.DueAt)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	clock.Advance(loot.DefaultOwnerLockDuration)
	handled, err := service.HandleScheduledDropTask(ownerTask)
	if err != nil {
		t.Fatalf("HandleScheduledDropTask(owner) error = %v", err)
	}
	if !handled.Handled {
		t.Fatal("owner-lock task Handled = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootOwnerLockExpired)
	recorder.Reset()

	again, err := service.HandleScheduledDropTask(ownerTask)
	if err != nil {
		t.Fatalf("duplicate HandleScheduledDropTask(owner) error = %v", err)
	}
	if again.Handled {
		t.Fatal("duplicate owner-lock task Handled = true, want false")
	}
	if !again.RetryAt.IsZero() {
		t.Fatalf("duplicate owner-lock task RetryAt = %s, want zero", again.RetryAt)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	clock.Advance(loot.DefaultTotalLifetime - loot.DefaultOwnerLockDuration)
	expireTask := result.ScheduledTasks[1]
	expired, err := service.HandleScheduledDropTask(expireTask)
	if err != nil {
		t.Fatalf("HandleScheduledDropTask(expire) error = %v", err)
	}
	if !expired.Handled {
		t.Fatal("expire task Handled = false, want true")
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootExpired)
}

func TestHandleScheduledDropTaskNoOpsForClaimedDrop(t *testing.T) {
	service, _, _, _ := newLootService(t, []int{0}, []float64{0})
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)
	result, err := service.CreateDropsForNPCKill(npcKilledEvent(), lootTable(t, 3, 3, 1))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	drop := result.Drops[0]
	if _, err := service.PickupDrop(loot.PickupInput{
		PlayerID:           drop.OwnerPlayerID,
		DropID:             drop.ID,
		Viewer:             viewerAt(drop.Position),
		ActiveCargo:        mustCargoLocation(t, "ship_1"),
		CargoCapacityUnits: 100,
	}); err != nil {
		t.Fatalf("PickupDrop() error = %v", err)
	}
	recorder.Reset()

	for _, task := range result.ScheduledTasks {
		taskResult, err := service.HandleScheduledDropTask(task)
		if err != nil {
			t.Fatalf("HandleScheduledDropTask(%s) error = %v", task.Kind, err)
		}
		if taskResult.Handled {
			t.Fatalf("claimed drop scheduled task %s Handled = true, want false", task.Kind)
		}
	}
	testutil.AssertRecordedEventTypes(t, recorder)
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

func assertLootXPReconciliation(t *testing.T, drop loot.Drop, status loot.LootXPReconciliationStatus, wantError string) {
	t.Helper()
	if drop.XPReconciliation == nil {
		t.Fatalf("drop %q XPReconciliation = nil, want %s", drop.ID, status)
	}
	reconciliation := drop.XPReconciliation
	if reconciliation.Status != status {
		t.Fatalf("drop %q XPReconciliation.Status = %q, want %q", drop.ID, reconciliation.Status, status)
	}
	if reconciliation.PlayerID != drop.ClaimedBy {
		t.Fatalf("drop %q XPReconciliation.PlayerID = %q, want claimed by %q", drop.ID, reconciliation.PlayerID, drop.ClaimedBy)
	}
	if reconciliation.SourceType != progression.XPSourceTypeLoot {
		t.Fatalf("drop %q XPReconciliation.SourceType = %q, want loot", drop.ID, reconciliation.SourceType)
	}
	if reconciliation.SourceID != progression.XPSourceID(drop.ID.String()) {
		t.Fatalf("drop %q XPReconciliation.SourceID = %q, want drop id", drop.ID, reconciliation.SourceID)
	}
	if reconciliation.IdempotencyKey != progression.XPIdempotencyKey("loot_pickup:"+drop.ID.String()) {
		t.Fatalf("drop %q XPReconciliation.IdempotencyKey = %q, want loot_pickup key", drop.ID, reconciliation.IdempotencyKey)
	}
	if reconciliation.AttemptedAt.IsZero() {
		t.Fatalf("drop %q XPReconciliation.AttemptedAt is zero", drop.ID)
	}
	if wantError != "" && reconciliation.Error != wantError {
		t.Fatalf("drop %q XPReconciliation.Error = %q, want %q", drop.ID, reconciliation.Error, wantError)
	}
	if wantError == "" && reconciliation.Error != "" {
		t.Fatalf("drop %q XPReconciliation.Error = %q, want empty", drop.ID, reconciliation.Error)
	}
	if status == loot.LootXPReconciliationGranted && reconciliation.GrantedAt == nil {
		t.Fatalf("drop %q XPReconciliation.GrantedAt = nil, want grant timestamp", drop.ID)
	}
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

type duplicateXPGranter struct{}

func (duplicateXPGranter) GrantXP(progression.GrantXPInput) (progression.GrantXPResult, error) {
	return progression.GrantXPResult{Duplicate: true}, nil
}

type lootXPOutboxStore struct {
	mu   sync.Mutex
	rows map[string]economy.OutboxRow
}

func newLootXPOutboxStore(t *testing.T) *lootXPOutboxStore {
	t.Helper()
	return &lootXPOutboxStore{rows: make(map[string]economy.OutboxRow)}
}

func (store *lootXPOutboxStore) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, exists := store.rows[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	store.rows[inserted.OutboxID] = inserted.Clone()
	return nil
}

func (store *lootXPOutboxStore) LoadOutboxRow(ctx context.Context, outboxID string) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.rows[outboxID]
	return row.Clone(), ok, nil
}

func (store *lootXPOutboxStore) LoadDueOutboxRows(
	ctx context.Context,
	query economy.OutboxDueRowsQuery,
) ([]economy.OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	rows := make([]economy.OutboxRow, 0, query.Limit)
	for _, row := range store.rows {
		if row.Status != economy.OutboxStatusPending && row.Status != economy.OutboxStatusFailed {
			continue
		}
		if row.AvailableAt.After(query.Now) {
			continue
		}
		rows = append(rows, row.Clone())
	}
	sort.Slice(rows, func(left int, right int) bool {
		if !rows[left].AvailableAt.Equal(rows[right].AvailableAt) {
			return rows[left].AvailableAt.Before(rows[right].AvailableAt)
		}
		if !rows[left].CreatedAt.Equal(rows[right].CreatedAt) {
			return rows[left].CreatedAt.Before(rows[right].CreatedAt)
		}
		return rows[left].OutboxID < rows[right].OutboxID
	})
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
	}
	return rows, nil
}

func (store *lootXPOutboxStore) LeaseOutboxRow(
	ctx context.Context,
	input economy.OutboxLeaseInput,
) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.rows[input.OutboxID]
	if !ok || !lootXPOutboxLeaseEligible(row, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusLeased
	row.LeaseOwner = input.LeaseOwner
	row.LeasedUntil = input.LeasedUntil
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *lootXPOutboxStore) MarkOutboxPublished(
	ctx context.Context,
	input economy.OutboxPublishInput,
) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.rows[input.OutboxID]
	if !ok || !lootXPOutboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.Status = economy.OutboxStatusPublished
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.AttemptCount++
	row.LastError = ""
	row.UpdatedAt = input.Now
	row.PublishedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func (store *lootXPOutboxStore) MarkOutboxFailed(
	ctx context.Context,
	input economy.OutboxFailureInput,
) (economy.OutboxRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	if err := input.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	row, ok := store.rows[input.OutboxID]
	if !ok || !lootXPOutboxLeaseMatches(row, input.LeaseOwner, input.Now) {
		return economy.OutboxRow{}, false, nil
	}
	row.AttemptCount++
	if row.AttemptCount >= row.MaxAttempts {
		row.Status = economy.OutboxStatusDead
	} else {
		row.Status = economy.OutboxStatusFailed
	}
	row.AvailableAt = input.AvailableAt
	row.LeaseOwner = ""
	row.LeasedUntil = time.Time{}
	row.LastError = input.LastError
	row.UpdatedAt = input.Now
	if err := row.Validate(); err != nil {
		return economy.OutboxRow{}, false, err
	}
	store.rows[input.OutboxID] = row.Clone()
	return row.Clone(), true, nil
}

func lootXPOutboxLeaseEligible(row economy.OutboxRow, now time.Time) bool {
	if (row.Status == economy.OutboxStatusPending || row.Status == economy.OutboxStatusFailed) && !row.AvailableAt.After(now) {
		return true
	}
	return row.Status == economy.OutboxStatusLeased && !row.LeasedUntil.IsZero() && !row.LeasedUntil.After(now)
}

func lootXPOutboxLeaseMatches(row economy.OutboxRow, owner string, now time.Time) bool {
	return row.Status == economy.OutboxStatusLeased &&
		row.LeaseOwner == owner &&
		!row.LeasedUntil.IsZero() &&
		row.LeasedUntil.After(now)
}

type failingLootMetrics struct{}

func (failingLootMetrics) RecordLootCreated(string, foundation.ItemID, int64) error {
	return errors.New("metric sink unavailable")
}

func (failingLootMetrics) RecordLootPicked(string, foundation.ItemID, int64) error {
	return errors.New("metric sink unavailable")
}

func requireLootMetricCounter(t *testing.T, snapshot observability.MetricSnapshot, name string) observability.CounterSnapshot {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			return counter
		}
	}
	t.Fatalf("missing counter %q in %#v", name, snapshot.Counters)
	return observability.CounterSnapshot{}
}

func assertLootMetricLabels(t *testing.T, got, want []observability.Label) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("label[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
