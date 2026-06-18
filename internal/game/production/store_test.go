package production

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestInMemoryStoreInitializePlanetProductionIsIdempotentAndCloned(t *testing.T) {
	store := NewInMemoryStore()
	claimTime := time.Date(2026, 6, 18, 9, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))

	first, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              "planet-1",
		LastCalculatedAt:      claimTime,
		StorageCapacityUnits:  100,
		EnergyCapacityPerHour: 25,
		UpdatedAt:             claimTime,
	})
	if err != nil {
		t.Fatalf("InitializePlanetProduction(first) error = %v, want nil", err)
	}
	if !first.Created {
		t.Fatalf("InitializePlanetProduction(first) Created = false, want true")
	}
	if first.Snapshot.State.LastCalculatedAt.Location() != time.UTC {
		t.Fatalf("LastCalculatedAt location = %v, want UTC", first.Snapshot.State.LastCalculatedAt.Location())
	}

	first.Snapshot.State.EnergyCapacityPerHour = 999
	first.Snapshot.Storage.CapacityUnits = 999
	second, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              "planet-1",
		LastCalculatedAt:      testTime(50),
		StorageCapacityUnits:  500,
		EnergyCapacityPerHour: 100,
		UpdatedAt:             testTime(50),
	})
	if err != nil {
		t.Fatalf("InitializePlanetProduction(second) error = %v, want nil", err)
	}
	if second.Created {
		t.Fatalf("InitializePlanetProduction(second) Created = true, want false")
	}
	if second.Snapshot.State.EnergyCapacityPerHour != 25 || second.Snapshot.Storage.CapacityUnits != 100 {
		t.Fatalf("existing snapshot = %+v, want original energy 25 capacity 100", second.Snapshot)
	}
}

func TestInMemoryStoreSnapshotAndBuildingListsAreDetachedAndSorted(t *testing.T) {
	store := NewInMemoryStore()
	if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              "planet-1",
		LastCalculatedAt:      testTime(0),
		StorageCapacityUnits:  100,
		EnergyCapacityPerHour: 25,
		UpdatedAt:             testTime(0),
	}); err != nil {
		t.Fatalf("InitializePlanetProduction() error = %v, want nil", err)
	}

	catalogRows := MustMVPCatalog()
	extractor, _ := catalogRows.Get(ProductionDefinitionIDIronExtractorL1)
	refinery, _ := catalogRows.Get(ProductionDefinitionIDAlloyFoundryL1)
	buildingB, err := NewPlanetBuilding("building-b", "planet-1", refinery, BuildingStateDisabled, testTime(1), testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetBuilding(disabled) error = %v, want nil", err)
	}
	buildingA, err := NewPlanetBuilding("building-a", "planet-1", extractor, BuildingStateActive, testTime(1), testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetBuilding(active) error = %v, want nil", err)
	}
	if _, created, err := store.UpsertBuilding(buildingB); err != nil || !created {
		t.Fatalf("UpsertBuilding(disabled) created = %v err = %v, want true nil", created, err)
	}
	if _, created, err := store.UpsertBuilding(buildingA); err != nil || !created {
		t.Fatalf("UpsertBuilding(active) created = %v err = %v, want true nil", created, err)
	}

	buildings, err := store.Buildings("planet-1")
	if err != nil {
		t.Fatalf("Buildings() error = %v, want nil", err)
	}
	if len(buildings) != 2 || buildings[0].BuildingID != BuildingID("building-a") || buildings[1].BuildingID != BuildingID("building-b") {
		t.Fatalf("Buildings() = %+v, want sorted building-a, building-b", buildings)
	}
	buildings[0].State = BuildingStateDisabled

	active, err := store.ActiveBuildings("planet-1")
	if err != nil {
		t.Fatalf("ActiveBuildings() error = %v, want nil", err)
	}
	if len(active) != 1 || active[0].BuildingID != BuildingID("building-a") || active[0].State != BuildingStateActive {
		t.Fatalf("ActiveBuildings() = %+v, want detached active building-a", active)
	}

	snapshot, ok, err := store.Snapshot("planet-1")
	if err != nil || !ok {
		t.Fatalf("Snapshot() ok = %v err = %v, want true nil", ok, err)
	}
	snapshot.Buildings[0].State = BuildingStateDisabled

	stored, ok, err := store.Building("planet-1", "building-a")
	if err != nil || !ok {
		t.Fatalf("Building() ok = %v err = %v, want true nil", ok, err)
	}
	if stored.State != BuildingStateActive {
		t.Fatalf("stored building state = %q, want active", stored.State)
	}
}

func TestInMemoryStoreSnapshotsArePlanetIDOrdered(t *testing.T) {
	store := NewInMemoryStore()
	for _, planetID := range []foundation.PlanetID{"planet-b", "planet-a"} {
		if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
			PlanetID:              planetID,
			LastCalculatedAt:      testTime(0),
			StorageCapacityUnits:  100,
			EnergyCapacityPerHour: 25,
			UpdatedAt:             testTime(0),
		}); err != nil {
			t.Fatalf("InitializePlanetProduction(%s) error = %v, want nil", planetID, err)
		}
	}

	snapshots := store.Snapshots()
	if len(snapshots) != 2 || snapshots[0].State.PlanetID != "planet-a" || snapshots[1].State.PlanetID != "planet-b" {
		t.Fatalf("Snapshots() = %+v, want planet-a then planet-b", snapshots)
	}
}
