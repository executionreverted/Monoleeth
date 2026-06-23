package production

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestInMemoryStoreBoundaryReadMethodsReturnDetachedRecords(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour))
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}

	references := store.SettlementReferences()
	if len(references) != 1 {
		t.Fatalf("SettlementReferences() len = %d, want 1", len(references))
	}
	references[0].SettlementWindow = "mutated"
	references[0].Kind = SettlementKindRoute

	storedReference, ok, err := store.SettlementReference(result.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("SettlementReference() ok = %v err = %v, want true nil", ok, err)
	}
	if storedReference.SettlementWindow != result.SettlementWindow || storedReference.Kind != SettlementKindProduction {
		t.Fatalf("stored reference = %+v, want original production reference/window", storedReference)
	}

	outbox := store.OutboxRecords()
	if len(outbox) == 0 {
		t.Fatal("OutboxRecords() len = 0, want records")
	}
	if len(outbox[0].Event.Payload) == 0 {
		t.Fatal("OutboxRecords()[0].Event.Payload empty, want JSON payload")
	}
	originalType := outbox[0].Event.Type
	originalPayloadFirstByte := outbox[0].Event.Payload[0]
	outbox[0].Status = ProductionOutboxStatus("sent")
	outbox[0].Event.Type = "mutated"
	outbox[0].Event.Payload[0] = 'x'

	storedOutbox := store.OutboxRecords()
	if storedOutbox[0].Status != ProductionOutboxStatusPending {
		t.Fatalf("stored outbox status = %q, want pending", storedOutbox[0].Status)
	}
	if storedOutbox[0].Event.Type != originalType {
		t.Fatalf("stored outbox event type = %q, want %q", storedOutbox[0].Event.Type, originalType)
	}
	if storedOutbox[0].Event.Payload[0] != originalPayloadFirstByte {
		t.Fatalf("stored outbox payload first byte = %q, want original %q", storedOutbox[0].Event.Payload[0], originalPayloadFirstByte)
	}
}

func TestSettlePlanetProductionRecordedReferenceReuseNoOpsBeforeMutation(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)
	window := wantSettlementWindow(testTime(0), now)
	reference := mustOfflineSettlementKey(t, "planet-1", window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindProduction,
		PlanetID:         "planet-1",
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	result, err := store.SettlePlanetProduction("planet-1", now)
	if err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	if !result.NoOp {
		t.Fatal("NoOp = false, want true for recorded reference reuse")
	}
	if got := result.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after iron_ore = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events after reference reuse = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox after reference reuse = %d, want 0", got)
	}
	if got := len(store.SettlementReferences()); got != 1 {
		t.Fatalf("references after reference reuse = %d, want 1", got)
	}
}

func TestSettleRouteRecordedReferenceReuseNoOpsBeforeMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	window := wantSettlementWindow(last, now)
	reference := mustRouteSettlementKey(t, route.RouteID, window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindRoute,
		RouteID:          route.RouteID,
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	service := newTestRouteSettlementService(t, store, now, nil)
	result, err := service.SettleRoute(route.RouteID)
	if err != nil {
		t.Fatalf("SettleRoute() error = %v, want nil", err)
	}
	if !result.NoOp {
		t.Fatal("NoOp = false, want true for recorded reference reuse")
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events after reference reuse = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox after reference reuse = %d, want 0", got)
	}
	if got := len(store.SettlementReferences()); got != 1 {
		t.Fatalf("references after reference reuse = %d, want 1", got)
	}
}

func TestApplyProductionSettlementTransactionReturnsCommittedReferenceAndOutbox(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)
	window := wantSettlementWindow(testTime(0), now)
	reference := mustOfflineSettlementKey(t, "planet-1", window)

	result, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-1",
		SettledAt: now,
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}
	if result.Settlement.ReferenceKey != reference || result.Settlement.SettlementWindow != window {
		t.Fatalf("transaction settlement evidence = %q/%q, want %q/%q", result.Settlement.ReferenceKey, result.Settlement.SettlementWindow, reference, window)
	}
	if result.Reference == nil {
		t.Fatal("transaction reference = nil, want committed production reference")
	}
	if result.Reference.Kind != SettlementKindProduction || result.Reference.PlanetID != "planet-1" || result.Reference.ReferenceKey != reference {
		t.Fatalf("transaction reference = %+v, want production reference %q", result.Reference, reference)
	}
	assertOutboxEventTypes(t, result.OutboxRecords,
		EventPlanetBuildingProduced,
		EventPlanetProductionSettled,
		EventOfflineSettlementCompleted,
	)
	assertOutboxRecordEvidence(t, result.OutboxRecords, EventPlanetProductionSettled, reference, window)
	if got := result.Settlement.After.Storage.QuantityOf("iron_ore"); got != 30 {
		t.Fatalf("after storage iron_ore = %d, want 30", got)
	}
}

func TestApplyProductionSettlementTransactionReferenceReuseReturnsNoNewRows(t *testing.T) {
	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	now := testTime(0).Add(time.Hour)
	window := wantSettlementWindow(testTime(0), now)
	reference := mustOfflineSettlementKey(t, "planet-1", window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindProduction,
		PlanetID:         "planet-1",
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	result, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-1",
		SettledAt: now,
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction(reuse) error = %v, want nil", err)
	}
	if !result.Settlement.NoOp {
		t.Fatal("transaction reuse NoOp = false, want true")
	}
	if result.Reference != nil {
		t.Fatalf("transaction reuse reference = %+v, want nil for no new row", result.Reference)
	}
	if len(result.OutboxRecords) != 0 {
		t.Fatalf("transaction reuse outbox = %+v, want no new rows", result.OutboxRecords)
	}
	if got := result.Settlement.After.Storage.QuantityOf("iron_ore"); got != 0 {
		t.Fatalf("after storage iron_ore = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events after transaction reuse = %d, want 0", got)
	}
}

