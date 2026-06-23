package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestAutomationRouteDurableStoreCommitsAndReadsRouteRecord(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))

	result, err := plan.ApplyDurableRouteCommit(store)
	if err != nil {
		t.Fatalf("ApplyDurableRouteCommit() error = %v, want nil", err)
	}
	if result.Duplicate || result.Record.Revision != 1 || result.Record.Route.RouteID != route.RouteID {
		t.Fatalf("durable route result = %+v, want revision 1 create record", result)
	}

	byRoute, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord() ok=%v err=%v, want true nil", ok, err)
	}
	byReference, ok, err := store.CommittedAutomationRouteDurableRecordByReference(plan.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecordByReference() ok=%v err=%v, want true nil", ok, err)
	}
	if byRoute.Revision != 1 || byReference.Route.RouteID != route.RouteID || byReference.ReferenceKey != plan.ReferenceKey {
		t.Fatalf("route readbacks = %+v / %+v, want committed create record", byRoute, byReference)
	}
}

func TestAutomationRouteDurableStoreCommitsSourceProductionStateEvidence(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	state, err := NewPlanetProductionState(route.SourcePlanetID, testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	state.EnergyReservedPerHour = route.EnergyCostPerHour
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state

	result, err := store.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if result.Record.SourceProductionState == nil ||
		result.Record.SourceProductionState.PlanetID != route.SourcePlanetID ||
		result.Record.SourceProductionState.EnergyReservedPerHour != route.EnergyCostPerHour {
		t.Fatalf("source production evidence = %+v, want route source reserved %d", result.Record.SourceProductionState, route.EnergyCostPerHour)
	}

	result.Record.SourceProductionState.EnergyReservedPerHour = 999
	recovered, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord() ok=%v err=%v, want true nil", ok, err)
	}
	if recovered.SourceProductionState == nil || recovered.SourceProductionState.EnergyReservedPerHour != route.EnergyCostPerHour {
		t.Fatalf("recovered source production evidence = %+v, want detached reserved %d", recovered.SourceProductionState, route.EnergyCostPerHour)
	}
}

func TestAutomationRouteDurableStoreDuplicateReferenceReplaysWithoutRevisionAdvance(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))

	first, err := store.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("first ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	duplicate, err := store.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("duplicate ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if first.Duplicate || !duplicate.Duplicate || duplicate.Record.Revision != first.Record.Revision {
		t.Fatalf("duplicate route commit first=%+v duplicate=%+v, want duplicate replay same revision", first, duplicate)
	}
	if records := store.AutomationRouteDurableRecords(); len(records) != 1 || records[0].Revision != 1 {
		t.Fatalf("durable route records = %+v, want one revision 1 row", records)
	}
}

