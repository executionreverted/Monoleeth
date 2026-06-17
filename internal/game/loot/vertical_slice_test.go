package loot_test

import (
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
	gameruntime "gameproject/internal/game/runtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

func TestCombatKillLootPickupAndXPVerticalSlice(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	combatService := combat.NewService(clock, testutil.NewFakeRNG(nil, []float64{0, 0}))
	inventory := economy.NewInventoryService(clock)
	cargo := economy.NewCargoService(inventory)
	progressionService := progression.NewProgressionService(clock, nil)
	lootService, err := loot.NewService(loot.Config{
		Clock:       clock,
		RNG:         testutil.NewFakeRNG([]int{0}, []float64{0}),
		Cargo:       cargo,
		Progression: progressionService,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog() error = %v", err)
	}
	shipService, err := ships.NewHangarService(
		shipCatalog,
		nil,
		ships.StaticPlayerRankProvider{"player_1": 1},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		t.Fatalf("NewHangarService() error = %v", err)
	}
	starter, err := shipService.EnsureStarterShip("player_1")
	if err != nil {
		t.Fatalf("EnsureStarterShip() error = %v", err)
	}
	if !starter.HasActiveShip || starter.ActiveShip.ShipID != ships.ShipIDStarter {
		t.Fatalf("starter active ship = %+v, want active starter", starter)
	}
	moduleCatalog := modules.MustMVPCatalog()
	loadoutStore := modules.NewInMemoryLoadoutStore()
	putVerticalSliceModuleItem(t, loadoutStore, "laser-instance-1", "laser_alpha_t1", "player_1", 100)
	if err := loadoutStore.ReplaceEquippedModules("player_1", starter.ActiveShip.ShipID, []modules.EquippedModule{{
		PlayerID:       "player_1",
		ShipID:         starter.ActiveShip.ShipID,
		SlotID:         modules.ModuleSlotOffensive1,
		ItemInstanceID: "laser-instance-1",
		EquippedAt:     clock.Now(),
	}}); err != nil {
		t.Fatalf("ReplaceEquippedModules() error = %v, want nil", err)
	}
	statInput, err := gameruntime.NewStatInputProvider(shipCatalog, moduleCatalog, loadoutStore)
	if err != nil {
		t.Fatalf("NewStatInputProvider() error = %v, want nil", err)
	}
	statSubject := stats.NewStatSubject("player_1", starter.ActiveShip.ShipID)
	statService, err := stats.NewStatService(clock, nil, nil, statInput)
	if err != nil {
		t.Fatalf("NewStatService() error = %v", err)
	}
	playerStats, err := statService.GetEffectiveStats(statSubject)
	if err != nil {
		t.Fatalf("GetEffectiveStats() error = %v", err)
	}
	zoneWorker, err := worker.NewWorker(worker.Config{
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		TickDelta: time.Second,
		Clock:     clock,
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	if err := zoneWorker.Submit(worker.SpawnPlayerCommand{
		PlayerID: "player_1",
		EntityID: "player_entity_1",
		Position: world.Vec2{},
		Speed:    playerStats.Stats.Core.Speed,
	}); err != nil {
		t.Fatalf("Submit(spawn player) error = %v", err)
	}
	if result := zoneWorker.Tick(); len(result.CommandErrors) != 0 {
		t.Fatalf("spawn command errors = %+v", result.CommandErrors)
	}
	intent, err := world.NewMovementIntent(world.Vec2{X: 700, Y: 0})
	if err != nil {
		t.Fatalf("NewMovementIntent() error = %v", err)
	}
	if err := zoneWorker.Submit(worker.MoveToCommand{PlayerID: "player_1", Intent: intent}); err != nil {
		t.Fatalf("Submit(move) error = %v", err)
	}
	for tick := 0; tick < 7; tick++ {
		if result := zoneWorker.Tick(); len(result.CommandErrors) != 0 {
			t.Fatalf("move tick %d command errors = %+v", tick, result.CommandErrors)
		}
		clock.Advance(time.Second)
	}
	playerEntity, ok := zoneWorker.PlayerEntity("player_1")
	if !ok {
		t.Fatal("PlayerEntity(player_1) ok = false, want true")
	}

	player, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  playerEntity.ID,
		Type:      world.EntityTypePlayer,
		PlayerID:  "player_1",
		WorldID:   playerEntity.WorldID,
		ZoneID:    playerEntity.ZoneID,
		Position:  playerEntity.Position,
		Signature: visibility.EntitySignature(1),
		Snapshot:  playerStats,
	})
	if err != nil {
		t.Fatalf("NewActorFromSnapshot(player) error = %v", err)
	}
	npc, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "npc_1",
		Type:      world.EntityTypeNPCPlaceholder,
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{X: 800, Y: 0},
		Signature: visibility.EntitySignature(1),
		Snapshot:  npcStatSnapshot(),
	})
	if err != nil {
		t.Fatalf("NewActorFromSnapshot(npc) error = %v", err)
	}
	if err := combatService.UpsertActor(player); err != nil {
		t.Fatalf("UpsertActor(player) error = %v", err)
	}
	if err := combatService.UpsertActor(npc); err != nil {
		t.Fatalf("UpsertActor(npc) error = %v", err)
	}

	first, err := combatService.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("first ExecuteBasicAttack() error = %v", err)
	}
	if first.Killed {
		t.Fatal("first attack killed NPC, want second attack to finish vertical slice")
	}
	clock.Advance(2 * time.Second)
	second, err := combatService.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("second ExecuteBasicAttack() error = %v", err)
	}
	if !second.Killed || second.KillEvent == nil {
		t.Fatalf("second attack result = %+v, want NPC killed", second)
	}

	combatXP, err := progressionService.GrantXP(progression.GrantXPInput{
		PlayerID:       second.KillEvent.OwnerPlayerID,
		Amount:         20,
		SourceType:     progression.XPSourceTypeCombat,
		SourceID:       progression.XPSourceID(second.KillEvent.NPCEntityID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("combat_kill:" + second.KillEvent.NPCEntityID.String()),
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeCombat, Amount: 20},
		},
	})
	if err != nil {
		t.Fatalf("GrantXP(combat) error = %v", err)
	}
	duplicateCombatXP, err := progressionService.GrantXP(progression.GrantXPInput{
		PlayerID:       second.KillEvent.OwnerPlayerID,
		Amount:         999,
		SourceType:     progression.XPSourceTypeCombat,
		SourceID:       progression.XPSourceID(second.KillEvent.NPCEntityID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("combat_kill:" + second.KillEvent.NPCEntityID.String()),
	})
	if err != nil {
		t.Fatalf("duplicate GrantXP(combat) error = %v", err)
	}
	if !duplicateCombatXP.Duplicate || duplicateCombatXP.Snapshot.Player.MainXP != combatXP.Snapshot.Player.MainXP {
		t.Fatalf("duplicate combat XP = %+v, want duplicate unchanged from %+v", duplicateCombatXP, combatXP)
	}

	drops, err := lootService.CreateDropsForNPCKill(*second.KillEvent, lootTable(t, 3, 3, 1))
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v", err)
	}
	if len(drops.Drops) != 1 {
		t.Fatalf("drops len = %d, want 1", len(drops.Drops))
	}

	cargoLocation := mustCargoLocation(t, starter.ActiveShip.ShipID.String())
	pickup, err := lootService.PickupDrop(loot.PickupInput{
		PlayerID:           second.KillEvent.OwnerPlayerID,
		DropID:             drops.Drops[0].ID,
		Viewer:             viewerAt(drops.Drops[0].Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: int64(playerStats.Stats.Core.CargoCapacity),
	})
	if err != nil {
		t.Fatalf("PickupDrop() error = %v", err)
	}
	if pickup.XPResult == nil {
		t.Fatal("loot pickup XPResult = nil, want loot XP grant")
	}
	if inventory.TotalItemQuantity("player_1", rawOreDefinition(t).ItemID, cargoLocation) != 3 {
		t.Fatalf("cargo raw ore quantity mismatch")
	}
	if pickup.XPResult.Snapshot.Player.MainXP != combatXP.Snapshot.Player.MainXP+5 {
		t.Fatalf("final main XP = %d, want combat XP + loot XP", pickup.XPResult.Snapshot.Player.MainXP)
	}
	playerSnapshot, err := progressionService.GetProgressionSnapshot("player_1")
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v", err)
	}
	if playerSnapshot.Player.MainXP != pickup.XPResult.Snapshot.Player.MainXP {
		t.Fatalf("player snapshot main XP = %d, want final XP %d", playerSnapshot.Player.MainXP, pickup.XPResult.Snapshot.Player.MainXP)
	}
}

func putVerticalSliceModuleItem(
	t *testing.T,
	store *modules.InMemoryLoadoutStore,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	owner foundation.PlayerID,
	durability int64,
) {
	t.Helper()
	quantity, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	item := economy.InstanceItem{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(itemID),
			Version:      modules.ModuleCatalogVersion,
		},
		ItemInstanceID:    itemInstanceID,
		ItemID:            itemID,
		OwnerPlayerID:     owner,
		Location:          economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID(owner.String())},
		Quantity:          quantity,
		DurabilityCurrent: durability,
		BoundState:        economy.BoundStateUnbound,
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("vertical slice module item Validate() error = %v, want nil", err)
	}
	if err := store.PutModuleItem(item); err != nil {
		t.Fatalf("PutModuleItem() error = %v, want nil", err)
	}
}

func npcStatSnapshot() stats.StatSnapshot {
	return stats.NewStatSnapshot(
		"",
		"npc_placeholder",
		1,
		stats.EffectiveStats{
			Core: stats.CoreStats{
				HPMax:     24,
				EnergyMax: 1,
			},
			Exploration: stats.ExplorationStats{
				RadarRange: 1,
			},
		},
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	)
}