func TestApplyProductionSettlementTransactionRequireWholeOutputNoOpsWithoutRows(t *testing.T) {
	base := testTime(0)
	store := newSettlementStore(t, "planet-1", base, 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	result, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:           "planet-1",
		SettledAt:          base.Add(time.Minute),
		RequireWholeOutput: true,
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction(sub-unit) error = %v, want nil", err)
	}
	if !result.Settlement.NoOp {
		t.Fatal("sub-unit transaction NoOp = false, want true")
	}
	if result.Reference != nil || len(result.OutboxRecords) != 0 {
		t.Fatalf("sub-unit transaction reference/outbox = %+v/%+v, want no rows", result.Reference, result.OutboxRecords)
	}
	stored, ok, err := store.ProductionState("planet-1")
	if err != nil || !ok {
		t.Fatalf("ProductionState() ok = %v err = %v, want true nil", ok, err)
	}
	if !stored.LastCalculatedAt.Equal(base) {
		t.Fatalf("LastCalculatedAt = %s, want unchanged %s", stored.LastCalculatedAt, base)
	}
}

func TestApplyProductionSettlementTransactionRejectsInvalidInputWithoutMutation(t *testing.T) {
	base := testTime(0)
	store := newSettlementStore(t, "planet-1", base, 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)

	cases := map[string]ProductionSettlementTransactionInput{
		"missing planet": {SettledAt: base.Add(time.Hour)},
		"zero time":      {PlanetID: "planet-1"},
		"missing rows":   {PlanetID: "planet-missing", SettledAt: base.Add(time.Hour)},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := store.ApplyProductionSettlementTransaction(input)
			if err == nil {
				t.Fatal("ApplyProductionSettlementTransaction() error = nil, want error")
			}
			stored, ok, lookupErr := store.ProductionState("planet-1")
			if lookupErr != nil || !ok {
				t.Fatalf("ProductionState() ok = %v err = %v, want true nil", ok, lookupErr)
			}
			if !stored.LastCalculatedAt.Equal(base) {
				t.Fatalf("LastCalculatedAt after invalid transaction = %s, want %s", stored.LastCalculatedAt, base)
			}
			if got := len(store.SettlementReferences()); got != 0 {
				t.Fatalf("references after invalid transaction = %d, want 0", got)
			}
			if got := len(store.OutboxRecords()); got != 0 {
				t.Fatalf("outbox after invalid transaction = %d, want 0", got)
			}
		})
	}
}

func TestApplyRouteSettlementTransactionReturnsCommittedReferenceOutboxAndLedger(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	window := wantSettlementWindow(last, now)
	reference := mustRouteSettlementKey(t, route.RouteID, window)

	result, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction() error = %v, want nil", err)
	}
	if result.Settlement.ReferenceKey != reference || result.Settlement.SettlementWindow != window {
		t.Fatalf("transaction settlement evidence = %q/%q, want %q/%q", result.Settlement.ReferenceKey, result.Settlement.SettlementWindow, reference, window)
	}
	if result.Reference == nil {
		t.Fatal("transaction reference = nil, want committed route reference")
	}
	if result.Reference.Kind != SettlementKindRoute || result.Reference.RouteID != route.RouteID || result.Reference.ReferenceKey != reference {
		t.Fatalf("transaction reference = %+v, want route reference %q", result.Reference, reference)
	}
	assertOutboxEventTypes(t, result.OutboxRecords, EventRouteTransferSettled)
	assertOutboxRecordEvidence(t, result.OutboxRecords, EventRouteTransferSettled, reference, window)
	assertRouteStorageLedgerEntries(t, result.StorageLedger,
		routeStorageLedgerWant{Operation: RouteStorageLedgerSourceDebit, PlanetID: "planet-1", CounterpartyPlanetID: "planet-2", Quantity: 40, BalanceAfter: 60, ReferenceKey: reference, SettlementWindow: window},
		routeStorageLedgerWant{Operation: RouteStorageLedgerDestinationCredit, PlanetID: "planet-2", CounterpartyPlanetID: "planet-1", Quantity: 40, BalanceAfter: 40, ReferenceKey: reference, SettlementWindow: window},
	)
	assertRouteDurableRecord(t, store, route.RouteID, reference, 2, result.Settlement.AfterRoute)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 60, now)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 40, now)
}

func TestApplyRouteSettlementTransactionReferenceReuseReturnsNoNewRows(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	window := wantSettlementWindow(last, now)
	reference := mustRouteSettlementKey(t, route.RouteID, window)

	store.mu.Lock()
	store.ensureMapsLocked()
	store.recordSettlementReferenceLocked(SettlementReferenceRecord{
		ReferenceKey:     reference,
		SettlementWindow: window,
		Kind:             SettlementKindRoute,
		RouteID:          route.RouteID,
		AppliedAt:        now,
		RecordedAt:       now,
	})
	store.mu.Unlock()

	result, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction(reuse) error = %v, want nil", err)
	}
	if !result.Settlement.NoOp {
		t.Fatal("transaction reuse NoOp = false, want true")
	}
	if result.Reference != nil {
		t.Fatalf("transaction reuse reference = %+v, want nil for no new row", result.Reference)
	}
	if len(result.OutboxRecords) != 0 || len(result.StorageLedger) != 0 {
		t.Fatalf("transaction reuse outbox/ledger = %+v/%+v, want no new rows", result.OutboxRecords, result.StorageLedger)
	}
	assertRouteSettlementRouteTime(t, store, route.RouteID, last)
	assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
	assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
}

