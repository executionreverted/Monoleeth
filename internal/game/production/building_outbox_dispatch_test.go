package production

import (
	"errors"
	"testing"
)

func TestBuildingMutationOutboxDispatchPlanFromBuildMutation(t *testing.T) {
	durable := buildingDurableCommitPlanForStoreTest(t)

	dispatch, err := NewBuildingMutationOutboxDispatchPlan(&durable.Reference, durable.OutboxRecords)
	if err != nil {
		t.Fatalf("NewBuildingMutationOutboxDispatchPlan() error = %v, want nil", err)
	}
	if dispatch.Reference.ReferenceKey != durable.Reference.ReferenceKey ||
		dispatch.Reference.Operation != BuildingMutationBuild ||
		len(dispatch.OutboxRecords) != len(durable.OutboxRecords) {
		t.Fatalf("dispatch plan = %+v, want build mutation dispatch for %+v", dispatch, durable.Reference)
	}
	assertBuildingMutationOutboxReferences(t, dispatch.OutboxRecords, durable.Reference.ReferenceKey, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)

	dispatch.OutboxRecords[0].Event.Payload[0] = 'x'
	replayed, err := NewBuildingMutationOutboxDispatchPlan(&durable.Reference, durable.OutboxRecords)
	if err != nil {
		t.Fatalf("NewBuildingMutationOutboxDispatchPlan(replay) error = %v, want nil", err)
	}
	if replayed.OutboxRecords[0].Event.Payload[0] == 'x' {
		t.Fatal("mutating dispatch plan payload changed source rows")
	}
}

func TestBuildingMutationOutboxDispatchPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewBuildingMutationOutboxDispatchPlan(nil, nil); err != nil || len(plan.OutboxRecords) != 0 || !plan.Reference.ReferenceKey.IsZero() {
		t.Fatalf("NewBuildingMutationOutboxDispatchPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	durable := buildingDurableCommitPlanForStoreTest(t)
	cases := map[string]func([]ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord){
		"missing reference": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			return nil, records
		},
		"empty outbox": func([]ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			return &durable.Reference, nil
		},
		"published row": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			records[0].Status = ProductionOutboxStatusPublished
			return &durable.Reference, records
		},
		"claimed row": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			records[0].ClaimedAt = testTime(12)
			records[0].ClaimToken = "claim-token"
			return &durable.Reference, records
		},
		"mismatched reference": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			records[0].ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
			return &durable.Reference, records
		},
		"settlement window": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			records[0].SettlementWindow = "planet-1:window"
			return &durable.Reference, records
		},
		"empty payload": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			records[0].Event.Payload = nil
			return &durable.Reference, records
		},
		"no building update row": func(records []ProductionOutboxRecord) (*BuildingMutationReferenceRecord, []ProductionOutboxRecord) {
			for index := range records {
				records[index].Event.Type = EventPlanetStorageUpdated
			}
			return &durable.Reference, records
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			ref, records := mutate(cloneProductionOutboxRecords(durable.OutboxRecords))
			_, err := NewBuildingMutationOutboxDispatchPlan(ref, records)
			if !errors.Is(err, ErrInvalidBuildingMutationOutboxDispatch) {
				t.Fatalf("NewBuildingMutationOutboxDispatchPlan(%s) error = %v, want ErrInvalidBuildingMutationOutboxDispatch", name, err)
			}
		})
	}
}
