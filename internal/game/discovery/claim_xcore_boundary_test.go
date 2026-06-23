package discovery

import (
	"errors"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestBeginPlanetClaimWithXCoreReturnsDebitEvidenceAndOwnerBoundary(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-xcore-owner-boundary")
	materializeClaimTestPlanet(t, store, planet)
	ref := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	key, err := foundation.PlanetClaimIdempotencyKey(claimTestPlayerID, planet.ID)
	if err != nil {
		t.Fatalf("PlanetClaimIdempotencyKey() error = %v, want nil", err)
	}
	input := beginClaimWithXCoreInputForTest(t, planet.ID, ref)
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
	if len(consumer.calls) != 1 {
		t.Fatalf("x core consumer calls = %d, want 1", len(consumer.calls))
	}
	if result.XCoreConsumption.ClaimReference != ref ||
		result.XCoreConsumption.ReferenceKey != key ||
		result.XCoreConsumption.PlayerID != claimTestPlayerID ||
		result.XCoreConsumption.PlanetID != planet.ID ||
		!result.XCoreConsumption.ConsumedAt.Equal(input.ConsumedAt) {
		t.Fatalf("x core consumption evidence = %+v, want claim/player/planet/key evidence", result.XCoreConsumption)
	}
	plan, err := NewClaimXCoreStorageMutationPlan(input.XCore, result.XCoreResult, result.XCoreConsumption, &result.Boundary.Boundary)
	if err != nil {
		t.Fatalf("NewClaimXCoreStorageMutationPlan() error = %v, want nil", err)
	}
	if !plan.HasBoundary ||
		len(plan.Result.StorageMutation.LedgerEntries) != 1 ||
		plan.Result.StorageMutation.LedgerEntries[0].ReferenceKey != key {
		t.Fatalf("x core storage plan = %+v, want boundary and canonical ledger evidence", plan)
	}
	if result.Boundary.Boundary.Status != ClaimBoundaryStatusPendingSideEffects ||
		result.Boundary.Boundary.ClaimReference != ref ||
		result.Boundary.Planet.OwnerPlayerID != claimTestPlayerID {
		t.Fatalf("owner boundary result = %+v, want pending owner boundary", result.Boundary)
	}
	assertClaimBoundaryStatus(t, store, ref, planet.ID, ClaimBoundaryStatusPendingSideEffects, 0)
}

func TestBeginPlanetClaimWithXCoreBeginFailureReturnsDebitEvidence(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-xcore-owner-begin-failure")
	materializeClaimTestPlanet(t, store, planet)
	beginErr := errors.New("owner cas failed")
	ref := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	input := beginClaimWithXCoreInputForTest(t, planet.ID, ref)
	consumer := &recordingClaimXCoreConsumer{
		result: claimXCoreConsumeResultForTest(t, input.XCore, false),
	}

	result, err := composedClaimXCoreOwnerBoundary{
		Consumer: consumer,
		Boundaries: &recordingClaimBoundaryStore{
			inner:    store,
			beginErr: beginErr,
		},
	}.BeginPlanetClaimWithXCore(input)
	if !errors.Is(err, beginErr) {
		t.Fatalf("BeginPlanetClaimWithXCore() error = %v, want begin error", err)
	}
	if len(consumer.calls) != 1 {
		t.Fatalf("x core consumer calls = %d, want 1", len(consumer.calls))
	}
	if result.XCoreConsumption.ClaimReference != ref || result.XCoreConsumption.PlayerID != claimTestPlayerID || result.XCoreConsumption.PlanetID != planet.ID {
		t.Fatalf("x core evidence after begin failure = %+v, want populated claim evidence", result.XCoreConsumption)
	}
	plan, planErr := NewClaimXCoreStorageMutationPlan(input.XCore, result.XCoreResult, result.XCoreConsumption, nil)
	if planErr != nil {
		t.Fatalf("NewClaimXCoreStorageMutationPlan(debit-only) error = %v, want nil", planErr)
	}
	if plan.HasBoundary {
		t.Fatalf("x core debit-only storage plan HasBoundary = true, want false")
	}
	if _, ok, lookupErr := store.ClaimBoundary(ref); lookupErr != nil || ok {
		t.Fatalf("ClaimBoundary() after failed begin ok = %v err = %v, want false nil", ok, lookupErr)
	}
}

func TestClaimXCoreStorageMutationPlanRejectsInvalidRows(t *testing.T) {
	planetID := foundation.PlanetID("planet-claim-xcore-storage-invalid")
	ref := canonicalClaimReference(t, claimTestPlayerID, planetID)
	input := beginClaimWithXCoreInputForTest(t, planetID, ref)
	validResult := claimXCoreConsumeResultForTest(t, input.XCore, false)
	consumption := newClaimXCoreConsumptionRecord(input.XCore, validResult, input.ConsumedAt)

	cases := []struct {
		name   string
		mutate func(*ClaimXCoreConsumeResult)
	}{
		{
			name: "missing ledger",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries = nil
			},
		},
		{
			name: "duplicate mismatch",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.Duplicate = true
			},
		},
		{
			name: "wrong reference key",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries[0].ReferenceKey = "planet_claim:player-other:planet-other"
			},
		},
		{
			name: "wrong player",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries[0].PlayerID = "player-other"
			},
		},
		{
			name: "wrong item",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries[0].ItemID = "x_core_other"
			},
		},
		{
			name: "wrong location",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries[0].Location = economy.ItemLocation{
					Kind: economy.LocationKindAccountInventory,
					ID:   "player-other",
				}
			},
		},
		{
			name: "wrong quantity",
			mutate: func(result *ClaimXCoreConsumeResult) {
				quantity, err := foundation.NewQuantity(2)
				if err != nil {
					t.Fatalf("NewQuantity: %v", err)
				}
				result.StorageMutation.LedgerEntries[0].Quantity = quantity
			},
		},
		{
			name: "increase action",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.LedgerEntries[0].Action = economy.LedgerActionIncrease
			},
		},
		{
			name: "wrong stackable owner",
			mutate: func(result *ClaimXCoreConsumeResult) {
				result.StorageMutation.StackableItems[0].OwnerPlayerID = "player-other"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := cloneClaimXCoreConsumeResult(validResult)
			tc.mutate(&result)
			if _, err := NewClaimXCoreStorageMutationPlan(input.XCore, result, consumption, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
				t.Fatalf("NewClaimXCoreStorageMutationPlan() error = %v, want ErrInvalidClaimDurableCommit", err)
			}
		})
	}
}

