package worker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestScheduleLootDropTasksMapsDueTasksToLootContract(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	zoneWorker, err := NewWorker(Config{
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		TickDelta: time.Second,
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	tasks := []loot.ScheduledDropTask{
		{
			ID:     "drop-1:owner",
			Kind:   loot.ScheduledDropTaskOwnerLockExpired,
			DropID: "drop-1",
			DueAt:  clock.Now().Add(time.Second),
		},
		{
			ID:     "drop-1:despawn",
			Kind:   loot.ScheduledDropTaskDespawn,
			DropID: "drop-1",
			DueAt:  clock.Now().Add(2 * time.Second),
		},
	}

	scheduled, err := zoneWorker.ScheduleLootDropTasks(tasks)
	if err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}
	if len(scheduled) != 2 {
		t.Fatalf("scheduled len = %d, want 2", len(scheduled))
	}

	clock.Advance(time.Second)
	result := zoneWorker.Tick()
	if len(result.DueTasks) != 1 {
		t.Fatalf("due task len = %d, want 1", len(result.DueTasks))
	}
	lootTask, ok := LootScheduledDropTask(result.DueTasks[0])
	if !ok {
		t.Fatalf("LootScheduledDropTask(%+v) ok = false, want true", result.DueTasks[0])
	}
	if lootTask.Kind != loot.ScheduledDropTaskOwnerLockExpired || lootTask.DropID != "drop-1" {
		t.Fatalf("loot scheduled task = %+v, want owner-lock task for drop-1", lootTask)
	}

	clock.Advance(time.Second)
	result = zoneWorker.Tick()
	if len(result.DueTasks) != 1 {
		t.Fatalf("second due task len = %d, want 1", len(result.DueTasks))
	}
	lootTask, ok = LootScheduledDropTask(result.DueTasks[0])
	if !ok {
		t.Fatalf("LootScheduledDropTask(%+v) ok = false, want true", result.DueTasks[0])
	}
	if lootTask.Kind != loot.ScheduledDropTaskDespawn || lootTask.DropID != "drop-1" {
		t.Fatalf("loot scheduled task = %+v, want despawn task for drop-1", lootTask)
	}
}

func TestTickDispatchesLootScheduledTasksThroughLootService(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	lootService := newWorkerLootService(t, clock)
	recorder := testutil.NewEventRecorder()
	lootService.SetEventEmitter(recorder)
	created, err := lootService.CreateDropsForNPCKill(workerNPCKilledEvent(), workerLootTable(t))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootCreated)
	recorder.Reset()

	handler, err := NewLootScheduledTaskHandler(lootService)
	if err != nil {
		t.Fatalf("NewLootScheduledTaskHandler() error = %v", err)
	}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 clock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if _, err := zoneWorker.ScheduleLootDropTasks(created.ScheduledTasks); err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}

	clock.Advance(loot.DefaultOwnerLockDuration)
	result := zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("owner-lock ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootOwnerLockExpired)
	recorder.Reset()

	clock.Advance(loot.DefaultTotalLifetime - loot.DefaultOwnerLockDuration)
	result = zoneWorker.Tick()
	if len(result.ScheduledTaskErrors) != 0 {
		t.Fatalf("despawn ScheduledTaskErrors = %+v, want none", result.ScheduledTaskErrors)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootExpired)
}