func TestApplyRouteSettlementTransactionRejectsInvalidInputWithoutMutation(t *testing.T) {
	last := testRouteNow()
	now := last.Add(time.Hour)
	route := validSettlementRoute(last)
	store := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)

	cases := map[string]RouteSettlementTransactionInput{
		"missing owner": {RouteID: route.RouteID, SettledAt: now},
		"missing route": {OwnerPlayerID: route.OwnerPlayerID, SettledAt: now},
		"zero time":     {OwnerPlayerID: route.OwnerPlayerID, RouteID: route.RouteID},
		"wrong owner":   {OwnerPlayerID: "player-2", RouteID: route.RouteID, SettledAt: now},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := store.ApplyRouteSettlementTransaction(input)
			if err == nil {
				t.Fatal("ApplyRouteSettlementTransaction() error = nil, want error")
			}
			assertRouteSettlementRouteTime(t, store, route.RouteID, last)
			assertRouteSettlementStorage(t, store, "planet-1", "refined_alloy", 100, last)
			assertRouteSettlementStorage(t, store, "planet-2", "refined_alloy", 0, last)
			if got := len(store.SettlementReferences()); got != 0 {
				t.Fatalf("references after invalid transaction = %d, want 0", got)
			}
			if got := len(store.OutboxRecords()); got != 0 {
				t.Fatalf("outbox after invalid transaction = %d, want 0", got)
			}
			assertNoRouteStorageLedger(t, store)
		})
	}
}

func TestAutomationRouteServiceUsesSettlementTransactionBoundaryForOwnerSettle(t *testing.T) {
	now := testRouteNow().Add(time.Hour)
	boundary := &fakeRouteSettlementTransactionStore{
		result: RouteSettlementTransactionResult{
			Settlement: RouteSettlementResult{
				RouteID:   "route-1",
				SettledAt: now,
				AfterRoute: AutomationRoute{
					RouteID:       "route-1",
					OwnerPlayerID: "player-1",
				},
			},
		},
	}
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:                 NewInMemoryStore(),
		SettlementTransaction: boundary,
		Clock:                 fixedRouteClock{now: now},
		Policy:                &fakeRoutePolicyProvider{policy: noLossRoutePolicy()},
		LossRoller:            defaultRouteLossRoller{},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}

	settlement, err := service.SettleRouteForOwner("player-1", "route-1")
	if err != nil {
		t.Fatalf("SettleRouteForOwner() error = %v, want nil", err)
	}
	if settlement.RouteID != "route-1" || boundary.calls != 1 {
		t.Fatalf("settlement/boundary calls = %+v/%d, want route-1/1", settlement, boundary.calls)
	}
	if boundary.lastInput.OwnerPlayerID != "player-1" || boundary.lastInput.RouteID != "route-1" || !boundary.lastInput.SettledAt.Equal(now) || boundary.lastInput.LossRoller == nil {
		t.Fatalf("boundary input = %+v, want owner/route/time/loss roller", boundary.lastInput)
	}
}

