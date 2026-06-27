package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

func routeDurablePlanForSmoke(suffix string, expectedRevision uint64) production.AutomationRouteDurableCommitPlan {
	return production.AutomationRouteDurableCommitPlan{
		Route: production.AutomationRoute{
			RouteID:           foundation.RouteID("route-smoke-" + suffix),
			OwnerPlayerID:     foundation.PlayerID("player-smoke-route"),
			SourcePlanetID:    foundation.PlanetID("planet-smoke-route-src"),
			SourceMapID:       production.RouteMapID("map-starter"),
			DestinationMapID:  production.RouteMapID("map-starter"),
			Destination:       production.RouteDestination{Type: production.RouteDestinationTypePlanet, ID: "planet-smoke-route-dst"},
			ResourceItemID:    foundation.ItemID("item-x-core"),
			AmountPerHour:     100,
			EnergyCostPerHour: 10,
			Enabled:           true,
			LastCalculatedAt:  time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
			UpdatedAt:         time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
			CreatedAt:         time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
		ReferenceKey:     foundation.IdempotencyKey("route_create:route-smoke-" + suffix + ":0"),
		ExpectedRevision: expectedRevision,
		RecordedAt:       time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
	}
}

func TestPostgresAutomationRouteDurableStorePersistsRecordAcrossReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := routeDurablePlanForSmoke("persist", 0)

	routeStore, err := contentdb.NewAutomationRouteDurableStore(store)
	if err != nil {
		t.Fatalf("NewAutomationRouteDurableStore() error = %v, want nil", err)
	}
	result, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if result.Record.Revision != 1 {
		t.Fatalf("first commit revision = %d, want 1", result.Record.Revision)
	}

	reopened, err := contentdb.NewAutomationRouteDurableStore(store)
	if err != nil {
		t.Fatalf("NewAutomationRouteDurableStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.CommittedAutomationRouteDurableRecord(plan.Route.RouteID)
	if err != nil {
		t.Fatalf("CommittedAutomationRouteDurableRecord() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("CommittedAutomationRouteDurableRecord(%q) ok = false, want true", plan.Route.RouteID)
	}
	if loaded.Route.RouteID != plan.Route.RouteID || loaded.Revision != 1 {
		t.Fatalf("loaded route = %+v, want persisted route revision 1", loaded)
	}
}

func TestPostgresAutomationRouteDurableStoreDuplicateReferenceIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := routeDurablePlanForSmoke("duplicate", 0)

	routeStore, err := contentdb.NewAutomationRouteDurableStore(store)
	if err != nil {
		t.Fatalf("NewAutomationRouteDurableStore() error = %v, want nil", err)
	}
	if _, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan); err != nil {
		t.Fatalf("first ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	second, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("second ApplyAutomationRouteDurableCommitPlan() error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate replay Duplicate = false, want true")
	}
}

func TestPostgresAutomationRouteDurableStoreRevisionCAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	routeStore, err := contentdb.NewAutomationRouteDurableStore(store)
	if err != nil {
		t.Fatalf("NewAutomationRouteDurableStore() error = %v, want nil", err)
	}

	plan1 := routeDurablePlanForSmoke("cas", 0)
	if _, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan1); err != nil {
		t.Fatalf("first commit error = %v", err)
	}

	// Wrong expected revision → must fail.
	plan2 := plan1
	plan2.ReferenceKey = foundation.IdempotencyKey("route_update:route-smoke-cas:wrong")
	plan2.ExpectedRevision = 99
	if _, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan2); err == nil {
		t.Fatalf("stale revision commit succeeded, want error")
	}

	// Correct expected revision → must succeed with revision 2.
	plan3 := plan1
	plan3.ReferenceKey = foundation.IdempotencyKey("route_update:route-smoke-cas:player-smoke-route:1")
	plan3.ExpectedRevision = 1
	result, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan3)
	if err != nil {
		t.Fatalf("correct revision commit error = %v", err)
	}
	if result.Record.Revision != 2 {
		t.Fatalf("second commit revision = %d, want 2", result.Record.Revision)
	}
}

func TestPostgresAutomationRouteDurableStoreListsRoutesByOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	routeStore, err := contentdb.NewAutomationRouteDurableStore(store)
	if err != nil {
		t.Fatalf("NewAutomationRouteDurableStore() error = %v, want nil", err)
	}

	plan1 := routeDurablePlanForSmoke("owner-1", 0)
	plan2 := routeDurablePlanForSmoke("owner-2", 0)
	if _, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan1); err != nil {
		t.Fatalf("commit route-1 error = %v", err)
	}
	if _, err := routeStore.ApplyAutomationRouteDurableCommitPlan(plan2); err != nil {
		t.Fatalf("commit route-2 error = %v", err)
	}

	records, err := routeStore.CommittedAutomationRouteDurableRecordsForOwner(foundation.PlayerID("player-smoke-route"))
	if err != nil {
		t.Fatalf("CommittedAutomationRouteDurableRecordsForOwner() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("owner route count = %d, want 2", len(records))
	}
}
