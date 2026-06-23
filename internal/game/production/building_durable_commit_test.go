package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
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

	dispatch, ok, err := store.CommittedBuildingMutationOutboxDispatchPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationOutboxDispatchPlan() = ok %v err %v, want true nil", ok, err)
	}
	if dispatch.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		len(dispatch.OutboxRecords) != len(plan.OutboxRecords) {
		t.Fatalf("dispatch readback = %+v, want committed build mutation dispatch", dispatch)
	}
	assertBuildingMutationOutboxReferences(t, dispatch.OutboxRecords, plan.Reference.ReferenceKey, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)

	dispatch.Reference.BuildingID = "building-mutated"
	dispatch.OutboxRecords[0].OutboxID = "outbox-mutated"
	againDispatch, ok, err := store.CommittedBuildingMutationOutboxDispatchPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationOutboxDispatchPlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if againDispatch.Reference.BuildingID == "building-mutated" ||
		againDispatch.OutboxRecords[0].OutboxID == "outbox-mutated" {
		t.Fatalf("dispatch readback reused mutable rows: %+v", againDispatch)
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
	if dispatch, ok, err := store.CommittedBuildingMutationOutboxDispatchPlan(plan.Reference.ReferenceKey); err != nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationOutboxDispatchPlan(missing) = %+v/%v/%v, want empty false nil", dispatch, ok, err)
	}
	if recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(""); err == nil || ok || !recovered.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedBuildingMutationOutboxDispatchPlan(""); err == nil || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationOutboxDispatchPlan(invalid) = %+v/%v/%v, want error false empty", dispatch, ok, err)
	}
}

func TestBuildingMutationDurableCommitStoreDispatchReadbackRejectsInvalidCommittedReference(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}

	store.mu.Lock()
	corrupted := cloneBuildingMutationDurableCommitPlan(store.plans[plan.Reference.ReferenceKey])
	corrupted.Reference.Result.ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
	store.plans[plan.Reference.ReferenceKey] = corrupted
	store.mu.Unlock()

	dispatch, ok, err := store.CommittedBuildingMutationOutboxDispatchPlan(plan.Reference.ReferenceKey)
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || !dispatch.Reference.ReferenceKey.IsZero() {
		t.Fatalf("CommittedBuildingMutationOutboxDispatchPlan(corrupt reference) = %+v/%v/%v, want durable commit error false empty", dispatch, ok, err)
	}
}

