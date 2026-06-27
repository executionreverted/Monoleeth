package contentdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

func TestPostgresBuildingMutationDurableStorePersistsPlanAcrossReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.BuildingMutationDurableCommitPlan{
		Reference: production.BuildingMutationReferenceRecord{
			ReferenceKey: foundation.IdempotencyKey("planet_building_build:planet-smoke-bp:building-bp"),
			Operation:    production.BuildingMutationKind("build"),
			PlanetID:     foundation.PlanetID("planet-smoke-building"),
			BuildingID:   production.BuildingID("building-smoke-1"),
			RecordedAt:   time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
	}

	mutationStore, err := contentdb.NewBuildingMutationDurableStore(store)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableStore() error = %v, want nil", err)
	}
	if _, err := mutationStore.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewBuildingMutationDurableStore(store)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.CommittedBuildingMutationDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("CommittedBuildingMutationDurableCommitPlan(%q) ok = false, want true", plan.Reference.ReferenceKey)
	}
	if loaded.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		loaded.Reference.PlanetID != plan.Reference.PlanetID ||
		loaded.Reference.BuildingID != plan.Reference.BuildingID {
		t.Fatalf("loaded building mutation = %+v, want persisted reference", loaded.Reference)
	}
}

func TestPostgresBuildingMutationDurableStoreDuplicateReplayIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.BuildingMutationDurableCommitPlan{
		Reference: production.BuildingMutationReferenceRecord{
			ReferenceKey: foundation.IdempotencyKey("planet_building_build:planet-smoke-bd:building-bd"),
			Operation:    production.BuildingMutationKind("build"),
			PlanetID:     foundation.PlanetID("planet-smoke-building-dup"),
			BuildingID:   production.BuildingID("building-smoke-dup-1"),
			RecordedAt:   time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
	}

	mutationStore, err := contentdb.NewBuildingMutationDurableStore(store)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableStore() error = %v, want nil", err)
	}
	if _, err := mutationStore.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("first ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	second, err := mutationStore.ApplyBuildingMutationDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("second ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate replay Duplicate = false, want true")
	}
}

func TestPostgresBuildingMutationDurableStoreRejectsConflictingReference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.BuildingMutationDurableCommitPlan{
		Reference: production.BuildingMutationReferenceRecord{
			ReferenceKey: foundation.IdempotencyKey("planet_building_build:planet-smoke-bc:building-bc"),
			Operation:    production.BuildingMutationKind("build"),
			PlanetID:     foundation.PlanetID("planet-smoke-building-conflict"),
			BuildingID:   production.BuildingID("building-smoke-conflict-1"),
			RecordedAt:   time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
	}

	mutationStore, err := contentdb.NewBuildingMutationDurableStore(store)
	if err != nil {
		t.Fatalf("NewBuildingMutationDurableStore() error = %v, want nil", err)
	}
	if _, err := mutationStore.ApplyBuildingMutationDurableCommitPlan(plan); err != nil {
		t.Fatalf("first ApplyBuildingMutationDurableCommitPlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.Reference.BuildingID = production.BuildingID("building-smoke-other")
	if _, err := mutationStore.ApplyBuildingMutationDurableCommitPlan(conflict); !errors.Is(err, production.ErrInvalidBuildingMutationDurableCommit) {
		t.Fatalf("conflicting ApplyBuildingMutationDurableCommitPlan() error = %v, want ErrInvalidBuildingMutationDurableCommit", err)
	}
}

func TestPostgresSettlementDurableStorePersistsPlanAcrossReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.SettlementDurableCommitPlan{
		Reference: production.SettlementReferenceRecord{
			ReferenceKey:     foundation.IdempotencyKey("offline_settlement:planet-smoke-sp:20260626T1200Z"),
			SettlementWindow: "2026-06-26T12:00:00Z",
			Kind:             production.SettlementKind("production"),
			PlanetID:         foundation.PlanetID("planet-smoke-settlement"),
			AppliedAt:        time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
			RecordedAt:       time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		},
	}

	settlementStore, err := contentdb.NewSettlementDurableStore(store)
	if err != nil {
		t.Fatalf("NewSettlementDurableStore() error = %v, want nil", err)
	}
	if _, err := settlementStore.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewSettlementDurableStore(store)
	if err != nil {
		t.Fatalf("NewSettlementDurableStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.CommittedSettlementDurableCommitPlan(plan.Reference.ReferenceKey)
	if err != nil {
		t.Fatalf("CommittedSettlementDurableCommitPlan() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("CommittedSettlementDurableCommitPlan(%q) ok = false, want true", plan.Reference.ReferenceKey)
	}
	if loaded.Reference.ReferenceKey != plan.Reference.ReferenceKey ||
		loaded.Reference.PlanetID != plan.Reference.PlanetID ||
		loaded.Reference.SettlementWindow != plan.Reference.SettlementWindow {
		t.Fatalf("loaded settlement = %+v, want persisted reference", loaded.Reference)
	}
}

func TestPostgresSettlementDurableStoreDuplicateReplayIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.SettlementDurableCommitPlan{
		Reference: production.SettlementReferenceRecord{
			ReferenceKey:     foundation.IdempotencyKey("offline_settlement:planet-smoke-sd:20260626T1300Z"),
			SettlementWindow: "2026-06-26T13:00:00Z",
			Kind:             production.SettlementKind("production"),
			PlanetID:         foundation.PlanetID("planet-smoke-settlement-dup"),
			AppliedAt:        time.Date(2026, 6, 26, 13, 0, 0, 0, time.UTC),
			RecordedAt:       time.Date(2026, 6, 26, 13, 0, 0, 0, time.UTC),
		},
	}

	settlementStore, err := contentdb.NewSettlementDurableStore(store)
	if err != nil {
		t.Fatalf("NewSettlementDurableStore() error = %v, want nil", err)
	}
	if _, err := settlementStore.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("first ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}
	second, err := settlementStore.ApplySettlementDurableCommitPlan(plan)
	if err != nil {
		t.Fatalf("second ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate replay Duplicate = false, want true")
	}
}

func TestPostgresSettlementDurableStoreRejectsConflictingReference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := production.SettlementDurableCommitPlan{
		Reference: production.SettlementReferenceRecord{
			ReferenceKey:     foundation.IdempotencyKey("offline_settlement:planet-smoke-sc:20260626T1400Z"),
			SettlementWindow: "2026-06-26T14:00:00Z",
			Kind:             production.SettlementKind("production"),
			PlanetID:         foundation.PlanetID("planet-smoke-settlement-conflict"),
			AppliedAt:        time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC),
			RecordedAt:       time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC),
		},
	}

	settlementStore, err := contentdb.NewSettlementDurableStore(store)
	if err != nil {
		t.Fatalf("NewSettlementDurableStore() error = %v, want nil", err)
	}
	if _, err := settlementStore.ApplySettlementDurableCommitPlan(plan); err != nil {
		t.Fatalf("first ApplySettlementDurableCommitPlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.Reference.PlanetID = foundation.PlanetID("planet-smoke-settlement-other")
	if _, err := settlementStore.ApplySettlementDurableCommitPlan(conflict); !errors.Is(err, production.ErrInvalidSettlementDurableCommit) {
		t.Fatalf("conflicting ApplySettlementDurableCommitPlan() error = %v, want ErrInvalidSettlementDurableCommit", err)
	}
}
