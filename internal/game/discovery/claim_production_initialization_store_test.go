package discovery

import (
	"errors"
	"reflect"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestClaimProductionInitializationDurableStoreCommitsPendingBoundaryPlan(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "pending", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()

	result, err := store.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(pending) error = %v, want nil", err)
	}
	if result.Duplicate ||
		result.Plan.Initialization.ClaimReference != plan.Initialization.ClaimReference ||
		result.Plan.Boundary.Status != ClaimBoundaryStatusPendingSideEffects {
		t.Fatalf("production initialization result = %+v, want first pending commit", result)
	}
	if refs := store.ClaimReferences(); !reflect.DeepEqual(refs, []PlanetClaimReference{plan.Initialization.ClaimReference}) {
		t.Fatalf("ClaimReferences() = %+v, want committed pending reference", refs)
	}
}

func TestClaimProductionInitializationDurableStoreCommitsCompleteBoundaryPlan(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "complete", true)
	store := NewInMemoryClaimProductionInitializationDurableStore()

	result, err := store.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(complete) error = %v, want nil", err)
	}
	if result.Duplicate ||
		result.Plan.Initialization.ClaimReference != plan.Initialization.ClaimReference ||
		result.Plan.Boundary.Status != ClaimBoundaryStatusComplete {
		t.Fatalf("production initialization result = %+v, want first complete commit", result)
	}
}