func TestAutomationRouteDurableStoreUpdatesWithExpectedRevisionCAS(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	createPlan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	created, err := store.ApplyAutomationRouteDurableCommitPlan(createPlan)
	if err != nil {
		t.Fatalf("create ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}

	updatedRoute := route
	updatedRoute.AmountPerHour = 75
	updatedRoute.UpdatedAt = testTime(3)
	updatePlan := automationRouteDurablePlanForTest(updatedRoute, "route_update:player-1:route-1:request-2", created.Record.Revision, testTime(3))
	updated, err := store.ApplyAutomationRouteDurableCommitPlan(updatePlan)
	if err != nil {
		t.Fatalf("update ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if updated.Record.Revision != 2 || updated.Record.Route.AmountPerHour != 75 {
		t.Fatalf("updated route durable record = %+v, want revision 2 amount 75", updated.Record)
	}

	createdByReference, ok, err := store.CommittedAutomationRouteDurableRecordByReference(createPlan.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecordByReference(create) ok=%v err=%v, want true nil", ok, err)
	}
	if createdByReference.Revision != 1 || createdByReference.Route.AmountPerHour != route.AmountPerHour {
		t.Fatalf("create reference record after update = %+v, want immutable revision 1 route", createdByReference)
	}
	duplicateCreate, err := store.ApplyAutomationRouteDurableCommitPlan(createPlan)
	if err != nil {
		t.Fatalf("duplicate create ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if !duplicateCreate.Duplicate || duplicateCreate.Record.Revision != 1 {
		t.Fatalf("duplicate create result = %+v, want immutable revision 1 replay", duplicateCreate)
	}
}

func TestAutomationRouteDurableStoreRejectsInvalidSourceProductionStateEvidence(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	state, err := NewPlanetProductionState("planet-other", testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state

	_, err = NewInMemoryAutomationRouteDurableStore().ApplyAutomationRouteDurableCommitPlan(plan)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) {
		t.Fatalf("mismatched source production state error = %v, want ErrInvalidAutomationRouteDurableCommit", err)
	}

	state.PlanetID = route.SourcePlanetID
	state.UpdatedAt = testTime(3)
	_, err = NewInMemoryAutomationRouteDurableStore().ApplyAutomationRouteDurableCommitPlan(plan)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) {
		t.Fatalf("stale source production state timestamp error = %v, want ErrInvalidAutomationRouteDurableCommit", err)
	}
}

func TestInMemoryStoreAppliesRouteDurableSourceProductionStateUnderCommit(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	state, err := NewPlanetProductionState(route.SourcePlanetID, testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	state.EnergyReservedPerHour = route.EnergyCostPerHour
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state
	store := NewInMemoryStore()

	if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	stored, ok, err := store.ProductionState(route.SourcePlanetID)
	if err != nil || !ok {
		t.Fatalf("ProductionState(%q) ok=%v err=%v, want true nil", route.SourcePlanetID, ok, err)
	}
	if stored.EnergyReservedPerHour != route.EnergyCostPerHour {
		t.Fatalf("stored source production reserved = %d, want %d", stored.EnergyReservedPerHour, route.EnergyCostPerHour)
	}
}

func TestAutomationRouteDurableStoreRejectsStaleRevisionWithoutMutation(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	if _, err := store.ApplyAutomationRouteDurableCommitPlan(automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))); err != nil {
		t.Fatalf("create ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}

	staleRoute := route
	staleRoute.AmountPerHour = 99
	staleRoute.UpdatedAt = testTime(3)
	_, err := store.ApplyAutomationRouteDurableCommitPlan(automationRouteDurablePlanForTest(staleRoute, "route_update:player-1:route-1:stale", 0, testTime(3)))
	if !errors.Is(err, ErrStaleAutomationRouteDurableCommit) {
		t.Fatalf("stale ApplyAutomationRouteDurableCommitPlan() error = %v, want ErrStaleAutomationRouteDurableCommit", err)
	}
	record, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord() ok=%v err=%v, want true nil", ok, err)
	}
	if record.Revision != 1 || record.Route.AmountPerHour != route.AmountPerHour {
		t.Fatalf("route durable record after stale update = %+v, want original revision 1", record)
	}
}

func TestAutomationRouteDurableStoreRejectsConflictingReferenceReuse(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}

	conflictRoute := route
	conflictRoute.AmountPerHour = 120
	conflictRoute.UpdatedAt = testTime(3)
	conflict := automationRouteDurablePlanForTest(conflictRoute, plan.ReferenceKey, 1, testTime(3))
	_, err := store.ApplyAutomationRouteDurableCommitPlan(conflict)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) {
		t.Fatalf("conflicting reference ApplyAutomationRouteDurableCommitPlan() error = %v, want ErrInvalidAutomationRouteDurableCommit", err)
	}
	if records := store.AutomationRouteDurableRecords(); len(records) != 1 || records[0].Route.AmountPerHour != route.AmountPerHour {
		t.Fatalf("route durable records after conflict = %+v, want original row only", records)
	}
}

func TestAutomationRouteDurableStoreRejectsConflictingSourceProductionReferenceReuse(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	state, err := NewPlanetProductionState(route.SourcePlanetID, testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	state.EnergyReservedPerHour = route.EnergyCostPerHour
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state
	if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.SourceProductionState = cloneProductionStatePointer(plan.SourceProductionState)
	conflict.SourceProductionState.EnergyReservedPerHour++
	_, err = store.ApplyAutomationRouteDurableCommitPlan(conflict)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) {
		t.Fatalf("conflicting source state ApplyAutomationRouteDurableCommitPlan() error = %v, want ErrInvalidAutomationRouteDurableCommit", err)
	}
}

func TestAutomationRouteDurableStoreDuplicateReplayRejectsCorruptReferenceRow(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}

	corrupt := store.references[plan.ReferenceKey]
	corrupt.Revision = 0
	store.references[plan.ReferenceKey] = corrupt

	duplicate, err := store.ApplyAutomationRouteDurableCommitPlan(plan)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || duplicate.Record.Route.RouteID != "" {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan(corrupt duplicate) = %+v/%v, want invalid durable commit", duplicate, err)
	}
	stored := store.references[plan.ReferenceKey]
	if stored.Revision != 0 {
		t.Fatalf("corrupt reference row mutated by failed duplicate replay: %+v", stored)
	}
	routeRecord := store.records[route.RouteID]
	if routeRecord.Revision != 1 {
		t.Fatalf("route record mutated by failed duplicate replay: %+v", routeRecord)
	}
}