func TestBuildingMutationDurableCommitStorePublishesCommittedOutboxRows(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	ledgerBefore := store.BuildingMaterialLedgerEntries()

	results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       10,
		ClaimedAt:   testTime(40),
		CompletedAt: testTime(41),
		Publish: func(record ProductionOutboxRecord) error {
			if record.Status != ProductionOutboxStatusInFlight || record.ClaimToken == "" {
				t.Fatalf("publish record = %+v, want in-flight claimed row", record)
			}
			if record.ReferenceKey != plan.Reference.ReferenceKey {
				t.Fatalf("publish record reference = %+v, want %s", record, plan.Reference.ReferenceKey)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox() error = %v, want nil", err)
	}
	if len(results) != len(plan.OutboxRecords) {
		t.Fatalf("published rows len = %d, want %d", len(results), len(plan.OutboxRecords))
	}
	for _, result := range results {
		if !result.Published || result.Failed || result.StaleClaim {
			t.Fatalf("publish result = %+v, want published", result)
		}
		if result.Record.Status != ProductionOutboxStatusPublished ||
			!result.Record.PublishedAt.Equal(testTime(41)) ||
			result.Record.ReferenceKey != plan.Reference.ReferenceKey {
			t.Fatalf("published durable row = %+v, want committed building evidence", result.Record)
		}
	}
	assertBuildingMutationOutboxReferences(t, store.OutboxRecords(), plan.Reference.ReferenceKey, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	if !buildingMutationMaterialLedgerEqual(store.BuildingMaterialLedgerEntries(), ledgerBefore) {
		t.Fatalf("material ledger after publish = %+v, want unchanged %+v", store.BuildingMaterialLedgerEntries(), ledgerBefore)
	}
	recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(after publish) = ok %v err %v, want true nil", ok, err)
	}
	for _, row := range recovered.OutboxRecords {
		if row.Status != ProductionOutboxStatusPublished || row.PublishedAt.IsZero() {
			t.Fatalf("recovered building outbox row after publish = %+v, want published delivery evidence", row)
		}
	}
}

func TestBuildingMutationDurableCommitStoreRecordsPublishFailures(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	temporaryErr := errors.New("publisher offline")
	failedType := EventPlanetStorageUpdated

	results, err := PublishPendingProductionOutbox(ProductionOutboxPublishInput{
		Store:       store,
		Limit:       10,
		ClaimedAt:   testTime(42),
		CompletedAt: testTime(43),
		Publish: func(record ProductionOutboxRecord) error {
			if record.Event.Type == failedType {
				return temporaryErr
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingProductionOutbox(failure) error = %v, want nil", err)
	}
	if len(results) != len(plan.OutboxRecords) {
		t.Fatalf("publish failure results len = %d, want %d", len(results), len(plan.OutboxRecords))
	}
	var failed ProductionOutboxRecord
	for _, result := range results {
		if result.Failed {
			failed = result.Record
			break
		}
	}
	if failed.OutboxID == "" {
		t.Fatalf("publish failure results = %+v, want one failed row", results)
	}
	if failed.Status != ProductionOutboxStatusFailed ||
		failed.LastError != temporaryErr.Error() ||
		!failed.FailedAt.Equal(testTime(43)) ||
		failed.Event.Type != failedType ||
		failed.ReferenceKey != plan.Reference.ReferenceKey {
		t.Fatalf("failed durable building outbox row = %+v, want failed committed evidence", failed)
	}
	recovered, ok, err := store.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil || !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(after failure) = ok %v err %v, want true nil", ok, err)
	}
	var recoveredFailed ProductionOutboxRecord
	for _, row := range recovered.OutboxRecords {
		if row.Event.Type == failedType {
			recoveredFailed = row
			break
		}
	}
	if recoveredFailed.Status != ProductionOutboxStatusFailed ||
		recoveredFailed.LastError != temporaryErr.Error() ||
		!recoveredFailed.FailedAt.Equal(testTime(43)) {
		t.Fatalf("recovered building outbox row after failure = %+v, want failed delivery evidence", recoveredFailed)
	}
}

func TestBuildingMutationDurableCommitStorePublisherRejectsStaleClaimTokens(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(50))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable building outbox rows = %+v, want one in-flight row", claimed)
	}

	if _, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, "wrong-token", testTime(51)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxPublished(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	if _, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, "wrong-token", "wrong", testTime(51)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxFailed(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	rows := store.OutboxRecords()
	if rows[0].Status != ProductionOutboxStatusInFlight || rows[0].ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("durable building outbox after stale token = %+v, want original in-flight row", rows[0])
	}
}

func TestBuildingMutationDurableCommitStoreReleasesExpiredOutboxLeases(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(60))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed durable building outbox rows = %+v, want one row", claimed)
	}

	released, err := store.ReleaseExpiredProductionOutboxRecords(1, testTime(61), testTime(62))
	if err != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(released) != 1 ||
		released[0].Status != ProductionOutboxStatusPending ||
		!released[0].ClaimedAt.IsZero() ||
		released[0].ClaimToken != "" ||
		released[0].Attempts != claimed[0].Attempts ||
		!released[0].RetriedAt.Equal(testTime(62)) ||
		released[0].ReferenceKey != plan.Reference.ReferenceKey {
		t.Fatalf("released durable building outbox rows = %+v, want pending committed evidence", released)
	}
	if _, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(63)); err != nil || ok {
		t.Fatalf("MarkProductionOutboxPublished(stale token after release) = ok %v err %v, want false nil", ok, err)
	}

	reclaimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(64))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords(reclaim) error = %v, want nil", err)
	}
	if len(reclaimed) != 1 ||
		reclaimed[0].Attempts != claimed[0].Attempts+1 ||
		reclaimed[0].ClaimToken == "" ||
		reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Fatalf("reclaimed durable building outbox rows = %+v, want fresh claim token", reclaimed)
	}
}

func TestBuildingMutationDurableCommitStoreLeaseReleaseNoOps(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	claimedAt := testTime(70)
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, claimedAt)
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed durable building outbox rows = %+v, want one row", claimed)
	}
	if released, err := store.ReleaseExpiredProductionOutboxRecords(0, claimedAt.Add(time.Second), testTime(71)); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords(limit 0) = %+v/%v, want nil nil", released, err)
	}
	if released, err := store.ReleaseExpiredProductionOutboxRecords(1, time.Time{}, testTime(71)); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords(zero cutoff) = %+v/%v, want nil nil", released, err)
	}
	if released, err := store.ReleaseExpiredProductionOutboxRecords(1, claimedAt, testTime(71)); err != nil || len(released) != 0 {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords(boundary equal) = %+v/%v, want empty nil", released, err)
	}
	rows := store.OutboxRecords()
	if rows[0].Status != ProductionOutboxStatusInFlight || rows[0].ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("durable building outbox after no-op releases = %+v, want original in-flight row", rows[0])
	}
}

