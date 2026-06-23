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
