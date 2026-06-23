package discovery

import (
	"errors"
	"testing"
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

func TestClaimDurableLifecycleStoreReadbackMissingAndInvalidReferences(t *testing.T) {
	plan := claimDurableLifecyclePlanForStoreTest(t)
	store := NewInMemoryClaimDurableLifecycleStore()

	if recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference); err != nil || ok || recovered.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(missing) = %+v/%v/%v, want empty false nil", recovered, ok, err)
	}
	if recovered, ok, err := store.CommittedClaimDurableLifecyclePlan(""); err == nil || ok || recovered.Commit.Boundary.ClaimReference != "" {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(invalid) = %+v/%v/%v, want error false empty", recovered, ok, err)
	}
}

func claimDurableLifecyclePlanForStoreTest(t *testing.T) ClaimDurableLifecyclePlan {
	t.Helper()
	beginPlan, initPlan, commitPlan := claimDurableLifecyclePlansForTest(t)
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