func TestBuildingMutationDurableCommitStorePublisherRejectsCorruptPendingReadbackRows(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	corrupt := cloneBuildingMutationDurableCommitPlan(plan)
	corrupt.OutboxRecords[0].ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
	store.plans[plan.Reference.ReferenceKey] = corrupt
	store.references = append(store.references, plan.Reference.ReferenceKey)

	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(72))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(claimed) != 0 {
		t.Fatalf("ClaimPendingProductionOutboxRecords(corrupt) = %+v/%v, want invalid durable commit", claimed, err)
	}
	rows := store.OutboxRecords()
	if len(rows) != len(plan.OutboxRecords) || rows[0].Status != ProductionOutboxStatusPending || rows[0].Attempts != 0 {
		t.Fatalf("corrupt pending outbox mutated after claim error: %+v", rows)
	}
}

func TestBuildingMutationDurableCommitStorePublisherBatchValidationPreventsPartialClaim(t *testing.T) {
	valid := buildingDurableCommitPlanForStoreTestWithSuffix(t, "valid")
	corrupt := buildingDurableCommitPlanForStoreTestWithSuffix(t, "corrupt")
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(valid); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan(valid) error = %v, want nil", err)
	}
	corrupt.OutboxRecords[0].ReferenceKey = valid.Reference.ReferenceKey
	store.plans[corrupt.Reference.ReferenceKey] = corrupt
	store.references = append(store.references, corrupt.Reference.ReferenceKey)

	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(73))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(claimed) != 0 {
		t.Fatalf("ClaimPendingProductionOutboxRecords(valid before corrupt) = %+v/%v, want invalid durable commit", claimed, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ProductionOutboxStatusPending || rows[0].Attempts != 0 {
		t.Fatalf("valid pending outbox mutated before corrupt claim error: %+v", rows)
	}
}

func TestBuildingMutationDurableCommitStorePublisherMutationsRejectCorruptReadbackRows(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(74))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable building outbox rows = %+v, want one in-flight row", claimed)
	}

	corruptInFlight := cloneBuildingMutationDurableCommitPlan(store.plans[plan.Reference.ReferenceKey])
	corruptInFlight.OutboxRecords[0].ReferenceKey = mustBuildingBuildKey(t, "planet-other", "building-1")
	store.plans[plan.Reference.ReferenceKey] = corruptInFlight

	if record, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(75)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxPublished(corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(75)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxFailed(corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	released, err := store.ReleaseExpiredProductionOutboxRecords(1, testTime(75), testTime(76))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(released) != 0 {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords(corrupt) = %+v/%v, want invalid durable commit", released, err)
	}

	corruptFailed := corruptInFlight
	corruptFailed.OutboxRecords[0].Status = ProductionOutboxStatusFailed
	corruptFailed.OutboxRecords[0].FailedAt = testTime(77)
	corruptFailed.OutboxRecords[0].LastError = "temporary"
	store.plans[plan.Reference.ReferenceKey] = corruptFailed
	retried, err := store.RetryFailedProductionOutboxRecords(1, testTime(78))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(retried) != 0 {
		t.Fatalf("RetryFailedProductionOutboxRecords(corrupt) = %+v/%v, want invalid durable commit", retried, err)
	}

	rows := store.OutboxRecords()
	if len(rows) != len(plan.OutboxRecords) || rows[0].Status != ProductionOutboxStatusFailed || rows[0].RetriedAt.Equal(testTime(78)) {
		t.Fatalf("corrupt failed outbox mutated after retry error: %+v", rows)
	}
}

func TestBuildingMutationDurableCommitStorePublisherMutationsValidateAllReadbackRows(t *testing.T) {
	valid := buildingDurableCommitPlanForStoreTestWithSuffix(t, "publish-valid")
	corrupt := buildingDurableCommitPlanForStoreTestWithSuffix(t, "publish-corrupt")
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(valid); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan(valid) error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(79))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords(valid) error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable building outbox rows = %+v, want one in-flight row", claimed)
	}
	corrupt.OutboxRecords[0].ReferenceKey = valid.Reference.ReferenceKey
	store.plans[corrupt.Reference.ReferenceKey] = corrupt
	store.references = append(store.references, corrupt.Reference.ReferenceKey)

	if record, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(80)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxPublished(valid before corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(80)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxFailed(valid before corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ProductionOutboxStatusInFlight || rows[0].PublishedAt.Equal(testTime(80)) || rows[0].FailedAt.Equal(testTime(80)) {
		t.Fatalf("valid in-flight outbox mutated before corrupt publish/fail error: %+v", rows)
	}
}

func TestBuildingMutationDurableCommitStorePublisherMutationsRejectCorruptLookupRows(t *testing.T) {
	plan := buildingDurableCommitPlanForStoreTest(t)
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(81))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" || claimed[0].OutboxID == "" {
		t.Fatalf("claimed durable building outbox rows = %+v, want one in-flight row", claimed)
	}

	corrupt := cloneBuildingMutationDurableCommitPlan(store.plans[plan.Reference.ReferenceKey])
	corrupt.OutboxRecords[0].OutboxID = ""
	store.plans[plan.Reference.ReferenceKey] = corrupt

	if record, ok, err := store.MarkProductionOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(82)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxPublished(corrupt lookup) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(82)); !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkProductionOutboxFailed(corrupt lookup) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ProductionOutboxStatusInFlight || rows[0].OutboxID != "" {
		t.Fatalf("corrupt lookup outbox mutated after publish/fail error: %+v", rows)
	}
}