func TestAutomationRouteDurableStoreUpdateRejectsCorruptCurrentRow(t *testing.T) {
	cases := map[string]func(*testing.T, *InMemoryAutomationRouteDurableStore, AutomationRouteDurableCommitPlan){
		"identity drift": func(t *testing.T, store *InMemoryAutomationRouteDurableStore, createPlan AutomationRouteDurableCommitPlan) {
			corrupt := store.records[createPlan.Route.RouteID]
			corrupt.Route.OwnerPlayerID = "player-other"
			store.records[createPlan.Route.RouteID] = corrupt
		},
		"reference key drift": func(t *testing.T, store *InMemoryAutomationRouteDurableStore, createPlan AutomationRouteDurableCommitPlan) {
			corrupt := store.records[createPlan.Route.RouteID]
			corrupt.ReferenceKey = "route_create:player-1:route-other"
			store.records[createPlan.Route.RouteID] = corrupt
		},
	}
	for name, corruptStore := range cases {
		t.Run(name, func(t *testing.T) {
			route := validSettlementRoute(testTime(1))
			store := NewInMemoryAutomationRouteDurableStore()
			createPlan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
			if _, err := store.ApplyAutomationRouteDurableCommitPlan(createPlan); err != nil {
				t.Fatalf("ApplyAutomationRouteDurableCommitPlan(create) error = %v, want nil", err)
			}
			corruptStore(t, store, createPlan)
			corruptBefore := store.records[route.RouteID]

			updatedRoute := route
			updatedRoute.AmountPerHour = 75
			updatedRoute.UpdatedAt = testTime(3)
			updatePlan := automationRouteDurablePlanForTest(updatedRoute, "route_update:player-1:route-1:request-2", 1, testTime(3))
			result, err := store.ApplyAutomationRouteDurableCommitPlan(updatePlan)
			if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || result.Record.Route.RouteID != "" {
				t.Fatalf("ApplyAutomationRouteDurableCommitPlan(corrupt current row) = %+v/%v, want invalid durable commit", result, err)
			}
			stored := store.records[route.RouteID]
			if !automationRouteDurableRecordsEqual(stored, corruptBefore) {
				t.Fatalf("corrupt route row mutated by failed update: before=%+v after=%+v", corruptBefore, stored)
			}
			if _, ok := store.references[updatePlan.ReferenceKey]; ok {
				t.Fatalf("update reference row committed despite corrupt current row: %+v", store.references[updatePlan.ReferenceKey])
			}
		})
	}
}

