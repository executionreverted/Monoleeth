package production

import (
	"errors"
	"testing"
	"time"
)

func TestSettlementDurableCommitStoreCommitsProductionPlan(t *testing.T) {
	plan := productionDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()

	result, err := store.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan(production) error = %v, want nil", err)
	}
	if result.Duplicate || result.Reference == nil || result.Reference.ReferenceKey != plan.Reference.ReferenceKey {
		t.Fatalf("production durable commit result = %+v, want committed reference", result)
	}
	if len(result.OutboxRecords) == 0 || len(result.RouteStorageLedger) != 0 {
		t.Fatalf("production durable commit rows = outbox %+v ledger %+v, want outbox and no route ledger", result.OutboxRecords, result.RouteStorageLedger)
	}
	if got := len(store.SettlementReferences()); got != 1 {
		t.Fatalf("SettlementReferences() len = %d, want 1", got)
	}
	if got := len(store.OutboxRecords()); got != len(plan.Outbox.OutboxRecords) {
		t.Fatalf("OutboxRecords() len = %d, want %d", got, len(plan.Outbox.OutboxRecords))
	}
}

func TestSettlementDurableCommitStoreCommitsRoutePlanWithLedger(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()

	result, err := store.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan(route) error = %v, want nil", err)
	}
	if result.Duplicate || result.Reference == nil || result.Reference.Kind != SettlementKindRoute {
		t.Fatalf("route durable commit result = %+v, want committed route reference", result)
	}
	if len(result.OutboxRecords) == 0 || len(result.RouteStorageLedger) != len(plan.RouteStorageLedger) {
		t.Fatalf("route durable commit rows = outbox %+v ledger %+v, want outbox and route ledger", result.OutboxRecords, result.RouteStorageLedger)
	}
	for _, row := range result.RouteStorageLedger {
		if row.ReferenceKey != plan.Reference.ReferenceKey || row.SettlementWindow != plan.Reference.SettlementWindow || row.RouteID != plan.Reference.RouteID {
			t.Fatalf("route ledger row = %+v, want plan reference/window/route", row)
		}
	}
}

