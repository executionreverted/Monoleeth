package ships

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestEnsureStarterShipCreatesAndActivatesStarterWhenNoActiveShip(t *testing.T) {
	service, _ := newTestHangarService(t)

	result, err := service.EnsureStarterShip("player-1")
	if err != nil {
		t.Fatalf("EnsureStarterShip error = %v, want nil", err)
	}
	if !result.Created {
		t.Fatalf("EnsureStarterShip Created = false, want true")
	}
	if !result.HasActiveShip || result.ActiveShip.ShipID != ShipIDStarter {
		t.Fatalf("active ship = %+v, has = %v, want starter", result.ActiveShip, result.HasActiveShip)
	}
	if !result.ActiveChanged {
		t.Fatalf("ActiveChanged = false, want true")
	}
	assertStatInvalidation(t, result.StatInvalidation, "player-1", "", ShipIDStarter)

	hangar, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar error = %v, want nil", err)
	}
	if got, want := len(hangar.Ships), 1; got != want {
		t.Fatalf("hangar ship count = %d, want %d", got, want)
	}
	if hangar.Ships[0].ShipID != ShipIDStarter || hangar.Ships[0].State != ShipStateActive {
		t.Fatalf("starter row = %+v, want active starter", hangar.Ships[0])
	}
	if hangar.ActiveShip.ShipID != ShipIDStarter {
		t.Fatalf("hangar active ship = %q, want %q", hangar.ActiveShip.ShipID, ShipIDStarter)
	}

	second, err := service.EnsureStarterShip("player-1")
	if err != nil {
		t.Fatalf("second EnsureStarterShip error = %v, want nil", err)
	}
	if second.Created || second.ActiveChanged || second.StatInvalidation != nil {
		t.Fatalf("second EnsureStarterShip = %+v, want idempotent no-op", second)
	}
}

func TestUnlockShipIsIdempotentByPlayerAndShip(t *testing.T) {
	service, clock := newTestHangarService(t)

	first, err := service.UnlockShip(UnlockShipInput{
		PlayerID:    "player-1",
		ShipID:      ShipIDFighterT1,
		Source:      "test",
		ReferenceID: "unlock-1",
	})
	if err != nil {
		t.Fatalf("UnlockShip first error = %v, want nil", err)
	}
	if !first.Unlocked || first.Duplicate {
		t.Fatalf("first UnlockShip = %+v, want unlocked non-duplicate", first)
	}

	clock.Advance(time.Minute)
	second, err := service.UnlockShip(UnlockShipInput{
		PlayerID:    "player-1",
		ShipID:      ShipIDFighterT1,
		Source:      "test",
		ReferenceID: "unlock-1-retry",
	})
	if err != nil {
		t.Fatalf("UnlockShip duplicate error = %v, want nil", err)
	}
	if !second.Duplicate || second.Unlocked {
		t.Fatalf("duplicate UnlockShip = %+v, want duplicate no-op", second)
	}
	if !second.PlayerShip.UnlockedAt.Equal(first.PlayerShip.UnlockedAt) {
		t.Fatalf("duplicate UnlockedAt = %s, want original %s", second.PlayerShip.UnlockedAt, first.PlayerShip.UnlockedAt)
	}

	hangar, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar error = %v, want nil", err)
	}
	if got, want := len(hangar.Ships), 1; got != want {
		t.Fatalf("hangar ship count = %d, want %d", got, want)
	}
	if hangar.Ships[0].ShipID != ShipIDFighterT1 || hangar.Ships[0].State != ShipStateAvailable {
		t.Fatalf("unlocked row = %+v, want available fighter", hangar.Ships[0])
	}
}

func TestUnlockShipRejectsRankGatedShipBeforePlayerRank(t *testing.T) {
	service, _ := newTestHangarServiceWithRanksAndCargo(t, StaticPlayerRankProvider{"player-1": 1}, BaseShipCargoCapacityProvider{})

	_, err := service.UnlockShip(UnlockShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
	})
	if !errors.Is(err, ErrShipRankRequirementNotMet) {
		t.Fatalf("UnlockShip(rank gated) error = %v, want ErrShipRankRequirementNotMet", err)
	}
}