func TestAutomationRouteServiceSettlementTransactionErrorStopsBeforeStoreMutation(t *testing.T) {
	now := testRouteNow().Add(time.Hour)
	store := NewInMemoryStore()
	boundary := &fakeRouteSettlementTransactionStore{err: ErrRouteOwnerMismatch}
	service, err := NewAutomationRouteService(AutomationRouteServiceConfig{
		Store:                 store,
		SettlementTransaction: boundary,
		Clock:                 fixedRouteClock{now: now},
		Policy:                &fakeRoutePolicyProvider{policy: noLossRoutePolicy()},
		LossRoller:            defaultRouteLossRoller{},
	})
	if err != nil {
		t.Fatalf("NewAutomationRouteService() error = %v, want nil", err)
	}

	if _, err := service.SettleRouteForOwner("player-1", "route-1"); !errors.Is(err, ErrRouteOwnerMismatch) {
		t.Fatalf("SettleRouteForOwner(boundary error) = %v, want ErrRouteOwnerMismatch", err)
	}
	if boundary.calls != 1 {
		t.Fatalf("boundary calls = %d, want 1", boundary.calls)
	}
	if got := len(store.SettlementReferences()); got != 0 {
		t.Fatalf("store references after boundary error = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("store outbox after boundary error = %d, want 0", got)
	}
	assertNoRouteStorageLedger(t, store)
}

func TestOutboxPendingRecordsFilterByStatusLimitAndAppendOrder(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	all := store.OutboxRecords()
	if len(all) < 4 {
		t.Fatalf("OutboxRecords() len = %d, want at least 4", len(all))
	}

	pending := store.PendingOutboxRecords(2)
	assertOutboxSequences(t, pending, 1, 2)

	claimed := store.ClaimPendingOutboxRecords(1, testTime(10))
	assertOutboxSequences(t, claimed, 1)
	pending = store.PendingOutboxRecords(10)
	assertOutboxSequences(t, pending, 2, 3, 4)

	if got := store.PendingOutboxRecords(0); got != nil {
		t.Fatalf("PendingOutboxRecords(0) = %+v, want nil", got)
	}
}

func TestOutboxClaimMovesPendingToInFlightWithDetachedRecords(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimedAt := testTime(11)

	claimed := store.ClaimPendingOutboxRecords(2, claimedAt)
	if len(claimed) != 2 {
		t.Fatalf("ClaimPendingOutboxRecords() len = %d, want 2", len(claimed))
	}
	for index, record := range claimed {
		if record.Status != ProductionOutboxStatusInFlight {
			t.Fatalf("claimed[%d] status = %q, want in_flight", index, record.Status)
		}
		if !record.ClaimedAt.Equal(claimedAt) {
			t.Fatalf("claimed[%d] ClaimedAt = %s, want %s", index, record.ClaimedAt, claimedAt)
		}
		if record.Attempts != 1 {
			t.Fatalf("claimed[%d] Attempts = %d, want 1", index, record.Attempts)
		}
		wantClaimToken := productionOutboxClaimToken(record.OutboxID, record.Attempts)
		if record.ClaimToken != wantClaimToken {
			t.Fatalf("claimed[%d] ClaimToken = %q, want %q", index, record.ClaimToken, wantClaimToken)
		}
	}
	if len(claimed[0].Event.Payload) == 0 {
		t.Fatal("claimed[0] payload empty, want payload")
	}
	originalPayloadFirstByte := claimed[0].Event.Payload[0]
	claimed[0].Status = ProductionOutboxStatusPublished
	claimed[0].Event.Payload[0] = 'x'

	stored := store.OutboxRecords()
	if stored[0].Status != ProductionOutboxStatusInFlight {
		t.Fatalf("stored[0] status = %q, want in_flight", stored[0].Status)
	}
	if stored[0].Event.Payload[0] != originalPayloadFirstByte {
		t.Fatalf("stored[0] payload first byte = %q, want original %q", stored[0].Event.Payload[0], originalPayloadFirstByte)
	}
}

func TestOutboxPublishedRecordsAreNotPendingOrRetryable(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(2, testTime(12))
	publishedAt := testTime(13)

	published, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, publishedAt)
	if !ok {
		t.Fatal("MarkClaimedOutboxPublished() ok = false, want true")
	}
	if published.Status != ProductionOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
	if !published.PublishedAt.Equal(publishedAt) {
		t.Fatalf("PublishedAt = %s, want %s", published.PublishedAt, publishedAt)
	}

	beforeLateFailure := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "late failure", testTime(14)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(published) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeLateFailure, "late failure after publish")
	retryable := store.RetryFailedOutboxRecords(10, testTime(15))
	if len(retryable) != 0 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 0 for published record", len(retryable))
	}
	pending := store.PendingOutboxRecords(10)
	for _, record := range pending {
		if record.OutboxID == published.OutboxID {
			t.Fatalf("published record %q appeared in pending records: %+v", published.OutboxID, pending)
		}
	}
	stored := store.OutboxRecords()[0]
	if stored.Status != ProductionOutboxStatusPublished || !stored.PublishedAt.Equal(publishedAt) {
		t.Fatalf("stored published record = %+v, want published with original PublishedAt %s", stored, publishedAt)
	}
}

func TestOutboxFailedRecordsRetryBackToPendingWithoutPayloadAliases(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(1, testTime(16))
	failedAt := testTime(17)

	failed, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary broker outage", failedAt)
	if !ok {
		t.Fatal("MarkClaimedOutboxFailed() ok = false, want true")
	}
	if failed.Status != ProductionOutboxStatusFailed {
		t.Fatalf("failed status = %q, want failed", failed.Status)
	}
	if failed.LastError != "temporary broker outage" || !failed.FailedAt.Equal(failedAt) {
		t.Fatalf("failure evidence = %q/%s, want reason/%s", failed.LastError, failed.FailedAt, failedAt)
	}
	failed.Event.Payload[0] = 'x'

	storedFailed := store.OutboxRecords()[0]
	if storedFailed.Event.Payload[0] == 'x' {
		t.Fatal("mutating failed returned payload changed stored payload")
	}

	retriedAt := testTime(18)
	retried := store.RetryFailedOutboxRecords(1, retriedAt)
	if len(retried) != 1 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 1", len(retried))
	}
	if retried[0].Status != ProductionOutboxStatusPending {
		t.Fatalf("retried status = %q, want pending", retried[0].Status)
	}
	if retried[0].ClaimToken != "" {
		t.Fatalf("retried ClaimToken = %q, want empty", retried[0].ClaimToken)
	}
	if !retried[0].FailedAt.Equal(failedAt) || retried[0].LastError != "temporary broker outage" {
		t.Fatalf("retried failure evidence = %q/%s, want preserved %q/%s", retried[0].LastError, retried[0].FailedAt, failed.LastError, failedAt)
	}
	if !retried[0].RetriedAt.Equal(retriedAt) {
		t.Fatalf("RetriedAt = %s, want %s", retried[0].RetriedAt, retriedAt)
	}
	retried[0].Event.Payload[0] = 'y'
	if store.OutboxRecords()[0].Event.Payload[0] == 'y' {
		t.Fatal("mutating retried returned payload changed stored payload")
	}

	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(19))
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim len = %d, want 1", len(reclaimed))
	}
	if reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed Attempts = %d, want 2", reclaimed[0].Attempts)
	}
	wantClaimToken := productionOutboxClaimToken(reclaimed[0].OutboxID, reclaimed[0].Attempts)
	if reclaimed[0].ClaimToken != wantClaimToken {
		t.Fatalf("reclaimed ClaimToken = %q, want %q", reclaimed[0].ClaimToken, wantClaimToken)
	}
}