func TestAutomationRouteDurableStoreRejectsEnabledRouteWithoutReservedSourceEnergy(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	state, err := NewPlanetProductionState(route.SourcePlanetID, testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	state.EnergyReservedPerHour = route.EnergyCostPerHour - 1
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state

	_, err = NewInMemoryAutomationRouteDurableStore().ApplyAutomationRouteDurableCommitPlan(plan)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) {
		t.Fatalf("insufficient source energy evidence error = %v, want ErrInvalidAutomationRouteDurableCommit", err)
	}
}

func TestAutomationRouteDurableStoreReturnsDetachedRecords(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	result, err := store.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	result.Record.Route.AmountPerHour = 999

	records := store.AutomationRouteDurableRecords()
	records[0].Route.RouteID = "route-mutated"
	recovered, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if err != nil || !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord() ok=%v err=%v, want true nil", ok, err)
	}
	if recovered.Route.AmountPerHour == 999 || recovered.Route.RouteID == "route-mutated" {
		t.Fatalf("route durable store returned live record: %+v", recovered)
	}
}

func TestAutomationRouteDurableStoreReadsOwnerRoutesInDeterministicOrder(t *testing.T) {
	store := NewInMemoryAutomationRouteDurableStore()
	routeB := validSettlementRoute(testTime(1))
	routeB.RouteID = "route-b"
	routeA := validSettlementRoute(testTime(1))
	routeA.RouteID = "route-a"
	other := validSettlementRoute(testTime(1))
	other.RouteID = "route-other"
	other.OwnerPlayerID = "player-2"

	for _, plan := range []AutomationRouteDurableCommitPlan{
		automationRouteDurablePlanForTest(routeB, "route_create:player-1:route-b", 0, testTime(2)),
		automationRouteDurablePlanForTest(other, "route_create:player-2:route-other", 0, testTime(2)),
		automationRouteDurablePlanForTest(routeA, "route_create:player-1:route-a", 0, testTime(2)),
	} {
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan(%s) error = %v, want nil", plan.Route.RouteID, err)
		}
	}

	records, err := store.CommittedAutomationRouteDurableRecordsForOwner("player-1")
	if err != nil {
		t.Fatalf("CommittedAutomationRouteDurableRecordsForOwner() error = %v, want nil", err)
	}
	if len(records) != 2 || records[0].Route.RouteID != "route-a" || records[1].Route.RouteID != "route-b" {
		t.Fatalf("owner route records = %+v, want route-a then route-b only", records)
	}
	records[0].Route.RouteID = "route-mutated"
	again, err := store.CommittedAutomationRouteDurableRecordsForOwner("player-1")
	if err != nil {
		t.Fatalf("CommittedAutomationRouteDurableRecordsForOwner(second) error = %v, want nil", err)
	}
	if again[0].Route.RouteID == "route-mutated" {
		t.Fatalf("owner route readback returned live records: %+v", again)
	}
}

