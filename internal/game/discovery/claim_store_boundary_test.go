package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestBeginPlanetClaimBoundaryCommitsOwnerCASAndPendingBoundary(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-boundary-claim")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, "player-old-scout", planet.ID, testTime(1))
	upsertClaimIntel(t, store, "player-fresh-scout", planet.ID, testTime(20))
	claimedAt := testTime(10)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)

	result, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       claimedAt,
		EventID:         "event_boundary_claim",
		SourceReference: "planet.claimed:event_boundary_claim",
	})
	if err != nil {
		t.Fatalf("BeginPlanetClaimBoundary() error = %v, want nil", err)
	}
	if result.Duplicate {
		t.Fatalf("BeginPlanetClaimBoundary() duplicate = true, want false")
	}
	if result.Planet.OwnerPlayerID != claimTestPlayerID || result.Planet.OwnerChangedAt == nil || !result.Planet.OwnerChangedAt.Equal(claimedAt) {
		t.Fatalf("boundary planet owner = %+v, want %q at %s", result.Planet, claimTestPlayerID, claimedAt)
	}
	if len(result.StaleIntel) != 1 || result.StaleIntel[0].PlayerID != "player-old-scout" || result.StaleIntel[0].State != IntelStateStale {
		t.Fatalf("stale intel = %+v, want only older scout stale", result.StaleIntel)
	}
	wantReferenceKey, err := foundation.PlanetClaimIdempotencyKey(claimTestPlayerID, planet.ID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey: %v", err)
	}
	if result.Boundary.Status != ClaimBoundaryStatusPendingSideEffects ||
		result.Boundary.ReferenceKey != wantReferenceKey ||
		result.Boundary.StaleIntelCount != 1 ||
		result.Boundary.StaleListingCount != 0 ||
		!result.Boundary.CompletedAt.IsZero() {
		t.Fatalf("boundary = %+v, want pending with claim evidence", result.Boundary)
	}

	storedPlanet, ok, err := store.Planet(planet.ID)
	if err != nil || !ok {
		t.Fatalf("Planet() ok = %v err = %v, want true nil", ok, err)
	}
	if storedPlanet.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("stored planet owner = %q, want %q", storedPlanet.OwnerPlayerID, claimTestPlayerID)
	}
	boundary, ok, err := store.ClaimBoundary(reference)
	if err != nil || !ok {
		t.Fatalf("ClaimBoundary() ok = %v err = %v, want true nil", ok, err)
	}
	if boundary.Status != ClaimBoundaryStatusPendingSideEffects || boundary.EventID != "event_boundary_claim" {
		t.Fatalf("stored boundary = %+v, want pending event evidence", boundary)
	}
}

func TestClaimBoundaryBeginDuplicateReplaysWithoutNewMutationAndConflictsOnReferenceMismatch(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-boundary-duplicate")
	otherPlanet := claimTestPlanet("planet-boundary-other")
	materializeClaimTestPlanet(t, store, planet)
	materializeClaimTestPlanet(t, store, otherPlanet)
	reference := PlanetClaimReference("claim_boundary_duplicate")

	first, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       testTime(10),
		EventID:         "event_boundary_duplicate",
		SourceReference: "planet.claimed:event_boundary_duplicate",
	})
	if err != nil {
		t.Fatalf("first BeginPlanetClaimBoundary() error = %v, want nil", err)
	}

	duplicate, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       testTime(20),
		EventID:         "event_boundary_duplicate_late",
		SourceReference: "planet.claimed:event_boundary_duplicate_late",
	})
	if err != nil {
		t.Fatalf("duplicate BeginPlanetClaimBoundary() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Boundary.ClaimedAt.Equal(first.Boundary.ClaimedAt) || duplicate.Boundary.EventID != first.Boundary.EventID {
		t.Fatalf("duplicate result = %+v, want replay of first %+v", duplicate, first)
	}
	if len(store.ClaimBoundaries()) != 1 {
		t.Fatalf("ClaimBoundaries() len = %d, want 1", len(store.ClaimBoundaries()))
	}

	_, err = store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        otherPlanet.ID,
		ClaimedAt:       testTime(30),
		EventID:         "event_boundary_conflict",
		SourceReference: "planet.claimed:event_boundary_conflict",
	})
	if !errors.Is(err, ErrPlanetClaimReferenceConflict) {
		t.Fatalf("conflicting BeginPlanetClaimBoundary() error = %v, want ErrPlanetClaimReferenceConflict", err)
	}
}

