package discovery

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestClaimDurableLifecycleStoreCommitsLifecyclePlan(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()

	result, err := store.ApplyClaimDurableLifecyclePlan(plan)
	if err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	if result.Duplicate || result.Plan.Commit.Boundary.ClaimReference != plan.Commit.Boundary.ClaimReference {
		t.Fatalf("claim durable lifecycle result = %+v, want first commit", result)
	}
	if !result.Plan.HasProductionInit || result.Plan.ProductionInitialized.Initialization.ClaimReference != plan.Commit.Boundary.ClaimReference {
		t.Fatalf("claim durable lifecycle production init = %+v, want committed init evidence", result.Plan.ProductionInitialized)
	}
	if len(store.ClaimReferences()) != 1 {
		t.Fatalf("ClaimReferences() len = %d, want 1", len(store.ClaimReferences()))
	}
}

func TestClaimDurableLifecycleStoreCommitsLifecycleWithoutProductionInit(t *testing.T) {
	beginPlan, _, commitPlan := claimDurableLifecyclePlansForTest(t)
	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, nil, &commitPlan)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan(no init) error = %v, want nil", err)
	}
	store := NewInMemoryClaimDurableLifecycleStore()

	result, err := store.ApplyClaimDurableLifecyclePlan(plan)
	if err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(no init) error = %v, want nil", err)
	}
	if result.Plan.HasProductionInit || result.Plan.ProductionInitialized.Initialization.ClaimReference != "" {
		t.Fatalf("claim durable lifecycle no-init result = %+v, want no production init evidence", result.Plan)
	}
}

func TestClaimDurableLifecycleStoreDuplicateReferenceReplaysWithoutDuplicateRows(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()

	first, err := store.ApplyClaimDurableLifecyclePlan(plan)
	if err != nil {
		t.Fatalf("first ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	duplicate, err := store.ApplyClaimDurableLifecyclePlan(plan)
	if err != nil {
		t.Fatalf("duplicate ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	if first.Duplicate || !duplicate.Duplicate {
		t.Fatalf("duplicate flags first=%v duplicate=%v, want false/true", first.Duplicate, duplicate.Duplicate)
	}
	if len(store.ClaimReferences()) != 1 {
		t.Fatalf("ClaimReferences() len = %d, want no duplicate append", len(store.ClaimReferences()))
	}
}

func TestClaimDurableLifecycleStoreRejectsConflictingReferenceReuse(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.Commit.Outbox.OutboxID = "claim-outbox-other"
	_, err := store.ApplyClaimDurableLifecyclePlan(conflict)
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("conflicting ApplyClaimDurableLifecyclePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	if len(store.ClaimReferences()) != 1 {
		t.Fatalf("ClaimReferences() len after conflict = %d, want 1", len(store.ClaimReferences()))
	}
}

func TestClaimDurableLifecycleStoreRejectsInvalidPlanWithoutMutation(t *testing.T) {
	valid := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if result, err := store.ApplyClaimDurableLifecyclePlan(ClaimDurableLifecyclePlan{}); err != nil || result.Plan.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(no-op) = %+v/%v, want empty nil", result, err)
	}

	partialNoOp := ClaimDurableLifecyclePlan{}
	partialNoOp.HasProductionInit = true
	_, err := store.ApplyClaimDurableLifecyclePlan(partialNoOp)
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("partial no-op ApplyClaimDurableLifecyclePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	invalid := valid
	invalid.HasProductionInit = false
	_, err = store.ApplyClaimDurableLifecyclePlan(invalid)
	if !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("invalid ApplyClaimDurableLifecyclePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	if len(store.ClaimReferences()) != 0 {
		t.Fatalf("ClaimReferences() len after invalid plan = %d, want 0", len(store.ClaimReferences()))
	}
}

func TestClaimDurableLifecycleStoreReadsCommittedPlanByReference(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Boundary.ClaimReference != plan.Commit.Boundary.ClaimReference ||
		recovered.Begin.XCoreConsumption.ClaimReference != plan.Begin.XCoreConsumption.ClaimReference ||
		!recovered.HasProductionInit {
		t.Fatalf("recovered claim durable lifecycle = %+v, want committed plan %+v", recovered, plan)
	}

	recovered.Commit.Outbox.OutboxID = "mutated-outbox"
	recovered.Begin.Planet.ID = "mutated-planet"
	again, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if again.Commit.Outbox.OutboxID == "mutated-outbox" || again.Begin.Planet.ID == "mutated-planet" {
		t.Fatalf("recovered claim durable lifecycle reused mutable rows: %+v", again)
	}
}

func TestClaimDurableLifecycleStoreReadbackPreservesCompletedProductionInit(t *testing.T) {
	beginPlan, pendingInitPlan, commitPlan := claimDurableLifecyclePlansForTest(t)
	completedInitPlan, err := NewClaimProductionInitializationDurablePlan(&pendingInitPlan.Initialization, &commitPlan.Boundary)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurablePlan(complete) error = %v, want nil", err)
	}
	commitPlanWithXCore, err := NewClaimDurableCommitPlan(
		&commitPlan.Boundary,
		&commitPlan.Reference,
		&commitPlan.Event,
		&commitPlan.Outbox,
		&beginPlan.XCoreConsumption,
	)
	if err != nil {
		t.Fatalf("NewClaimDurableCommitPlan(with xcore) error = %v, want nil", err)
	}
	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, &completedInitPlan, &commitPlanWithXCore)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan(complete init) error = %v, want nil", err)
	}
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(complete init) error = %v, want nil", err)
	}

	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(complete init) = ok %v err %v, want true nil", ok, err)
	}
	if !recovered.HasProductionInit ||
		recovered.ProductionInitialized.Boundary.Status != ClaimBoundaryStatusComplete ||
		recovered.ProductionInitialized.Boundary.StaleListingCount != commitPlan.Boundary.StaleListingCount ||
		recovered.ProductionInitialized.Initialization.ClaimReference != completedInitPlan.Initialization.ClaimReference ||
		recovered.ProductionInitialized.Initialization.PlanetID != completedInitPlan.Initialization.PlanetID {
		t.Fatalf("recovered completed production init = %+v, want completed lifecycle evidence %+v", recovered.ProductionInitialized, completedInitPlan)
	}

	recovered.ProductionInitialized.Boundary.Status = ClaimBoundaryStatusPendingSideEffects
	recovered.ProductionInitialized.Initialization.PlanetID = "mutated-planet"
	again, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(second complete init) = ok %v err %v, want true nil", ok, err)
	}
	if again.ProductionInitialized.Boundary.Status != ClaimBoundaryStatusComplete ||
		again.ProductionInitialized.Initialization.PlanetID == "mutated-planet" {
		t.Fatalf("recovered completed production init reused mutable rows: %+v", again.ProductionInitialized)
	}
}