func TestAutomationRouteDurableStoreReadbacksRejectCorruptCommittedRows(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	state, err := NewPlanetProductionState(route.SourcePlanetID, testTime(1), 40, testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	state.EnergyReservedPerHour = route.EnergyCostPerHour
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	plan.SourceProductionState = &state

	t.Run("route key mismatch", func(t *testing.T) {
		store := NewInMemoryAutomationRouteDurableStore()
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
		}
		corrupt := store.records[route.RouteID]
		corrupt.Route.RouteID = "route-corrupt"
		store.records[route.RouteID] = corrupt

		record, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
			t.Fatalf("CommittedAutomationRouteDurableRecord(corrupt) = %+v/%v/%v, want invalid durable row", record, ok, err)
		}
		records, err := store.CommittedAutomationRouteDurableRecordsForOwner(route.OwnerPlayerID)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || len(records) != 0 {
			t.Fatalf("CommittedAutomationRouteDurableRecordsForOwner(corrupt) = %+v/%v, want invalid durable row", records, err)
		}
		if stored := store.records[route.RouteID]; stored.Route.RouteID != "route-corrupt" {
			t.Fatalf("corrupt route row mutated by failed readback: %+v", stored)
		}
	})

	t.Run("reference key mismatch", func(t *testing.T) {
		store := NewInMemoryAutomationRouteDurableStore()
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
		}
		corrupt := store.references[plan.ReferenceKey]
		corrupt.ReferenceKey = "route_create:player-1:route-other"
		store.references[plan.ReferenceKey] = corrupt

		record, ok, err := store.CommittedAutomationRouteDurableRecordByReference(plan.ReferenceKey)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
			t.Fatalf("CommittedAutomationRouteDurableRecordByReference(corrupt) = %+v/%v/%v, want invalid durable row", record, ok, err)
		}
		if stored := store.references[plan.ReferenceKey]; stored.ReferenceKey == plan.ReferenceKey {
			t.Fatalf("corrupt reference row mutated by failed readback: %+v", stored)
		}
	})

	t.Run("reference row points at wrong route record", func(t *testing.T) {
		store := NewInMemoryAutomationRouteDurableStore()
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
		}
		otherRoute := route
		otherRoute.RouteID = "route-other"
		otherPlan := automationRouteDurablePlanForTest(otherRoute, "route_create:player-1:route-other", 0, testTime(2))
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(otherPlan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan(other) error = %v, want nil", err)
		}
		corrupt := store.references[plan.ReferenceKey]
		corrupt.Route = otherRoute
		store.references[plan.ReferenceKey] = corrupt

		record, ok, err := store.CommittedAutomationRouteDurableRecordByReference(plan.ReferenceKey)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
			t.Fatalf("CommittedAutomationRouteDurableRecordByReference(wrong route) = %+v/%v/%v, want invalid durable row", record, ok, err)
		}
	})

	t.Run("current route row identity drift", func(t *testing.T) {
		store := NewInMemoryAutomationRouteDurableStore()
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
		}
		corrupt := store.records[route.RouteID]
		corrupt.Route.OwnerPlayerID = "player-other"
		store.records[route.RouteID] = corrupt

		record, ok, err := store.CommittedAutomationRouteDurableRecordByReference(plan.ReferenceKey)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
			t.Fatalf("CommittedAutomationRouteDurableRecordByReference(identity drift) = %+v/%v/%v, want invalid durable row", record, ok, err)
		}
	})

	t.Run("source production evidence mismatch", func(t *testing.T) {
		store := NewInMemoryAutomationRouteDurableStore()
		if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
			t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
		}
		corrupt := store.records[route.RouteID]
		corrupt.SourceProductionState = cloneProductionStatePointer(plan.SourceProductionState)
		corrupt.SourceProductionState.EnergyReservedPerHour = route.EnergyCostPerHour - 1
		store.records[route.RouteID] = corrupt

		record, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
		if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
			t.Fatalf("CommittedAutomationRouteDurableRecord(corrupt source state) = %+v/%v/%v, want invalid durable row", record, ok, err)
		}
	})
}

func TestInMemoryStoreAutomationRouteDurableReadbacksRejectCorruptRows(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	route.Enabled = false
	store := NewInMemoryStore()
	plan := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	if _, err := store.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	corrupt := store.routeDurableRecords[route.RouteID]
	corrupt.Revision = 0
	store.routeDurableRecords[route.RouteID] = corrupt

	record, ok, err := store.CommittedAutomationRouteDurableRecord(route.RouteID)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || ok || !record.Route.RouteID.IsZero() {
		t.Fatalf("CommittedAutomationRouteDurableRecord(corrupt runtime store) = %+v/%v/%v, want invalid durable row", record, ok, err)
	}
	records, err := store.CommittedAutomationRouteDurableRecordsForOwner(route.OwnerPlayerID)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || len(records) != 0 {
		t.Fatalf("CommittedAutomationRouteDurableRecordsForOwner(corrupt runtime store) = %+v/%v, want invalid durable row", records, err)
	}
}

