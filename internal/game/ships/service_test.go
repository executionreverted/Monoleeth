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

	starter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDStarter)
	if !ok {
		t.Fatalf("starter ship missing after swap")
	}
	if starter.State != ShipStateAvailable {
		t.Fatalf("starter state = %q, want available", starter.State)
	}
	fighter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDFighterT1)
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

func TestDisableActiveShipForDeathDisablesActiveAndBlocksActivation(t *testing.T) {
	service, _ := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)

	result, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{
		PlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("DisableActiveShipForDeath error = %v, want nil", err)
	}
	if !result.Disabled || result.Duplicate {
		t.Fatalf("DisableActiveShipForDeath result = %+v, want disabled non-duplicate", result)
	}
	if result.PlayerShip.ShipID != ShipIDFighterT1 || result.PlayerShip.State != ShipStateDisabled {
		t.Fatalf("disabled ship = %+v, want disabled fighter", result.PlayerShip)
	}
	if result.PlayerShip.DisabledReason != DisabledReasonDeath {
		t.Fatalf("DisabledReason = %q, want %q", result.PlayerShip.DisabledReason, DisabledReasonDeath)
	}
	if result.PlayerShip.DisabledAt == nil || !result.PlayerShip.DisabledAt.Equal(testShipServiceNow) {
		t.Fatalf("DisabledAt = %v, want %s", result.PlayerShip.DisabledAt, testShipServiceNow)
	}
	assertStatInvalidationAt(
		t,
		result.StatInvalidation,
		"player-1",
		"",
		ShipIDFighterT1,
		StatInvalidationReasonActiveShipStateChanged,
		testShipServiceNow,
	)

	_, err = service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	})
	if !errors.Is(err, ErrShipDisabled) {
		t.Fatalf("SetActiveShip(disabled active) error = %v, want ErrShipDisabled", err)
	}
}

func TestDisableActiveShipForDeathDuplicateDoesNotMutateTwice(t *testing.T) {
	service, clock := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)

	first, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{
		PlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("DisableActiveShipForDeath first error = %v, want nil", err)
	}
	if first.PlayerShip.DisabledAt == nil {
		t.Fatalf("first DisabledAt = nil, want timestamp")
	}
	firstDisabledAt := *first.PlayerShip.DisabledAt
	firstActiveUpdatedAt := first.ActiveShip.UpdatedAt

	clock.Advance(time.Minute)
	second, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{
		PlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("DisableActiveShipForDeath duplicate error = %v, want nil", err)
	}
	if !second.Duplicate || second.Disabled {
		t.Fatalf("duplicate result = %+v, want duplicate no-op", second)
	}
	if second.StatInvalidation != nil {
		t.Fatalf("duplicate stat invalidation = %+v, want nil", second.StatInvalidation)
	}
	if second.PlayerShip.DisabledAt == nil || !second.PlayerShip.DisabledAt.Equal(firstDisabledAt) {
		t.Fatalf("duplicate DisabledAt = %v, want %s", second.PlayerShip.DisabledAt, firstDisabledAt)
	}
	if !second.ActiveShip.UpdatedAt.Equal(firstActiveUpdatedAt) {
		t.Fatalf("duplicate active UpdatedAt = %s, want %s", second.ActiveShip.UpdatedAt, firstActiveUpdatedAt)
	}
}

func TestWithActiveShipCombatLeaseRunsOnlyForActiveShip(t *testing.T) {
	service, _ := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)

	ran := false
	if err := service.WithActiveShipCombatLease("player-1", func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("WithActiveShipCombatLease(active) error = %v, want nil", err)
	}
	if !ran {
		t.Fatal("WithActiveShipCombatLease(active) did not run action")
	}

	if _, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{PlayerID: "player-1"}); err != nil {
		t.Fatalf("DisableActiveShipForDeath error = %v, want nil", err)
	}
	ran = false
	err := service.WithActiveShipCombatLease("player-1", func() error {
		ran = true
		return nil
	})
	if !errors.Is(err, ErrShipDisabled) {
		t.Fatalf("WithActiveShipCombatLease(disabled) error = %v, want ErrShipDisabled", err)
	}
	if ran {
		t.Fatal("WithActiveShipCombatLease(disabled) ran action, want blocked")
	}
}

