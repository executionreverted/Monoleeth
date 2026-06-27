package contentdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
)

func productionInitPlanForSmoke(suffix string) discovery.ClaimProductionInitializationDurablePlan {
	playerID := foundation.PlayerID("player-prodinit-smoke")
	planetID := foundation.PlanetID("planet-prodinit-smoke-" + suffix)
	referenceKey, err := foundation.PlanetClaimIdempotencyKey(playerID, planetID)
	if err != nil {
		panic(err)
	}
	claimedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	reference := discovery.PlanetClaimReference(referenceKey.String())
	return discovery.ClaimProductionInitializationDurablePlan{
		Initialization: discovery.ClaimProductionInitializationRecord{
			ClaimReference: reference,
			ReferenceKey:   referenceKey,
			PlayerID:       playerID,
			PlanetID:       planetID,
			PlanetLevel:    3,
			ClaimedAt:      claimedAt,
			InitializedAt:  claimedAt.Add(time.Second),
			Created:        true,
		},
		Boundary: discovery.ClaimBoundaryRecord{
			ClaimReference:  reference,
			ReferenceKey:    referenceKey,
			PlayerID:        playerID,
			PlanetID:        planetID,
			Status:          discovery.ClaimBoundaryStatusPendingSideEffects,
			EventID:         foundation.EventID("claim-event-" + suffix),
			ClaimedAt:       claimedAt,
			RecordedAt:      claimedAt,
			StaleIntelCount: 1,
		},
	}
}

func completeProductionInitPlanForSmoke(plan discovery.ClaimProductionInitializationDurablePlan) discovery.ClaimProductionInitializationDurablePlan {
	plan.Boundary.Status = discovery.ClaimBoundaryStatusComplete
	plan.Boundary.CompletedAt = plan.Boundary.ClaimedAt.Add(30 * time.Second)
	plan.Boundary.StaleListingCount = 1
	return plan
}

func TestPostgresClaimProductionInitializationDurableStorePersistsPlanAcrossReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := productionInitPlanForSmoke("persist")

	initStore, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore() error = %v, want nil", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan); err != nil {
		t.Fatalf("ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.CommittedClaimProductionInitializationDurablePlan(plan.Initialization.ClaimReference)
	if err != nil {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan(%q) ok = false, want true", plan.Initialization.ClaimReference)
	}
	if loaded.Initialization.ClaimReference != plan.Initialization.ClaimReference ||
		loaded.Initialization.PlanetID != plan.Initialization.PlanetID {
		t.Fatalf("loaded production init = %+v, want persisted plan", loaded.Initialization)
	}
}

func TestPostgresClaimProductionInitializationDurableStoreDuplicateReplayIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := productionInitPlanForSmoke("duplicate")

	initStore, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore() error = %v, want nil", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan); err != nil {
		t.Fatalf("first ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	second, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan)
	if err != nil {
		t.Fatalf("second ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate replay Duplicate = false, want true")
	}
}

func TestPostgresClaimProductionInitializationDurableStoreListsPendingPlans(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	initStore, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore() error = %v, want nil", err)
	}
	plan1 := productionInitPlanForSmoke("list-1")
	plan2 := productionInitPlanForSmoke("list-2")
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan1); err != nil {
		t.Fatalf("commit plan-1 error = %v", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan2); err != nil {
		t.Fatalf("commit plan-2 error = %v", err)
	}

	pending, err := initStore.PendingClaimProductionInitializationDurablePlans(10)
	if err != nil {
		t.Fatalf("PendingClaimProductionInitializationDurablePlans() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending plan count = %d, want 2", len(pending))
	}
}

func TestPostgresClaimProductionInitializationDurableStoreRejectsConflictingReferenceReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := productionInitPlanForSmoke("conflict")
	conflict := plan
	conflict.Initialization.PlanetLevel = plan.Initialization.PlanetLevel + 1

	initStore, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore() error = %v, want nil", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(plan); err != nil {
		t.Fatalf("first ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(conflict); !errors.Is(err, discovery.ErrInvalidClaimDurableCommit) {
		t.Fatalf("conflicting ApplyClaimProductionInitializationDurablePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}

func TestPostgresClaimProductionInitializationDurableStoreAdvancesPendingToCompleteAndFiltersPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	pendingPlan := productionInitPlanForSmoke("advance")
	completePlan := completeProductionInitPlanForSmoke(pendingPlan)

	initStore, err := contentdb.NewClaimProductionInitializationDurableStore(store)
	if err != nil {
		t.Fatalf("NewClaimProductionInitializationDurableStore() error = %v, want nil", err)
	}
	if _, err := initStore.ApplyClaimProductionInitializationDurablePlan(pendingPlan); err != nil {
		t.Fatalf("pending ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	advanced, err := initStore.ApplyClaimProductionInitializationDurablePlan(completePlan)
	if err != nil {
		t.Fatalf("complete ApplyClaimProductionInitializationDurablePlan() error = %v, want nil", err)
	}
	if advanced.Duplicate {
		t.Fatalf("advanced Duplicate = true, want false")
	}
	loaded, ok, err := initStore.CommittedClaimProductionInitializationDurablePlan(pendingPlan.Initialization.ClaimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() = ok %v err %v, want true nil", ok, err)
	}
	if loaded.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		t.Fatalf("loaded status = %q, want complete", loaded.Boundary.Status)
	}
	pending, err := initStore.PendingClaimProductionInitializationDurablePlans(10)
	if err != nil {
		t.Fatalf("PendingClaimProductionInitializationDurablePlans() error = %v, want nil", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending plan count after complete = %d, want 0", len(pending))
	}
	staleReplay, err := initStore.ApplyClaimProductionInitializationDurablePlan(pendingPlan)
	if err != nil {
		t.Fatalf("stale pending replay error = %v, want nil", err)
	}
	if !staleReplay.Duplicate || staleReplay.Plan.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		t.Fatalf("stale replay = %+v, want duplicate complete plan", staleReplay)
	}
}
