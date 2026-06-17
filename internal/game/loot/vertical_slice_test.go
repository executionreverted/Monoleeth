package loot_test

import (
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/progression"
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
	statSubject := stats.NewStatSubject("player_1", starter.ActiveShip.ShipID)
	statService, err := stats.NewStatService(clock, nil, nil, stats.StaticStatInputProvider{
		statSubject: starterCombatStatInput(t, shipCatalog),
	})
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

func starterCombatStatInput(t *testing.T, shipCatalog ships.Catalog) stats.StatBuildInput {
	t.Helper()
	starter, err := shipCatalog.MustGet(ships.ShipIDStarter)
	if err != nil {
		t.Fatalf("MustGet(starter) error = %v", err)
	}
	laser, ok := modules.MustMVPCatalog().Lookup("laser_alpha_t1")
	if !ok {
		t.Fatal("module catalog missing laser_alpha_t1")
	}
	return stats.StatBuildInput{
		BaseShip: shipBaseEffectiveStats(starter.BaseStats),
		Modules: []stats.ModuleModifier{
			moduleDefinitionModifier(laser),
		},
	}
}

func shipBaseEffectiveStats(base ships.ShipBaseStats) stats.EffectiveStats {
	return stats.EffectiveStats{
		Core: stats.CoreStats{
			HPMax:         float64(base.HP),
			ShieldMax:     float64(base.Shield),
			EnergyMax:     float64(base.Energy),
			EnergyRegen:   float64(base.EnergyRegen),
			Speed:         float64(base.Speed),
			CargoCapacity: float64(base.CargoCapacity),
		},
		Exploration: stats.ExplorationStats{
			RadarRange:      float64(base.Radar),
			SignatureRadius: float64(base.Signature),
		},
	}
}

func moduleDefinitionModifier(definition modules.ModuleDefinition) stats.ModuleModifier {
	modifier := stats.ModuleModifier{
		SourceID: definition.ItemID.String(),
	}
	for _, statModifier := range definition.StatModifiers {
		value := moduleStatValue(statModifier)
		switch statModifier.Kind {
		case modules.StatModifierFlat:
			applyFlatModuleStat(&modifier.Flat, statModifier.Stat, value)
		case modules.StatModifierPercent:
			applyPercentModuleStat(&modifier.Percent, statModifier.Stat, value)
		}
	}
	if definition.Energy.ActivationCost > 0 {
		modifier.Flat.Combat.WeaponEnergyCost = float64(definition.Energy.ActivationCost)
	}
	for _, cooldown := range definition.Cooldowns {
		if cooldown.Key == modules.CooldownBasicAttack {
			modifier.Flat.Combat.WeaponCooldown = float64(cooldown.DurationMS) / 1000
		}
	}
	return modifier
}

func moduleStatValue(modifier modules.StatModifier) float64 {
	value := float64(modifier.Value)
	if modifier.Kind == modules.StatModifierPercent || modifier.Stat == modules.StatAccuracy {
		return value / 10_000
	}
	return value
}

func applyFlatModuleStat(target *stats.FlatStats, stat modules.StatKey, value float64) {
	switch stat {
	case modules.StatWeaponDamage:
		target.Combat.WeaponDamage = value
	case modules.StatWeaponRange:
		target.Combat.WeaponRange = value
	case modules.StatAccuracy:
		target.Combat.Accuracy = value
	case modules.StatShieldMax:
		target.Core.ShieldMax = value
	case modules.StatShieldRegen:
		target.Core.ShieldRegen = value
	case modules.StatRadarRange:
		target.Exploration.RadarRange = value
	case modules.StatCargoCapacity:
		target.Core.CargoCapacity = value
	}
}

func applyPercentModuleStat(target *stats.PercentStats, stat modules.StatKey, value float64) {
	switch stat {
	case modules.StatWeaponDamage:
		target.Combat.WeaponDamage = value
	case modules.StatWeaponRange:
		target.Combat.WeaponRange = value
	case modules.StatAccuracy:
		target.Combat.Accuracy = value
	case modules.StatShieldMax:
		target.Core.ShieldMax = value
	case modules.StatShieldRegen:
		target.Core.ShieldRegen = value
	case modules.StatRadarRange:
		target.Exploration.RadarRange = value
	case modules.StatCargoCapacity:
		target.Core.CargoCapacity = value
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
