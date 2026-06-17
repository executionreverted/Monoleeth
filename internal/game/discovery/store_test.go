package discovery

import (
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestMaterializePlanetReturnsExistingByCandidateKeyWithoutMutation(t *testing.T) {
	store := NewInMemoryStore()
	first := testPlanet("planet-1", testTime(0))

	result, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: "world-1:cell-4:-2:planet",
		Planet:       first,
	})
	if err != nil {
		t.Fatalf("MaterializePlanet(first) error = %v, want nil", err)
	}
	if !result.Created {
		t.Fatalf("MaterializePlanet(first) Created = false, want true")
	}

	duplicate := testPlanet("planet-2", testTime(10))
	duplicate.Coordinates = world.Vec2{X: 9999, Y: 8888}
	duplicate.Level = 99
	duplicate.DiscoveredBy = "player-other"
	result, err = store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: "world-1:cell-4:-2:planet",
		Planet:       duplicate,
	})
	if err != nil {
		t.Fatalf("MaterializePlanet(duplicate) error = %v, want nil", err)
	}
	if result.Created {
		t.Fatalf("MaterializePlanet(duplicate) Created = true, want false")
	}
	if result.Planet.ID != first.ID || result.Planet.Level != first.Level || result.Planet.DiscoveredBy != first.DiscoveredBy {
		t.Fatalf("duplicate returned planet = %+v, want original %+v", result.Planet, first)
	}
	if got := store.Planets(); len(got) != 1 {
		t.Fatalf("Planets() len = %d, want 1", len(got))
	}
}

func TestMaterializePlanetReturnsExistingByPlanetIDWithoutAddingCandidateAlias(t *testing.T) {
	store := NewInMemoryStore()
	first := testPlanet("planet-1", testTime(0))

	if _, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: "candidate-a",
		Planet:       first,
	}); err != nil {
		t.Fatalf("MaterializePlanet(first) error = %v, want nil", err)
	}

	duplicate := first
	duplicate.Level = first.Level + 1
	duplicate.DiscoveredAt = testTime(5)
	result, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: "candidate-b",
		Planet:       duplicate,
	})
	if err != nil {
		t.Fatalf("MaterializePlanet(same id) error = %v, want nil", err)
	}
	if result.Created {
		t.Fatalf("MaterializePlanet(same id) Created = true, want false")
	}
	if result.Planet.Level != first.Level || !result.Planet.DiscoveredAt.Equal(first.DiscoveredAt) {
		t.Fatalf("same-id materialization mutated planet = %+v, want original %+v", result.Planet, first)
	}

	if _, ok, err := store.PlanetByCandidateKey("candidate-b"); err != nil || ok {
		t.Fatalf("PlanetByCandidateKey(candidate-b) = ok %v err %v, want ok false nil", ok, err)
	}
}

func TestUpsertPlayerPlanetIntelPreservesFresherReceiverIntel(t *testing.T) {
	store := NewInMemoryStore()
	fresh := testIntel("player-receiver", "planet-1", testTime(10), IntelStateVerified, 100, "scan-new")

	if _, updated, err := store.UpsertPlayerPlanetIntel(fresh); err != nil || !updated {
		t.Fatalf("UpsertPlayerPlanetIntel(fresh) updated = %v err = %v, want true nil", updated, err)
	}

	staleShare := testIntel("player-receiver", "planet-1", testTime(1), IntelStateStale, 40, "share-old")
	staleShare.SourceType = IntelSourceShareReceived
	got, updated, err := store.UpsertPlayerPlanetIntel(staleShare)
	if err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(stale share) error = %v, want nil", err)
	}
	if updated {
		t.Fatalf("UpsertPlayerPlanetIntel(stale share) updated = true, want false")
	}
	if got.SourceReference != fresh.SourceReference || got.State != fresh.State || !got.LastSeenAt.Equal(fresh.LastSeenAt) {
		t.Fatalf("stored intel = %+v, want preserved fresh %+v", got, fresh)
	}
}

