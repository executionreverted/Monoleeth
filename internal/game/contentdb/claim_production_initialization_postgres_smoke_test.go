package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
)

func productionInitPlanForSmoke(suffix string) discovery.ClaimProductionInitializationDurablePlan {
	return discovery.ClaimProductionInitializationDurablePlan{
		Initialization: discovery.ClaimProductionInitializationRecord{
			ClaimReference: discovery.PlanetClaimReference("prodinit-smoke-" + suffix),
			PlayerID:       foundation.PlayerID("player-prodinit-smoke"),
			PlanetID:       foundation.PlanetID("planet-prodinit-smoke-" + suffix),
			PlanetLevel:    3,
			ClaimedAt:      time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
	}
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
