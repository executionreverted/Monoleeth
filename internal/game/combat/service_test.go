package combat_test

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestExecuteBasicAttackRejectsHiddenTarget(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	hidden, _ := service.Actor("npc_1")
	hidden.Hidden = true
	if err := service.UpsertActor(hidden); err != nil {
		t.Fatalf("UpsertActor(hidden) error = %v", err)
	}

	_, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})

	if !errors.Is(err, combat.ErrTargetNotVisible) {
		t.Fatalf("ExecuteBasicAttack() error = %v, want ErrTargetNotVisible", err)
	}
}

func TestExecuteBasicAttackRejectsOutOfRangeTarget(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	target, _ := service.Actor("npc_1")
	target.Position = world.Vec2{X: 150, Y: 0}
	if err := service.UpsertActor(target); err != nil {
		t.Fatalf("UpsertActor(target) error = %v", err)
	}

	_, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})

	if !errors.Is(err, combat.ErrOutOfRange) {
		t.Fatalf("ExecuteBasicAttack() error = %v, want ErrOutOfRange", err)
	}
}

func TestExecuteBasicAttackSpendsExactEnergyAndStartsCooldown(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	attacker, _ := service.Actor("player_entity_1")
	attacker.Energy = attacker.Stats.Stats.Combat.WeaponEnergyCost
	if err := service.UpsertActor(attacker); err != nil {
		t.Fatalf("UpsertActor(attacker) error = %v", err)
	}

	result, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("ExecuteBasicAttack() error = %v", err)
	}

	if result.Attacker.Energy != 0 {
		t.Fatalf("attacker energy = %v, want 0", result.Attacker.Energy)
	}
	if result.CooldownReadyAt.IsZero() {
		t.Fatal("CooldownReadyAt is zero, want cooldown started")
	}

	_, err = service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if !errors.Is(err, combat.ErrCooldownNotReady) {
		t.Fatalf("second ExecuteBasicAttack() error = %v, want ErrCooldownNotReady", err)
	}
}

func TestExecuteBasicAttackRejectsEnergyShortageBeforeMutation(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	attacker, _ := service.Actor("player_entity_1")
	attacker.Energy = attacker.Stats.Stats.Combat.WeaponEnergyCost - 1
	if err := service.UpsertActor(attacker); err != nil {
		t.Fatalf("UpsertActor(attacker) error = %v", err)
	}

	_, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if !errors.Is(err, combat.ErrNotEnoughEnergy) {
		t.Fatalf("ExecuteBasicAttack() error = %v, want ErrNotEnoughEnergy", err)
	}
	after, _ := service.Actor("player_entity_1")
	if after.Energy != attacker.Energy {
		t.Fatalf("energy mutated on failure: got %v, want %v", after.Energy, attacker.Energy)
	}
}

func TestExecuteBasicAttackAppliesShieldOverflowAndKillsNPCOnce(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	target, _ := service.Actor("npc_1")
	target.Shield = 10
	target.HP = 20
	if err := service.UpsertActor(target); err != nil {
		t.Fatalf("UpsertActor(target) error = %v", err)
	}

	result, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("ExecuteBasicAttack() error = %v", err)
	}
	if !result.Killed {
		t.Fatal("Killed = false, want true")
	}
	if result.KillEvent == nil || result.KillEvent.OwnerPlayerID != foundation.PlayerID("player_1") {
		t.Fatalf("KillEvent = %+v, want owner player_1", result.KillEvent)
	}
	if result.ShieldDamage != 10 || result.HPDamage != 20 {
		t.Fatalf("damage split = shield %v hp %v, want 10/20", result.ShieldDamage, result.HPDamage)
	}

	_, err = service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if !errors.Is(err, combat.ErrTargetDead) {
		t.Fatalf("second ExecuteBasicAttack() error = %v, want ErrTargetDead", err)
	}
}

