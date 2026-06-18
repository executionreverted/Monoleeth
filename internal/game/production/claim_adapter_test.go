package production

import (
	"testing"
	"time"

	"gameproject/internal/game/discovery"
)

func TestClaimProductionInitializerInitializesStoreWithConfiguredDefaults(t *testing.T) {
	store := NewInMemoryStore()
	initializer, err := NewClaimProductionInitializer(ClaimProductionInitializerConfig{
		Store: store,
		Defaults: ClaimProductionInitializationDefaults{
			StorageCapacityUnits:  250,
			EnergyCapacityPerHour: 40,
		},
	})
	if err != nil {
		t.Fatalf("NewClaimProductionInitializer() error = %v, want nil", err)
	}
	claimedAt := time.Date(2026, 6, 18, 13, 45, 0, 0, time.FixedZone("UTC+3", 3*60*60))

	result, err := initializer.InitializeClaimProduction(discovery.ClaimProductionInitializeInput{
		PlayerID:       "player-1",
		PlanetID:       "planet-1",
		PlanetLevel:    3,
		ClaimedAt:      claimedAt,
		ClaimReference: "claim-1",
	})
	if err != nil {
		t.Fatalf("InitializeClaimProduction() error = %v, want nil", err)
	}
	if !result.Created || result.AlreadyInitialized {
		t.Fatalf("InitializeClaimProduction() result = %+v, want created only", result)
	}

	snapshot, ok, err := store.Snapshot("planet-1")
	if err != nil || !ok {
		t.Fatalf("Snapshot() ok = %v err = %v, want true nil", ok, err)
	}
	if !snapshot.State.LastCalculatedAt.Equal(claimedAt.UTC()) || !snapshot.State.UpdatedAt.Equal(claimedAt.UTC()) {
		t.Fatalf("state timestamps = %s/%s, want %s", snapshot.State.LastCalculatedAt, snapshot.State.UpdatedAt, claimedAt.UTC())
	}
	if snapshot.Storage.CapacityUnits != 250 {
		t.Fatalf("storage capacity = %d, want 250", snapshot.Storage.CapacityUnits)
	}
	if snapshot.State.EnergyCapacityPerHour != 40 {
		t.Fatalf("energy capacity = %d, want 40", snapshot.State.EnergyCapacityPerHour)
	}
}

func TestClaimProductionInitializerReportsAlreadyInitializedWithoutOverwriting(t *testing.T) {
	store := NewInMemoryStore()
	first, err := NewClaimProductionInitializer(ClaimProductionInitializerConfig{
		Store: store,
		Defaults: ClaimProductionInitializationDefaults{
			StorageCapacityUnits:  250,
			EnergyCapacityPerHour: 40,
		},
	})
	if err != nil {
		t.Fatalf("NewClaimProductionInitializer(first) error = %v, want nil", err)
	}
	claimedAt := testTime(10)
	input := discovery.ClaimProductionInitializeInput{
		PlayerID:       "player-1",
		PlanetID:       "planet-1",
		PlanetLevel:    3,
		ClaimedAt:      claimedAt,
		ClaimReference: "claim-1",
	}
	if _, err := first.InitializeClaimProduction(input); err != nil {
		t.Fatalf("InitializeClaimProduction(first) error = %v, want nil", err)
	}

	second, err := NewClaimProductionInitializer(ClaimProductionInitializerConfig{
		Store: store,
		Defaults: ClaimProductionInitializationDefaults{
			StorageCapacityUnits:  999,
			EnergyCapacityPerHour: 99,
		},
	})
	if err != nil {
		t.Fatalf("NewClaimProductionInitializer(second) error = %v, want nil", err)
	}
	input.ClaimedAt = testTime(20)
	result, err := second.InitializeClaimProduction(input)
	if err != nil {
		t.Fatalf("InitializeClaimProduction(second) error = %v, want nil", err)
	}
	if result.Created || !result.AlreadyInitialized {
		t.Fatalf("InitializeClaimProduction(second) result = %+v, want already initialized", result)
	}

	snapshot, ok, err := store.Snapshot("planet-1")
	if err != nil || !ok {
		t.Fatalf("Snapshot() ok = %v err = %v, want true nil", ok, err)
	}
	if snapshot.Storage.CapacityUnits != 250 || snapshot.State.EnergyCapacityPerHour != 40 {
		t.Fatalf("snapshot defaults = capacity %d energy %d, want original 250/40", snapshot.Storage.CapacityUnits, snapshot.State.EnergyCapacityPerHour)
	}
	if !snapshot.State.LastCalculatedAt.Equal(claimedAt) {
		t.Fatalf("last calculated at = %s, want original %s", snapshot.State.LastCalculatedAt, claimedAt)
	}
}
