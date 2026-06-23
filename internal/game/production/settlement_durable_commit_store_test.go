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

func TestSettlementDurableCommitStoreReadsCommittedPlanByReference(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if _, err := store.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	recovered, ok, err := store.CommittedSettlementDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedSettlementDurableCommitPlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		len(recovered.Outbox.OutboxRecords) != len(plan.Outbox.OutboxRecords) ||
		len(recovered.RouteStorageLedger) != len(plan.RouteStorageLedger) {
		t.Fatalf("recovered durable plan = %+v, want committed plan %+v", recovered, plan)
	}

	recovered.Reference.RouteID = "route-mutated"
	recovered.Outbox.OutboxRecords[0].OutboxID = "outbox-mutated"
	recovered.RouteStorageLedger[0].LedgerID = "ledger-mutated"
	again, ok, err := store.CommittedSettlementDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedSettlementDurableCommitPlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if again.Reference.RouteID == "route-mutated" ||
		again.Outbox.OutboxRecords[0].OutboxID == "outbox-mutated" ||
		again.RouteStorageLedger[0].LedgerID == "ledger-mutated" {
		t.Fatalf("recovered durable plan reused mutable rows: %+v", again)
	}
}

func TestSettlementDurableCommitStoreReadsCommittedDispatchPlanByReference(t *testing.T) {
	cases := []struct {
		name      string
		plan      SettlementDurableCommitPlan
		eventType string
	}{
		{
			name:      "production",
			plan:      productionDurableCommitPlanForStoreTest(t),
			eventType: EventPlanetProductionSettled,
		},
		{
			name:      "route",
			plan:      routeDurableCommitPlanForStoreTest(t),
			eventType: EventRouteTransferSettled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewInMemorySettlementDurableCommitStore()
			if _, err := store.ApplySettlementDurableCommitPlan(tc.plan); err != nil {
				t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
			}

			dispatch, ok, err := store.CommittedSettlementOutboxDispatchPlan(tc.plan.Reference.ReferenceKey)
			if err != nil || !ok {
				t.Fatalf("CommittedSettlementOutboxDispatchPlan() = ok %v err %v, want true nil", ok, err)
			}
			if dispatch.Reference.ReferenceKey != tc.plan.Reference.ReferenceKey ||
				dispatch.Reference.SettlementWindow != tc.plan.Reference.SettlementWindow ||
				len(dispatch.OutboxRecords) != len(tc.plan.Outbox.OutboxRecords) {
				t.Fatalf("dispatch plan = %+v, want committed outbox dispatch for %+v", dispatch, tc.plan.Reference)
			}
			assertOutboxRecordEvidence(t, dispatch.OutboxRecords, tc.eventType, tc.plan.Reference.ReferenceKey, tc.plan.Reference.SettlementWindow)
		})
	}
}

