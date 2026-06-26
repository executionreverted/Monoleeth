package contentdb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
)

func publishSmokeInput(t *testing.T, id, version, idempotencyKey, expectedCurrent string, snapshot content.Snapshot, auditEntries []content.AuditLogEntryInput) content.PublishSnapshotInput {
	t.Helper()
	return content.PublishSnapshotInput{
		ID:                   id,
		Version:              version,
		Snapshot:             snapshot,
		ValidationReportJSON: []byte(`{}`),
		IdempotencyKey:       idempotencyKey,
		ExpectedCurrentID:    expectedCurrent,
		Notes:                "publish smoke",
		BalanceTag:           "smoke_balance",
		CreatedBy:            "smoke",
		PublishedBy:          "smoke",
		PublishedAt:          time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC),
		AuditEntries:         auditEntries,
	}
}

func smokeAuditEntry(auditID, versionID, itemSuffix string) content.AuditLogEntryInput {
	return content.AuditLogEntryInput{
		ID:               auditID,
		ContentVersionID: versionID,
		ContentType:      content.ContentTypeItem,
		ContentID:        content.ContentID("publish_smoke_item_" + itemSuffix),
		Action:           content.AuditActionPublish,
		FieldPath:        "$",
		NewValueJSON:     []byte(`{"content_id":"x","enabled":true,"data_json":{}}`),
		ActorAccountID:   "smoke",
		Note:             "publish smoke",
	}
}

// TestPostgresPublishIsIdempotentForDuplicateKey verifies a replayed publish
// (same idempotency key + snapshot) returns the original version idempotently
// without minting a second version or duplicate audit rows.
func TestPostgresPublishIsIdempotentForDuplicateKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	snapshot := buildPostgresSmokeSnapshot(t, "content_idem_v1")
	key := "content_publish:idem-smoke-1"
	first := publishSmokeInput(t, "11111111-1111-5111-8111-111111111111", "content_idem_v1", key, "", snapshot, []content.AuditLogEntryInput{smokeAuditEntry("dddddddd-1111-5ddd-8ddd-ddddddddddd1", "11111111-1111-5111-8111-111111111111", "idem")})
	result, err := store.PublishContentSnapshot(ctx, first)
	if err != nil {
		t.Fatalf("first PublishContentSnapshot() error = %v, want nil", err)
	}
	if result.Idempotent {
		t.Fatalf("first publish idempotent = true, want false")
	}

	replay := publishSmokeInput(t, "22222222-2222-5222-8222-222222222222", "content_idem_v1", key, "", snapshot, []content.AuditLogEntryInput{smokeAuditEntry("dddddddd-2222-5ddd-8ddd-ddddddddddd2", "22222222-2222-5222-8222-222222222222", "replay")})
	replayed, err := store.PublishContentSnapshot(ctx, replay)
	if err != nil {
		t.Fatalf("replay PublishContentSnapshot() error = %v, want nil", err)
	}
	if !replayed.Idempotent || replayed.Record.ID != result.Record.ID {
		t.Fatalf("replay result = %+v, want idempotent with original id %s", replayed, result.Record.ID)
	}
	if got := countContentVersions(t, ctx, db); got != 1 {
		t.Fatalf("content_versions count = %d, want 1 after idempotent replay", got)
	}
	if got := countAuditRowsForVersion(t, ctx, db, result.Record.ID); got != 1 {
		t.Fatalf("audit rows for version = %d, want 1 (no duplicate from replay)", got)
	}
}

// TestPostgresPublishRejectsStaleCurrentVersion verifies the publish CAS guard:
// a publish whose ExpectedCurrentID no longer matches the live current version
// is rejected with ErrContentPublishConflict and leaves no partial state.
func TestPostgresPublishRejectsStaleCurrentVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, store := openPostgresSmokeStore(t, ctx)
	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	v1 := buildPostgresSmokeSnapshot(t, "content_cas_v1")
	first := publishSmokeInput(t, "11111111-1111-5111-8111-111111111111", "content_cas_v1", "content_publish:cas-v1", "", v1, nil)
	firstResult, err := store.PublishContentSnapshot(ctx, first)
	if err != nil {
		t.Fatalf("v1 PublishContentSnapshot() error = %v, want nil", err)
	}

	v2 := buildPostgresSmokeSnapshot(t, "content_cas_v2")
	second := publishSmokeInput(t, "22222222-2222-5222-8222-222222222222", "content_cas_v2", "content_publish:cas-v2", firstResult.Record.ID, v2, nil)
	secondResult, err := store.PublishContentSnapshot(ctx, second)
	if err != nil {
		t.Fatalf("v2 PublishContentSnapshot() error = %v, want nil", err)
	}
	if !secondResult.Record.Current {
		t.Fatalf("v2 record current = false, want true")
	}

	stale := publishSmokeInput(t, "33333333-3333-5333-8333-333333333333", "content_cas_v3", "content_publish:cas-v3-stale", firstResult.Record.ID, v2, []content.AuditLogEntryInput{smokeAuditEntry("dddddddd-3333-5ddd-8ddd-ddddddddddd3", "33333333-3333-5333-8333-333333333333", "stale")})
	if _, err := store.PublishContentSnapshot(ctx, stale); !errors.Is(err, contentdb.ErrContentPublishConflict) {
		t.Fatalf("stale publish error = %v, want ErrContentPublishConflict", err)
	}
	if got := countContentVersions(t, ctx, db); got != 2 {
		t.Fatalf("content_versions count = %d, want 2 (stale publish must not add a version)", got)
	}
	if got := countAuditRowsForVersion(t, ctx, db, "33333333-3333-5333-8333-333333333333"); got != 0 {
		t.Fatalf("audit rows for rejected version = %d, want 0 (conflict must not mutate)", got)
	}
}

func countAuditRowsForVersion(t *testing.T, ctx context.Context, db *sql.DB, versionID string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM content_audit_log WHERE content_version_id = $1::uuid`, versionID).Scan(&count); err != nil {
		t.Fatalf("count audit rows error = %v, want nil", err)
	}
	return count
}