func TestBeginPlanetClaimWithXCoreRejectsMismatchedFactsBeforeDebit(t *testing.T) {
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-xcore-owner-mismatch")
	materializeClaimTestPlanet(t, store, planet)
	consumer := &recordingClaimXCoreConsumer{}
	ref := canonicalClaimReference(t, claimTestPlayerID, planet.ID)
	input := beginClaimWithXCoreInputForTest(t, planet.ID, ref)
	input.Boundary.PlanetID = "planet-claim-xcore-owner-other"

	_, err := composedClaimXCoreOwnerBoundary{
		Consumer:   consumer,
		Boundaries: store,
	}.BeginPlanetClaimWithXCore(input)
	if !errors.Is(err, ErrPlanetClaimReferenceConflict) {
		t.Fatalf("BeginPlanetClaimWithXCore(mismatch) error = %v, want ErrPlanetClaimReferenceConflict", err)
	}
	if len(consumer.calls) != 0 {
		t.Fatalf("x core consumer calls after mismatch = %d, want 0", len(consumer.calls))
	}
}

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

func beginClaimWithXCoreInputForTest(t *testing.T, planetID foundation.PlanetID, ref PlanetClaimReference) BeginPlanetClaimWithXCoreInput {
	t.Helper()
	sourceLocation, err := defaultClaimXCoreSourceProvider{}.ClaimXCoreSourceLocation(ClaimXCoreSourceInput{
		PlayerID: claimTestPlayerID,
		PlanetID: planetID,
	})
	if err != nil {
		t.Fatalf("ClaimXCoreSourceLocation() error = %v, want nil", err)
	}
	return BeginPlanetClaimWithXCoreInput{
		XCore: ClaimXCoreConsumeInput{
			PlayerID: claimTestPlayerID,
			PlanetID: planetID,
			ItemRef: economy.RemoveItemRef{
				Definition: claimTestXCoreDefinition(t),
			},
			SourceLocation: sourceLocation,
			Quantity:       defaultClaimXCoreQuantity,
			Reason:         defaultClaimReason,
			Reference:      ref,
		},
		Boundary: BeginPlanetClaimBoundaryInput{
			ClaimReference:  ref,
			PlayerID:        claimTestPlayerID,
			PlanetID:        planetID,
			ClaimedAt:       testTime(10),
			EventID:         "event_xcore_owner_boundary",
			SourceReference: "planet.claimed:event_xcore_owner_boundary",
		},
		ConsumedAt: testTime(11),
	}
}

func claimXCoreConsumeResultForTest(t *testing.T, input ClaimXCoreConsumeInput, duplicate bool) ClaimXCoreConsumeResult {
	t.Helper()
	quantity, err := foundation.NewQuantity(input.Quantity)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", input.Quantity, err)
	}
	key, ok := input.Reference.IdempotencyKey(input.PlayerID, input.PlanetID)
	if !ok {
		t.Fatalf("IdempotencyKey(%q): want canonical key", input.Reference)
	}
	entry, err := economy.NewItemLedgerEntry(
		"ledger_claim_xcore_storage",
		input.PlayerID,
		input.ItemRef.Definition.ItemID,
		input.ItemRef.ItemInstanceID,
		quantity,
		economy.LedgerActionDecrease,
		4,
		input.SourceLocation,
		input.Reason,
		key,
	)
	if err != nil {
		t.Fatalf("NewItemLedgerEntry: %v", err)
	}
	item, err := economy.NewStackableItem(
		input.ItemRef.Definition.Source,
		"item_claim_xcore_storage",
		input.ItemRef.Definition.ItemID,
		input.PlayerID,
		input.SourceLocation,
		quantity,
	)
	if err != nil {
		t.Fatalf("NewStackableItem: %v", err)
	}
	return ClaimXCoreConsumeResult{
		StorageMutation: economy.RemoveItemResult{
			StackableItems: []economy.StackableItem{item},
			LedgerEntries:  []economy.ItemLedgerEntry{entry},
			Duplicate:      duplicate,
		},
		Duplicate: duplicate,
	}
}