func TestBuildingMutationDurableCommitStorePublisherBatchValidationPreventsPartialReleaseAndRetry(t *testing.T) {
	valid := buildingDurableCommitPlanForStoreTestWithSuffix(t, "release-valid")
	corrupt := buildingDurableCommitPlanForStoreTestWithSuffix(t, "release-corrupt")
	store := NewInMemoryBuildingMutationDurableCommitStore()
	if _, err := store.ApplyBuildingMutationDurableCommitPlan(valid); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan(valid) error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingProductionOutboxRecords(1, testTime(83))
	if err != nil {
		t.Fatalf("ClaimPendingProductionOutboxRecords() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable building outbox rows = %+v, want one in-flight row", claimed)
	}
	corrupt.OutboxRecords[0].ReferenceKey = valid.Reference.ReferenceKey
	store.plans[corrupt.Reference.ReferenceKey] = corrupt
	store.references = append(store.references, corrupt.Reference.ReferenceKey)

	released, err := store.ReleaseExpiredProductionOutboxRecords(1, testTime(84), testTime(85))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(released) != 0 {
		t.Fatalf("ReleaseExpiredProductionOutboxRecords(valid before corrupt) = %+v/%v, want invalid durable commit", released, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ProductionOutboxStatusInFlight || rows[0].ClaimToken == "" {
		t.Fatalf("valid in-flight outbox mutated before corrupt release error: %+v", rows)
	}

	delete(store.plans, corrupt.Reference.ReferenceKey)
	store.references = store.references[:1]
	if _, ok, err := store.MarkProductionOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(86)); err != nil || !ok {
		t.Fatalf("MarkProductionOutboxFailed(valid) = ok %v err %v, want true nil", ok, err)
	}
	store.plans[corrupt.Reference.ReferenceKey] = corrupt
	store.references = append(store.references, corrupt.Reference.ReferenceKey)

	retried, err := store.RetryFailedProductionOutboxRecords(1, testTime(87))
	if !errors.Is(err, ErrInvalidBuildingMutationDurableCommit) || len(retried) != 0 {
		t.Fatalf("RetryFailedProductionOutboxRecords(valid before corrupt) = %+v/%v, want invalid durable commit", retried, err)
	}
	rows = store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ProductionOutboxStatusFailed || rows[0].RetriedAt.Equal(testTime(87)) {
		t.Fatalf("valid failed outbox mutated before corrupt retry error: %+v", rows)
	}
}

func buildingDurableCommitPlanForStoreTest(t *testing.T) BuildingMutationDurableCommitPlan {
	t.Helper()
	return buildingDurableCommitPlanForStoreTestWithSuffix(t, "1")
}

func buildingDurableCommitPlanForStoreTestWithSuffix(t *testing.T, suffix string) BuildingMutationDurableCommitPlan {
	t.Helper()
	planetID := foundation.PlanetID("planet-1")
	buildingID := BuildingID("building-" + suffix)
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), nil, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
		},
	})
	result, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     planetID,
		BuildingID:   buildingID,
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: mustBuildingBuildKey(t, planetID, buildingID),
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