func TestUpsertPlayerPlanetIntelReplacesOlderIntel(t *testing.T) {
	store := NewInMemoryStore()
	oldIntel := testIntel("player-1", "planet-1", testTime(1), IntelStateStale, 40, "share-old")
	oldIntel.SourceType = IntelSourceShareReceived
	if _, updated, err := store.UpsertPlayerPlanetIntel(oldIntel); err != nil || !updated {
		t.Fatalf("UpsertPlayerPlanetIntel(old) updated = %v err = %v, want true nil", updated, err)
	}

	newIntel := testIntel("player-1", "planet-1", testTime(2), IntelStateFresh, 90, "scan-new")
	got, updated, err := store.UpsertPlayerPlanetIntel(newIntel)
	if err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(new) error = %v, want nil", err)
	}
	if !updated {
		t.Fatalf("UpsertPlayerPlanetIntel(new) updated = false, want true")
	}
	if got.SourceReference != newIntel.SourceReference || got.State != newIntel.State {
		t.Fatalf("stored intel = %+v, want new %+v", got, newIntel)
	}
}

func TestRecordPlanetOwnerChangeMarksOlderIntelStale(t *testing.T) {
	store := NewInMemoryStore()
	planetID := foundation.PlanetID("planet-1")
	if _, err := store.MaterializePlanet(MaterializePlanetInput{
		CandidateKey: "candidate-a",
		Planet:       testPlanet(planetID, testTime(0)),
	}); err != nil {
		t.Fatalf("MaterializePlanet() error = %v, want nil", err)
	}

	older := testIntel("player-scout", planetID, testTime(1), IntelStateVerified, 100, "scan-old")
	recent := testIntel("player-recent", planetID, testTime(20), IntelStateVerified, 90, "scan-recent")
	if _, _, err := store.UpsertPlayerPlanetIntel(older); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(older) error = %v, want nil", err)
	}
	if _, _, err := store.UpsertPlayerPlanetIntel(recent); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(recent) error = %v, want nil", err)
	}

	result, err := store.RecordPlanetOwnerChange(PlanetOwnerChangeInput{
		PlanetID:         planetID,
		NewOwnerPlayerID: "player-owner",
		ChangedAt:        testTime(10),
		SourceReference:  "planet.claimed:event-1",
	})
	if err != nil {
		t.Fatalf("RecordPlanetOwnerChange() error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatalf("RecordPlanetOwnerChange() Changed = false, want true")
	}
	if result.Planet.OwnerPlayerID != "player-owner" || result.Planet.OwnerChangedAt == nil || !result.Planet.OwnerChangedAt.Equal(testTime(10)) {
		t.Fatalf("owner state = %+v, want owner player-owner at %s", result.Planet, testTime(10))
	}
	if len(result.StaleIntel) != 1 {
		t.Fatalf("StaleIntel len = %d, want 1", len(result.StaleIntel))
	}
	stale := result.StaleIntel[0]
	if stale.PlayerID != "player-scout" || stale.State != IntelStateStale || stale.Confidence != staleIntelConfidence {
		t.Fatalf("stale intel = %+v, want player-scout stale confidence %d", stale, staleIntelConfidence)
	}
	if stale.SourceType != IntelSourcePlanetOwnerChanged || stale.SourceReference != "planet.claimed:event-1" {
		t.Fatalf("stale source = %s/%s, want owner-change event source", stale.SourceType, stale.SourceReference)
	}

	storedRecent, ok, err := store.PlayerPlanetIntel("player-recent", planetID)
	if err != nil || !ok {
		t.Fatalf("PlayerPlanetIntel(recent) ok = %v err = %v, want true nil", ok, err)
	}
	if storedRecent.State != IntelStateVerified || !storedRecent.LastSeenAt.Equal(recent.LastSeenAt) {
		t.Fatalf("recent intel = %+v, want preserved %+v", storedRecent, recent)
	}

	oldShare := testIntel("player-scout", planetID, testTime(5), IntelStateFresh, 100, "share-after-stale")
	oldShare.SourceType = IntelSourceShareReceived
	got, updated, err := store.UpsertPlayerPlanetIntel(oldShare)
	if err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(old share) error = %v, want nil", err)
	}
	if updated || got.State != IntelStateStale || !got.LastSeenAt.Equal(testTime(10)) {
		t.Fatalf("old share updated = %v intel = %+v, want preserved stale marker at %s", updated, got, testTime(10))
	}
}
