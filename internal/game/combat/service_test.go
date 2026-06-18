package combat_test

import (
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
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

func TestExecuteBasicAttackRecordsCombatActionMetric(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	recorder := observability.NewMetricRecorder()
	service.SetMetricRecorder(recorder)

	result, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if err != nil {
		t.Fatalf("ExecuteBasicAttack() error = %v", err)
	}
	if !result.Hit || result.Killed {
		t.Fatalf("result = %+v, want non-lethal hit", result)
	}

	counter := requireMetricCounter(t, recorder.Snapshot(), observability.MetricCombatActionsPerSecond)
	if counter.Value != 1 {
		t.Fatalf("combat action metric value = %d, want 1", counter.Value)
	}
	assertMetricLabels(t, counter.Labels, []observability.Label{
		{Name: "action", Value: "basic_attack"},
		{Name: "result", Value: "hit"},
	})
}

func TestCombatMetricFailureDoesNotBlockAttack(t *testing.T) {
	service := newCombatService(t, []float64{0})
	addDefaultActors(t, service)
	service.SetMetricRecorder(failingCombatMetrics{})

	if _, err := service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"}); err != nil {
		t.Fatalf("ExecuteBasicAttack() error = %v, want success despite metric failure", err)
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
	if result.KillEvent.NPCType != "pirate" {
		t.Fatalf("KillEvent.NPCType = %q, want pirate", result.KillEvent.NPCType)
	}
	if result.ShieldDamage != 10 || result.HPDamage != 20 {
		t.Fatalf("damage split = shield %v hp %v, want 10/20", result.ShieldDamage, result.HPDamage)
	}

	_, err = service.ExecuteBasicAttack(combat.BasicAttackInput{AttackerID: "player_entity_1", TargetID: "npc_1"})
	if !errors.Is(err, combat.ErrTargetDead) {
		t.Fatalf("second ExecuteBasicAttack() error = %v, want ErrTargetDead", err)
	}
}

func TestSimultaneousLethalDamageProcessesNPCDeathOnce(t *testing.T) {
	service := newCombatService(t, []float64{0, 0})
	recorder := testutil.NewEventRecorder()
	service.SetEventEmitter(recorder)
	first := playerActor("player_entity_1", "player_1", world.Vec2{})
	first.Stats.Stats.Combat.WeaponDamage = 100
	second := playerActor("player_entity_2", "player_2", world.Vec2{X: 0, Y: 5})
	second.Stats.Stats.Combat.WeaponDamage = 100
	target := npcActor("npc_1", world.Vec2{X: 10, Y: 0})
	target.HP = 50
	target.Shield = 0
	for _, actor := range []combat.ActorState{first, second, target} {
		if err := service.UpsertActor(actor); err != nil {
			t.Fatalf("UpsertActor(%s) error = %v", actor.EntityID, err)
		}
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]combat.BasicAttackResult, 2)
	errs := make([]error, 2)
	for index, attackerID := range []world.EntityID{"player_entity_1", "player_entity_2"} {
		wg.Add(1)
		go func(index int, attackerID world.EntityID) {
			defer wg.Done()
			<-start
			results[index], errs[index] = service.ExecuteBasicAttack(combat.BasicAttackInput{
				AttackerID: attackerID,
				TargetID:   "npc_1",
			})
		}(index, attackerID)
	}
	close(start)
	wg.Wait()

	kills := 0
	deadTargetErrors := 0
	for index, err := range errs {
		if err == nil && results[index].Killed && results[index].KillEvent != nil {
			kills++
			continue
		}
		if errors.Is(err, combat.ErrTargetDead) {
			deadTargetErrors++
			continue
		}
		t.Fatalf("attack %d result = %+v, error = %v; want one kill and one ErrTargetDead", index, results[index], err)
	}
	if kills != 1 || deadTargetErrors != 1 {
		t.Fatalf("kills = %d, dead target errors = %d; want 1/1", kills, deadTargetErrors)
	}
	testutil.AssertRecordedEventTypes(t, recorder, combat.EventBasicAttack, combat.EventNPCKilled)
	events := recorder.Events()
	var killPayload combat.NPCKilledEvent
	if err := json.Unmarshal(events[1].Payload, &killPayload); err != nil {
		t.Fatalf("unmarshal kill payload: %v", err)
	}
	if killPayload.NPCType != "pirate" {
		t.Fatalf("kill payload NPCType = %q, want pirate", killPayload.NPCType)
	}

	after, ok := service.Actor("npc_1")
	if !ok {
		t.Fatal("Actor(npc_1) ok = false, want true")
	}
	if !after.Dead || after.DiedAt == nil || after.HP != 0 {
		t.Fatalf("target after concurrent lethal damage = %+v, want dead once with zero HP", after)
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

func TestUpsertActorRejectsInvalidEffectiveCombatStats(t *testing.T) {
	service := newCombatService(t, nil)
	actor := playerActor("player_entity_1", "player_1", world.Vec2{})
	actor.Stats.Stats.Combat.WeaponCooldown = math.Inf(1)

	err := service.UpsertActor(actor)

	if !errors.Is(err, combat.ErrInvalidActorState) {
		t.Fatalf("UpsertActor() error = %v, want ErrInvalidActorState", err)
	}
}

func TestNewActorFromSnapshotUsesAuthoritativeStatsAndRejectsMismatch(t *testing.T) {
	snapshot := statSnapshot("player_1", 100, 8, 12)
	actor, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "player_entity_1",
		Type:      world.EntityTypePlayer,
		PlayerID:  "player_1",
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{X: 1, Y: 2},
		Signature: visibility.EntitySignature(10),
		Snapshot:  snapshot,
	})
	if err != nil {
		t.Fatalf("NewActorFromSnapshot() error = %v", err)
	}
	if actor.HP != snapshot.Stats.Core.HPMax ||
		actor.Shield != snapshot.Stats.Core.ShieldMax ||
		actor.Energy != snapshot.Stats.Core.EnergyMax {
		t.Fatalf("actor live resources = hp %v shield %v energy %v, want snapshot maxes", actor.HP, actor.Shield, actor.Energy)
	}

	npcSnapshot := statSnapshot("", 100, 0, 12)
	npc, err := combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "npc_1",
		Type:      world.EntityTypeNPCPlaceholder,
		NPCType:   "pirate",
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{X: 2, Y: 3},
		Signature: visibility.EntitySignature(10),
		Snapshot:  npcSnapshot,
	})
	if err != nil {
		t.Fatalf("NewActorFromSnapshot(npc) error = %v", err)
	}
	if npc.NPCType != "pirate" {
		t.Fatalf("npc NPCType = %q, want pirate", npc.NPCType)
	}

	_, err = combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "npc_missing_type",
		Type:      world.EntityTypeNPCPlaceholder,
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{},
		Signature: visibility.EntitySignature(10),
		Snapshot:  npcSnapshot,
	})
	if !errors.Is(err, combat.ErrInvalidActorState) {
		t.Fatalf("missing NPCType NewActorFromSnapshot() error = %v, want ErrInvalidActorState", err)
	}

	_, err = combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "player_entity_2",
		Type:      world.EntityTypePlayer,
		PlayerID:  "player_2",
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{},
		Signature: visibility.EntitySignature(10),
		Snapshot:  snapshot,
	})
	if !errors.Is(err, combat.ErrInvalidActorState) {
		t.Fatalf("mismatched NewActorFromSnapshot() error = %v, want ErrInvalidActorState", err)
	}

	invalidSnapshot := snapshot
	invalidSnapshot.Version = 0
	_, err = combat.NewActorFromSnapshot(combat.ActorFromSnapshotInput{
		EntityID:  "player_entity_1",
		Type:      world.EntityTypePlayer,
		PlayerID:  "player_1",
		WorldID:   "world_1",
		ZoneID:    "zone_1",
		Position:  world.Vec2{},
		Signature: visibility.EntitySignature(10),
		Snapshot:  invalidSnapshot,
	})
	if !errors.Is(err, combat.ErrInvalidActorState) {
		t.Fatalf("zero-version NewActorFromSnapshot() error = %v, want ErrInvalidActorState", err)
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
		NPCType:   "pirate",
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

type failingCombatMetrics struct{}

func (failingCombatMetrics) RecordCombatAction(string, string) error {
	return errors.New("metric sink unavailable")
}

func requireMetricCounter(t *testing.T, snapshot observability.MetricSnapshot, name string) observability.CounterSnapshot {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name == name {
			return counter
		}
	}
	t.Fatalf("missing counter %q in %#v", name, snapshot.Counters)
	return observability.CounterSnapshot{}
}

func assertMetricLabels(t *testing.T, got, want []observability.Label) {
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