func TestSetActiveShipSwapsInSafeHangarAndReturnsStatInvalidation(t *testing.T) {
	service, _ := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)

	result, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea:  true,
			CurrentCargoUnits: 10,
		},
	})
	if err != nil {
		t.Fatalf("SetActiveShip error = %v, want nil", err)
	}
	if !result.ActiveChanged {
		t.Fatalf("ActiveChanged = false, want true")
	}
	if result.PreviousShipID != ShipIDStarter {
		t.Fatalf("PreviousShipID = %q, want %q", result.PreviousShipID, ShipIDStarter)
	}
	if result.ActiveShip.ShipID != ShipIDFighterT1 {
		t.Fatalf("ActiveShip = %q, want %q", result.ActiveShip.ShipID, ShipIDFighterT1)
	}
	assertStatInvalidation(t, result.StatInvalidation, "player-1", ShipIDStarter, ShipIDFighterT1)

	starter, ok := service.store.PlayerShip("player-1", ShipIDStarter)
	if !ok {
		t.Fatalf("starter ship missing after swap")
	}
	if starter.State != ShipStateAvailable {
		t.Fatalf("starter state = %q, want available", starter.State)
	}
	fighter, ok := service.store.PlayerShip("player-1", ShipIDFighterT1)
	if !ok {
		t.Fatalf("fighter ship missing after swap")
	}
	if fighter.State != ShipStateActive {
		t.Fatalf("fighter state = %q, want active", fighter.State)
	}

	retry, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea:  true,
			CurrentCargoUnits: 10,
		},
	})
	if err != nil {
		t.Fatalf("SetActiveShip retry error = %v, want nil", err)
	}
	if retry.ActiveChanged || retry.StatInvalidation != nil {
		t.Fatalf("retry result = %+v, want no active change or invalidation", retry)
	}
}

func TestSetActiveShipRejectsRankGatedActiveShip(t *testing.T) {
	service, _ := newTestHangarServiceWithRanksAndCargo(t, StaticPlayerRankProvider{"player-1": 1}, BaseShipCargoCapacityProvider{})
	if _, err := service.EnsureStarterShip("player-1"); err != nil {
		t.Fatalf("EnsureStarterShip error = %v, want nil", err)
	}
	fighter, err := NewPlayerShipState("player-1", ShipIDFighterT1, ShipStateAvailable)
	if err != nil {
		t.Fatalf("NewPlayerShipState(fighter) error = %v, want nil", err)
	}
	if err := service.store.PutPlayerShip(fighter); err != nil {
		t.Fatalf("PutPlayerShip(fighter) error = %v, want nil", err)
	}

	_, err = service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	})
	if !errors.Is(err, ErrShipRankRequirementNotMet) {
		t.Fatalf("SetActiveShip(rank gated) error = %v, want ErrShipRankRequirementNotMet", err)
	}
}

func TestSetActiveShipUsesEffectiveTargetCargoCapacity(t *testing.T) {
	service, _ := newTestHangarServiceWithCargo(t, StaticShipCargoCapacityProvider{
		NewShipCargoCapacityKey("player-1", ShipIDFighterT1): 80,
	})
	ensureStarterAndUnlockFighter(t, service)

	result, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea:  true,
			CurrentCargoUnits: 75,
		},
	})
	if err != nil {
		t.Fatalf("SetActiveShip with effective cargo error = %v, want nil", err)
	}
	if !result.ActiveChanged || result.ActiveShip.ShipID != ShipIDFighterT1 {
		t.Fatalf("SetActiveShip result = %+v, want active fighter", result)
	}
}

func TestSetActiveShipRejectsInvalidEffectiveTargetCargoCapacity(t *testing.T) {
	service, _ := newTestHangarServiceWithCargo(t, StaticShipCargoCapacityProvider{
		NewShipCargoCapacityKey("player-1", ShipIDFighterT1): -1,
	})
	ensureStarterAndUnlockFighter(t, service)

	_, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	})
	if !errors.Is(err, ErrInvalidTargetCargoCapacity) {
		t.Fatalf("SetActiveShip invalid target capacity error = %v, want ErrInvalidTargetCargoCapacity", err)
	}
	active, ok := service.store.ActiveShip("player-1")
	if !ok {
		t.Fatalf("active ship missing after failed swap")
	}
	if active.ShipID != ShipIDStarter {
		t.Fatalf("active ship = %q, want %q after failed swap", active.ShipID, ShipIDStarter)
	}
}