func TestOutboxWrongAndStaleClaimTokensDoNotMutateRecords(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(1, testTime(20))
	first := claimed[0]

	beforeWrongPublish := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(first.OutboxID, "wrong-token", testTime(21)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeWrongPublish, "wrong publish token")

	beforeWrongFail := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxFailed(first.OutboxID, "wrong-token", "wrong token failure", testTime(22)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(wrong token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeWrongFail, "wrong fail token")

	if _, ok := store.MarkClaimedOutboxFailed(first.OutboxID, first.ClaimToken, "temporary broker outage", testTime(23)); !ok {
		t.Fatal("MarkClaimedOutboxFailed(current token) ok = false, want true")
	}
	retried := store.RetryFailedOutboxRecords(1, testTime(24))
	if len(retried) != 1 {
		t.Fatalf("RetryFailedOutboxRecords() len = %d, want 1", len(retried))
	}
	if retried[0].ClaimToken != "" {
		t.Fatalf("retried ClaimToken = %q, want empty", retried[0].ClaimToken)
	}

	beforePendingStale := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(first.OutboxID, first.ClaimToken, testTime(25)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed(first.OutboxID, first.ClaimToken, "stale pending failure", testTime(26)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(stale pending token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforePendingStale, "stale token while pending")

	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(27))
	if len(reclaimed) != 1 {
		t.Fatalf("reclaim len = %d, want 1", len(reclaimed))
	}
	second := reclaimed[0]
	if second.ClaimToken == first.ClaimToken {
		t.Fatalf("reclaimed ClaimToken = first token %q, want new attempt token", second.ClaimToken)
	}

	beforeReclaimedStale := store.OutboxRecords()
	if record, ok := store.MarkClaimedOutboxPublished(second.OutboxID, first.ClaimToken, testTime(28)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed(second.OutboxID, first.ClaimToken, "stale reclaimed failure", testTime(29)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(stale reclaimed token) = %+v/%v, want zero/false", record, ok)
	}
	assertOutboxUnchanged(t, store, beforeReclaimedStale, "stale token after reclaim")

	published, ok := store.MarkClaimedOutboxPublished(second.OutboxID, second.ClaimToken, testTime(30))
	if !ok {
		t.Fatal("MarkClaimedOutboxPublished(new token) ok = false, want true")
	}
	if published.Status != ProductionOutboxStatusPublished {
		t.Fatalf("published status = %q, want published", published.Status)
	}
}

func TestOutboxExpiredInFlightReleaseReturnsRecordsToPending(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	oldClaimedAt := testTime(60)
	boundaryClaimedAt := testTime(70)
	retriedAt := testTime(80)

	claimed := store.ClaimPendingOutboxRecords(3, oldClaimedAt)
	if len(claimed) != 3 {
		t.Fatalf("claimed len = %d, want 3", len(claimed))
	}
	if _, ok := store.MarkClaimedOutboxFailed(claimed[1].OutboxID, claimed[1].ClaimToken, "temporary broker outage", testTime(61)); !ok {
		t.Fatal("fail second record ok = false, want true")
	}
	if retried := store.RetryFailedOutboxRecords(1, testTime(62)); len(retried) != 1 || retried[0].OutboxID != claimed[1].OutboxID {
		t.Fatalf("retried failed record = %+v, want second record", retried)
	}
	reclaimedWithFailure := store.ClaimPendingOutboxRecords(1, oldClaimedAt)
	if len(reclaimedWithFailure) != 1 || reclaimedWithFailure[0].OutboxID != claimed[1].OutboxID || reclaimedWithFailure[0].LastError != "temporary broker outage" {
		t.Fatalf("reclaimed failed record = %+v, want second record with preserved failure", reclaimedWithFailure)
	}
	if _, ok := store.MarkClaimedOutboxPublished(claimed[2].OutboxID, claimed[2].ClaimToken, testTime(61)); !ok {
		t.Fatal("publish third record ok = false, want true")
	}
	store.ClaimPendingOutboxRecords(1, boundaryClaimedAt)

	released := store.ReleaseExpiredOutboxRecords(1, boundaryClaimedAt, retriedAt)
	assertOutboxSequences(t, released, 1)
	if released[0].Status != ProductionOutboxStatusPending || !released[0].ClaimedAt.IsZero() || released[0].ClaimToken != "" {
		t.Fatalf("released record = %+v, want pending with cleared claim", released[0])
	}
	if released[0].Attempts != 1 || !released[0].RetriedAt.Equal(retriedAt) {
		t.Fatalf("released attempts/retried = %d/%s, want 1/%s", released[0].Attempts, released[0].RetriedAt, retriedAt)
	}
	if record, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(81)); ok || record.OutboxID != "" {
		t.Fatalf("stale publish after release = %+v/%v, want zero/false", record, ok)
	}
	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(82))
	if len(reclaimed) != 1 || reclaimed[0].OutboxID != claimed[0].OutboxID {
		t.Fatalf("reclaimed = %+v, want first released record", reclaimed)
	}
	if reclaimed[0].Attempts != 2 || reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Fatalf("reclaimed attempts/token = %d/%q, want attempt 2 with new token", reclaimed[0].Attempts, reclaimed[0].ClaimToken)
	}

	secondRelease := store.ReleaseExpiredOutboxRecords(10, boundaryClaimedAt, testTime(83))
	assertOutboxSequences(t, secondRelease, 2)
	if secondRelease[0].LastError != "temporary broker outage" {
		t.Fatalf("released failure evidence = %q, want preserved error", secondRelease[0].LastError)
	}
	for _, record := range secondRelease {
		if record.Sequence == 3 {
			t.Fatalf("published record released unexpectedly: %+v", secondRelease)
		}
	}
}

