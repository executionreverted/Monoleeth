package discovery

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestCompletePlanetClaimBoundaryResultDurableCommitPlan(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-commit")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)

	if _, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planet.ID,
		ClaimedAt:       testTime(10),
		EventID:         "event_claim_durable_commit",
		SourceReference: "planet.claimed:event_claim_durable_commit",
	}); err != nil {
		t.Fatalf("BeginPlanetClaimBoundary() error = %v, want nil", err)
	}

	completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(11),
		StaleListingCount: 2,
	})
	if err != nil {
		t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
	}

	plan, err := completed.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan() error = %v, want nil", err)
	}
	if plan.Boundary.ClaimReference != reference ||
		plan.Reference.ReferenceKey != plan.Boundary.ReferenceKey ||
		plan.Event.EventID != plan.Boundary.EventID ||
		plan.Outbox.Event.EventID != plan.Event.EventID ||
		plan.Outbox.Status != ClaimOutboxStatusPending {
		t.Fatalf("durable commit plan = %+v, want matching claim completion evidence", plan)
	}

	duplicate, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(12),
		StaleListingCount: 99,
	})
	if err != nil {
		t.Fatalf("duplicate CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	duplicatePlan, err := duplicate.DurableCommitPlan()
	if err != nil {
		t.Fatalf("duplicate DurableCommitPlan() error = %v, want nil", err)
	}
	if !duplicatePlan.Boundary.CompletedAt.Equal(plan.Boundary.CompletedAt) ||
		duplicatePlan.Outbox.OutboxID != plan.Outbox.OutboxID ||
		duplicatePlan.Reference != plan.Reference {
		t.Fatalf("duplicate durable commit plan = %+v, want replay of %+v", duplicatePlan, plan)
	}
}

func TestClaimDurableCommitPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewClaimDurableCommitPlan(nil, nil, nil, nil, nil); err != nil || plan.Boundary.ClaimReference != "" {
		t.Fatalf("NewClaimDurableCommitPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-invalid")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	completed := completeClaimBoundaryForDurableCommitTest(t, store, reference, planet.ID)

	if _, err := NewClaimDurableCommitPlan(nil, &completed.Reference, &completed.Event, &completed.Outbox, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(partial) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	mismatchedReference := completed.Reference
	mismatchedReference.ReferenceKey = "planet_claim:player-other:planet-other"
	if _, err := NewClaimDurableCommitPlan(&completed.Boundary, &mismatchedReference, &completed.Event, &completed.Outbox, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(mismatched reference) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	mismatchedEvent := completed.Event
	mismatchedEvent.EventID = "event_claim_durable_other"
	if _, err := NewClaimDurableCommitPlan(&completed.Boundary, &completed.Reference, &mismatchedEvent, &completed.Outbox, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(mismatched event) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	inFlightOutbox := completed.Outbox
	inFlightOutbox.Status = ClaimOutboxStatusInFlight
	inFlightOutbox.ClaimToken = "claim-outbox-1-attempt-1"
	if _, err := NewClaimDurableCommitPlan(&completed.Boundary, &completed.Reference, &completed.Event, &inFlightOutbox, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(in-flight outbox) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestClaimDurableCommitPlanValidatesXCoreEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-xcore")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	completed := completeClaimBoundaryForDurableCommitTest(t, store, reference, planet.ID)
	xcore := claimDurableCommitXCoreForTest(t, planet.ID, reference)

	plan, err := NewClaimDurableCommitPlan(&completed.Boundary, &completed.Reference, &completed.Event, &completed.Outbox, &xcore)
	if err != nil {
		t.Fatalf("NewClaimDurableCommitPlan(with xcore) error = %v, want nil", err)
	}
	if plan.XCoreConsumption.ClaimReference != reference || plan.XCoreConsumption.Quantity != defaultClaimXCoreQuantity {
		t.Fatalf("xcore durable commit evidence = %+v, want claim reference and quantity", plan.XCoreConsumption)
	}

	xcore.PlanetID = "planet-claim-durable-xcore-other"
	if _, err := NewClaimDurableCommitPlan(&completed.Boundary, &completed.Reference, &completed.Event, &completed.Outbox, &xcore); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(wrong xcore) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	xcore = claimDurableCommitXCoreForTest(t, planet.ID, reference)
	xcore.ReferenceKey = "planet_claim:player-other:planet-other"
	if _, err := NewClaimDurableCommitPlan(&completed.Boundary, &completed.Reference, &completed.Event, &completed.Outbox, &xcore); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableCommitPlan(wrong xcore key) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func completeClaimBoundaryForDurableCommitTest(
	t *testing.T,
	store *InMemoryStore,
	reference PlanetClaimReference,
	planetID foundation.PlanetID,
) CompletePlanetClaimBoundaryResult {
	t.Helper()
	if _, err := store.BeginPlanetClaimBoundary(BeginPlanetClaimBoundaryInput{
		ClaimReference:  reference,
		PlayerID:        claimTestPlayerID,
		PlanetID:        planetID,
		ClaimedAt:       testTime(20),
		EventID:         "event_claim_durable_invalid",
		SourceReference: "planet.claimed:event_claim_durable_invalid",
	}); err != nil {
		t.Fatalf("BeginPlanetClaimBoundary() error = %v, want nil", err)
	}
	completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planetID,
		CompletedAt:       testTime(21),
		StaleListingCount: 1,
	})
	if err != nil {
		t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	return completed
}

func claimDurableCommitXCoreForTest(t *testing.T, planetID foundation.PlanetID, reference PlanetClaimReference) ClaimXCoreConsumptionRecord {
	t.Helper()
	input := beginClaimWithXCoreInputForTest(t, planetID, reference)
	return newClaimXCoreConsumptionRecord(input.XCore, ClaimXCoreConsumeResult{}, input.ConsumedAt)
}