func TestClaimDurableLifecycleStoreReadsCommittedDispatchPlanByReference(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	dispatch, ok, err := store.CommittedClaimOutboxDispatchPlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimOutboxDispatchPlan() = ok %v err %v, want true nil", ok, err)
	}
	if dispatch.Reference.ClaimReference != plan.Commit.Boundary.ClaimReference ||
		dispatch.Outbox.ClaimReference != plan.Commit.Boundary.ClaimReference ||
		dispatch.Outbox.Event.Type != ClaimEventPlanetClaimed {
		t.Fatalf("dispatch plan = %+v, want committed claim outbox dispatch", dispatch)
	}

	dispatch.Outbox.OutboxID = "mutated-outbox"
	again, ok, err := store.CommittedClaimOutboxDispatchPlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimOutboxDispatchPlan(second) = ok %v err %v, want true nil", ok, err)
	}
	if again.Outbox.OutboxID == "mutated-outbox" {
		t.Fatalf("recovered dispatch plan reused mutable rows: %+v", again)
	}
}

func TestClaimDurableLifecycleStorePublishesCommittedClaimOutboxRows(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	results, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store:       store,
		Limit:       10,
		ClaimedAt:   testTime(50),
		CompletedAt: testTime(51),
		Publish: func(record ClaimOutboxRecord) error {
			if record.Status != ClaimOutboxStatusInFlight || record.ClaimToken == "" {
				t.Fatalf("publish record = %+v, want in-flight claimed row", record)
			}
			if record.ClaimReference != plan.Commit.Boundary.ClaimReference ||
				record.ReferenceKey != plan.Commit.Boundary.ReferenceKey ||
				record.Event.Type != ClaimEventPlanetClaimed {
				t.Fatalf("publish record evidence = %+v, want committed claim evidence", record)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingClaimOutbox() error = %v, want nil", err)
	}
	if len(results) != 1 || !results[0].Published || results[0].Failed || results[0].StaleClaim {
		t.Fatalf("publish results = %+v, want one published result", results)
	}
	if results[0].Record.Status != ClaimOutboxStatusPublished ||
		!results[0].Record.PublishedAt.Equal(testTime(51)) ||
		results[0].Record.ClaimReference != plan.Commit.Boundary.ClaimReference ||
		results[0].Record.ReferenceKey != plan.Commit.Boundary.ReferenceKey {
		t.Fatalf("published durable claim outbox row = %+v, want committed published evidence", results[0].Record)
	}
	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusPublished {
		t.Fatalf("OutboxRecords() = %+v, want published committed row", rows)
	}
	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(after publish) = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Outbox.Status != ClaimOutboxStatusPublished ||
		!recovered.Commit.Outbox.PublishedAt.Equal(testTime(51)) {
		t.Fatalf("recovered lifecycle after publish = %+v, want published outbox evidence", recovered.Commit.Outbox)
	}
}

func TestClaimDurableLifecycleStoreRecordsCommittedClaimOutboxPublishFailures(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	temporaryErr := errors.New("publisher offline")

	results, err := PublishPendingClaimOutbox(ClaimOutboxPublishInput{
		Store:       store,
		Limit:       1,
		ClaimedAt:   testTime(52),
		CompletedAt: testTime(53),
		Publish: func(ClaimOutboxRecord) error {
			return temporaryErr
		},
	})
	if err != nil {
		t.Fatalf("PublishPendingClaimOutbox(failure) error = %v, want nil", err)
	}
	if len(results) != 1 || !results[0].Failed || results[0].Published || results[0].StaleClaim {
		t.Fatalf("publish failure results = %+v, want one failed result", results)
	}
	if results[0].Record.Status != ClaimOutboxStatusFailed ||
		results[0].Record.LastError != temporaryErr.Error() ||
		!results[0].Record.FailedAt.Equal(testTime(53)) ||
		results[0].Record.ClaimReference != plan.Commit.Boundary.ClaimReference {
		t.Fatalf("failed durable claim outbox row = %+v, want failed committed evidence", results[0].Record)
	}
	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(after failure) = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Outbox.Status != ClaimOutboxStatusFailed ||
		recovered.Commit.Outbox.LastError != temporaryErr.Error() ||
		!recovered.Commit.Outbox.FailedAt.Equal(testTime(53)) {
		t.Fatalf("recovered lifecycle after failure = %+v, want failed outbox evidence", recovered.Commit.Outbox)
	}
	retried, err := store.RetryFailedClaimOutboxRecordsForPublish(1, testTime(54))
	if err != nil {
		t.Fatalf("RetryFailedClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(retried) != 1 || retried[0].Status != ClaimOutboxStatusPending || retried[0].LastError != temporaryErr.Error() {
		t.Fatalf("retried durable claim outbox rows = %+v, want pending with failure evidence", retried)
	}
	recovered, ok, err = store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(after retry) = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Outbox.Status != ClaimOutboxStatusPending ||
		recovered.Commit.Outbox.LastError != temporaryErr.Error() ||
		!recovered.Commit.Outbox.FailedAt.Equal(testTime(53)) {
		t.Fatalf("recovered lifecycle after retry = %+v, want pending row with preserved failure evidence", recovered.Commit.Outbox)
	}
}

func TestClaimDurableLifecycleStorePublisherRejectsStaleClaimTokens(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(60))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one in-flight row", claimed)
	}

	if _, ok, err := store.MarkClaimOutboxPublished(claimed[0].OutboxID, "wrong-token", testTime(61)); err != nil || ok {
		t.Fatalf("MarkClaimOutboxPublished(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	if _, ok, err := store.MarkClaimOutboxFailed(claimed[0].OutboxID, "wrong-token", "wrong", testTime(61)); err != nil || ok {
		t.Fatalf("MarkClaimOutboxFailed(wrong token) = ok %v err %v, want false nil", ok, err)
	}
	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusInFlight || rows[0].ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("durable claim outbox after stale token = %+v, want original in-flight row", rows)
	}
	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(in-flight) = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Outbox.Status != ClaimOutboxStatusInFlight ||
		recovered.Commit.Outbox.ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("recovered lifecycle while in-flight = %+v, want in-flight outbox evidence", recovered.Commit.Outbox)
	}
}

