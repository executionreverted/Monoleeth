package discovery

import (
	"errors"
	"testing"
)

func TestClaimDurableLifecyclePlanWithProductionInitialization(t *testing.T) {
	beginPlan, initPlan, commitPlan := claimDurableLifecyclePlansForTest(t)
	commitPlanWithXCore, err := NewClaimDurableCommitPlan(
		&commitPlan.Boundary,
		&commitPlan.Reference,
		&commitPlan.Event,
		&commitPlan.Outbox,
		&beginPlan.XCoreConsumption,
	)
	if err != nil {
		t.Fatalf("NewClaimDurableCommitPlan(with begin xcore) error = %v, want nil", err)
	}

	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &commitPlanWithXCore)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	if plan.Begin.Boundary.ClaimReference != commitPlanWithXCore.Boundary.ClaimReference ||
		!plan.HasProductionInit ||
		plan.ProductionInitialized.Initialization.PlanetID != commitPlanWithXCore.Boundary.PlanetID {
		t.Fatalf("claim durable lifecycle plan = %+v, want same claim with production init", plan)
	}

	beginPlan.Boundary.PlanetID = "mutated"
	if plan.Begin.Boundary.PlanetID == "mutated" {
		t.Fatal("claim durable lifecycle plan reused input boundary, want cloned evidence")
	}
}

func TestClaimDurableLifecyclePlanWithoutProductionInitialization(t *testing.T) {
	beginPlan, _, commitPlan := claimDurableLifecyclePlansForTest(t)

	plan, err := NewClaimDurableLifecyclePlan(&beginPlan, nil, &commitPlan)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecyclePlan(no production init) error = %v, want nil", err)
	}
	if plan.HasProductionInit || plan.ProductionInitialized.Initialization.ClaimReference != "" {
		t.Fatalf("claim durable lifecycle no-init plan = %+v, want no production init evidence", plan)
	}
}

func TestClaimDurableLifecyclePlanNoOpAndInvalidRows(t *testing.T) {
	if plan, err := NewClaimDurableLifecyclePlan(nil, nil, nil); err != nil || plan.Begin.Boundary.ClaimReference != "" {
		t.Fatalf("NewClaimDurableLifecyclePlan(no-op) = %+v/%v, want empty nil", plan, err)
	}

	beginPlan, initPlan, commitPlan := claimDurableLifecyclePlansForTest(t)
	if _, err := NewClaimDurableLifecyclePlan(nil, &initPlan, &commitPlan); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(missing begin) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, nil); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(missing commit) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	debitOnlyBegin := ClaimDurableBeginPlan{XCoreConsumption: beginPlan.XCoreConsumption}
	if _, err := NewClaimDurableLifecyclePlan(&debitOnlyBegin, nil, &commitPlan); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(debit-only begin) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongCommit := commitPlan
	wrongCommit.Boundary.EventID = "event_other"
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &wrongCommit); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong commit) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongCommit = commitPlan
	wrongCommit.Boundary.CompletedAt = beginPlan.Boundary.ClaimedAt.Add(-1)
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &wrongCommit); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(commit before begin) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongCommit = commitPlan
	wrongCommit.Boundary.StaleIntelCount++
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &wrongCommit); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong stale intel count) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	commitWithXCore, err := NewClaimDurableCommitPlan(
		&commitPlan.Boundary,
		&commitPlan.Reference,
		&commitPlan.Event,
		&commitPlan.Outbox,
		&beginPlan.XCoreConsumption,
	)
	if err != nil {
		t.Fatalf("NewClaimDurableCommitPlan(with xcore) error = %v, want nil", err)
	}
	commitWithXCore.XCoreConsumption.ConsumedAt = commitWithXCore.XCoreConsumption.ConsumedAt.Add(1)
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &initPlan, &commitWithXCore); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong commit xcore) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongInit := initPlan
	wrongInit.Initialization.PlanetID = "planet-other"
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &wrongInit, &commitPlan); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong init) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongInit = initPlan
	wrongInit.Initialization.PlanetLevel++
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &wrongInit, &commitPlan); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong init planet level) error = %v, want ErrInvalidClaimDurableCommit", err)
	}

	wrongInit = initPlan
	wrongInit.Boundary.EventID = "event_other"
	if _, err := NewClaimDurableLifecyclePlan(&beginPlan, &wrongInit, &commitPlan); !errors.Is(err, ErrInvalidClaimDurableCommit) {
		t.Fatalf("NewClaimDurableLifecyclePlan(wrong init boundary) error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func claimDurableLifecyclePlansForTest(
	t *testing.T,
) (ClaimDurableBeginPlan, ClaimProductionInitializationDurablePlan, ClaimDurableCommitPlan) {
	t.Helper()
	store := NewInMemoryStore()
	planet := claimTestPlanet("planet-claim-durable-lifecycle")
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
	return beginPlan, initPlan, commitPlan
}