func TestSettlementDurableCommitStoreReadsCommittedProductionWindowPlans(t *testing.T) {
	plan := productionDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if _, err := store.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	recovered, ok, err := store.CommittedProductionSettlementDurableCommitPlan(plan.Reference.PlanetID, plan.Reference.SettlementWindow)
	if err != nil || !ok {
		t.Fatalf("CommittedProductionSettlementDurableCommitPlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		recovered.Reference.Kind != SettlementKindProduction ||
		recovered.Reference.PlanetID != plan.Reference.PlanetID ||
		recovered.Reference.SettlementWindow != plan.Reference.SettlementWindow {
		t.Fatalf("production window durable plan = %+v, want committed production window %+v", recovered.Reference, plan.Reference)
	}
	dispatch, ok, err := store.CommittedProductionSettlementOutboxDispatchPlan(plan.Reference.PlanetID, plan.Reference.SettlementWindow)
	if err != nil || !ok {
		t.Fatalf("CommittedProductionSettlementOutboxDispatchPlan() = ok %v err %v, want true nil", ok, err)
	}
	assertOutboxRecordEvidence(t, dispatch.OutboxRecords, EventPlanetProductionSettled, plan.Reference.ReferenceKey, plan.Reference.SettlementWindow)

	if recovered, ok, err := store.CommittedProductionSettlementDurableCommitPlan(plan.Reference.PlanetID, "missing-window"); err != nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedProductionSettlementDurableCommitPlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedProductionSettlementOutboxDispatchPlan("", plan.Reference.SettlementWindow); err == nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedProductionSettlementOutboxDispatchPlan(invalid planet) = %+v/%v/%v, want error false empty", dispatch, ok, err)
	}
}

func TestSettlementDurableCommitStoreReadsCommittedRouteWindowPlans(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if _, err := store.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	recovered, ok, err := store.CommittedRouteSettlementDurableCommitPlan(plan.Reference.RouteID, plan.Reference.SettlementWindow)
	if err != nil || !ok {
		t.Fatalf("CommittedRouteSettlementDurableCommitPlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		recovered.Reference.Kind != SettlementKindRoute ||
		recovered.Reference.RouteID != plan.Reference.RouteID ||
		recovered.Reference.SettlementWindow != plan.Reference.SettlementWindow ||
		len(recovered.RouteStorageLedger) != len(plan.RouteStorageLedger) {
		t.Fatalf("route window durable plan = %+v, want committed route window %+v", recovered, plan)
	}
	dispatch, ok, err := store.CommittedRouteSettlementOutboxDispatchPlan(plan.Reference.RouteID, plan.Reference.SettlementWindow)
	if err != nil || !ok {
		t.Fatalf("CommittedRouteSettlementOutboxDispatchPlan() = ok %v err %v, want true nil", ok, err)
	}
	assertOutboxRecordEvidence(t, dispatch.OutboxRecords, EventRouteTransferSettled, plan.Reference.ReferenceKey, plan.Reference.SettlementWindow)

	if recovered, ok, err := store.CommittedRouteSettlementDurableCommitPlan(plan.Reference.RouteID, "missing-window"); err != nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedRouteSettlementDurableCommitPlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedRouteSettlementOutboxDispatchPlan("", plan.Reference.SettlementWindow); err == nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedRouteSettlementOutboxDispatchPlan(invalid route) = %+v/%v/%v, want error false empty", dispatch, ok, err)
	}
}

func TestSettlementDurableCommitStorePublishesCommittedSettlementOutboxRows(t *testing.T) {
	cases := []struct {
		name      string
		plan      SettlementDurableCommitPlan
		eventType string
	}{
		{
			name:      "production",
			plan:      productionDurableCommitPlanForStoreTest(t),
			eventType: EventPlanetProductionSettled,
		},
		{
			name:      "route",
			plan:      routeDurableCommitPlanForStoreTest(t),
			eventType: EventRouteTransferSettled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewInMemorySettlementDurableCommitStore()
			if _, err := store.ApplySettlementDurableCommitPlan(tc.plan); err != nil {
				t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
			}
			ledgerBefore := store.RouteStorageLedgerEntries()

			published, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
				Store:       store,
				Limit:       10,
				ClaimedAt:   testTime(50),
				CompletedAt: testTime(51),
				Publish: func(record ProductionOutboxRecord) error {
					hasEvidence := !record.ReferenceKey.IsZero() || record.SettlementWindow != ""
					if record.Status != ProductionOutboxStatusInFlight || record.ClaimToken == "" {
						t.Fatalf("published settlement outbox row = %+v, want claimed row", record)
					}
					if hasEvidence &&
						(record.ReferenceKey != tc.plan.Reference.ReferenceKey ||
							record.SettlementWindow != tc.plan.Reference.SettlementWindow) {
						t.Fatalf("published settlement outbox evidence = %+v, want committed reference/window", record)
					}
					return nil
				},
			})
			if err != nil {
				t.Fatalf("PublishPendingProductionOutbox() error = %v, want nil", err)
			}
			if len(published) != len(tc.plan.Outbox.OutboxRecords) {
				t.Fatalf("published rows len = %d, want %d", len(published), len(tc.plan.Outbox.OutboxRecords))
			}
			for _, result := range published {
				if !result.Published || result.Failed || result.StaleClaim {
					t.Fatalf("publish result = %+v, want published", result)
				}
				hasEvidence := !result.Record.ReferenceKey.IsZero() || result.Record.SettlementWindow != ""
				if result.Record.Status != ProductionOutboxStatusPublished {
					t.Fatalf("published durable row = %+v, want published", result.Record)
				}
				if hasEvidence &&
					(result.Record.ReferenceKey != tc.plan.Reference.ReferenceKey ||
						result.Record.SettlementWindow != tc.plan.Reference.SettlementWindow) {
					t.Fatalf("published durable row evidence = %+v, want committed evidence", result.Record)
				}
			}
			assertOutboxRecordEvidence(t, store.OutboxRecords(), tc.eventType, tc.plan.Reference.ReferenceKey, tc.plan.Reference.SettlementWindow)
			if got := store.RouteStorageLedgerEntries(); len(got) != len(ledgerBefore) {
				t.Fatalf("route ledger rows after publish = %+v, want unchanged %+v", got, ledgerBefore)
			}
		})
	}
}