func TestOutboxExpiredInFlightReleaseIgnoresBoundaryAndInvalidInputs(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimedAt := testTime(90)
	claimed := store.ClaimPendingOutboxRecords(1, claimedAt)

	if got := store.ReleaseExpiredOutboxRecords(0, claimedAt.Add(time.Second), testTime(91)); got != nil {
		t.Fatalf("ReleaseExpiredOutboxRecords(limit 0) = %+v, want nil", got)
	}
	if got := store.ReleaseExpiredOutboxRecords(1, time.Time{}, testTime(91)); got != nil {
		t.Fatalf("ReleaseExpiredOutboxRecords(zero cutoff) = %+v, want nil", got)
	}
	if got := store.ReleaseExpiredOutboxRecords(1, claimedAt, testTime(91)); len(got) != 0 {
		t.Fatalf("ReleaseExpiredOutboxRecords(boundary equal) = %+v, want empty", got)
	}
	if record, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(92)); !ok || record.Status != ProductionOutboxStatusPublished {
		t.Fatalf("publish after boundary release check = %+v/%v, want published", record, ok)
	}
}

func TestOutboxClaimWithZeroTimeCanBeReleased(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimed := store.ClaimPendingOutboxRecords(1, time.Time{})
	if len(claimed) != 1 {
		t.Fatalf("claimed len = %d, want 1", len(claimed))
	}
	if claimed[0].ClaimedAt.IsZero() {
		t.Fatalf("claimed_at is zero, want normalized releaseable timestamp")
	}

	released := store.ReleaseExpiredOutboxRecords(1, time.Unix(1, 0).UTC(), testTime(93))
	if len(released) != 1 || released[0].OutboxID != claimed[0].OutboxID {
		t.Fatalf("released zero-time claimed record = %+v, want claimed record", released)
	}
	if record, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "stale", testTime(94)); ok || record.OutboxID != "" {
		t.Fatalf("stale fail after zero-time release = %+v/%v, want zero/false", record, ok)
	}
}

func TestReleaseExpiredProductionOutboxLeasesUsesDurableBoundaryContract(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	oldClaimedAt := testTime(95)
	boundaryClaimedAt := testTime(96)
	releasedAt := testTime(97)
	claimed := store.ClaimPendingOutboxRecords(2, oldClaimedAt)
	if len(claimed) != 2 {
		t.Fatalf("claimed len = %d, want 2", len(claimed))
	}

	released, err := ReleaseExpiredProductionOutboxLeases(ProductionOutboxLeaseReleaseInput{
		Store:         store,
		Limit:         1,
		ClaimedBefore: boundaryClaimedAt,
		ReleasedAt:    releasedAt,
	})
	if err != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxLeases() error = %v, want nil", err)
	}
	assertOutboxSequences(t, released, 1)
	if released[0].Status != ProductionOutboxStatusPending || !released[0].ClaimedAt.IsZero() || released[0].ClaimToken != "" {
		t.Fatalf("released outbox = %+v, want pending with cleared claim", released[0])
	}
	if !released[0].RetriedAt.Equal(releasedAt) || released[0].Attempts != 1 {
		t.Fatalf("released retry evidence = %+v, want retried_at %s and attempts preserved", released[0], releasedAt)
	}
	if record, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(98)); ok || record.OutboxID != "" {
		t.Fatalf("stale publish after lease release = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "stale", testTime(98)); ok || record.OutboxID != "" {
		t.Fatalf("stale fail after lease release = %+v/%v, want zero/false", record, ok)
	}

	reclaimed := store.ClaimPendingOutboxRecords(1, testTime(99))
	if len(reclaimed) != 1 || reclaimed[0].OutboxID != claimed[0].OutboxID || reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Fatalf("reclaimed released lease = %+v, want first outbox with fresh token", reclaimed)
	}
}

func TestReleaseExpiredProductionOutboxLeasesRejectsInvalidStoreAndIgnoresNoOpInputs(t *testing.T) {
	if released, err := ReleaseExpiredProductionOutboxLeases(ProductionOutboxLeaseReleaseInput{}); !errors.Is(err, ErrInvalidProductionOutboxPublisher) || released != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxLeases(invalid) = %+v/%v, want invalid publisher error", released, err)
	}

	store := newOutboxStateMachineStore(t)
	claimedAt := testTime(100)
	claimed := store.ClaimPendingOutboxRecords(1, claimedAt)
	if len(claimed) != 1 {
		t.Fatalf("claimed len = %d, want 1", len(claimed))
	}
	if released, err := ReleaseExpiredProductionOutboxLeases(ProductionOutboxLeaseReleaseInput{
		Store:         store,
		Limit:         0,
		ClaimedBefore: claimedAt.Add(time.Second),
		ReleasedAt:    testTime(101),
	}); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxLeases(limit 0) = %+v/%v, want nil nil", released, err)
	}
	if released, err := ReleaseExpiredProductionOutboxLeases(ProductionOutboxLeaseReleaseInput{
		Store:      store,
		Limit:      1,
		ReleasedAt: testTime(101),
	}); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxLeases(zero cutoff) = %+v/%v, want nil nil", released, err)
	}
	if released, err := ReleaseExpiredProductionOutboxLeases(ProductionOutboxLeaseReleaseInput{
		Store:         store,
		Limit:         1,
		ClaimedBefore: claimedAt,
		ReleasedAt:    testTime(101),
	}); err != nil || len(released) != 0 {
		t.Fatalf("ReleaseExpiredProductionOutboxLeases(boundary equal) = %+v/%v, want empty nil", released, err)
	}
	if record, ok := store.MarkClaimedOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(102)); !ok || record.Status != ProductionOutboxStatusPublished {
		t.Fatalf("publish after no-op releases = %+v/%v, want published true", record, ok)
	}
}

