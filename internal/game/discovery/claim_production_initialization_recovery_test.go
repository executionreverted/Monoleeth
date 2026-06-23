package discovery

import (
	"errors"
	"reflect"
	"testing"
)

func TestRecoverPendingClaimProductionInitializationsCompletesFromLifecycleReadback(t *testing.T) {
	lifecyclePlan := claimDurableLifecyclePlanForStoreTestWithSuffix(t, "recover-complete")
	pendingPlan := claimProductionInitPendingPlanFromLifecycleForRecoveryTest(t, lifecyclePlan)
	initStore := NewInMemoryClaimProductionInitializationDurableStore()
	lifecycleStore := NewInMemoryClaimDurableLifecycleStore()
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(pendingPlan); err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(pending) error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(lifecyclePlan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	result, err := RecoverPendingClaimProductionInitializations(ClaimProductionInitializationRecoveryInput{
		ProductionInitializations: initStore,
		Lifecycles:                lifecycleStore,
		Limit:                     10,
	})
	if err != nil {
		t.Fatalf("RecoverPendingClaimProductionInitializations() error = %v, want nil", err)
	}
	if result.Scanned != 1 || result.Completed != 1 || result.SkippedMissingClaim != 0 ||
		!reflect.DeepEqual(result.References, []PlanetClaimReference{lifecyclePlan.Commit.Boundary.ClaimReference}) {
		t.Fatalf("recovery result = %+v, want one completed reference", result)
	}
	recovered, ok, err := initStore.CommittedClaimProductionInitializationDurablePlan(lifecyclePlan.Commit.Boundary.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() = ok %v err %v, want true nil", ok, err)
	}
	if recovered.Boundary.Status != ClaimBoundaryStatusComplete ||
		recovered.Boundary.StaleListingCount != lifecyclePlan.Commit.Boundary.StaleListingCount {
		t.Fatalf("recovered init boundary = %+v, want completed lifecycle boundary", recovered.Boundary)
	}

	again, err := RecoverPendingClaimProductionInitializations(ClaimProductionInitializationRecoveryInput{
		ProductionInitializations: initStore,
		Lifecycles:                lifecycleStore,
		Limit:                     10,
	})
	if err != nil {
		t.Fatalf("second RecoverPendingClaimProductionInitializations() error = %v, want nil", err)
	}
	if again.Scanned != 0 || again.Completed != 0 {
		t.Fatalf("second recovery result = %+v, want no pending rows after completion", again)
	}
}

func TestRecoverPendingClaimProductionInitializationsSkipsMissingLifecycle(t *testing.T) {
	pendingPlan := claimProductionInitializationDurableStorePlanForTest(t, "recover-missing", false)
	initStore := NewInMemoryClaimProductionInitializationDurableStore()
	lifecycleStore := NewInMemoryClaimDurableLifecycleStore()
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(pendingPlan); err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(pending) error = %v, want nil", err)
	}

	result, err := RecoverPendingClaimProductionInitializations(ClaimProductionInitializationRecoveryInput{
		ProductionInitializations: initStore,
		Lifecycles:                lifecycleStore,
		Limit:                     10,
	})
	if err != nil {
		t.Fatalf("RecoverPendingClaimProductionInitializations(missing) error = %v, want nil", err)
	}
	if result.Scanned != 1 || result.Completed != 0 || result.SkippedMissingClaim != 1 || len(result.References) != 0 {
		t.Fatalf("missing lifecycle recovery result = %+v, want skipped pending row", result)
	}
	pending, err := initStore.PendingClaimProductionInitializationDurablePlans(10)
	if err != nil {
		t.Fatalf("PendingClaimProductionInitializationDurablePlans() error = %v, want nil", err)
	}
	if len(pending) != 1 || pending[0].Boundary.Status != ClaimBoundaryStatusPendingSideEffects {
		t.Fatalf("pending after missing lifecycle = %+v, want row preserved", pending)
	}
}

func TestRecoverPendingClaimProductionInitializationsRejectsLifecycleWithoutInit(t *testing.T) {
	beginPlan, pendingInitPlan, commitPlan := claimDurableLifecyclePlansForTest(t)
	lifecyclePlan, err := NewClaimDurableLifecyclePlan(&beginPlan, nil, &commitPlan)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan(no init) error = %v, want nil", err)
	}
	initStore := NewInMemoryClaimProductionInitializationDurableStore()
	lifecycleStore := NewInMemoryClaimDurableLifecycleStore()
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(pendingInitPlan); err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan(pending) error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(lifecyclePlan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan(no init) error = %v, want nil", err)
	}

	result, err := RecoverPendingClaimProductionInitializations(ClaimProductionInitializationRecoveryInput{
		ProductionInitializations: initStore,
		Lifecycles:                lifecycleStore,
		Limit:                     10,
	})
	if !errors.Is(err, ErrInvalidClaimDurableCommit) || result.Scanned != 0 {
		t.Fatalf("RecoverPendingClaimProductionInitializations(no init) = %+v/%v, want invalid durable commit", result, err)
	}
}

func claimProductionInitPendingPlanFromLifecycleForRecoveryTest(
	t *testing.T,
	lifecycle ClaimDurableLifecyclePlan,
) ClaimProductionInitializationDurablePlan {
	t.Helper()
	pendingBoundary := lifecycle.Begin.Boundary
	pendingPlan, err := lifecycle.ProductionInitialized.Initialization.DurablePlan(&pendingBoundary)
	if err != nil {
		t.Fatalf("pending DurablePlan() error = %v, want nil", err)
	}
	return pendingPlan
}
