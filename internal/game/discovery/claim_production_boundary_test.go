package discovery

import (
	"errors"
	"testing"
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