func TestClaimDurableLifecycleStoreReleasesExpiredClaimOutboxLeases(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(70))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one row", claimed)
	}

	released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(1, testTime(71), testTime(72))
	if err != nil {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(released) != 1 ||
		released[0].Status != ClaimOutboxStatusPending ||
		!released[0].ClaimedAt.IsZero() ||
		released[0].ClaimToken != "" ||
		released[0].Attempts != claimed[0].Attempts ||
		!released[0].RetriedAt.Equal(testTime(72)) ||
		released[0].ClaimReference != plan.Commit.Boundary.ClaimReference ||
		released[0].ReferenceKey != plan.Commit.Boundary.ReferenceKey {
		t.Fatalf("released durable claim outbox rows = %+v, want pending committed evidence", released)
	}
	if _, ok, err := store.MarkClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(73)); err != nil || ok {
		t.Fatalf("MarkClaimOutboxPublished(stale token after release) = ok %v err %v, want false nil", ok, err)
	}
	recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(after release) = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Commit.Outbox.Status != ClaimOutboxStatusPending ||
		recovered.Commit.Outbox.ClaimToken != "" ||
		!recovered.Commit.Outbox.RetriedAt.Equal(testTime(72)) {
		t.Fatalf("recovered lifecycle after lease release = %+v, want pending released evidence", recovered.Commit.Outbox)
	}

	reclaimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(74))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish(reclaim) error = %v, want nil", err)
	}
	if len(reclaimed) != 1 ||
		reclaimed[0].Attempts != claimed[0].Attempts+1 ||
		reclaimed[0].ClaimToken == "" ||
		reclaimed[0].ClaimToken == claimed[0].ClaimToken {
		t.Fatalf("reclaimed durable claim outbox rows = %+v, want fresh claim token", reclaimed)
	}
}