func TestCompletePlanetClaimBoundaryMarksSideEffectsCompleteAndReplaysDuplicate(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-boundary-complete")
	materializeClaimTestPlanet(t, store, planet)
	reference := PlanetClaimReference("claim_boundary_complete")
	if _, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       testTime(10),
		EventID:         "event_boundary_complete",
		SourceReference: "planet.claimed:event_boundary_complete",
	}); err != nil {
		t.Fatalf("BeginPlanetClaimBoundary() error = %v, want nil", err)
	}

	completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(11),
		StaleListingCount: 3,
	})
	if err != nil {
		t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	if completed.Duplicate || completed.Boundary.Status != ClaimBoundaryStatusComplete || !completed.Boundary.CompletedAt.Equal(testTime(11)) || completed.Boundary.StaleListingCount != 3 {
		t.Fatalf("completed boundary = %+v, want complete at %s with listing count", completed, testTime(11))
	}

	duplicate, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(20),
		StaleListingCount: 99,
	})
	if err != nil {
		t.Fatalf("duplicate CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || !duplicate.Boundary.CompletedAt.Equal(testTime(11)) || duplicate.Boundary.StaleListingCount != 3 {
		t.Fatalf("duplicate complete = %+v, want replay of first completion", duplicate)
	}
}

func TestClaimBoundaryReadAPIsReturnDetachedSortedRecordsAndValidateLookup(t *testing.T) {
	store := NewInMemoryStore()
	planetB := claimTestPlanet("planet-boundary-b")
	planetA := claimTestPlanet("planet-boundary-a")
	materializeClaimTestPlanet(t, store, planetB)
	materializeClaimTestPlanet(t, store, planetA)

	beginClaimBoundaryForTest(t, store, "claim_boundary_b", planetB.ID, "event_boundary_b", testTime(10))
	beginClaimBoundaryForTest(t, store, "claim_boundary_a", planetA.ID, "event_boundary_a", testTime(11))

	records := store.ClaimBoundaries()
	if len(records) != 2 {
		t.Fatalf("ClaimBoundaries() len = %d, want 2", len(records))
	}
	if records[0].ClaimReference != "claim_boundary_a" || records[1].ClaimReference != "claim_boundary_b" {
		t.Fatalf("ClaimBoundaries() order = %+v, want sorted by reference", records)
	}
	records[0].Status = ClaimBoundaryStatusComplete
	stored, ok, err := store.ClaimBoundary("claim_boundary_a")
	if err != nil || !ok {
		t.Fatalf("ClaimBoundary(claim_boundary_a) ok = %v err = %v, want true nil", ok, err)
	}
	if stored.Status != ClaimBoundaryStatusPendingSideEffects {
		t.Fatalf("stored boundary after returned mutation = %+v, want pending", stored)
	}
	if _, ok, err := store.ClaimBoundary(""); err == nil || ok {
		t.Fatalf("ClaimBoundary(invalid) ok = %v err = %v, want false error", ok, err)
	}
}

func TestCompletePlanetClaimBoundaryRejectsMissingAndConflictingReferences(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-boundary-missing")
	materializeClaimTestPlanet(t, store, planet)

	_, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    "claim_boundary_missing",
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(10),
		StaleListingCount: 0,
	})
	if !errors.Is(err, ErrClaimBoundaryNotFound) {
		t.Fatalf("CompletePlanetClaimBoundary(missing) error = %v, want ErrClaimBoundaryNotFound", err)
	}

	beginClaimBoundaryForTest(t, store, "claim_boundary_conflict", planet.ID, "event_boundary_conflict", testTime(10))
	_, err = store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    "claim_boundary_conflict",
		PlayerID:          "player-other",
		PlanetID:          planet.ID,
		CompletedAt:       testTime(11),
		StaleListingCount: 0,
	})
	if !errors.Is(err, ErrPlanetClaimReferenceConflict) {
		t.Fatalf("CompletePlanetClaimBoundary(conflict) error = %v, want ErrPlanetClaimReferenceConflict", err)
	}
}

func canonicalClaimReference(t *testing.T, playerID foundation.PlayerID, planetID foundation.PlanetID) PlanetClaimReference {
	t.Helper()
	key, err := foundation.PlanetClaimIdempotencyKey(playerID, planetID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey: %v", err)
	}
	return PlanetClaimReference(key.String())
}

func beginClaimBoundaryForTest(
	t *testing.T,
	store *InMemoryStore,
	ref PlanetClaimReference,
	planetID foundation.PlanetID,
	eventID foundation.EventID,
	claimedAt time.Time,
) {
	t.Helper()
	if _, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  ref,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planetID,
		ClaimedAt:       claimedAt,
		EventID:         eventID,
		SourceReference: "planet.claimed:" + eventID.String(),
	}); err != nil {
		t.Fatalf("BeginPlanetClaimBoundary(%q) error = %v, want nil", ref, err)
	}
}
