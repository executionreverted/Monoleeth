package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

func TestCombatUseSkillIgnoresClientTimestampForCooldown(t *testing.T) {
	start := time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC)
	clock := testutil.NewFakeClock(start)
	combatService := combat.NewService(clock, nil)
	upsertRuntimeCombatActor(t, combatService, runtimePlayerCombatActor("player-entity-1", "player-1", world.Vec2{}))
	upsertRuntimeCombatActor(t, combatService, runtimeNPCCombatActor("npc-1", world.Vec2{X: 20, Y: 0}))

	combatHandler, err := NewCombatCommandHandler(
		combatService,
		staticCombatActorResolver{"player-1": "player-entity-1"},
		staticActiveShipGuard{"player-1": runtimeActiveShipSnapshot("player-1", ships.ShipStateActive)},
	)
	if err != nil {
		t.Fatalf("NewCombatCommandHandler() error = %v, want nil", err)
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock: clock,
		Sessions: staticRuntimeSessionResolver{
			"session-1": {
				SessionID: "session-1",
				PlayerID:  "player-1",
				WorldID:   "world-1",
				ZoneID:    "zone-1",
			},
		},
		Handlers: combatHandler.Handlers(),
	})
	if err != nil {
		t.Fatalf("NewGateway() error = %v, want nil", err)
	}

	first := gateway.HandleRequest("session-1", []byte(fmt.Sprintf(
		`{"request_id":"request-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1","client_timestamp":%d},"client_seq":1,"v":1}`,
		start.Add(time.Hour).UnixMilli(),
	)))
	if first.HasError {
		t.Fatalf("first HandleRequest() error = %+v, want success", first.Error)
	}
	var firstPayload struct {
		CooldownReadyAtMS int64 `json:"cooldown_ready_at_ms"`
	}
	if err := json.Unmarshal(first.Response.Payload, &firstPayload); err != nil {
		t.Fatalf("unmarshal first response payload: %v", err)
	}
	if got, want := firstPayload.CooldownReadyAtMS, start.Add(5*time.Second).UnixMilli(); got != want {
		t.Fatalf("cooldown ready at = %d, want server time %d", got, want)
	}

	second := gateway.HandleRequest("session-1", []byte(fmt.Sprintf(
		`{"request_id":"request-2","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1","client_timestamp":%d},"client_seq":2,"v":1}`,
		start.Add(2*time.Hour).UnixMilli(),
	)))
	if !second.HasError {
		t.Fatalf("second HandleRequest() HasError = false, want cooldown error")
	}
	if second.Error.Error.Code != foundation.CodeCooldown {
		t.Fatalf("second error code = %s, want %s", second.Error.Error.Code, foundation.CodeCooldown)
	}
	attacker, ok := combatService.Actor("player-entity-1")
	if !ok {
		t.Fatal("Actor(player-entity-1) ok = false, want true")
	}
	if got, want := attacker.Energy, 90.0; got != want {
		t.Fatalf("attacker energy after rejected timestamp spoof = %v, want %v", got, want)
	}
}

func TestCombatUseSkillHidesDifferentZoneTargets(t *testing.T) {
	start := time.Date(2026, 6, 18, 20, 5, 0, 0, time.UTC)
	clock := testutil.NewFakeClock(start)
	combatService := combat.NewService(clock, nil)
	upsertRuntimeCombatActor(t, combatService, runtimePlayerCombatActor("player-entity-1", "player-1", world.Vec2{}))
	target := runtimeNPCCombatActor("npc-1", world.Vec2{X: 20, Y: 0})
	target.ZoneID = "zone-2"
	upsertRuntimeCombatActor(t, combatService, target)

	combatHandler, err := NewCombatCommandHandler(
		combatService,
		staticCombatActorResolver{"player-1": "player-entity-1"},
		staticActiveShipGuard{"player-1": runtimeActiveShipSnapshot("player-1", ships.ShipStateActive)},
	)
	if err != nil {
		t.Fatalf("NewCombatCommandHandler() error = %v, want nil", err)
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock: clock,
		Sessions: staticRuntimeSessionResolver{
			"session-1": {
				SessionID: "session-1",
				PlayerID:  "player-1",
				WorldID:   "world-1",
				ZoneID:    "zone-1",
			},
		},
		Handlers: combatHandler.Handlers(),
	})
	if err != nil {
		t.Fatalf("NewGateway() error = %v, want nil", err)
	}

	result := gateway.HandleRequest("session-1", []byte(
		`{"request_id":"request-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1","client_timestamp":1},"client_seq":1,"v":1}`,
	))

	if !result.HasError {
		t.Fatalf("HandleRequest() HasError = false, want safe visibility error")
	}
	if result.Error.Error.Code != foundation.CodeNotVisible {
		t.Fatalf("error code = %s, want %s", result.Error.Error.Code, foundation.CodeNotVisible)
	}
	attacker, ok := combatService.Actor("player-entity-1")
	if !ok {
		t.Fatal("Actor(player-entity-1) ok = false, want true")
	}
	if got, want := attacker.Energy, 100.0; got != want {
		t.Fatalf("attacker energy after rejected cross-zone target = %v, want %v", got, want)
	}
}

