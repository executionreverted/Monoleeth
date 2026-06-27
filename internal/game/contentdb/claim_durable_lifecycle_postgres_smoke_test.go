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

// claimDurableLifecyclePlanForSmoke builds one lifecycle plan whose boundary
// reference is unique per test. The Postgres adapter persists the whole plan
// as opaque JSON, so the plan does not need to pass discovery-domain
// validation here; the adapter only checks the claim reference and the
// serialized plan bytes for idempotency.
func claimDurableLifecyclePlanForSmoke(t *testing.T, suffix string) discovery.ClaimDurableLifecyclePlan {
	t.Helper()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	return discovery.ClaimDurableLifecyclePlan{
		Commit: discovery.ClaimDurableCommitPlan{
			Boundary: discovery.ClaimBoundaryRecord{
				ClaimReference: discovery.PlanetClaimReference("claim-smoke-" + suffix),
				ReferenceKey:   foundation.IdempotencyKey("idem-smoke-" + suffix),
				PlayerID:       foundation.PlayerID("player-smoke-" + suffix),
				PlanetID:       foundation.PlanetID("planet-smoke-" + suffix),
				Status:         discovery.ClaimBoundaryStatusComplete,
				EventID:        foundation.EventID("event-smoke-" + suffix),
				ClaimedAt:      now,
				RecordedAt:     now,
				CompletedAt:    now.Add(time.Second),
			},
		},
	}
}

func TestPostgresClaimDurableLifecycleStorePersistsPlanAcrossReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := claimDurableLifecyclePlanForSmoke(t, "persist")

	lifecycleStore, err := contentdb.NewClaimDurableLifecycleStore(store)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecycleStore() error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewClaimDurableLifecycleStore(store)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecycleStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.CommittedClaimDurableLifecyclePlan(plan.Commit.Boundary.ClaimReference)
	if err != nil {
		t.Fatalf("CommittedClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan(%q) ok = false, want true", plan.Commit.Boundary.ClaimReference)
	}
	if loaded.Commit.Boundary.ClaimReference != plan.Commit.Boundary.ClaimReference ||
		loaded.Commit.Boundary.PlayerID != plan.Commit.Boundary.PlayerID ||
		loaded.Commit.Boundary.PlanetID != plan.Commit.Boundary.PlanetID ||
		!loaded.Commit.Boundary.ClaimedAt.Equal(plan.Commit.Boundary.ClaimedAt) {
		t.Fatalf("loaded lifecycle boundary = %+v, want persisted boundary from %+v", loaded.Commit.Boundary, plan.Commit.Boundary)
	}
}

func TestPostgresClaimDurableLifecycleStoreDuplicateReplayIsIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := claimDurableLifecyclePlanForSmoke(t, "duplicate")

	lifecycleStore, err := contentdb.NewClaimDurableLifecycleStore(store)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecycleStore() error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("first ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	second, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(plan)
	if err != nil {
		t.Fatalf("second ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatalf("duplicate replay Duplicate = false, want true")
	}
}

func TestPostgresClaimDurableLifecycleStorePublishesCommittedOutboxRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := claimDurableLifecyclePlanForSmoke(t, "outbox-publish")
	plan.Commit.Outbox = discovery.ClaimOutboxRecord{
		OutboxID:       "claim-outbox-smoke-publish",
		Status:         discovery.ClaimOutboxStatusPending,
		ReferenceKey:   plan.Commit.Boundary.ReferenceKey,
		ClaimReference: plan.Commit.Boundary.ClaimReference,
		CreatedAt:      plan.Commit.Boundary.CompletedAt,
	}

	lifecycleStore, err := contentdb.NewClaimDurableLifecycleStore(store)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecycleStore() error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}
	claimed, err := lifecycleStore.ClaimPendingClaimOutboxRecordsForPublish(1, plan.Commit.Boundary.CompletedAt.Add(time.Second))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimPendingClaimOutboxRecordsForPublish() = %+v/%v, want one row nil", claimed, err)
	}
	if _, ok, err := lifecycleStore.MarkClaimOutboxPublished(claimed[0].OutboxID, claimed[0].ClaimToken, plan.Commit.Boundary.CompletedAt.Add(2*time.Second)); err != nil || !ok {
		t.Fatalf("MarkClaimOutboxPublished() ok=%v err=%v, want true nil", ok, err)
	}
	records := lifecycleStore.OutboxRecords()
	if len(records) != 1 || records[0].Status != discovery.ClaimOutboxStatusPublished {
		t.Fatalf("OutboxRecords() = %+v, want one published row", records)
	}
}

func TestPostgresClaimDurableLifecycleStoreRejectsConflictingReference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	plan := claimDurableLifecyclePlanForSmoke(t, "conflict")

	lifecycleStore, err := contentdb.NewClaimDurableLifecycleStore(store)
	if err != nil {
		t.Fatalf("NewClaimDurableLifecycleStore() error = %v, want nil", err)
	}
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(plan); err != nil {
		t.Fatalf("first ApplyClaimDurableLifecyclePlan() error = %v, want nil", err)
	}

	conflict := plan
	conflict.Commit.Boundary.PlayerID = foundation.PlayerID("player-smoke-conflict-other")
	if _, err := lifecycleStore.ApplyClaimDurableLifecyclePlan(conflict); !errors.Is(err, discovery.ErrInvalidClaimDurableCommit) {
		t.Fatalf("conflicting ApplyClaimDurableLifecyclePlan() error = %v, want ErrInvalidClaimDurableCommit", err)
	}
}