func TestClaimDurableLifecycleStoreLeaseReleaseNoOps(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimedAt := testTime(80)
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, claimedAt)
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one row", claimed)
	}
	if released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(0, claimedAt.Add(time.Second), testTime(81)); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish(limit 0) = %+v/%v, want nil nil", released, err)
	}
	if released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(1, time.Time{}, testTime(81)); err != nil || released != nil {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish(zero cutoff) = %+v/%v, want nil nil", released, err)
	}
	if released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(1, claimedAt, testTime(81)); err != nil || len(released) != 0 {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish(boundary equal) = %+v/%v, want empty nil", released, err)
	}
	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusInFlight || rows[0].ClaimToken != claimed[0].ClaimToken {
		t.Fatalf("durable claim outbox after no-op releases = %+v, want original in-flight row", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherRejectsCorruptPendingReadbackRows(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	corrupt := cloneClaimDurableLifecyclePlan(plan)
	corrupt.Commit.Outbox.ReferenceKey = "claim_planet:wrong-player:wrong-planet"
	store.plans[plan.Commit.Boundary.ClaimReference] = corrupt
	store.references = append(store.references, plan.Commit.Boundary.ClaimReference)

	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(82))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(claimed) != 0 {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish(corrupt) = %+v/%v, want invalid durable commit", claimed, err)
	}
	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusPending || rows[0].Attempts != 0 {
		t.Fatalf("corrupt pending outbox mutated after claim error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherBatchValidationPreventsPartialClaim(t *testing.T) {
	valid := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "valid")
	corrupt := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "corrupt")
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(valid); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(valid) error = %v, want nil", err)
	}
	corrupt.Commit.Outbox.ReferenceKey = valid.Commit.Boundary.ReferenceKey
	store.plans[corrupt.Commit.Boundary.ClaimReference] = corrupt
	store.references = append(store.references, corrupt.Commit.Boundary.ClaimReference)

	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(83))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(claimed) != 0 {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish(valid before corrupt) = %+v/%v, want invalid durable commit", claimed, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ClaimOutboxStatusPending || rows[0].Attempts != 0 {
		t.Fatalf("valid pending outbox mutated before corrupt claim error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherMutationsRejectCorruptReadbackRows(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(84))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one in-flight row", claimed)
	}

	corruptInFlight := cloneClaimDurableLifecyclePlan(store.plans[plan.Commit.Boundary.ClaimReference])
	corruptInFlight.Commit.Outbox.ReferenceKey = "claim_planet:wrong-player:wrong-planet"
	store.plans[plan.Commit.Boundary.ClaimReference] = corruptInFlight

	if record, ok, err := store.MarkClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(85)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxPublished(corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkClaimOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(85)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxFailed(corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(1, testTime(85), testTime(86))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(released) != 0 {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish(corrupt) = %+v/%v, want invalid durable commit", released, err)
	}

	corruptFailed := corruptInFlight
	corruptFailed.Commit.Outbox.Status = ClaimOutboxStatusFailed
	corruptFailed.Commit.Outbox.FailedAt = testTime(87)
	corruptFailed.Commit.Outbox.LastError = "temporary"
	store.plans[plan.Commit.Boundary.ClaimReference] = corruptFailed
	retried, err := store.RetryFailedClaimOutboxRecordsForPublish(1, testTime(88))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(retried) != 0 {
		t.Fatalf("RetryFailedClaimOutboxRecordsForPublish(corrupt) = %+v/%v, want invalid durable commit", retried, err)
	}

	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusFailed || rows[0].RetriedAt.Equal(testTime(88)) {
		t.Fatalf("corrupt failed outbox mutated after retry error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherMutationsValidateAllReadbackRows(t *testing.T) {
	valid := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "publish-valid")
	corrupt := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "publish-corrupt")
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(valid); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(valid) error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(88))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish(valid) error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one in-flight row", claimed)
	}
	corrupt.Commit.Outbox.ReferenceKey = valid.Commit.Boundary.ReferenceKey
	store.plans[corrupt.Commit.Boundary.ClaimReference] = corrupt
	store.references = append(store.references, corrupt.Commit.Boundary.ClaimReference)

	if record, ok, err := store.MarkClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(89)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxPublished(valid before corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkClaimOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(89)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxFailed(valid before corrupt) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ClaimOutboxStatusInFlight || rows[0].PublishedAt.Equal(testTime(89)) || rows[0].FailedAt.Equal(testTime(89)) {
		t.Fatalf("valid in-flight outbox mutated before corrupt publish/fail error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherMutationsRejectCorruptLookupRows(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(90))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" || claimed[0].OutboxID == "" {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one in-flight row", claimed)
	}

	corrupt := cloneClaimDurableLifecyclePlan(store.plans[plan.Commit.Boundary.ClaimReference])
	corrupt.Commit.Outbox.OutboxID = ""
	store.plans[plan.Commit.Boundary.ClaimReference] = corrupt

	if record, ok, err := store.MarkClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, testTime(91)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxPublished(corrupt lookup) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	if record, ok, err := store.MarkClaimOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(91)); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || record.OutboxID != "" {
		t.Fatalf("MarkClaimOutboxFailed(corrupt lookup) = %+v/%v/%v, want invalid durable commit", record, ok, err)
	}
	rows := store.OutboxRecords()
	if len(rows) != 1 || rows[0].Status != ClaimOutboxStatusInFlight || rows[0].OutboxID != "" {
		t.Fatalf("corrupt lookup outbox mutated after publish/fail error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStorePublisherBatchValidationPreventsPartialReleaseAndRetry(t *testing.T) {
	valid := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "release-valid")
	corrupt := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "release-corrupt")
	store := NewInMemoryClaimDurableLifecycleStore()
	if _, err := store.ApplyClaimDurableLifecyclePlan(valid); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(valid) error = %v, want nil", err)
	}
	claimed, err := store.ClaimPendingClaimOutboxRecordsForPublish(1, testTime(89))
	if err != nil {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() error = %v, want nil", err)
	}
	if len(claimed) != 1 || claimed[0].ClaimToken == "" {
		t.Fatalf("claimed durable claim outbox rows = %+v, want one in-flight row", claimed)
	}
	corrupt.Commit.Outbox.ReferenceKey = valid.Commit.Boundary.ReferenceKey
	store.plans[corrupt.Commit.Boundary.ClaimReference] = corrupt
	store.references = append(store.references, corrupt.Commit.Boundary.ClaimReference)

	released, err := store.ReleaseExpiredClaimOutboxRecordsForPublish(1, testTime(90), testTime(91))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(released) != 0 {
		t.Fatalf("ReleaseExpiredClaimOutboxRecordsForPublish(valid before corrupt) = %+v/%v, want invalid durable commit", released, err)
	}
	rows := store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ClaimOutboxStatusInFlight || rows[0].ClaimToken == "" {
		t.Fatalf("valid in-flight outbox mutated before corrupt release error: %+v", rows)
	}

	delete(store.plans, corrupt.Commit.Boundary.ClaimReference)
	store.references = store.references[:1]
	if _, ok, err := store.MarkClaimOutboxFailed(claimed[0].OutboxID, claimed[0].ClaimToken, "temporary", testTime(92)); err != nil || !ok {
		t.Fatalf("MarkClaimOutboxFailed(valid) = ok %v err %v, want true nil", ok, err)
	}
	store.plans[corrupt.Commit.Boundary.ClaimReference] = corrupt
	store.references = append(store.references, corrupt.Commit.Boundary.ClaimReference)

	retried, err := store.RetryFailedClaimOutboxRecordsForPublish(1, testTime(93))
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || len(retried) != 0 {
		t.Fatalf("RetryFailedClaimOutboxRecordsForPublish(valid before corrupt) = %+v/%v, want invalid durable commit", retried, err)
	}
	rows = store.OutboxRecords()
	if len(rows) == 0 || rows[0].Status != ClaimOutboxStatusFailed || rows[0].RetriedAt.Equal(testTime(93)) {
		t.Fatalf("valid failed outbox mutated before corrupt retry error: %+v", rows)
	}
}