func TestInMemoryStoreRouteDuplicateReplayRejectsCorruptDurableReference(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	route.Enabled = false
	store := NewInMemoryStore()
	requestID := foundation.RequestID("request-enable-route-1")
	store.ensureMapsLocked()
	store.routes[route.RouteID] = cloneAutomationRoute(route)
	ensureRouteProductionStateForTest(t, store, route.SourcePlanetID, 100, testTime(1))

	first, err := store.EnableRouteForOwnerWithRequest(route.OwnerPlayerID, route.RouteID, testTime(2), requestID)
	if err != nil {
		t.Fatalf("EnableRoute(first) error = %v, want nil", err)
	}
	if !first.Changed || !first.Route.Enabled {
		t.Fatalf("EnableRoute(first) = %+v, want enabled mutation", first)
	}
	referenceKey, err := foundation.RouteEnableIdempotencyKey(route.OwnerPlayerID, route.RouteID, requestID)
	if err != nil {
		t.Fatalf("RouteEnableIdempotencyKey() error = %v, want nil", err)
	}
	corrupt := store.routeDurableReferences[referenceKey]
	corrupt.Revision = 0
	store.routeDurableReferences[referenceKey] = corrupt

	duplicate, err := store.EnableRouteForOwnerWithRequest(route.OwnerPlayerID, route.RouteID, testTime(3), requestID)
	if !errors.Is(err, ErrInvalidAutomationRouteDurableCommit) || duplicate.Route.RouteID != "" {
		t.Fatalf("EnableRoute(duplicate corrupt reference) = %+v/%v, want invalid durable commit", duplicate, err)
	}
	stored, ok := store.routeDurableReferences[referenceKey]
	if !ok || stored.Revision != 0 {
		t.Fatalf("corrupt durable reference mutated by failed duplicate replay: %+v ok=%v", stored, ok)
	}
}

func TestAutomationRouteDurableStoreRejectsInvalidPlanWithoutMutation(t *testing.T) {
	route := validSettlementRoute(testTime(1))
	store := NewInMemoryAutomationRouteDurableStore()
	if result, err := store.ApplyAutomationRouteDurableCommitPlan(AutomationRouteDurableCommitPlan{}); err != nil || !result.Record.Route.RouteID.IsZero() {
		t.Fatalf("empty ApplyAutomationRouteDurableCommitPlan() = %+v/%v, want no-op nil", result, err)
	}

	invalid := automationRouteDurablePlanForTest(route, "route_create:player-1:route-1", 0, testTime(2))
	invalid.RecordedAt = time.Time{}
	_, err := store.ApplyAutomationRouteDurableCommitPlan(invalid)
	if !errors.Is(err, ErrZeroProductionTimestamp) {
		t.Fatalf("invalid ApplyAutomationRouteDurableCommitPlan() error = %v, want ErrZeroProductionTimestamp", err)
	}
	if records := store.AutomationRouteDurableRecords(); len(records) != 0 {
		t.Fatalf("route durable records after invalid plan = %+v, want none", records)
	}
}

func automationRouteDurablePlanForTest(
	route AutomationRoute,
	referenceKey foundation.IdempotencyKey,
	expectedRevision uint64,
	recordedAt time.Time,
) AutomationRouteDurableCommitPlan {
	return AutomationRouteDurableCommitPlan{
		Route:            route,
		ReferenceKey:     referenceKey,
		ExpectedRevision: expectedRevision,
		RecordedAt:       recordedAt,
	}
}
