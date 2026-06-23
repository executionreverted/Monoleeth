package production

import (
	"errors"
	"testing"
	"time"
)

func TestSettlementOutboxDispatchPlanFromProductionTransaction(t *testing.T) {
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

	plan, err := NewSettlementOutboxDispatchPlan(result.Reference, result.OutboxRecords)
	if err != nil {
		t.Fatalf("NewSettlementOutboxDispatchPlan() error = %v, want nil", err)
	}
	if plan.Reference.Kind != SettlementKindProduction || plan.Reference.ReferenceKey != reference || plan.Reference.SettlementWindow != window {
		t.Fatalf("dispatch reference = %+v, want production reference %q/%q", plan.Reference, reference, window)
	}
	assertOutboxEventTypes(t, plan.OutboxRecords,
		EventPlanetBuildingProduced,
		EventPlanetProductionSettled,
		EventOfflineSettlementCompleted,
	)
	assertOutboxRecordEvidence(t, plan.OutboxRecords, EventPlanetProductionSettled, reference, window)

	plan.OutboxRecords[0].Event.Payload[0] = 'x'
	replayed, err := NewSettlementOutboxDispatchPlan(result.Reference, result.OutboxRecords)
	if err != nil {
		t.Fatalf("NewSettlementOutboxDispatchPlan(replay) error = %v, want nil", err)
	}
	if replayed.OutboxRecords[0].Event.Payload[0] == 'x' {
		t.Fatal("mutating dispatch plan payload changed source rows")
	}
}

func TestSettlementOutboxDispatchPlanFromRouteTransaction(t *testing.T) {
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

	plan, err := NewSettlementOutboxDispatchPlan(result.Reference, result.OutboxRecords)
	if err != nil {
		t.Fatalf("NewSettlementOutboxDispatchPlan(route) error = %v, want nil", err)
	}
	if plan.Reference.Kind != SettlementKindRoute || plan.Reference.RouteID != route.RouteID || plan.Reference.ReferenceKey != reference {
		t.Fatalf("dispatch route reference = %+v, want route reference %q", plan.Reference, reference)
	}
	assertOutboxEventTypes(t, plan.OutboxRecords, EventRouteTransferSettled)
	assertOutboxRecordEvidence(t, plan.OutboxRecords, EventRouteTransferSettled, reference, window)
}

func TestSettlementOutboxDispatchPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewSettlementOutboxDispatchPlan(nil, nil); err != nil || len(plan.OutboxRecords) != 0 || !plan.Reference.ReferenceKey.IsZero() {
		t.Fatalf("NewSettlementOutboxDispatchPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	store := newSettlementStore(t, "planet-1", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-1", "building-1", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	result, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-1",
		SettledAt: testTime(0).Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}

	cases := map[string]func([]ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord){
		"missing reference": func(records []ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord) {
			return nil, records
		},
		"published row": func(records []ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord) {
			records[0].Status = ProductionOutboxStatusPublished
			return result.Reference, records
		},
		"mismatched evidence": func(records []ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord) {
			records[0].SettlementWindow = "wrong-window"
			return result.Reference, records
		},
		"empty payload": func(records []ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord) {
			records[0].Event.Payload = nil
			return result.Reference, records
		},
		"bad reference": func(records []ProductionOutboxRecord) (*SettlementReferenceRecord, []ProductionOutboxRecord) {
			bad := *result.Reference
			bad.RecordedAt = time.Time{}
			return &bad, records
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			ref, records := mutate(cloneProductionOutboxRecords(result.OutboxRecords))
			_, err := NewSettlementOutboxDispatchPlan(ref, records)
			if !errors.Is(err, ErrInvalidSettlementOutboxDispatch) {
				t.Fatalf("NewSettlementOutboxDispatchPlan(%s) error = %v, want ErrInvalidSettlementOutboxDispatch", name, err)
			}
		})
	}
}