func TestCombatCommandHandlerConstructorsRejectNilDependencies(t *testing.T) {
	if _, err := NewCombatCommandHandler(nil, staticCombatActorResolver{}, staticActiveShipGuard{}); !errors.Is(err, ErrNilCombatService) {
		t.Fatalf("NewCombatCommandHandler(nil service) error = %v, want ErrNilCombatService", err)
	}
	if _, err := NewCombatCommandHandler(combat.NewService(nil, nil), nil, staticActiveShipGuard{}); !errors.Is(err, ErrNilCombatActorResolver) {
		t.Fatalf("NewCombatCommandHandler(nil resolver) error = %v, want ErrNilCombatActorResolver", err)
	}
	if _, err := NewCombatCommandHandler(combat.NewService(nil, nil), staticCombatActorResolver{}, nil); !errors.Is(err, ErrNilActiveShipGuard) {
		t.Fatalf("NewCombatCommandHandler(nil ships) error = %v, want ErrNilActiveShipGuard", err)
	}
}

func TestCombatUseSkillRejectsClientAuthoredAttackerID(t *testing.T) {
	handler, err := NewCombatCommandHandler(
		combat.NewService(nil, nil),
		staticCombatActorResolver{"player-1": "player-entity-1"},
		staticActiveShipGuard{"player-1": runtimeActiveShipSnapshot("player-1", ships.ShipStateActive)},
	)
	if err != nil {
		t.Fatalf("NewCombatCommandHandler() error = %v, want nil", err)
	}
	request := realtime.NewRequestEnvelope(
		"request-1",
		realtime.OperationCombatUseSkill,
		json.RawMessage(`{"skill_id":"basic_laser","target_id":"npc-1","attacker_id":"spoofed-entity","client_timestamp":1}`),
		1,
	)

	_, err = handler.HandleUseSkill(validRuntimeCommandContext(), request)

	if !foundation.IsCode(err, foundation.CodeInvalidPayload) {
		t.Fatalf("HandleUseSkill() error = %v, want %s", err, foundation.CodeInvalidPayload)
	}
}

func TestCombatUseSkillRejectsDisabledActiveShipBeforeMutation(t *testing.T) {
	start := time.Date(2026, 6, 18, 20, 10, 0, 0, time.UTC)
	clock := testutil.NewFakeClock(start)
	combatService := combat.NewService(clock, nil)
	upsertRuntimeCombatActor(t, combatService, runtimePlayerCombatActor("player-entity-1", "player-1", world.Vec2{}))
	upsertRuntimeCombatActor(t, combatService, runtimeNPCCombatActor("npc-1", world.Vec2{X: 20, Y: 0}))

	combatHandler, err := NewCombatCommandHandler(
		combatService,
		staticCombatActorResolver{"player-1": "player-entity-1"},
		staticActiveShipGuard{"player-1": runtimeActiveShipSnapshot("player-1", ships.ShipStateDisabled)},
	)
	if err != nil {
		t.Fatalf("NewCombatCommandHandler() error = %v, want nil", err)
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock: clock,
		Sessions: staticRuntimeSessionResolver{
			"session-1": {
				SessionID: "session-1",
				PlayerID:  "player-1",
				WorldID:   "world-1",
				ZoneID:    "zone-1",
			},
		},
		Handlers: combatHandler.Handlers(),
	})
	if err != nil {
		t.Fatalf("NewGateway() error = %v, want nil", err)
	}

	result := gateway.HandleRequest("session-1", []byte(
		`{"request_id":"request-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"npc-1"},"client_seq":1,"v":1}`,
	))

	if !result.HasError {
		t.Fatalf("HandleRequest() HasError = false, want disabled ship error")
	}
	if result.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("error code = %s, want %s", result.Error.Error.Code, foundation.CodeNotFound)
	}
	attacker, ok := combatService.Actor("player-entity-1")
	if !ok {
		t.Fatal("Actor(player-entity-1) ok = false, want true")
	}
	if got, want := attacker.Energy, 100.0; got != want {
		t.Fatalf("attacker energy after disabled ship rejection = %v, want %v", got, want)
	}
	if !attacker.Cooldowns[combat.BasicLaserCooldownKey].IsZero() {
		t.Fatalf("cooldown after disabled ship rejection = %s, want zero", attacker.Cooldowns[combat.BasicLaserCooldownKey])
	}
}

