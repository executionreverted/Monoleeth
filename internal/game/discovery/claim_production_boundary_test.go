package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestClaimPlanetStaleListingRetryUsesRecordedProductionInitialization(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-production-init-evidence")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	consumer := &recordingClaimXCoreConsumer{}
	initializer := &recordingClaimProductionInitializer{}
	markerErr := errors.New("stale listing marker unavailable")
	staleMarker := &recordingClaimListedIntelStaleMarker{markedCount: 2, err: markerErr}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    consumer,
		initializer: initializer,
		staleMarker: staleMarker,
	})
	input := claimInput("claim_production_init_evidence", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, markerErr) {
		t.Fatalf("ClaimPlanet() error = %v, want stale marker error", err)
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls after marker failure = %d, want 1", got)
	}
	records := service.ClaimProductionInitializations()
	if len(records) != 1 || records[0].ClaimReference != input.ClaimReference || !records[0].Created {
		t.Fatalf("ClaimProductionInitializations() = %+v, want one created record", records)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after marker failure = %d, want 0", got)
	}
	assertClaimBoundaryStatus(t, store, input.ClaimReference, planet.ID, ClaimBoundaryStatusPendingSideEffects, 0)

	staleMarker.err = nil
	retry, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("retry ClaimPlanet() error = %v, want nil", err)
	}
	if !retry.Claimed || retry.AlreadyOwned || retry.Duplicate || retry.StaleListingCount != 2 {
		t.Fatalf("retry result = %+v, want original claim completion with stale listings", retry)
	}
	if got := len(initializer.calls); got != 1 {
		t.Fatalf("production initializer calls after retry = %d, want still 1", got)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after retry = %d, want 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("claim events after retry = %d, want 1", got)
	}
	assertClaimBoundaryStatus(t, store, input.ClaimReference, planet.ID, ClaimBoundaryStatusComplete, 2)
}

func TestClaimPlanetProductionInitializationErrorDoesNotRecordEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-production-init-error-evidence")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	initErr := errors.New("production init unavailable")
	initializer := &recordingClaimProductionInitializer{err: initErr}
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    &recordingClaimXCoreConsumer{},
		initializer: initializer,
	})

	_, err := service.ClaimPlanet(claimInput("claim_production_init_error_evidence", planet.ID))
	if !errors.Is(err, initErr) {
		t.Fatalf("ClaimPlanet() error = %v, want init error", err)
	}
	if records := service.ClaimProductionInitializations(); len(records) != 0 {
		t.Fatalf("ClaimProductionInitializations() after init error = %+v, want none", records)
	}
}

func TestClaimProductionInitializationRecordDurablePlan(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-production-init-durable")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	markerErr := errors.New("stale listing marker unavailable")
	service := newClaimTestService(t, claimTestServiceOptions{
		store:       store,
		rank:        planet.Level,
		inRange:     true,
		consumer:    &recordingClaimXCoreConsumer{},
		initializer: &recordingClaimProductionInitializer{},
		staleMarker: &recordingClaimListedIntelStaleMarker{err: markerErr},
	})
	input := claimInput(canonicalClaimReference(t, claimTestPlayerID, planet.ID), planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, markerErr) {
		t.Fatalf("ClaimPlanet() error = %v, want stale marker error", err)
	}
	records := service.ClaimProductionInitializations()
	if len(records) != 1 {
		t.Fatalf("ClaimProductionInitializations() = %+v, want one record", records)
	}
	boundary, ok, err := store.ClaimBoundary(input.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("ClaimBoundary() ok = %v err = %v, want true nil", ok, err)
	}

	plan, err := records[0].DurablePlan(&boundary)
	if err != nil {
		t.Fatalf("DurablePlan(pending boundary) error = %v, want nil", err)
	}
	if plan.Initialization.ClaimReference != input.ClaimReference ||
		plan.Initialization.PlanetLevel != planet.Level ||
		plan.Boundary.Status != ClaimBoundaryStatusPendingSideEffects {
		t.Fatalf("production init durable plan = %+v, want pending claim-production evidence", plan)
	}

	completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    input.ClaimReference,
		PlayerID:          input.PlayerID,
		PlanetID:          input.PlanetID,
		CompletedAt:       testTime(20),
		StaleListingCount: 1,
	})
	if err != nil {
		t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	if _, err := records[0].DurablePlan(&completed.Boundary); err != nil {
		t.Fatalf("DurablePlan(complete boundary) error = %v, want nil", err)
	}
}

func TestClaimProductionInitializationDurablePlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewClaimProductionInitializationDurablePlan(nil, nil); err != nil || plan.Initialization.ClaimReference != "" {
		t.Fatalf("NewClaimProductionInitializationDurablePlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-production-init-invalid")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	begin := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	record := claimProductionInitializationRecordForTest(t, reference, planet.ID, planet.Level, begin.Boundary.Boundary.ClaimedAt)

	if _, err := NewClaimProductionInitializationDurablePlan(nil, &begin.Boundary.Boundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimProductionInitializationDurablePlan(boundary only) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongKey := record
	wrongKey.ReferenceKey = "planet_claim:player-other:planet-other"
	if _, err := wrongKey.DurablePlan(&begin.Boundary.Boundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(wrong key) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	noResult := record
	noResult.Created = false
	noResult.AlreadyInitialized = false
	if _, err := noResult.DurablePlan(&begin.Boundary.Boundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(no result) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	doubleResult := record
	doubleResult.AlreadyInitialized = true
	if _, err := doubleResult.DurablePlan(&begin.Boundary.Boundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(double result) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	timeTravel := record
	timeTravel.InitializedAt = timeTravel.ClaimedAt.Add(-time.Second)
	if _, err := timeTravel.DurablePlan(&begin.Boundary.Boundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(time travel) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongBoundary := begin.Boundary.Boundary
	wrongBoundary.PlanetID = "planet-claim-production-init-other"
	if _, err := record.DurablePlan(&wrongBoundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(wrong boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	invalidPendingBoundary := begin.Boundary.Boundary
	invalidPendingBoundary.CompletedAt = testTime(40)
	if _, err := record.DurablePlan(&invalidPendingBoundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(invalid pending boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	completeBoundary := begin.Boundary.Boundary
	completeBoundary.Status = ClaimBoundaryStatusComplete
	completeBoundary.CompletedAt = time.Time{}
	if _, err := record.DurablePlan(&completeBoundary); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("DurablePlan(invalid complete boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func claimProductionInitializationRecordForTest(
	t *testing.T,
	reference PlanetClaimReference,
	planetID foundation.PlanetID,
	planetLevel int,
	claimedAt time.Time,
) ClaimProductionInitializationRecord {
	t.Helper()
	key, ok := reference.IdempotencyKey(claimTestPlayerID, planetID)
	if !ok {
		t.Fatalf("IdempotencyKey(%q) ok = false, want true", reference)
	}
	return ClaimProductionInitializationRecord{
		ClaimReference: reference,
		ReferenceKey:   key,
		PlayerID:       claimTestPlayerID,
		PlanetID:       planetID,
		PlanetLevel:    planetLevel,
		ClaimedAt:      claimedAt,
		InitializedAt:  testTime(30),
		Created:        true,
	}
}