func TestSetActiveShipRejectsCombatUnsafeCargoAndDisabledTargets(t *testing.T) {
	tests := []struct {
		name    string
		context ShipSwapContext
		mutate  func(t *testing.T, service *HangarService)
		wantErr error
	}{
		{
			name: "combat",
			context: ShipSwapContext{
				InSafeHangarArea: true,
				InCombat:         true,
			},
			wantErr: ErrCannotSwapInCombat,
		},
		{
			name: "outside safe hangar",
			context: ShipSwapContext{
				InSafeHangarArea: false,
			},
			wantErr: ErrNotInHangarArea,
		},
		{
			name: "cargo exceeds target capacity",
			context: ShipSwapContext{
				InSafeHangarArea:  true,
				CurrentCargoUnits: 41,
			},
			wantErr: ErrCargoExceedsTargetCapacity,
		},
		{
			name: "disabled target",
			context: ShipSwapContext{
				InSafeHangarArea: true,
			},
			mutate: func(t *testing.T, service *HangarService) {
				t.Helper()
				fighter, ok := service.store.PlayerShip("player-1", ShipIDFighterT1)
				if !ok {
					t.Fatalf("fighter ship missing")
				}
				fighter.State = ShipStateDisabled
				fighter.DisabledReason = "death"
				if err := service.store.PutPlayerShip(fighter); err != nil {
					t.Fatalf("PutPlayerShip(disabled) error = %v", err)
				}
			},
			wantErr: ErrShipDisabled,
		},
		{
			name: "repairing target unavailable",
			context: ShipSwapContext{
				InSafeHangarArea: true,
			},
			mutate: func(t *testing.T, service *HangarService) {
				t.Helper()
				fighter, ok := service.store.PlayerShip("player-1", ShipIDFighterT1)
				if !ok {
					t.Fatalf("fighter ship missing")
				}
				fighter.State = ShipStateRepairing
				if err := service.store.PutPlayerShip(fighter); err != nil {
					t.Fatalf("PutPlayerShip(repairing) error = %v", err)
				}
			},
			wantErr: ErrShipUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestHangarService(t)
			ensureStarterAndUnlockFighter(t, service)
			if test.mutate != nil {
				test.mutate(t, service)
			}

			_, err := service.SetActiveShip(SetActiveShipInput{
				PlayerID: "player-1",
				ShipID:   ShipIDFighterT1,
				Context:  test.context,
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("SetActiveShip error = %v, want %v", err, test.wantErr)
			}

			active, ok := service.store.ActiveShip("player-1")
			if !ok {
				t.Fatalf("active ship missing after failed swap")
			}
			if active.ShipID != ShipIDStarter {
				t.Fatalf("active ship = %q, want %q after failed swap", active.ShipID, ShipIDStarter)
			}
		})
	}
}

func TestSetActiveShipRejectsUnknownLockedAndInvalidCargo(t *testing.T) {
	service, _ := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)

	if _, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDScoutT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	}); !errors.Is(err, ErrShipNotUnlocked) {
		t.Fatalf("locked ship error = %v, want ErrShipNotUnlocked", err)
	}

	if _, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea:  true,
			CurrentCargoUnits: -1,
		},
	}); !errors.Is(err, ErrInvalidCurrentCargoAmount) {
		t.Fatalf("negative cargo error = %v, want ErrInvalidCurrentCargoAmount", err)
	}
}

func newTestHangarService(t *testing.T) (*HangarService, *testutil.FakeClock) {
	t.Helper()
	return newTestHangarServiceWithRanksAndCargo(t, StaticPlayerRankProvider{"player-1": 2}, BaseShipCargoCapacityProvider{})
}

func newTestHangarServiceWithCargo(t *testing.T, cargo ShipCargoCapacityProvider) (*HangarService, *testutil.FakeClock) {
	t.Helper()
	return newTestHangarServiceWithRanksAndCargo(t, StaticPlayerRankProvider{"player-1": 2}, cargo)
}

func newTestHangarServiceWithRanksAndCargo(
	t *testing.T,
	ranks StaticPlayerRankProvider,
	cargo ShipCargoCapacityProvider,
) (*HangarService, *testutil.FakeClock) {
	t.Helper()

	catalogRows, err := MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog error = %v", err)
	}
	clock := testutil.NewFakeClock(testShipServiceNow)
	service, err := NewHangarService(catalogRows, NewInMemoryHangarStore(), ranks, cargo, clock)
	if err != nil {
		t.Fatalf("NewHangarService error = %v", err)
	}
	return service, clock
}

func ensureStarterAndUnlockFighter(t *testing.T, service *HangarService) {
	t.Helper()

	if _, err := service.EnsureStarterShip("player-1"); err != nil {
		t.Fatalf("EnsureStarterShip error = %v", err)
	}
	if _, err := service.UnlockShip(UnlockShipInput{PlayerID: "player-1", ShipID: ShipIDFighterT1}); err != nil {
		t.Fatalf("UnlockShip(fighter) error = %v", err)
	}
}

func assertStatInvalidation(
	t *testing.T,
	signal *StatInvalidationSignal,
	playerID foundation.PlayerID,
	previousShipID foundation.ShipID,
	activeShipID foundation.ShipID,
) {
	t.Helper()

	if signal == nil {
		t.Fatalf("stat invalidation = nil, want signal")
	}
	if signal.PlayerID != playerID {
		t.Fatalf("signal player = %q, want %q", signal.PlayerID, playerID)
	}
	if signal.PreviousShipID != previousShipID {
		t.Fatalf("signal previous ship = %q, want %q", signal.PreviousShipID, previousShipID)
	}
	if signal.ActiveShipID != activeShipID {
		t.Fatalf("signal active ship = %q, want %q", signal.ActiveShipID, activeShipID)
	}
	if signal.Reason != StatInvalidationReasonActiveShipChanged {
		t.Fatalf("signal reason = %q, want %q", signal.Reason, StatInvalidationReasonActiveShipChanged)
	}
	if !signal.CreatedAt.Equal(testShipServiceNow) {
		t.Fatalf("signal CreatedAt = %s, want %s", signal.CreatedAt, testShipServiceNow)
	}
}

var testShipServiceNow = time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC)