func TestClaimProductionInitializationDurableStoreDuplicateReferenceReplaysWithoutDuplicateRows(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "duplicate", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()

	first, err := store.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("first ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	duplicate, err := store.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("duplicate ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	if first.Duplicate || !duplicate.Duplicate {
		t.Fatalf("duplicate flags first=%v duplicate=%v, want false/true", first.Duplicate, duplicate.Duplicate)
	}
	if len(store.ClaimReferences()) != 1 {
		t.Fatalf("ClaimReferences() len = %d, want no duplicate row", len(store.ClaimReferences()))
	}
	if !reflect.DeepEqual(first.Plan, duplicate.Plan) {
		t.Fatalf("duplicate plan = %+v, want original plan %+v", duplicate.Plan, first.Plan)
	}
}

func TestClaimProductionInitializationDurableStoreRejectsConflictingReferenceReuse(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "conflict", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()
	if _, err := store.ApplyClaimProductionInitializationDurablePlan(plan); err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.Initialization.PlanetLevel++
	_, err := store.ApplyClaimProductionInitializationDurablePlan(conflict)
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("conflicting ApplyClaimProductionInitializationDurablePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	if len(store.ClaimReferences()) != 1 {
		t.Fatalf("ClaimReferences() len after conflict = %d, want 1", len(store.ClaimReferences()))
	}
	recovered, ok, err := store.CommittedClaimProductionInitializationDurablePlan(plan.Initialization.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Initialization.PlanetLevel != plan.Initialization.PlanetLevel {
		t.Fatalf("recovered planet level = %d, want original %d", recovered.Initialization.PlanetLevel, plan.Initialization.PlanetLevel)
	}
}

func TestClaimProductionInitializationDurableStoreRejectsInvalidPlanWithoutMutation(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "invalid", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()
	if result, err := store.ApplyClaimProductionInitializationDurablePlan(ClaimProductionInitializationDurablePlan{}); err != nil || result.Plan.Initialization.ClaimReference != "" {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(no-op) = %+v/%v, want empty nil", result, err)
	}

	invalid := plan
	invalid.Initialization.Created = false
	invalid.Initialization.AlreadyInitialized = false
	_, err := store.ApplyClaimProductionInitializationDurablePlan(invalid)
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("invalid ApplyClaimProductionInitializationDurablePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	if len(store.ClaimReferences()) != 0 {
		t.Fatalf("ClaimReferences() len after invalid plan = %d, want 0", len(store.ClaimReferences()))
	}
}

func TestClaimProductionInitializationDurableStoreReadbackDetachedRows(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "detached", true)
	store := NewInMemoryClaimProductionInitializationDurableStore()
	result, err := store.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	result.Plan.Initialization.PlanetLevel = 999

	recovered, ok, err := store.CommittedClaimProductionInitializationDurablePlan(plan.Initialization.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() = ok %v err %v, want true nil", ok, err)
	}
	recovered.Initialization.PlanetLevel = 999
	recovered.Boundary.StaleListingCount = 99

	again, ok, err := store.CommittedClaimProductionInitializationDurablePlan(plan.Initialization.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if again.Initialization.PlanetLevel != plan.Initialization.PlanetLevel ||
		again.Boundary.StaleListingCount != plan.Boundary.StaleListingCount {
		t.Fatalf("recovered production initialization reused mutable rows: %+v", again)
	}
}

func TestClaimProductionInitializationDurableStoreReadbackMissingAndInvalidReferences(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "missing", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()

	if recovered, ok, err := store.CommittedClaimProductionInitializationDurablePlan(plan.Initialization.ClaimReference); err != nil || ok || recovered.Initialization.ClaimReference != "" {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if recovered, ok, err := store.CommittedClaimProductionInitializationDurablePlan(""); err == nil || ok || recovered.Initialization.ClaimReference != "" {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
}

func TestClaimProductionInitializationDurablePlanApplyDurableProductionInitialization(t *testing.T) {
	plan := claimProductionInitializationDurableStorePlanForTest(t, "apply", false)
	if result, err := plan.ApplyDurableProductionInitialization(nil); !errors.Is(err, ErrInvalidClaimDurableCommit) || result.Plan.Initialization.ClaimReference != "" {
		t.Fatalf("ApplyDurableProductionInitialization(nil store) = %+v/%v, want invalid durable commit", result, err)
	}
	store := NewInMemoryClaimProductionInitializationDurableStore()

	committed, err := plan.ApplyDurableProductionInitialization(store)
	if err != nil {
		t.Fatalf("ApplyDurableProductionInitialization() error = %v, want nil", err)
	}
	if committed.Duplicate || committed.Plan.Initialization.ClaimReference != plan.Initialization.ClaimReference {
		t.Fatalf("ApplyDurableProductionInitialization() result = %+v, want first commit", committed)
	}

	duplicate, err := plan.ApplyDurableProductionInitialization(store)
	if err != nil {
		t.Fatalf("duplicate ApplyDurableProductionInitialization() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || len(store.ClaimReferences()) != 1 {
		t.Fatalf("duplicate ApplyDurableProductionInitialization() = %+v refs %d, want duplicate without append", duplicate, len(store.ClaimReferences()))
	}
}

func TestClaimProductionInitializationDurableStoreClaimReferencesDeterministic(t *testing.T) {
	first := claimProductionInitializationDurableStorePlanForTest(t, "refs-a", false)
	second := claimProductionInitializationDurableStorePlanForTest(t, "refs-b", false)
	store := NewInMemoryClaimProductionInitializationDurableStore()
	for _, plan := range []ClaimProductionInitializationDurablePlan{second, first} {
		if _, err := store.ApplyClaimProductionInitializationDurablePlan(plan); err != nil {
			t.Fatalf("ApplyClaimProductionInitializationDurablePlan(%q) error = %v, want nil", plan.Initialization.ClaimReference, err)
		}
	}

	refs := store.ClaimReferences()
	want := []PlanetClaimReference{first.Initialization.ClaimReference, second.Initialization.ClaimReference}
	if want[1] < want[0] {
		want[0], want[1] = want[1], want[0]
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("ClaimReferences() = %+v, want sorted %+v", refs, want)
	}
}

func claimProductionInitializationDurableStorePlanForTest(
	t *testing.T,
	suffix string,
	complete bool,
) ClaimProductionInitializationDurablePlan {
	t.Helper()
	store := NewInMemoryStore()
	planet := claimTestPlanet(foundation.PlanetID("planet-claim-production-init-store-" + suffix))
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	begin := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	boundary := begin.Boundary.Boundary
	if complete {
		completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
			ClaimReference:    reference,
			PlayerID:          claimTestPlayerID,
			PlanetID:          planet.ID,
			CompletedAt:       testTime(40),
			StaleListingCount: 1,
		})
		if err != nil {
			t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
		}
		boundary = completed.Boundary
	}
	record := claimProductionInitializationRecordForTest(
		t,
		reference,
		planet.ID,
		planet.Level,
		begin.Boundary.Boundary.ClaimedAt,
	)
	plan, err := record.DurablePlan(&boundary)
	if err != nil {
		t.Fatalf("DurablePlan() error = %v, want nil", err)
	}
	return plan
}