func TestClaimDurableLifecycleStoreReadbackMissingAndInvalidReferences(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()

	if recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference); err != nil || ok || recovered.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedClaimOutboxDispatchPlan(plan.Commit.Boundary.ClaimReference); err != nil || ok || dispatch.Reference.ClaimReference != "" {
		t.Fatalf("CommittedClaimOutboxDispatchPlan(missing) = %+v/%v/%v, want empty false nil", dispatch, ok, err)
	}
	if recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(""); err == nil || ok || recovered.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
	if dispatch, ok, err := store.CommittedClaimOutboxDispatchPlan(""); err == nil || ok || dispatch.Reference.ClaimReference != "" {
		t.Fatalf("CommittedClaimOutboxDispatchPlan(invalid) = %+v/%v/%v, want error false empty", dispatch, ok, err)
	}

	corruptDelivery := plan
	corruptDelivery.Commit.Outbox.Status = ClaimOutboxStatusPublished
	corruptDelivery.Commit.Outbox.PublishedAt = testTime(90)
	store.plans[plan.Commit.Boundary.ClaimReference] = cloneClaimDurableLifecyclePlan(corruptDelivery)
	store.references = append(store.references, plan.Commit.Boundary.ClaimReference)
	if recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference); !errors.Is(err, ErrInvalidClaimDurableCommit) || ok || recovered.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(corrupt published) = %+v/%v/%v, want invalid durable commit", recovered, ok, err)
	}
}

