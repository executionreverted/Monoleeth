package production

import (
	"errors"
	"testing"
)

func TestBuildingMutationDurableCommitPlanFromBuildMutation(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)

	if plan.Reference.Operation != BuildingMutationBuild || plan.Reference.ReferenceKey.IsZero() {
		t.Fatalf("durable reference = %+v, want build reference", plan.Reference)
	}
	assertBuildingMutationOutboxReferences(t, plan.OutboxRecords, plan.Reference.ReferenceKey, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	if len(plan.MaterialLedger) != 1 || plan.MaterialLedger[0].ReferenceKey != plan.Reference.ReferenceKey {
		t.Fatalf("durable material ledger = %+v, want one matching reference row", plan.MaterialLedger)
	}

	plan.OutboxRecords[0].OutboxID = "mutated-outbox"
	replayed, err := NewBuildingMutationDurableCommitPlan(&plan.Reference, plan.Reference.Result.OutboxRecords, plan.Reference.Result.MaterialLedger)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableCommitPlan(replay) error = %v, want nil", err)
	}
	if replayed.OutboxRecords[0].OutboxID == "mutated-outbox" {
		t.Fatal("durable commit plan reused mutable outbox rows")
	}
}

func TestBuildingMutationDurableCommitPlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewBuildingMutationDurableCommitPlan(nil, nil, nil); err != nil || !plan.Reference.ReferenceKey.IsZero() {
		t.Fatalf("NewBuildingMutationDurableCommitPlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	valid := buildingDurableCommitPlanForStoreTest(t)
	cases := map[string]func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry){
		"missing reference with rows": func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry) {
			return nil, valid.OutboxRecords, valid.MaterialLedger
		},
		"published outbox": func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry) {
			outbox := cloneProductionOutboxRecords(valid.OutboxRecords)
			outbox[0].Status = ProductionOutboxStatusPublished
			return &valid.Reference, outbox, valid.MaterialLedger
		},
		"mismatched outbox evidence": func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry) {
			outbox := cloneProductionOutboxRecords(valid.OutboxRecords)
			outbox[0].ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
			return &valid.Reference, outbox, valid.MaterialLedger
		},
		"mismatched ledger reference": func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry) {
			ledger := cloneBuildingMaterialLedgerEntries(valid.MaterialLedger)
			ledger[0].ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
			return &valid.Reference, valid.OutboxRecords, ledger
		},
		"reference missing outbox evidence": func() (*BuildingMutationReferenceRecord, []ProductionOutboxRecord, []BuildingMaterialLedgerEntry) {
			reference := cloneBuildingMutationReferenceRecord(valid.Reference)
			reference.Result.OutboxRecords = nil
			return &reference, valid.OutboxRecords, valid.MaterialLedger
		},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			reference, outbox, ledger := input()
			_, err := NewBuildingMutationDurableCommitPlan(reference, outbox, ledger)
			if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) {
				t.Fatalf("NewBuildingMutationDurableCommitPlan(%s) error = %v, want ErrInvalidBuildingMutationDurableCommit", name, err)
			}
		})
	}
}

func TestBuildingMutationDurableCommitStoreExactReplayConflictAndReadback(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()

	first, err := plan.ApplyDurableCommit(store)
	if err != nil {
		t.Fatalf("ApplyDurableCommit() error = %v, want nil", err)
	}
	duplicate, err := store.ApplyBuildingMutationDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("duplicate ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	if first.Duplicate || !duplicate.Duplicate || duplicate.Reference == nil {
		t.Fatalf("duplicate flags/result first=%+v duplicate=%+v, want false/true with reference", first, duplicate)
	}
	if len(store.BuildingMutationReferences()) != 1 ||
		len(store.OutboxRecords()) != len(plan.OutboxRecords) ||
		len(store.BuildingMaterialLedgerEntries()) != len(plan.MaterialLedger) {
		t.Fatalf("durable store rows refs=%d outbox=%d ledger=%d, want no duplicate append",
			len(store.BuildingMutationReferences()),
			len(store.OutboxRecords()),
			len(store.BuildingMaterialLedgerEntries()))
	}

	conflict := cloneBuildingMutationDurableCommitPlan(plan)
	conflict.OutboxRecords[0].Sequence++
	conflict.Reference.Result.OutboxRecords[0].Sequence++
	_, err = store.ApplyBuildingMutationDurableCommitPlan(conflict)
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) {
		t.Fatalf("conflicting ApplyBuildingMutationDurableCommitPlan() error = %v, want ErrInvalidBuildingMutationDurableCommit", err)
	}
	if len(store.BuildingMutationReferences()) != 1 ||
		len(store.OutboxRecords()) != len(plan.OutboxRecords) ||
		len(store.BuildingMaterialLedgerEntries()) != len(plan.MaterialLedger) {
		t.Fatalf("durable store mutated after conflict refs=%d outbox=%d ledger=%d",
			len(store.BuildingMutationReferences()),
			len(store.OutboxRecords()),
			len(store.BuildingMaterialLedgerEntries()))
	}

	recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan() = ok %v err %v, want true nil", ok, err)
	}
	recovered.Reference.BuildingID = "building-mutated"
	recovered.OutboxRecords[0].OutboxID = "outbox-mutated"
	recovered.MaterialLedger[0].LedgerID = "ledger-mutated"
	again, ok, err := store.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if again.Reference.BuildingID == "building-mutated" ||
		again.OutboxRecords[0].OutboxID == "outbox-mutated" ||
		again.MaterialLedger[0].LedgerID == "ledger-mutated" {
		t.Fatalf("readback reused mutable rows: %+v", again)
	}
}

func TestBuildingMutationDurableCommitStoreRejectsInvalidPlanAndMissingReadback(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()

	if result, err := store.ApplyBuildingMutationDurableCommitPlan(BuildingMutationDurableCommitPlan{}); err != nil || result.Reference != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan(no-op) = %+v/%v, want empty nil", result, err)
	}

	invalid := cloneBuildingMutationDurableCommitPlan(plan)
	invalid.OutboxRecords[0].Status = ProductionOutboxStatusPublished
	_, err := store.ApplyBuildingMutationDurableCommitPlan(invalid)
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) {
		t.Fatalf("invalid ApplyBuildingMutationDurableCommitPlan() error = %v, want ErrInvalidBuildingMutationDurableCommit", err)
	}
	if len(store.BuildingMutationReferences()) != 0 || len(store.OutboxRecords()) != 0 || len(store.BuildingMaterialLedgerEntries()) != 0 {
		t.Fatalf("durable store rows after invalid plan refs=%d outbox=%d ledger=%d, want empty",
			len(store.BuildingMutationReferences()),
			len(store.OutboxRecords()),
			len(store.BuildingMaterialLedgerEntries()))
	}

	if recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey); err != nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(""); err == nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
}

func buildingDurableCommitPlanForStoreTest(t *testing.T) BuildingMutationDurableCommitPlan {
	t.Helper()
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), nil, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
		},
	})
	result, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
	})
	if err != nil {
		t.Fatalf("BuildPlanetBuilding() error = %v, want nil", err)
	}
	references := store.BuildingMutationReferences()
	if len(references) != 1 {
		t.Fatalf("BuildingMutationReferences() len = %d, want 1", len(references))
	}
	plan, err := NewBuildingMutationDurableCommitPlan(&references[0], result.OutboxRecords, result.MaterialLedger)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	return plan
}
