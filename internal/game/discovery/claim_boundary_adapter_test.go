package discovery

import (
	"errors"
	"testing"
)

func TestClaimPlanetBoundaryStoreReadErrorStopsBeforeXCoreConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-boundary-read-error")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	boundaryErr := errors.New("claim boundary row lock failed")
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimBoundaryAdapterTestService(t, store, &recordingClaimBoundaryStore{
		inner:            store,
		claimBoundaryErr: boundaryErr,
	}, consumer)

	_, err := service.ClaimPlanet(claimInput("claim_boundary_read_error", planet.ID))
	if !errors.Is(err, boundaryErr) {
		t.Fatalf("ClaimPlanet() error = %v, want boundary read error", err)
	}
	if got := len(consumer.calls); got != 0 {
		t.Fatalf("x core consume calls after boundary read error = %d, want 0", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after boundary read error = %d, want 0", got)
	}
	if got := len(service.ClaimOutboxRecords()); got != 0 {
		t.Fatalf("claim outbox after boundary read error = %d, want 0", got)
	}
}

func TestClaimPlanetBoundaryStoreCompleteErrorCanRetryWithoutSecondXCoreConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-boundary-complete-error")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	boundaries := &recordingClaimBoundaryStore{
		inner:       store,
		completeErr: ErrClaimBoundaryNotFound,
	}
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimBoundaryAdapterTestService(t, store, boundaries, consumer)
	input := claimInput("claim_boundary_complete_error", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, ErrClaimBoundaryNotFound) {
		t.Fatalf("ClaimPlanet() error = %v, want missing boundary error", err)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after complete error = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after complete error = %d, want 0", got)
	}
	if got := len(service.ClaimOutboxRecords()); got != 0 {
		t.Fatalf("claim outbox after complete error = %d, want 0", got)
	}
	assertClaimBoundaryStatus(t, store, input.ClaimReference, planet.ID, ClaimBoundaryStatusPendingSideEffects, 0)

	boundaries.completeErr = nil
	retry, err := service.ClaimPlanet(input)
	if err != nil {
		t.Fatalf("retry ClaimPlanet() error = %v, want nil", err)
	}
	if !retry.Claimed || retry.AlreadyOwned || retry.Duplicate {
		t.Fatalf("retry result = %+v, want original claim completion", retry)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after retry = %d, want still 1", got)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("claim events after retry = %d, want 1", got)
	}
	if got := len(service.ClaimOutboxRecords()); got != 1 {
		t.Fatalf("claim outbox after retry = %d, want 1", got)
	}
	assertClaimBoundaryStatus(t, store, input.ClaimReference, planet.ID, ClaimBoundaryStatusComplete, 0)
}

func newClaimBoundaryAdapterTestService(
	t *testing.T,
	store *InMemoryStore,
	boundaries ClaimBoundaryStore,
	consumer *recordingClaimXCoreConsumer,
) *ClaimService {
	t.Helper()
	service, err := NewClaimService(ClaimServiceConfig{
		Store:               store,
		ClaimBoundaries:     boundaries,
		Clock:               fixedClaimClock{now: claimTestTime()},
		Ranks:               fixedClaimRankProvider{rank: 2},
		Proximity:           fixedClaimProximityProvider{inRange: true},
		XCoreConsumer:       consumer,
		XCoreItemDefinition: claimTestXCoreDefinition(t),
	})
	if err != nil {
		t.Fatalf("NewClaimService() error = %v, want nil", err)
	}
	return service
}

type recordingClaimBoundaryStore struct {
	inner            *InMemoryStore
	claimBoundaryErr error
	completeErr      error
}

func (store *recordingClaimBoundaryStore) BeginPlanetClaimBoundary(input BeginPlanetClaimBoundaryInput) (BeginPlanetClaimBoundaryResult, error) {
	return store.inner.BeginPlanetClaimBoundary(input)
}

func (store *recordingClaimBoundaryStore) CompletePlanetClaimBoundary(input CompletePlanetClaimBoundaryInput) (CompletePlanetClaimBoundaryResult, error) {
	if store.completeErr != nil {
		return CompletePlanetClaimBoundaryResult{}, store.completeErr
	}
	return store.inner.CompletePlanetClaimBoundary(input)
}

func (store *recordingClaimBoundaryStore) ClaimBoundary(ref PlanetClaimReference) (ClaimBoundaryRecord, bool, error) {
	if store.claimBoundaryErr != nil {
		return ClaimBoundaryRecord{}, false, store.claimBoundaryErr
	}
	return store.inner.ClaimBoundary(ref)
}

var _ ClaimBoundaryStore = (*recordingClaimBoundaryStore)(nil)
