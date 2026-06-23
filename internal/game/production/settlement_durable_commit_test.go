package production

import (
	"errors"
	"testing"
	"time"
)

func TestSettlementDurableCommitPlanFromProductionTransaction(t *testing.T) {
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

	plan, err := NewSettlementDurableCommitPlan(result.Reference, result.OutboxRecords, nil)
	if err != nil {
		t.Fatalf("NewSettlementDurableCommitPlan(production) error = %v, want nil", err)
	}
	if plan.Reference.Kind != SettlementKindProduction || plan.Reference.ReferenceKey != reference {
		t.Fatalf("durable production reference = %+v, want %q", plan.Reference, reference)
	}
	if len(plan.RouteStorageLedger) != 0 {
		t.Fatalf("durable production route ledger = %+v, want empty", plan.RouteStorageLedger)
	}
	assertOutboxRecordEvidence(t, plan.Outbox.OutboxRecords, EventPlanetProductionSettled, reference, window)
}

func TestProductionSettlementTransactionResultDurableCommitPlan(t *testing.T) {
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

	plan, err := result.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan(production) error = %v, want nil", err)
	}
	if plan.Reference.ReferenceKey != reference || len(plan.Outbox.OutboxRecords) == 0 {
		t.Fatalf("production durable commit plan = %+v, want reference %q and outbox rows", plan, reference)
	}
	assertOutboxRecordEvidence(t, plan.Outbox.OutboxRecords, EventPlanetProductionSettled, reference, window)

	duplicate, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-1",
		SettledAt: now,
	})
	if err != nil {
		t.Fatalf("duplicate ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}
	empty, err := duplicate.DurableCommitPlan()
	if err != nil {
		t.Fatalf("duplicate DurableCommitPlan() error = %v, want nil", err)
	}
	if !empty.Reference.ReferenceKey.IsZero() || len(empty.Outbox.OutboxRecords) != 0 || len(empty.RouteStorageLedger) != 0 {
		t.Fatalf("duplicate durable commit plan = %+v, want empty no-op plan", empty)
	}
}

func TestSettlementDurableCommitPlanFromRouteTransaction(t *testing.T) {
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

	plan, err := NewSettlementDurableCommitPlan(result.Reference, result.OutboxRecords, result.StorageLedger)
	if err != nil {
		t.Fatalf("NewSettlementDurableCommitPlan(route) error = %v, want nil", err)
	}
	if plan.Reference.Kind != SettlementKindRoute || plan.Reference.RouteID != route.RouteID || plan.Reference.ReferenceKey != reference {
		t.Fatalf("durable route reference = %+v, want route %q reference %q", plan.Reference, route.RouteID, reference)
	}
	if len(plan.RouteStorageLedger) != 2 {
		t.Fatalf("durable route ledger len = %d, want 2; rows = %+v", len(plan.RouteStorageLedger), plan.RouteStorageLedger)
	}
	for _, row := range plan.RouteStorageLedger {
		if row.ReferenceKey != reference || row.SettlementWindow != window || row.RouteID != route.RouteID {
			t.Fatalf("route ledger evidence = %+v, want route/reference/window", row)
		}
	}
	assertOutboxRecordEvidence(t, plan.Outbox.OutboxRecords, EventRouteTransferSettled, reference, window)
}

func TestRouteSettlementTransactionResultDurableCommitPlan(t *testing.T) {
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

	plan, err := result.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan(route) error = %v, want nil", err)
	}
	if plan.Reference.ReferenceKey != reference || len(plan.RouteStorageLedger) != 2 {
		t.Fatalf("route durable commit plan = %+v, want reference %q and route ledger rows", plan, reference)
	}
	for _, row := range plan.RouteStorageLedger {
		if row.ReferenceKey != reference || row.SettlementWindow != window || row.RouteID != route.RouteID {
			t.Fatalf("route durable commit ledger row = %+v, want route/reference/window evidence", row)
		}
	}

	duplicate, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("duplicate ApplyRouteSettlementTransaction() error = %v, want nil", err)
	}
	empty, err := duplicate.DurableCommitPlan()
	if err != nil {
		t.Fatalf("duplicate route DurableCommitPlan() error = %v, want nil", err)
	}
	if !empty.Reference.ReferenceKey.IsZero() || len(empty.Outbox.OutboxRecords) != 0 || len(empty.RouteStorageLedger) != 0 {
		t.Fatalf("duplicate route durable commit plan = %+v, want empty no-op plan", empty)
	}
}

func TestSettlementDurableCommitPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewSettlementDurableCommitPlan(nil, nil, nil); err != nil || !plan.Reference.ReferenceKey.IsZero() {
		t.Fatalf("NewSettlementDurableCommitPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

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
	result, err := store.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     now,
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction() error = %v, want nil", err)
	}

	cases := map[string]func() (*SettlementReferenceRecord, []ProductionOutboxRecord, []RouteStorageLedgerEntry){
		"missing reference with rows": func() (*SettlementReferenceRecord, []ProductionOutboxRecord, []RouteStorageLedgerEntry) {
			return nil, result.OutboxRecords, result.StorageLedger
		},
		"mismatched ledger reference": func() (*SettlementReferenceRecord, []ProductionOutboxRecord, []RouteStorageLedgerEntry) {
			ledger := cloneRouteStorageLedgerEntries(result.StorageLedger)
			ledger[0].ReferenceKey = "route_settle:other:window"
			return result.Reference, result.OutboxRecords, ledger
		},
		"mismatched ledger route": func() (*SettlementReferenceRecord, []ProductionOutboxRecord, []RouteStorageLedgerEntry) {
			ledger := cloneRouteStorageLedgerEntries(result.StorageLedger)
			ledger[0].RouteID = "route-other"
			return result.Reference, result.OutboxRecords, ledger
		},
		"published outbox": func() (*SettlementReferenceRecord, []ProductionOutboxRecord, []RouteStorageLedgerEntry) {
			outbox := cloneProductionOutboxRecords(result.OutboxRecords)
			outbox[0].Status = ProductionOutboxStatusPublished
			return result.Reference, outbox, result.StorageLedger
		},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			ref, outbox, ledger := input()
			_, err := NewSettlementDurableCommitPlan(ref, outbox, ledger)
			if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
				t.Fatalf("NewSettlementDurableCommitPlan(%s) error = %v, want ErrInvalidSettlementDurableCommit", name, err)
			}
		})
	}
}