func TestClaimDurableLifecyclePlanApplyDurableLifecycle(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()

	committed, err := plan.ApplyDurableLifecycle(store)
	if err != nil {
		t.Fatalf("ApplyDurableLifecycle() error = %v, want nil", err)
	}
	if committed.Duplicate || committed.Plan.Commit.Boundary.ClaimReference != plan.Commit.Boundary.ClaimReference {
		t.Fatalf("ApplyDurableLifecycle() result = %+v, want first commit", committed)
	}

	duplicate, err := plan.ApplyDurableLifecycle(store)
	if err != nil {
		t.Fatalf("duplicate ApplyDurableLifecycle() error = %v, want nil", err)
	}
	if !duplicate.Duplicate || len(store.ClaimReferences()) != 1 {
		t.Fatalf("duplicate ApplyDurableLifecycle() = %+v refs %d, want duplicate without append", duplicate, len(store.ClaimReferences()))
	}
}

func TestClaimDurableLifecyclePlanApplyDurableLifecycleRejectsInvalidInputs(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	if result, err := plan.ApplyDurableLifecycle(nil); !errors.Is(err, ErrInvalidClaimDurableCommit) || result.Plan.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("ApplyDurableLifecycle(nil store) = %+v/%v, want invalid durable commit", result, err)
	}

	invalid := plan
	invalid.Commit.Outbox.Status = ClaimOutboxStatusPublished
	store := NewInMemoryClaimDurableLifecycleStore()
	if result, err := invalid.ApplyDurableLifecycle(store); !errors.Is(err, ErrInvalidClaimDurableCommit) || result.Plan.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("ApplyDurableLifecycle(invalid plan) = %+v/%v, want invalid durable commit", result, err)
	}
	if len(store.ClaimReferences()) != 0 {
		t.Fatalf("ClaimReferences() len after invalid ApplyDurableLifecycle = %d, want 0", len(store.ClaimReferences()))
	}

	invalidInit := plan
	invalidInit.ProductionInitialized.Initialization.Created = false
	invalidInit.ProductionInitialized.Initialization.AlreadyInitialized = false
	if result, err := invalidInit.ApplyDurableLifecycle(store); !errors.Is(err, ErrInvalidClaimDurableCommit) || result.Plan.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("ApplyDurableLifecycle(invalid production init) = %+v/%v, want invalid durable commit", result, err)
	}
	if len(store.ClaimReferences()) != 0 {
		t.Fatalf("ClaimReferences() len after invalid production init = %d, want 0", len(store.ClaimReferences()))
	}

	if result, err := (ClaimDurableLifecyclePlan{}).ApplyDurableLifecycle(store); err != nil || result.Plan.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("ApplyDurableLifecycle(no-op) = %+v/%v, want empty nil", result, err)
	}
}

