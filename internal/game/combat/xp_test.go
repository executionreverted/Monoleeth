package combat_test

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestNPCKillXPHandlerGrantsCombatXPOnce(t *testing.T) {
	service := progression.NewProgressionService(testutil.NewFakeClock(time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC)), nil)
	handler, err := combat.NewNPCKillXPHandler(service, combat.DefaultNPCKillXPReward())
	if err != nil {
		t.Fatalf("NewNPCKillXPHandler() error = %v, want nil", err)
	}

	result, err := handler.GrantNPCKillXP(testNPCKilledEvent())
	if err != nil {
		t.Fatalf("GrantNPCKillXP() error = %v, want nil", err)
	}
	if result.Duplicate {
		t.Fatal("first GrantNPCKillXP Duplicate = true, want false")
	}
	if result.Snapshot.Player.MainXP != 20 {
		t.Fatalf("main XP = %d, want 20", result.Snapshot.Player.MainXP)
	}
	role, ok := result.Snapshot.RoleLevel(progression.RoleTypeCombat)
	if !ok || role.XP != 20 {
		t.Fatalf("combat role = %+v ok=%t, want 20 XP", role, ok)
	}

	duplicate, err := handler.GrantNPCKillXP(testNPCKilledEvent())
	if err != nil {
		t.Fatalf("duplicate GrantNPCKillXP() error = %v, want nil", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate GrantNPCKillXP Duplicate = false, want true")
	}
	if duplicate.Snapshot.Player.MainXP != result.Snapshot.Player.MainXP {
		t.Fatalf("duplicate main XP = %d, want %d", duplicate.Snapshot.Player.MainXP, result.Snapshot.Player.MainXP)
	}
}

func TestNPCKillXPHandlerRejectsInvalidInputWithoutMutation(t *testing.T) {
	service := progression.NewProgressionService(testutil.NewFakeClock(time.Date(2026, 6, 17, 18, 30, 0, 0, time.UTC)), nil)
	handler, err := combat.NewNPCKillXPHandler(service, combat.DefaultNPCKillXPReward())
	if err != nil {
		t.Fatalf("NewNPCKillXPHandler() error = %v, want nil", err)
	}
	event := testNPCKilledEvent()
	event.OwnerPlayerID = ""

	if _, err := handler.GrantNPCKillXP(event); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("GrantNPCKillXP(invalid owner) error = %v, want ErrEmptyID", err)
	}
	snapshot, err := service.GetProgressionSnapshot("player_1")
	if err != nil {
		t.Fatalf("GetProgressionSnapshot() error = %v, want nil", err)
	}
	if snapshot.Player.MainXP != 0 {
		t.Fatalf("main XP after rejected event = %d, want 0", snapshot.Player.MainXP)
	}
}

func TestNPCKillXPHandlerConstructorValidation(t *testing.T) {
	if _, err := combat.NewNPCKillXPHandler(nil, combat.DefaultNPCKillXPReward()); !errors.Is(err, combat.ErrNilXPGranter) {
		t.Fatalf("NewNPCKillXPHandler(nil) error = %v, want ErrNilXPGranter", err)
	}
	service := progression.NewProgressionService(nil, nil)
	if _, err := combat.NewNPCKillXPHandler(service, combat.NPCKillXPReward{MainXP: 0, Role: progression.RoleTypeCombat, RoleXP: 20}); !errors.Is(err, combat.ErrInvalidXPReward) {
		t.Fatalf("NewNPCKillXPHandler(zero main xp) error = %v, want ErrInvalidXPReward", err)
	}
	if _, err := combat.NewNPCKillXPHandler(service, combat.NPCKillXPReward{MainXP: 20, Role: progression.RoleType("bogus"), RoleXP: 20}); !errors.Is(err, progression.ErrInvalidRoleType) {
		t.Fatalf("NewNPCKillXPHandler(invalid role) error = %v, want ErrInvalidRoleType", err)
	}
	if _, err := combat.NewNPCKillXPHandler(service, combat.NPCKillXPReward{MainXP: 20, Role: progression.RoleTypeCombat, RoleXP: 0}); !errors.Is(err, combat.ErrInvalidXPReward) {
		t.Fatalf("NewNPCKillXPHandler(zero role xp) error = %v, want ErrInvalidXPReward", err)
	}
}

func testNPCKilledEvent() combat.NPCKilledEvent {
	return combat.NPCKilledEvent{
		SourceID:      "npc_1",
		NPCEntityID:   "npc_1",
		WorldID:       "world_1",
		ZoneID:        "zone_1",
		Position:      world.Vec2{},
		OwnerPlayerID: "player_1",
		KilledAt:      time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC),
	}
}
