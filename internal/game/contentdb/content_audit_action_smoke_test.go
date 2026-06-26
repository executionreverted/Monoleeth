package contentdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
)

// TestPostgresAuditActionColumnRoundTripsPublishAndRollback verifies the P09
// lane-F audit `action` migration landed and that publish/rollback actions
// persist and filter correctly on the live content_audit_log table.
func TestPostgresAuditActionColumnRoundTripsPublishAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	publishEntry := contentdb.AuditEntry{
		ID:           "aaaaaaaa-aaaa-5aaa-8aaa-aaaaaaaaaaa1",
		ContentType:  content.ContentTypeItem,
		ContentID:    "audit_action_item_publish",
		Action:       content.AuditActionPublish,
		FieldPath:    "$",
		NewValueJSON: []byte(`{"content_id":"audit_action_item_publish","enabled":true,"display_json":{},"data_json":{"stackable":true}}`),
		ActorAccountID: "account-admin",
		Note:         "publish action",
	}
	rollbackEntry := contentdb.AuditEntry{
		ID:           "bbbbbbbb-bbbb-5bbb-8bbb-bbbbbbbbbbb2",
		ContentType:  content.ContentTypeItem,
		ContentID:    "audit_action_item_rollback",
		Action:       content.AuditActionRollback,
		FieldPath:    "$",
		NewValueJSON: []byte(`{"content_id":"audit_action_item_rollback","enabled":true,"display_json":{},"data_json":{"stackable":false}}`),
		ActorAccountID: "account-admin",
		Note:         "rollback action",
	}
	if err := store.InsertAudit(ctx, publishEntry); err != nil {
		t.Fatalf("InsertAudit(publish) error = %v, want nil", err)
	}
	if err := store.InsertAudit(ctx, rollbackEntry); err != nil {
		t.Fatalf("InsertAudit(rollback) error = %v, want nil", err)
	}

	publishLog, err := store.ListContentAudit(ctx, content.AuditLogInput{Action: content.AuditActionPublish})
	if err != nil {
		t.Fatalf("ListContentAudit(publish) error = %v, want nil", err)
	}
	if publishLog.Total != 1 || publishLog.Entries[0].Action != content.AuditActionPublish {
		t.Fatalf("publish audit = %+v, want single publish action", publishLog)
	}

	rollbackLog, err := store.ListContentAudit(ctx, content.AuditLogInput{Action: content.AuditActionRollback})
	if err != nil {
		t.Fatalf("ListContentAudit(rollback) error = %v, want nil", err)
	}
	if rollbackLog.Total != 1 || rollbackLog.Entries[0].Action != content.AuditActionRollback {
		t.Fatalf("rollback audit = %+v, want single rollback action", rollbackLog)
	}

	allLog, err := store.ListContentAudit(ctx, content.AuditLogInput{})
	if err != nil {
		t.Fatalf("ListContentAudit(all) error = %v, want nil", err)
	}
	if allLog.Total != 2 {
		t.Fatalf("audit total = %d, want 2", allLog.Total)
	}
}

// TestPostgresAuditRejectsUnknownAction verifies the action CHECK constraint /
// store guard refuses an unsupported action so audit rows stay canonical.
func TestPostgresAuditRejectsUnknownAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}

	err := store.InsertAudit(ctx, contentdb.AuditEntry{
		ID:          "cccccccc-cccc-5ccc-8ccc-ccccccccccc3",
		ContentType: content.ContentTypeItem,
		ContentID:   "audit_action_item_bad",
		Action:      "delete",
	})
	if err == nil || !errors.Is(err, contentdb.ErrUnknownAuditAction) {
		t.Fatalf("InsertAudit(unknown action) error = %v, want ErrUnknownAuditAction", err)
	}
}