func TestTickRetriesEarlyLootScheduledTaskUntilLootClockIsDue(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	workerClock := testutil.NewFakeClock(start)
	lootClock := testutil.NewFakeClock(start)
	lootService := newWorkerLootService(t, lootClock)
	recorder := testutil.NewEventRecorder()
	lootService.SetEventEmitter(recorder)
	created, err := lootService.CreateDropsForNPCKill(workerNPCKilledEvent(), workerLootTable(t))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootCreated)
	recorder.Reset()

	handler, err := NewLootScheduledTaskHandler(lootService)
	if err != nil {
		t.Fatalf("NewLootScheduledTaskHandler() error = %v", err)
	}
	zoneWorker, err := NewWorker(Config{
		WorldID:               "world-1",
		ZoneID:                "zone-1",
		TickDelta:             time.Second,
		Clock:                 workerClock,
		ScheduledTaskHandlers: []ScheduledTaskHandler{handler},
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if _, err := zoneWorker.ScheduleLootDropTasks(created.ScheduledTasks); err != nil {
		t.Fatalf("ScheduleLootDropTasks() error = %v", err)
	}

	workerClock.Advance(loot.DefaultOwnerLockDuration)
	early := zoneWorker.Tick()
	if len(early.ScheduledTaskErrors) != 0 {
		t.Fatalf("early ScheduledTaskErrors = %+v, want none", early.ScheduledTaskErrors)
	}
	if got, want := taskIDs(early.DueTasks), []string{created.ScheduledTasks[0].ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("early due task ids = %v, want %v", got, want)
	}
	testutil.AssertRecordedEventTypes(t, recorder)

	lootClock.Advance(loot.DefaultOwnerLockDuration)
	retried := zoneWorker.Tick()
	if len(retried.ScheduledTaskErrors) != 0 {
		t.Fatalf("retry ScheduledTaskErrors = %+v, want none", retried.ScheduledTaskErrors)
	}
	if got, want := taskIDs(retried.DueTasks), []string{created.ScheduledTasks[0].ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retry due task ids = %v, want %v", got, want)
	}
	testutil.AssertRecordedEventTypes(t, recorder, loot.EventLootOwnerLockExpired)
}

func TestScheduleLootDropTasksRejectsInvalidLootTask(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)

	_, err := zoneWorker.ScheduleLootDropTasks([]loot.ScheduledDropTask{
		{
			ID:     "drop-1:unknown",
			Kind:   "loot.unknown",
			DropID: "drop-1",
			DueAt:  time.Date(2026, 6, 17, 12, 0, 1, 0, time.UTC),
		},
	})
	if !errors.Is(err, ErrInvalidWorkerConfig) {
		t.Fatalf("ScheduleLootDropTasks(unknown kind) error = %v, want ErrInvalidWorkerConfig", err)
	}

	_, err = zoneWorker.ScheduleLootDropTasks([]loot.ScheduledDropTask{
		{
			ID:    "missing-drop",
			Kind:  loot.ScheduledDropTaskDespawn,
			DueAt: time.Date(2026, 6, 17, 12, 0, 1, 0, time.UTC),
		},
	})
	if err == nil {
		t.Fatal("ScheduleLootDropTasks(empty drop) error = nil, want validation error")
	}
}

func newWorkerLootService(t *testing.T, clock *testutil.FakeClock) *loot.Service {
	t.Helper()
	inventory := economy.NewInventoryService(clock)
	service, err := loot.NewService(loot.Config{
		Clock:       clock,
		Cargo:       economy.NewCargoService(inventory),
		Progression: progression.NewProgressionService(clock, nil),
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func workerNPCKilledEvent() combat.NPCKilledEvent {
	return combat.NPCKilledEvent{
		SourceID:      "npc-1",
		NPCEntityID:   "npc-1",
		WorldID:       "world-1",
		ZoneID:        "zone-1",
		Position:      world.Vec2{X: 10, Y: 0},
		OwnerPlayerID: "player-1",
		KilledAt:      time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
}

func workerLootTable(t *testing.T) loot.LootTable {
	t.Helper()
	source, err := catalog.NewLootTableSource("worker_loot_table", "v1")
	if err != nil {
		t.Fatalf("NewLootTableSource() error = %v", err)
	}
	itemSource, err := catalog.NewVersionedDefinitionFromStrings("raw_ore", "v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings() error = %v", err)
	}
	maxStack, err := foundation.NewQuantity(999)
	if err != nil {
		t.Fatalf("NewQuantity(max) error = %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(weight) error = %v", err)
	}
	item, err := economy.NewItemDefinition(
		itemSource,
		"raw_ore",
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagDroppable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v", err)
	}
	return loot.LootTable{
		Source: source,
		Rows: []loot.LootRow{
			{ItemDefinition: item, MinQuantity: 1, MaxQuantity: 1, Chance: 1},
		},
	}
}
