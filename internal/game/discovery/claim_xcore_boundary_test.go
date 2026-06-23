package discovery

import (
	"errors"
	"testing"
)

func TestClaimPlanetBeginBoundaryRetryUsesRecordedXCoreConsumption(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-xcore-begin-retry")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	beginErr := errors.New("claim boundary begin unavailable")
	boundaries := &recordingClaimBoundaryStore{
		inner:    store,
		beginErr: beginErr,
	}
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimBoundaryAdapterTestService(t, store, boundaries, consumer)
	input := claimInput("claim_xcore_begin_retry", planet.ID)

	_, err := service.ClaimPlanet(input)
	if !errors.Is(err, beginErr) {
		t.Fatalf("ClaimPlanet() error = %v, want begin error", err)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after begin error = %d, want 1", got)
	}
	if records := service.ClaimXCoreConsumptions(); len(records) != 1 || records[0].ClaimReference != input.ClaimReference {
		t.Fatalf("ClaimXCoreConsumptions() = %+v, want one record for claim", records)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after begin error = %d, want 0", got)
	}

	boundaries.beginErr = nil
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
	if records := service.ClaimXCoreConsumptions(); len(records) != 1 || records[0].PlayerID != input.PlayerID || records[0].PlanetID != input.PlanetID {
		t.Fatalf("ClaimXCoreConsumptions() after retry = %+v, want stable one record", records)
	}
	if got := len(service.Events()); got != 1 {
		t.Fatalf("claim events after retry = %d, want 1", got)
	}
	assertClaimBoundaryStatus(t, store, input.ClaimReference, planet.ID, ClaimBoundaryStatusComplete, 0)
}

func TestClaimPlanetRecordedXCoreReferenceConflictRejectsBeforeSecondConsume(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-xcore-conflict")
	otherPlanet := claimTestPlanet("planet-claim-xcore-conflict-other")
	materializeClaimTestPlanet(t, store, planet)
	materializeClaimTestPlanet(t, store, otherPlanet)
	upsertClaimIntel(t, store, claimTestPlayerID, planet.ID, testTime(1))
	upsertClaimIntel(t, store, claimTestPlayerID, otherPlanet.ID, testTime(1))
	beginErr := errors.New("claim boundary begin unavailable")
	boundaries := &recordingClaimBoundaryStore{
		inner:    store,
		beginErr: beginErr,
	}
	consumer := &recordingClaimXCoreConsumer{}
	service := newClaimBoundaryAdapterTestService(t, store, boundaries, consumer)

	_, err := service.ClaimPlanet(claimInput("claim_xcore_conflict", planet.ID))
	if !errors.Is(err, beginErr) {
		t.Fatalf("ClaimPlanet() error = %v, want begin error", err)
	}
	_, err = service.ClaimPlanet(claimInput("claim_xcore_conflict", otherPlanet.ID))
	if !errors.Is(err, ErrPlanetClaimReferenceConflict) {
		t.Fatalf("conflicting ClaimPlanet() error = %v, want ErrPlanetClaimReferenceConflict", err)
	}
	if got := len(consumer.calls); got != 1 {
		t.Fatalf("x core consume calls after conflict = %d, want 1", got)
	}
	if got := len(service.Events()); got != 0 {
		t.Fatalf("claim events after conflict = %d, want 0", got)
	}
}