func TestOutboxUnknownClaimedMarkReturnsFalseWithoutMutation(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	before := store.OutboxRecords()

	if record, ok := store.MarkClaimedOutboxPublished("missing-outbox", "missing-token", testTime(31)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxPublished(missing) = %+v/%v, want zero/false", record, ok)
	}
	if record, ok := store.MarkClaimedOutboxFailed("missing-outbox", "missing-token", "missing", testTime(32)); ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimedOutboxFailed(missing) = %+v/%v, want zero/false", record, ok)
	}
	after := store.OutboxRecords()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("outbox mutated after unknown mark\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestPublishPendingProductionOutboxPublishesAndFailsWithClaimTokens(t *testing.T) {
	store := newOutboxStateMachineStore(t)
	claimAt := testTime(50)
	completedAt := testTime(51)
	temporaryErr := errors.New("temporary broker outage")
	publishedIDs := make([]string, 0, 2)

	results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       2,
		ClaimedAt:   claimAt,
		CompletedAt: completedAt,
		Publish: func(record ProductionOutboxRecord) error {
			publishedIDs = append(publishedIDs, record.OutboxID)
			if len(publishedIDs) == 2 {
				return temporaryErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("PublishPendingProductionOutbox() len = %d, want 2; results = %+v", len(results), results)
	}
	if !results[0].Published || results[0].Failed || results[0].StaleClaim {
		t.Fatalf("first publish result = %+v, want published only", results[0])
	}
	if !results[1].Failed || results[1].Published || results[1].StaleClaim || results[1].Error != temporaryErr.Error() {
		t.Fatalf("second publish result = %+v, want failed with error", results[1])
	}
	if results[0].ClaimToken == "" || results[1].ClaimToken == "" {
		t.Fatalf("publish claim tokens = %q/%q, want non-empty", results[0].ClaimToken, results[1].ClaimToken)
	}

	stored := store.OutboxRecords()
	if stored[0].Status != ProductionOutboxStatusPublished || !stored[0].PublishedAt.Equal(completedAt) {
		t.Fatalf("stored first outbox = %+v, want published at %s", stored[0], completedAt)
	}
	if stored[1].Status != ProductionOutboxStatusFailed || stored[1].LastError != temporaryErr.Error() || !stored[1].FailedAt.Equal(completedAt) {
		t.Fatalf("stored second outbox = %+v, want failed with error at %s", stored[1], completedAt)
	}
	pending := store.PendingOutboxRecords(10)
	for _, record := range pending {
		if record.OutboxID == stored[0].OutboxID || record.OutboxID == stored[1].OutboxID {
			t.Fatalf("published/failed record appeared as pending: %+v", pending)
		}
	}
}

func TestPublishPendingProductionOutboxRejectsInvalidPublisher(t *testing.T) {
	if results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{}); !errors.Is(err, ErrInvalidProductionOutboxPublisher) || results != nil {
		t.Fatalf("PublishPendingProductionOutbox(invalid) = %+v/%v, want invalid publisher error", results, err)
	}
	store := newOutboxStateMachineStore(t)
	if results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store: store,
		Limit: 1,
	}); !errors.Is(err, ErrInvalidProductionOutboxPublisher) || results != nil {
		t.Fatalf("PublishPendingProductionOutbox(nil publish) = %+v/%v, want invalid publisher error", results, err)
	}
}

func TestPublishPendingProductionOutboxReportsStaleClaimWhenMarkRejected(t *testing.T) {
	temporaryErr := errors.New("temporary broker outage")

	successStore := staleProductionOutboxPublisherStore{
		InMemoryStore: newOutboxStateMachineStore(t),
		stalePublish:  true,
	}
	successResults, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       successStore,
		Limit:       1,
		ClaimedAt:   testTime(52),
		CompletedAt: testTime(53),
		Publish:     func(ProductionOutboxRecord) error { return nil },
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox(stale publish) error = %v, want nil", err)
	}
	if len(successResults) != 1 || !successResults[0].StaleClaim || successResults[0].Published || successResults[0].Failed || successResults[0].StoreError {
		t.Fatalf("stale publish result = %+v, want stale claim only", successResults)
	}

	failStore := staleProductionOutboxPublisherStore{
		InMemoryStore: newOutboxStateMachineStore(t),
		staleFail:     true,
	}
	failResults, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       failStore,
		Limit:       1,
		ClaimedAt:   testTime(54),
		CompletedAt: testTime(55),
		Publish:     func(ProductionOutboxRecord) error { return temporaryErr },
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox(stale fail) error = %v, want nil", err)
	}
	if len(failResults) != 1 || !failResults[0].StaleClaim || failResults[0].Published || failResults[0].Failed || failResults[0].StoreError || failResults[0].Error != temporaryErr.Error() {
		t.Fatalf("stale fail result = %+v, want stale claim with publish error", failResults)
	}
}