func TestWithActiveShipCombatLeaseSerializesDeathDisable(t *testing.T) {
	service, _ := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)

	enteredLease := make(chan struct{})
	releaseLease := make(chan struct{})
	leaseDone := make(chan error, 1)
	go func() {
		leaseDone <- service.WithActiveShipCombatLease("player-1", func() error {
			close(enteredLease)
			<-releaseLease
			return nil
		})
	}()

	<-enteredLease
	disableDone := make(chan error, 1)
	go func() {
		_, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{PlayerID: "player-1"})
		disableDone <- err
	}()

	select {
	case err := <-disableDone:
		t.Fatalf("DisableActiveShipForDeath completed while combat lease was held, err = %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseLease)
	if err := <-leaseDone; err != nil {
		t.Fatalf("WithActiveShipCombatLease error = %v, want nil", err)
	}
	if err := <-disableDone; err != nil {
		t.Fatalf("DisableActiveShipForDeath after lease release error = %v, want nil", err)
	}
	hangar, err := service.GetHangar("player-1")
	if err != nil {
		t.Fatalf("GetHangar error = %v, want nil", err)
	}
	var activeRow PlayerShipState
	for _, playerShip := range hangar.Ships {
		if playerShip.ShipID == hangar.ActiveShip.ShipID {
			activeRow = playerShip
			break
		}
	}
	if activeRow.State != ShipStateDisabled {
		t.Fatalf("active ship state after queued death disable = %q, want disabled", activeRow.State)
	}
}

func TestRepairShipRejectsNonDisabledShip(t *testing.T) {
	service, _ := newTestHangarService(t)
	if _, err := service.EnsureStarterShip("player-1"); err != nil {
		t.Fatalf("EnsureStarterShip error = %v, want nil", err)
	}

	_, err := service.RepairShip(RepairShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDStarter,
	})
	if !errors.Is(err, ErrShipNotDisabled) {
		t.Fatalf("RepairShip(non-disabled) error = %v, want ErrShipNotDisabled", err)
	}
}

func TestRepairShipRestoresInactiveDisabledShipAvailable(t *testing.T) {
	service, clock := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	disabledAt := testShipServiceNow
	fighter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDFighterT1)
	if !ok {
		t.Fatalf("fighter ship missing")
	}
	fighter.State = ShipStateDisabled
	fighter.DisabledReason = DisabledReasonDeath
	fighter.DisabledAt = &disabledAt
	if err := inMemoryHangarStore(t, service).PutPlayerShip(fighter); err != nil {
		t.Fatalf("PutPlayerShip(disabled fighter) error = %v", err)
	}

	repairedAt := clock.Advance(time.Minute)
	result, err := service.RepairShip(RepairShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
	})
	if err != nil {
		t.Fatalf("RepairShip(inactive) error = %v, want nil", err)
	}
	if result.PlayerShip.State != ShipStateAvailable {
		t.Fatalf("inactive repaired ship state = %q, want available", result.PlayerShip.State)
	}
	if result.PlayerShip.LastRepairedAt == nil || !result.PlayerShip.LastRepairedAt.Equal(repairedAt) {
		t.Fatalf("inactive LastRepairedAt = %v, want %s", result.PlayerShip.LastRepairedAt, repairedAt)
	}
	if result.StatInvalidation != nil {
		t.Fatalf("inactive repair stat invalidation = %+v, want nil", result.StatInvalidation)
	}
}

func TestRepairShipRestoresDisabledShip(t *testing.T) {
	service, clock := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)
	if _, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{PlayerID: "player-1"}); err != nil {
		t.Fatalf("DisableActiveShipForDeath error = %v, want nil", err)
	}

	repairedAt := clock.Advance(time.Minute)
	result, err := service.RepairShip(RepairShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
	})
	if err != nil {
		t.Fatalf("RepairShip error = %v, want nil", err)
	}
	if !result.Repaired {
		t.Fatalf("RepairShip Repaired = false, want true")
	}
	if result.PlayerShip.State != ShipStateActive {
		t.Fatalf("repaired active ship state = %q, want active", result.PlayerShip.State)
	}
	if result.PlayerShip.DisabledReason != "" || result.PlayerShip.DisabledAt != nil {
		t.Fatalf("repaired disabled fields = reason %q at %v, want cleared", result.PlayerShip.DisabledReason, result.PlayerShip.DisabledAt)
	}
	if result.PlayerShip.LastRepairedAt == nil || !result.PlayerShip.LastRepairedAt.Equal(repairedAt) {
		t.Fatalf("LastRepairedAt = %v, want %s", result.PlayerShip.LastRepairedAt, repairedAt)
	}
	assertStatInvalidationAt(
		t,
		result.StatInvalidation,
		"player-1",
		"",
		ShipIDFighterT1,
		StatInvalidationReasonActiveShipStateChanged,
		repairedAt,
	)

	retry, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	})
	if err != nil {
		t.Fatalf("SetActiveShip(repaired active) error = %v, want nil", err)
	}
	if retry.ActiveChanged {
		t.Fatalf("SetActiveShip(repaired active) ActiveChanged = true, want no-op")
	}
}