func TestSettlementDurableCommitStoreDuplicateReferenceReplaysWithoutDuplicateRows(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()

	first, err := store.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("first ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}
	duplicate, err := store.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("duplicate ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}
	if first.Duplicate || !duplicate.Duplicate {
		t.Fatalf("duplicate flags first=%v duplicate=%v, want false/true", first.Duplicate, duplicate.Duplicate)
	}
	if len(store.SettlementReferences()) != 1 ||
		len(store.OutboxRecords()) != len(plan.Outbox.OutboxRecords) ||
		len(store.RouteStorageLedgerEntries()) != len(plan.RouteStorageLedger) {
		t.Fatalf("durable store rows refs=%d outbox=%d ledger=%d, want no duplicate append",
			len(store.SettlementReferences()),
			len(store.OutboxRecords()),
			len(store.RouteStorageLedgerEntries()))
	}
}

func TestSettlementDurableCommitStoreRejectsConflictingReferenceReuse(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if _, err := store.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.RouteStorageLedger = cloneRouteStorageLedgerEntries(plan.RouteStorageLedger)
	conflict.RouteStorageLedger[0].Quantity++
	_, err := store.ApplySettlementDurableCommitPlan(conflict)
	if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
		t.Fatalf("conflicting ApplySettlementDurableCommitPlan() error = %v, want ErrInvalidSettlementDurableCommit", err)
	}
	if len(store.SettlementReferences()) != 1 ||
		len(store.OutboxRecords()) != len(plan.Outbox.OutboxRecords) ||
		len(store.RouteStorageLedgerEntries()) != len(plan.RouteStorageLedger) {
		t.Fatalf("durable store mutated after conflict refs=%d outbox=%d ledger=%d",
			len(store.SettlementReferences()),
			len(store.OutboxRecords()),
			len(store.RouteStorageLedgerEntries()))
	}
}

func TestSettlementDurableCommitStoreRejectsInvalidPlanWithoutMutation(t *testing.T) {
	valid := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if result, err := store.ApplySettlementDurableCommitPlan(SettlementDurableCommitPlan{}); err != nil || result.Reference != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan(no-op) = %+v/%v, want empty nil", result, err)
	}

	partialNoOp := SettlementDurableCommitPlan{}
	partialNoOp.Reference.Kind = SettlementKindRoute
	_, err := store.ApplySettlementDurableCommitPlan(partialNoOp)
	if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
		t.Fatalf("partial no-op ApplySettlementDurableCommitPlan() error = %v, want ErrInvalidSettlementDurableCommit", err)
	}

	invalid := valid
	invalid.Outbox.OutboxRecords = cloneProductionOutboxRecords(valid.Outbox.OutboxRecords)
	invalid.Outbox.OutboxRecords[0].Status = ProductionOutboxStatusPublished
	_, err = store.ApplySettlementDurableCommitPlan(invalid)
	if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
		t.Fatalf("invalid ApplySettlementDurableCommitPlan() error = %v, want ErrInvalidSettlementDurableCommit", err)
	}

	conflictingEmbeddedReference := valid
	conflictingEmbeddedReference.Outbox.Reference.SettlementWindow = "wrong-window"
	_, err = store.ApplySettlementDurableCommitPlan(conflictingEmbeddedReference)
	if !errors.Is(err, ErrInvalidSettlementDurableCommit) {
		t.Fatalf("conflicting embedded reference ApplySettlementDurableCommitPlan() error = %v, want ErrInvalidSettlementDurableCommit", err)
	}
	if len(store.SettlementReferences()) != 0 || len(store.OutboxRecords()) != 0 || len(store.RouteStorageLedgerEntries()) != 0 {
		t.Fatalf("durable store rows after invalid plan refs=%d outbox=%d ledger=%d, want empty",
			len(store.SettlementReferences()),
			len(store.OutboxRecords()),
			len(store.RouteStorageLedgerEntries()))
	}
}

func TestSettlementDurableCommitStoreReturnsDetachedRows(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	result, err := store.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	result.Reference.RouteID = "route-mutated"
	result.OutboxRecords[0].OutboxID = "outbox-mutated"
	result.RouteStorageLedger[0].LedgerID = "ledger-mutated"

	references := store.SettlementReferences()
	outbox := store.OutboxRecords()
	ledger := store.RouteStorageLedgerEntries()
	if references[0].RouteID == "route-mutated" || outbox[0].OutboxID == "outbox-mutated" || ledger[0].LedgerID == "ledger-mutated" {
		t.Fatalf("durable store returned live rows: refs=%+v outbox=%+v ledger=%+v", references, outbox, ledger)
	}
}