type staleProductionOutboxPublisherStore struct {
	*InMemoryStore
	stalePublish bool
	staleFail    bool
}

type fakeRouteSettlementTransactionStore struct {
	calls     int
	lastInput RouteSettlementTransactionInput
	result    RouteSettlementTransactionResult
	err       error
}

func (store *fakeRouteSettlementTransactionStore) ApplyRouteSettlementTransaction(input RouteSettlementTransactionInput) (RouteSettlementTransactionResult, error) {
	store.calls++
	store.lastInput = input
	if store.err != nil {
		return RouteSettlementTransactionResult{}, store.err
	}
	return store.result, nil
}

func (store staleProductionOutboxPublisherStore) MarkProductionOutboxPublished(outboxID string, claimToken string, publishedAt time.Time) (ProductionOutboxRecord, bool, error) {
	if store.stalePublish {
		return ProductionOutboxRecord{}, false, nil
	}
	return store.InMemoryStore.MarkProductionOutboxPublished(outboxID, claimToken, publishedAt)
}

func (store staleProductionOutboxPublisherStore) MarkProductionOutboxFailed(outboxID string, claimToken string, reason string, failedAt time.Time) (ProductionOutboxRecord, bool, error) {
	if store.staleFail {
		return ProductionOutboxRecord{}, false, nil
	}
	return store.InMemoryStore.MarkProductionOutboxFailed(outboxID, claimToken, reason, failedAt)
}

func assertOutboxUnchanged(t *testing.T, store *InMemoryStore, before []ProductionOutboxRecord, context string) {
	t.Helper()
	after := store.OutboxRecords()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("outbox mutated after %s\nbefore=%+v\nafter=%+v", context, before, after)
	}
}

func assertSettlementReferenceRecord(
	t *testing.T,
	records []SettlementReferenceRecord,
	kind SettlementKind,
	planetID foundation.PlanetID,
	routeID foundation.RouteID,
	reference foundation.IdempotencyKey,
	window string,
	appliedAt time.Time,
) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("SettlementReferences() len = %d, want 1; records = %+v", len(records), records)
	}
	record := records[0]
	if record.ReferenceKey != reference || record.SettlementWindow != window || record.Kind != kind {
		t.Fatalf("reference record key/window/kind = %q/%q/%q, want %q/%q/%q", record.ReferenceKey, record.SettlementWindow, record.Kind, reference, window, kind)
	}
	if record.PlanetID != planetID || record.RouteID != routeID {
		t.Fatalf("reference record planet/route = %q/%q, want %q/%q", record.PlanetID, record.RouteID, planetID, routeID)
	}
	if !record.AppliedAt.Equal(appliedAt) || !record.RecordedAt.Equal(appliedAt) {
		t.Fatalf("reference record applied/recorded = %s/%s, want %s", record.AppliedAt, record.RecordedAt, appliedAt)
	}
}

func assertOutboxEventTypes(t *testing.T, records []ProductionOutboxRecord, want ...string) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("OutboxRecords() len = %d, want %d; records = %+v", len(records), len(want), records)
	}
	for index, record := range records {
		if record.Event.Type != want[index] {
			t.Fatalf("outbox[%d] type = %q, want %q; records = %+v", index, record.Event.Type, want[index], records)
		}
		if record.Sequence != uint64(index+1) {
			t.Fatalf("outbox[%d] sequence = %d, want %d", index, record.Sequence, index+1)
		}
		if record.OutboxID == "" {
			t.Fatalf("outbox[%d] id is empty", index)
		}
		if record.Status != ProductionOutboxStatusPending {
			t.Fatalf("outbox[%d] status = %q, want pending", index, record.Status)
		}
		if record.CreatedAt.IsZero() {
			t.Fatalf("outbox[%d] CreatedAt is zero", index)
		}
		if len(record.Event.Payload) == 0 {
			t.Fatalf("outbox[%d] event payload is empty", index)
		}
	}
}

func assertOutboxRecordEvidence(
	t *testing.T,
	records []ProductionOutboxRecord,
	eventType string,
	reference foundation.IdempotencyKey,
	window string,
) {
	t.Helper()
	for _, record := range records {
		if record.Event.Type != eventType {
			continue
		}
		if record.ReferenceKey != reference || record.SettlementWindow != window {
			t.Fatalf("outbox %q evidence = %q/%q, want %q/%q", eventType, record.ReferenceKey, record.SettlementWindow, reference, window)
		}
		return
	}
	t.Fatalf("outbox event %q missing in %+v", eventType, records)
}

func newOutboxStateMachineStore(t *testing.T) *InMemoryStore {
	t.Helper()
	store := newSettlementStore(t, "planet-1", testTime(0), 200, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	addSettlementBuilding(t, store, "planet-1", "building-2", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	if _, err := store.SettlePlanetProduction("planet-1", testTime(0).Add(time.Hour)); err != nil {
		t.Fatalf("SettlePlanetProduction() error = %v, want nil", err)
	}
	return store
}

func assertOutboxSequences(t *testing.T, records []ProductionOutboxRecord, want ...uint64) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("outbox records len = %d, want %d; records = %+v", len(records), len(want), records)
	}
	for index, record := range records {
		if record.Sequence != want[index] {
			t.Fatalf("record[%d] sequence = %d, want %d; records = %+v", index, record.Sequence, want[index], records)
		}
	}
}
