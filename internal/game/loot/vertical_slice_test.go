package loot_test

import (
	"testing"
	"time"

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

	player := verticalCombatActor("player_entity_1", "player_1", world.EntityTypePlayer, world.Vec2{}, 50, 100)
	npc := verticalCombatActor("npc_1", "", world.EntityTypeNPCPlaceholder, world.Vec2{X: 40, Y: 0}, 0, 0)
	npc.HP = 100
	npc.Shield = 0
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
	clock.Advance(5 * time.Second)
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

	cargoLocation := mustCargoLocation(t, "ship_1")
	pickup, err := lootService.PickupDrop(loot.PickupInput{
		PlayerID:           second.KillEvent.OwnerPlayerID,
		DropID:             drops.Drops[0].ID,
		Viewer:             viewerAt(drops.Drops[0].Position),
		ActiveCargo:        cargoLocation,
		CargoCapacityUnits: 100,
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
}

func verticalCombatActor(entityID world.EntityID, playerID foundation.PlayerID, entityType world.EntityType, position world.Vec2, damage float64, energy float64) combat.ActorState {
	return combat.ActorState{
		EntityID:  entityID,
		Type:      entityType,
		PlayerID:  playerID,
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
		Stats: stats.NewStatSnapshot(
			playerID,
			"ship_1",
			1,
			stats.EffectiveStats{
				Core: stats.CoreStats{
					HPMax:       100,
					ShieldMax:   50,
					EnergyMax:   100,
					EnergyRegen: 10,
				},
				Combat: stats.CombatStats{
					WeaponDamage:     damage,
					WeaponRange:      100,
					WeaponCooldown:   5,
					WeaponEnergyCost: 10,
					Accuracy:         1,
				},
				Exploration: stats.ExplorationStats{
					RadarRange: 200,
				},
			},
			time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
		),
		HP:     100,
		Shield: 50,
		Energy: energy,
	}
}