func TestEnsureStarterShipFallbackRestoresAndActivatesStarterWhenAllShipsDisabled(t *testing.T) {
	service, clock := newTestHangarService(t)
	ensureStarterAndUnlockFighter(t, service)
	activateFighter(t, service)

	disabledAt := testShipServiceNow
	starter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDStarter)
	if !ok {
		t.Fatalf("starter ship missing")
	}
	starter.State = ShipStateDisabled
	starter.DisabledReason = DisabledReasonDeath
	starter.DisabledAt = &disabledAt
	if err := inMemoryHangarStore(t, service).PutPlayerShip(starter); err != nil {
		t.Fatalf("PutPlayerShip(disabled starter) error = %v", err)
	}
	if _, err := service.DisableActiveShipForDeath(DisableActiveShipForDeathInput{PlayerID: "player-1"}); err != nil {
		t.Fatalf("DisableActiveShipForDeath(fighter) error = %v, want nil", err)
	}

	restoredAt := clock.Advance(time.Minute)
	result, err := service.EnsureStarterShip("player-1")
	if err != nil {
		t.Fatalf("EnsureStarterShip fallback error = %v, want nil", err)
	}
	if !result.Restored || !result.ActiveChanged {
		t.Fatalf("fallback result = %+v, want restored active change", result)
	}
	if !result.HasActiveShip || result.ActiveShip.ShipID != ShipIDStarter {
		t.Fatalf("fallback active ship = %+v has=%v, want starter", result.ActiveShip, result.HasActiveShip)
	}
	if result.PlayerShip.State != ShipStateActive {
		t.Fatalf("starter state = %q, want active", result.PlayerShip.State)
	}
	if result.PlayerShip.DisabledReason != "" || result.PlayerShip.DisabledAt != nil {
		t.Fatalf("starter disabled fields = reason %q at %v, want cleared", result.PlayerShip.DisabledReason, result.PlayerShip.DisabledAt)
	}
	if result.PlayerShip.LastRepairedAt == nil || !result.PlayerShip.LastRepairedAt.Equal(restoredAt) {
		t.Fatalf("starter LastRepairedAt = %v, want %s", result.PlayerShip.LastRepairedAt, restoredAt)
	}
	assertStatInvalidationAt(
		t,
		result.StatInvalidation,
		"player-1",
		ShipIDFighterT1,
		ShipIDStarter,
		StatInvalidationReasonActiveShipChanged,
		restoredAt,
	)

	fighter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDFighterT1)
	if !ok {
		t.Fatalf("fighter ship missing")
	}
	if fighter.State != ShipStateDisabled {
		t.Fatalf("fighter state = %q, want disabled after fallback", fighter.State)
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
	if err := inMemoryHangarStore(t, service).PutPlayerShip(fighter); err != nil {
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
	active, ok := inMemoryHangarStore(t, service).ActiveShip("player-1")
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
				fighter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDFighterT1)
				if !ok {
					t.Fatalf("fighter ship missing")
				}
				fighter.State = ShipStateDisabled
				fighter.DisabledReason = "death"
				if err := inMemoryHangarStore(t, service).PutPlayerShip(fighter); err != nil {
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
				fighter, ok := inMemoryHangarStore(t, service).PlayerShip("player-1", ShipIDFighterT1)
				if !ok {
					t.Fatalf("fighter ship missing")
				}
				fighter.State = ShipStateRepairing
				if err := inMemoryHangarStore(t, service).PutPlayerShip(fighter); err != nil {
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

			active, ok := inMemoryHangarStore(t, service).ActiveShip("player-1")
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

func inMemoryHangarStore(t *testing.T, service *HangarService) *InMemoryHangarStore {
	t.Helper()

	store, ok := service.store.(*InMemoryHangarStore)
	if !ok {
		t.Fatalf("service store = %T, want *InMemoryHangarStore", service.store)
	}
	return store
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

func activateFighter(t *testing.T, service *HangarService) {
	t.Helper()

	result, err := service.SetActiveShip(SetActiveShipInput{
		PlayerID: "player-1",
		ShipID:   ShipIDFighterT1,
		Context: ShipSwapContext{
			InSafeHangarArea: true,
		},
	})
	if err != nil {
		t.Fatalf("SetActiveShip(fighter) error = %v, want nil", err)
	}
	if !result.ActiveChanged || result.ActiveShip.ShipID != ShipIDFighterT1 {
		t.Fatalf("SetActiveShip(fighter) = %+v, want active fighter", result)
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

	assertStatInvalidationAt(
		t,
		signal,
		playerID,
		previousShipID,
		activeShipID,
		StatInvalidationReasonActiveShipChanged,
		testShipServiceNow,
	)
}

func assertStatInvalidationAt(
	t *testing.T,
	signal *StatInvalidationSignal,
	playerID foundation.PlayerID,
	previousShipID foundation.ShipID,
	activeShipID foundation.ShipID,
	reason StatInvalidationReason,
	createdAt time.Time,
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
	if signal.Reason != reason {
		t.Fatalf("signal reason = %q, want %q", signal.Reason, reason)
	}
	if !signal.CreatedAt.Equal(createdAt) {
		t.Fatalf("signal CreatedAt = %s, want %s", signal.CreatedAt, createdAt)
	}
}

var testShipServiceNow = time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC)