func TestHighestDamageContributorReceivesKill(t *testing.T) {
	service := newCombatService(t, []float64{0, 0})
	addDefaultActors(t, service)
	second := playerActor("player_entity_2", "player_2", world.Vec2{X: 0, Y: 5})
	second.Stats.Stats.Combat.WeaponDamage = 50
	if err := service.UpsertActor(second); err != nil {
		t.Fatalf("UpsertActor(second) error = %v", err)
	}
	target, _ := service.Actor("npc_1")
	target.HP = 90
	target.Shield = 0
	if err := service.UpsertActor(target); err != nil {
		t.Fatalf("UpsertActor(target) error = %v", err)
	}

	if _, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_2", TargetID: "npc_1"}); err != nil {
		t.Fatalf("first ExecuteBasicAttack() error = %v", err)
	}
	first, _ := service.Actor("player_entity_1")
	first.Cooldowns = combat.CooldownState{}
	first.Stats.Stats.Combat.WeaponDamage = 60
	if err := service.UpsertActor(first); err != nil {
		t.Fatalf("UpsertActor(first) error = %v", err)
	}
	result, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("second ExecuteBasicAttack() error = %v", err)
	}

	if result.KillEvent == nil || result.KillEvent.OwnerPlayerID != foundation.PlayerID("player_1") {
		t.Fatalf("kill owner = %+v, want player_1", result.KillEvent)
	}
}

func TestRegenerateEnergyCapsAtStatMaximum(t *testing.T) {
	service := newCombatService(t, nil)
	addDefaultActors(t, service)
	attacker, _ := service.Actor("player_entity_1")
	attacker.Energy = 10
	if err := service.UpsertActor(attacker); err != nil {
		t.Fatalf("UpsertActor(attacker) error = %v", err)
	}

	got, err := service.RegenerateEnergy("player_entity_1", 20*time.Second)
	if err != nil {
		t.Fatalf("RegenerateEnergy() error = %v", err)
	}

	if got.Energy != got.Stats.Stats.Core.EnergyMax {
		t.Fatalf("energy = %v, want capped at %v", got.Energy, got.Stats.Stats.Core.EnergyMax)
	}
}

func newCombatService(t *testing.T, floats []float64) *combat.Service {
	t.Helper()
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	return combat.NewService(testutil.NewFakeClock(start), testutil.NewFakeRNG(nil, floats))
}

func addDefaultActors(t *testing.T, service *combat.Service) {
	t.Helper()
	if err := service.UpsertActor(playerActor("player_entity_1", "player_1", world.Vec2{})); err != nil {
		t.Fatalf("UpsertActor(player) error = %v", err)
	}
	if err := service.UpsertActor(npcActor("npc_1", world.Vec2{X: 50, Y: 0})); err != nil {
		t.Fatalf("UpsertActor(npc) error = %v", err)
	}
}

func playerActor(entityID world.EntityID, playerID foundation.PlayerID, position world.Vec2) combat.ActorState {
	return combat.ActorState{
		EntityID:  entityID,
		Type:      world.EntityTypePlayer,
		PlayerID:  playerID,
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
		Stats:     statSnapshot(playerID, 100, 10, 100),
		HP:        100,
		Shield:    50,
		Energy:    100,
	}
}

func npcActor(entityID world.EntityID, position world.Vec2) combat.ActorState {
	return combat.ActorState{
		EntityID:  entityID,
		Type:      world.EntityTypeNPCPlaceholder,
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
		Stats:     statSnapshot("", 100, 0, 0),
		HP:        100,
		Shield:    20,
		Energy:    0,
	}
}

func statSnapshot(playerID foundation.PlayerID, rangeUnits float64, energyCost float64, weaponDamage float64) stats.StatSnapshot {
	return stats.NewStatSnapshot(
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
				WeaponDamage:     weaponDamage,
				WeaponRange:      rangeUnits,
				WeaponCooldown:   5,
				WeaponEnergyCost: energyCost,
				Accuracy:         1,
			},
			Exploration: stats.ExplorationStats{
				RadarRange: 200,
			},
		},
		time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	)
}
