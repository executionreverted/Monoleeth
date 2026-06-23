package discovery

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestBeginPlanetClaimWithXCoreResultDurableBeginPlan(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, "player-old-scout", planet.ID, testTime(1))
	upsertClaimIntel(t, store, "player-fresh-scout", planet.ID, testTime(20))
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	input := beginClaimWithXCoreInputForTest(t, planet.ID, reference)
	consumer := &recordingClaimXCoreConsumer{
		result: claimXCoreConsumeResultForTest(t, input.XCore, false),
	}

	result, err := composedClaimXCoreOwnerBoundary{
		Consumer:   consumer,
		Boundaries: store,
	}.BeginPlanetClaimWithXCore(input)
	if err != nil {
		t.Fatalf("BeginPlanetClaimWithXCore() error = %v, want nil", err)
	}

	plan, err := result.DurableBeginPlan()
	if err != nil {
		t.Fatalf("DurableBeginPlan() error = %v, want nil", err)
	}
	if plan.XCoreConsumption.ClaimReference != reference ||
		!plan.HasXCoreStorageMutation ||
		plan.XCoreStorageMutation.Result.StorageMutation.LedgerEntries[0].ReferenceKey != plan.XCoreConsumption.ReferenceKey ||
		plan.Boundary.Status != ClaimBoundaryStatusPendingSideEffects ||
		plan.Planet.OwnerPlayerID != claimTestPlayerID ||
		len(plan.StaleIntel) != 1 ||
		plan.StaleIntel[0].PlayerID != "player-old-scout" {
		t.Fatalf("durable begin plan = %+v, want matching xcore owner-CAS evidence", plan)
	}
}

func TestClaimDurableBeginPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewClaimDurableBeginPlan(nil, nil, nil, nil); err != nil || plan.Boundary.ClaimReference != "" {
		t.Fatalf("NewClaimDurableBeginPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin-invalid")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	result := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	storagePlan := claimXCoreStoragePlanForDurableBeginTest(t, result, &result.Boundary.Boundary)

	if _, err := NewClaimDurableBeginPlan(nil, nil, &result.Boundary.Boundary, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(partial) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	completedBoundary := result.Boundary.Boundary
	completedBoundary.Status = ClaimBoundaryStatusComplete
	completedBoundary.CompletedAt = testTime(12)
	if _, err := NewClaimDurableBeginPlan(&storagePlan, &result.Boundary.Planet, &completedBoundary, result.Boundary.StaleIntel); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(completed boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongStorage := storagePlan
	wrongStorage.Consumption.ReferenceKey = "planet_claim:player-other:planet-other"
	if _, err := NewClaimDurableBeginPlan(&wrongStorage, &result.Boundary.Planet, &result.Boundary.Boundary, result.Boundary.StaleIntel); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(wrong storage) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongPlanet := result.Boundary.Planet
	wrongPlanet.OwnerPlayerID = "player-other"
	if _, err := NewClaimDurableBeginPlan(&storagePlan, &wrongPlanet, &result.Boundary.Boundary, result.Boundary.StaleIntel); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(wrong planet) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestClaimDurableBeginPlanValidatesStaleIntelEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin-stale")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, "player-old-scout", planet.ID, testTime(1))
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	result := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	storagePlan := claimXCoreStoragePlanForDurableBeginTest(t, result, &result.Boundary.Boundary)

	plan, err := result.DurableBeginPlan()
	if err != nil {
		t.Fatalf("DurableBeginPlan(stale intel) error = %v, want nil", err)
	}
	if len(plan.StaleIntel) != 1 || plan.StaleIntel[0].State != IntelStateStale {
		t.Fatalf("stale intel evidence = %+v, want one stale row", plan.StaleIntel)
	}

	wrongRows := cloneClaimBoundaryStaleIntel(result.Boundary.StaleIntel)
	wrongRows[0].SourceReference = "planet.claimed:event-other"
	if _, err := NewClaimDurableBeginPlan(&storagePlan, &result.Boundary.Planet, &result.Boundary.Boundary, wrongRows); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(wrong stale intel) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	if _, err := NewClaimDurableBeginPlan(&storagePlan, &result.Boundary.Planet, &result.Boundary.Boundary, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(missing stale intel) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestClaimDurableBeginPlanAllowsDebitOnlyBeginFailureRecovery(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin-failure")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	beginErr := errors.New("owner cas failed")
	input := beginClaimWithXCoreInputForTest(t, planet.ID, reference)

	result, err := composedClaimXCoreOwnerBoundary{
		Consumer: &recordingClaimXCoreConsumer{
			result: claimXCoreConsumeResultForTest(t, input.XCore, false),
		},
		Boundaries: &recordingClaimBoundaryStore{
			inner:    store,
			beginErr: beginErr,
		},
	}.BeginPlanetClaimWithXCore(input)
	if !errors.Is(err, beginErr) {
		t.Fatalf("BeginPlanetClaimWithXCore() error = %v, want begin failure", err)
	}
	if result.XCoreConsumption.ClaimReference == "" {
		t.Fatal("BeginPlanetClaimWithXCore() xcore evidence empty, want debit evidence for retry")
	}
	plan, err := result.DurableBeginPlan()
	if err != nil {
		t.Fatalf("DurableBeginPlan(debit only) error = %v, want nil", err)
	}
	if plan.XCoreConsumption.ClaimReference != reference ||
		!plan.HasXCoreStorageMutation ||
		plan.XCoreStorageMutation.HasBoundary ||
		plan.Boundary.ClaimReference != "" {
		t.Fatalf("debit-only durable begin plan = %+v, want xcore/storage recovery evidence only", plan)
	}

	wrongKey := result.XCoreConsumption
	wrongKey.ReferenceKey = "planet_claim:player-other:planet-other"
	wrongStorage := plan.XCoreStorageMutation
	wrongStorage.Consumption = wrongKey
	if _, err := NewClaimDurableBeginPlan(&wrongStorage, nil, nil, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(wrong debit-only storage key) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestClaimDurableBeginPlanWithStorageRejectsPartialOrMismatchedEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin-storage-invalid")
	materializeClaimTestPlanet(t, store, planet)
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	input := beginClaimWithXCoreInputForTest(t, planet.ID, reference)
	result := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	storageResult := claimXCoreConsumeResultForTest(t, input.XCore, false)
	storagePlan := claimXCoreStoragePlanForDurableBeginTest(t, result, &result.Boundary.Boundary)

	storagePlanWithoutBoundary := claimXCoreStoragePlanForDurableBeginTest(t, result, nil)
	if _, err := NewClaimDurableBeginPlan(
		&storagePlanWithoutBoundary,
		&result.Boundary.Planet,
		&result.Boundary.Boundary,
		result.Boundary.StaleIntel,
	); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(storage without boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongInput := input.XCore
	wrongInput.PlanetID = "planet-other"
	wrongStorage, err := NewClaimXCoreStorageMutationPlan(wrongInput, storageResult, result.XCoreConsumption, nil)
	if err == nil {
		t.Fatalf("NewClaimXCoreStorageMutationPlan(wrong input) error = nil, want ErrInvalidClaimDurableCommit")
	}
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimXCoreStorageMutationPlan(wrong input) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	wrongStorage = storagePlan
	wrongStorage.Boundary.EventID = "event_other"
	if _, err := NewClaimDurableBeginPlan(
		&wrongStorage,
		&result.Boundary.Planet,
		&result.Boundary.Boundary,
		result.Boundary.StaleIntel,
	); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableBeginPlan(wrong storage boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestClaimDurableBeginPlanDuplicateReplaysOriginalBoundary(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-begin-duplicate")
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, "player-old-scout", planet.ID, testTime(1))
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	input := beginClaimWithXCoreInputForTest(t, planet.ID, reference)
	consumer := &recordingClaimXCoreConsumer{
		result: claimXCoreConsumeResultForTest(t, input.XCore, false),
	}
	boundary := composedClaimXCoreOwnerBoundary{
		Consumer:   consumer,
		Boundaries: store,
	}

	first, err := boundary.BeginPlanetClaimWithXCore(input)
	if err != nil {
		t.Fatalf("first BeginPlanetClaimWithXCore() error = %v, want nil", err)
	}
	firstPlan, err := first.DurableBeginPlan()
	if err != nil {
		t.Fatalf("first DurableBeginPlan() error = %v, want nil", err)
	}

	lateInput := beginClaimWithXCoreInputForTest(t, planet.ID, reference)
	lateInput.Boundary.ClaimedAt = testTime(30)
	lateInput.Boundary.EventID = "event_xcore_owner_boundary_late"
	lateInput.Boundary.SourceReference = "planet.claimed:event_xcore_owner_boundary_late"
	lateInput.ConsumedAt = testTime(31)
	duplicate, err := boundary.BeginPlanetClaimWithXCore(lateInput)
	if err != nil {
		t.Fatalf("duplicate BeginPlanetClaimWithXCore() error = %v, want nil", err)
	}
	duplicatePlan, err := duplicate.DurableBeginPlan()
	if err != nil {
		t.Fatalf("duplicate DurableBeginPlan() error = %v, want nil", err)
	}
	if !duplicate.Boundary.Duplicate ||
		duplicatePlan.Boundary.EventID != firstPlan.Boundary.EventID ||
		!duplicatePlan.Boundary.ClaimedAt.Equal(firstPlan.Boundary.ClaimedAt) ||
		!duplicatePlan.Planet.OwnerChangedAt.Equal(*firstPlan.Planet.OwnerChangedAt) ||
		len(duplicatePlan.StaleIntel) != len(firstPlan.StaleIntel) ||
		len(duplicatePlan.StaleIntel) != 1 {
		t.Fatalf("duplicate durable begin plan = %+v, want original boundary facts %+v", duplicatePlan, firstPlan)
	}
}

func beginClaimWithXCoreForDurableBeginTest(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	reference PlanetClaimReference,
) BeginPlanetClaimWithXCoreResult {
	t.Helper()
	result, err := composedClaimXCoreOwnerBoundary{
		Consumer: &recordingClaimXCoreConsumer{
			result: claimXCoreConsumeResultForTest(t, beginClaimWithXCoreInputForTest(t, planetID, reference).XCore, false),
		},
		Boundaries: store,
	}.BeginPlanetClaimWithXCore(beginClaimWithXCoreInputForTest(t, planetID, reference))
	if err != nil {
		t.Fatalf("BeginPlanetClaimWithXCore() error = %v, want nil", err)
	}
	return result
}

func claimXCoreStoragePlanForDurableBeginTest(
	t *testing.T,
	result BeginPlanetClaimWithXCoreResult,
	boundary *ClaimBoundaryRecord,
) ClaimXCoreStorageMutationPlan {
	t.Helper()
	plan, err := NewClaimXCoreStorageMutationPlan(
		result.XCoreInput,
		result.XCoreResult,
		result.XCoreConsumption,
		boundary,
	)
	if err != nil {
		t.Fatalf("NewClaimXCoreStorageMutationPlan() error = %v, want nil", err)
	}
	return plan
}