type staticRuntimeSessionResolver map[realtime.SessionID]realtime.CommandContext

func (resolver staticRuntimeSessionResolver) ResolveSession(sessionID realtime.SessionID) (realtime.CommandContext, error) {
	ctx, ok := resolver[sessionID]
	if !ok {
		return realtime.CommandContext{}, foundation.NewDomainError(foundation.CodeUnauthenticated, "Authenticated session is required.")
	}
	return ctx, nil
}

type staticCombatActorResolver map[foundation.PlayerID]world.EntityID

func (resolver staticCombatActorResolver) ActiveCombatActor(ctx realtime.CommandContext) (world.EntityID, error) {
	entityID, ok := resolver[ctx.PlayerID]
	if !ok {
		return "", combat.ErrUnknownActor
	}
	return entityID, nil
}

type staticActiveShipGuard map[foundation.PlayerID]ships.HangarSnapshot

func (guard staticActiveShipGuard) WithActiveShipCombatLease(playerID foundation.PlayerID, action func() error) error {
	snapshot, ok := guard[playerID]
	if !ok {
		return ships.ErrNoActiveShip
	}
	if action == nil {
		return ships.ErrNilActiveShipCombatAction
	}
	if !snapshot.HasActiveShip {
		return ships.ErrNoActiveShip
	}
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID != snapshot.ActiveShip.ShipID {
			continue
		}
		switch playerShip.State {
		case ships.ShipStateActive:
			return action()
		case ships.ShipStateDisabled:
			return ships.ErrShipDisabled
		default:
			return ships.ErrShipUnavailable
		}
	}
	return ships.ErrShipNotUnlocked
}

func runtimeActiveShipSnapshot(playerID foundation.PlayerID, state ships.ShipState) ships.HangarSnapshot {
	return ships.HangarSnapshot{
		PlayerID: playerID,
		Ships: []ships.PlayerShipState{
			{
				PlayerID:       playerID,
				ShipID:         "ship-1",
				UnlockedAt:     time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC),
				State:          state,
				DisabledReason: disabledReasonForState(state),
			},
		},
		ActiveShip: ships.ActiveShipState{
			PlayerID:    playerID,
			ShipID:      "ship-1",
			ActivatedAt: time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC),
		},
		HasActiveShip: true,
	}
}

func disabledReasonForState(state ships.ShipState) string {
	if state == ships.ShipStateDisabled {
		return ships.DisabledReasonDeath
	}
	return ""
}

func upsertRuntimeCombatActor(t *testing.T, service *combat.Service, actor combat.ActorState) {
	t.Helper()
	if err := service.UpsertActor(actor); err != nil {
		t.Fatalf("UpsertActor(%s) error = %v, want nil", actor.EntityID, err)
	}
}

func runtimePlayerCombatActor(entityID world.EntityID, playerID foundation.PlayerID, position world.Vec2) combat.ActorState {
	return combat.ActorState{
		EntityID:  entityID,
		Type:      world.EntityTypePlayer,
		PlayerID:  playerID,
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
		Stats:     runtimeCombatStats(playerID, 100, 10, 1),
		HP:        100,
		Shield:    50,
		Energy:    100,
	}
}

func runtimeNPCCombatActor(entityID world.EntityID, position world.Vec2) combat.ActorState {
	return combat.ActorState{
		EntityID:  entityID,
		Type:      world.EntityTypeNPCPlaceholder,
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		Position:  position,
		Signature: visibility.EntitySignature(1),
		Stats:     runtimeCombatStats("", 100, 0, 0),
		HP:        100,
		Shield:    20,
		Energy:    0,
	}
}

func runtimeCombatStats(
	playerID foundation.PlayerID,
	rangeUnits float64,
	energyCost float64,
	weaponDamage float64,
) stats.StatSnapshot {
	return stats.NewStatSnapshot(
		playerID,
		"ship-1",
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
		time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC),
	)
}