func TestProductionSettlementTransactionResultApplyDurableCommit(t *testing.T) {
	transactionStore := newSettlementStore(t, "planet-durable-apply-production", testTime(0), 100, 10)
	addSettlementBuilding(t, transactionStore, "planet-durable-apply-production", "building-durable-apply", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	settledAt := testTime(0).Add(time.Hour)
	result, err := transactionStore.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-durable-apply-production",
		SettledAt: settledAt,
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}
	durableStore := NewInMemorySettlementDurableCommitStore()

	committed, err := result.ApplyDurableCommit(durableStore)
	if err != nil {
		t.Fatalf("ApplyDurableCommit(production) error = %v, want nil", err)
	}
	if committed.Duplicate || committed.Reference == nil || committed.Reference.Kind != SettlementKindProduction {
		t.Fatalf("production durable commit result = %+v, want first production commit", committed)
	}

	duplicateTransaction, err := transactionStore.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-durable-apply-production",
		SettledAt: settledAt,
	})
	if err != nil {
		t.Fatalf("duplicate ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}
	duplicateCommit, err := duplicateTransaction.ApplyDurableCommit(durableStore)
	if err != nil {
		t.Fatalf("duplicate ApplyDurableCommit(production) error = %v, want nil", err)
	}
	if duplicateCommit.Reference != nil || duplicateCommit.Duplicate {
		t.Fatalf("duplicate production durable commit = %+v, want no-op empty result", duplicateCommit)
	}
	if len(durableStore.SettlementReferences()) != 1 || len(durableStore.OutboxRecords()) != len(result.OutboxRecords) {
		t.Fatalf("durable production rows refs=%d outbox=%d, want one transaction commit",
			len(durableStore.SettlementReferences()),
			len(durableStore.OutboxRecords()))
	}
}

func TestRouteSettlementTransactionResultApplyDurableCommit(t *testing.T) {
	last := testRouteNow()
	route := validSettlementRoute(last)
	transactionStore := newRouteSettlementStore(
		t,
		route,
		100,
		[]StoredItem{{ItemID: "refined_alloy", Quantity: 100}},
		100,
		nil,
	)
	result, err := transactionStore.ApplyRouteSettlementTransaction(RouteSettlementTransactionInput{
		OwnerPlayerID: route.OwnerPlayerID,
		RouteID:       route.RouteID,
		SettledAt:     last.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction() error = %v, want nil", err)
	}
	durableStore := NewInMemorySettlementDurableCommitStore()

	committed, err := result.ApplyDurableCommit(durableStore)
	if err != nil {
		t.Fatalf("ApplyDurableCommit(route) error = %v, want nil", err)
	}
	if committed.Duplicate || committed.Reference == nil || committed.Reference.Kind != SettlementKindRoute {
		t.Fatalf("route durable commit result = %+v, want first route commit", committed)
	}
	if len(committed.RouteStorageLedger) != len(result.StorageLedger) {
		t.Fatalf("route durable ledger len = %d, want %d", len(committed.RouteStorageLedger), len(result.StorageLedger))
	}
	if len(durableStore.RouteStorageLedgerEntries()) != len(result.StorageLedger) {
		t.Fatalf("durable route ledger rows = %d, want %d", len(durableStore.RouteStorageLedgerEntries()), len(result.StorageLedger))
	}
}

func TestSettlementTransactionResultApplyDurableCommitRejectsInvalidStoreAndRows(t *testing.T) {
	routeResult := RouteSettlementTransactionResult{}
	if committed, err := routeResult.ApplyDurableCommit(nil); !errors.Is(err, ErrInvalidSettlementDurableCommit) || committed.Reference != nil {
		t.Fatalf("ApplyDurableCommit(nil store) = %+v/%v, want invalid durable commit", committed, err)
	}

	routeResult = RouteSettlementTransactionResult{
		Reference:     &SettlementReferenceRecord{Kind: SettlementKindRoute},
		OutboxRecords: nil,
		StorageLedger: nil,
	}
	if committed, err := routeResult.ApplyDurableCommit(NewInMemorySettlementDurableCommitStore()); !errors.Is(err, ErrInvalidSettlementDurableCommit) || committed.Reference != nil {
		t.Fatalf("ApplyDurableCommit(invalid rows) = %+v/%v, want invalid durable commit", committed, err)
	}
}

func productionDurableCommitPlanForStoreTest(t *testing.T) SettlementDurableCommitPlan {
	t.Helper()
	store := newSettlementStore(t, "planet-durable-store-production", testTime(0), 100, 10)
	addSettlementBuilding(t, store, "planet-durable-store-production", "building-durable-store", ProductionDefinitionIDIronExtractorL1, BuildingStateActive)
	result, err := store.ApplyProductionSettlementTransaction(ProductionSettlementTransactionInput{
		PlanetID:  "planet-durable-store-production",
		SettledAt: testTime(0).Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyProductionSettlementTransaction() error = %v, want nil", err)
	}
	plan, err := result.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan(production) error = %v, want nil", err)
	}
	return plan
}

func routeDurableCommitPlanForStoreTest(t *testing.T) SettlementDurableCommitPlan {
	t.Helper()
	last := testRouteNow()
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
		SettledAt:     last.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ApplyRouteSettlementTransaction() error = %v, want nil", err)
	}
	plan, err := result.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan(route) error = %v, want nil", err)
	}
	return plan
}