func TestSettlementDurableCommitStorePublisherRejectsStaleClaimTokens(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()
	if _, err := store.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(60))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable settlement outbox rows = %+v, want one in-flight row", claimed)
	}
	if _, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, "wrong-token", testTime(61)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxPublished(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	if _, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, "wrong-token", "wrong", testTime(61)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxFailed(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	rows := store.OutboxRecords()
	if rows[0].Status != ProductionOutboxStatusInFlight || rows[0].ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("durable outbox after stale token = %+v, want original in-flight row", rows[0])
	}

	released, err := store.ReleaseExpiredProductionOutboxRecords(1, testTime(61), testTime(62))
	if err != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(released) != 1 ||
		released[0].Status != ProductionOutboxStatusPending ||
		released[0].ClaimToken != "" ||
		released[0].ReferenceKey != plan.Reference.ReferenceKey ||
		released[0].SettlementWindow != plan.Reference.SettlementWindow {
		t.Fatalf("released durable settlement outbox rows = %+v, want pending committed evidence", released)
	}
	if _, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(63)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxPublished(stale token after release) = ok %v err %v, want false nil", ok, err)
	}
}

func TestSettlementDurableCommitStoreReadbackMissingAndInvalidReferences(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	store := NewInMemorySettlementDurableCommitStore()

	if recovered, ok, err := store.CommittedSettlementDurableCommitPlan(plan.Reference.ReferenceKey); err != nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedSettlementDurableCommitPlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedSettlementOutboxDispatchPlan(plan.Reference.ReferenceKey); err != nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedSettlementOutboxDispatchPlan(missing) = %+v/%v/%v, want empty false nil", dispatch, ok, err)
	}

	if recovered, ok, err := store.CommittedSettlementDurableCommitPlan(""); err == nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedSettlementDurableCommitPlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedSettlementOutboxDispatchPlan(""); err == nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedSettlementOutboxDispatchPlan(invalid) = %+v/%v/%v, want error false empty", dispatch, ok, err)
	}
}

func TestSettlementDurableCommitPlanApplyDurableCommit(t *testing.T) {
	cases := []struct {
		name string
		plan SettlementDurableCommitPlan
		kind SettlementKind
	}{
		{
			name: "production",
			plan: productionDurableCommitPlanForStoreTest(t),
			kind: SettlementKindProduction,
		},
		{
			name: "route",
			plan: routeDurableCommitPlanForStoreTest(t),
			kind: SettlementKindRoute,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewInMemorySettlementDurableCommitStore()

			committed, err := tc.plan.ApplyDurableCommit(store)
			if err != nil {
				t.Fatalf("ApplyDurableCommit() error = %v, want nil", err)
			}
			if committed.Duplicate || committed.Reference == nil || committed.Reference.Kind != tc.kind {
				t.Fatalf("ApplyDurableCommit() = %+v, want first %s commit", committed, tc.kind)
			}

			duplicate, err := tc.plan.ApplyDurableCommit(store)
			if err != nil {
				t.Fatalf("duplicate ApplyDurableCommit() error = %v, want nil", err)
			}
			if !duplicate.Duplicate || len(store.SettlementReferences()) != 1 {
				t.Fatalf("duplicate ApplyDurableCommit() = %+v refs %d, want duplicate without append", duplicate, len(store.SettlementReferences()))
			}
		})
	}
}

func TestSettlementDurableCommitPlanApplyDurableCommitRejectsInvalidInputs(t *testing.T) {
	plan := routeDurableCommitPlanForStoreTest(t)
	if result, err := plan.ApplyDurableCommit(nil); !errors.Is(err, ErrInvalidSettlementDurableCommit) || result.Reference != nil {
		t.Fatalf("ApplyDurableCommit(nil store) = %+v/%v, want invalid durable commit", result, err)
	}

	invalid := plan
	invalid.Outbox.OutboxRecords = cloneProductionOutboxRecords(plan.Outbox.OutboxRecords)
	invalid.Outbox.OutboxRecords[0].Status = ProductionOutboxStatusPublished
	store := NewInMemorySettlementDurableCommitStore()
	if result, err := invalid.ApplyDurableCommit(store); !errors.Is(err, ErrInvalidSettlementDurableCommit) || result.Reference != nil {
		t.Fatalf("ApplyDurableCommit(invalid plan) = %+v/%v, want invalid durable commit", result, err)
	}
	if len(store.SettlementReferences()) != 0 || len(store.OutboxRecords()) != 0 || len(store.RouteStorageLedgerEntries()) != 0 {
		t.Fatalf("durable store rows after invalid ApplyDurableCommit refs=%d outbox=%d ledger=%d, want empty",
			len(store.SettlementReferences()),
			len(store.OutboxRecords()),
			len(store.RouteStorageLedgerEntries()))
	}

	if result, err := (SettlementDurableCommitPlan{}).ApplyDurableCommit(store); err != nil || result.Reference != nil {
		t.Fatalf("ApplyDurableCommit(no-op) = %+v/%v, want empty nil", result, err)
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