func claimDurableLifecyclePlanForStoreTest(t *testing.T) ClaimDurableLifecyclePlan {
	t.Helper()
	return claimDurableLifecyclePlanForStoreTestWithSuffix(t, "store")
}

func claimDurableLifecyclePlanForStoreTestWithSuffix(t *testing.T, suffix string) ClaimDurableLifecyclePlan {
	t.Helper()
	store := NewInMemoryStore()
	planet := claimTestPlanet(foundation.PlanetID("planet-claim-durable-lifecycle-" + suffix))
	materializeClaimTestPlanet(t, store, planet)
	upsertClaimIntel(t, store, "player-old-scout", planet.ID, testTime(1))
	reference := canonicalClaimReference(t, claimTestPlayerID, planet.ID)

	beginResult := beginClaimWithXCoreForDurableBeginTest(t, store, planet.ID, reference)
	beginPlan, err := beginResult.DurableBeginPlan()
	if err != nil {
		t.Fatalf("DurableBeginPlan() error = %v, want nil", err)
	}

	initRecord := claimProductionInitializationRecordForTest(
		t,
		reference,
		planet.ID,
		planet.Level,
		beginResult.Boundary.Boundary.ClaimedAt,
	)
	initPlan, err := initRecord.DurablePlan(&beginResult.Boundary.Boundary)
	if err != nil {
		t.Fatalf("production init DurablePlan() error = %v, want nil", err)
	}

	completed, err := store.CompletePlanetClaimBoundary(CompletePlanetClaimBoundaryInput{
		ClaimReference:    reference,
		PlayerID:          claimTestPlayerID,
		PlanetID:          planet.ID,
		CompletedAt:       testTime(40),
		StaleListingCount: 1,
	})
	if err != nil {
		t.Fatalf("CompletePlanetClaimBoundary() error = %v, want nil", err)
	}
	commitPlan, err := completed.DurableCommitPlan()
	if err != nil {
		t.Fatalf("DurableCommitPlan() error = %v, want nil", err)
	}
	commitPlanWithXCore, err := NewClaimDurableCommitPlan(
		&commitPlan.Boundary,
		&commitPlan.Reference,
		&commitPlan.Event,
		&commitPlan.Outbox,
		&beginPlan.XCoreConsumption,
	)
	if err != nil {
		t.Fatalf("NewClaimDurableCommitPlan(with xcore) error = %v, want nil", err)
	}
	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &commitPlanWithXCore)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	return plan
}
