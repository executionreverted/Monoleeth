package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/content"
	"gameproject/internal/game/testutil"
)

func TestContentServiceListVersionsRequiresStore(t *testing.T) {
	service := admin.NewContentService(admin.ContentServiceConfig{})

	_, err := service.ListVersions(context.Background(), content.VersionListInput{})
	if !errors.Is(err, admin.ErrMissingContentVersionStore) {
		t.Fatalf("ListVersions() error = %v, want ErrMissingContentVersionStore", err)
	}
}

func TestContentServiceListDraftRowsRequiresStore(t *testing.T) {
	service := admin.NewContentService(admin.ContentServiceConfig{})

	_, err := service.ListDraftRows(context.Background(), content.DraftListInput{ContentType: content.ContentTypeModule})
	if !errors.Is(err, admin.ErrMissingContentDraftStore) {
		t.Fatalf("ListDraftRows() error = %v, want ErrMissingContentDraftStore", err)
	}
}

func TestContentServiceListVersionsNormalizesPaginationAndGeneratedAt(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 30, 0, 0, time.UTC)
	store := &fakeContentVersionStore{
		list: content.VersionList{
			Total: 1,
			Versions: []content.VersionSummary{{
				ID:        "11111111-1111-5111-8111-111111111111",
				Version:   "content_mvp_seed_v1",
				Status:    "published",
				Current:   true,
				CreatedAt: now,
			}},
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Versions: store,
		Clock:    testutil.NewFakeClock(now),
	})

	list, err := service.ListVersions(context.Background(), content.VersionListInput{Limit: 500, Offset: -10})
	if err != nil {
		t.Fatalf("ListVersions() error = %v, want nil", err)
	}
	if store.input.Limit != content.MaxVersionListLimit || store.input.Offset != 0 {
		t.Fatalf("store input = %+v, want clamped limit and zero offset", store.input)
	}
	if list.Limit != content.MaxVersionListLimit || list.Offset != 0 || !list.GeneratedAt.Equal(now) {
		t.Fatalf("list metadata = limit %d offset %d generated %s, want normalized and now", list.Limit, list.Offset, list.GeneratedAt)
	}
	if len(list.Versions) != 1 || !list.Versions[0].Current {
		t.Fatalf("versions = %+v, want current version", list.Versions)
	}
}

func TestContentServiceListDraftRowsPaginatesAndClonesRows(t *testing.T) {
	now := time.Date(2026, 6, 24, 13, 0, 0, 0, time.UTC)
	store := &fakeContentDraftStore{
		rows: []content.DraftRow{
			{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(`{"attack_damage":8}`), DisplayJSON: []byte(`{"display_name":"Prism Lance I"}`)},
			{ContentID: "shield_mk1", Enabled: true, DataJSON: []byte(`{"shield":25}`), DisplayJSON: []byte(`{"display_name":"Shield I"}`)},
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts: store,
		Clock:  testutil.NewFakeClock(now),
	})

	list, err := service.ListDraftRows(context.Background(), content.DraftListInput{
		ContentType: content.ContentTypeModule,
		Limit:       1,
		Offset:      1,
	})
	if err != nil {
		t.Fatalf("ListDraftRows() error = %v, want nil", err)
	}
	if store.contentType != content.ContentTypeModule {
		t.Fatalf("store content type = %q, want module", store.contentType)
	}
	if list.Total != 2 || list.Limit != 1 || list.Offset != 1 || !list.GeneratedAt.Equal(now) {
		t.Fatalf("list metadata = %+v, want paged metadata", list)
	}
	if len(list.Rows) != 1 || list.Rows[0].ContentID != "shield_mk1" {
		t.Fatalf("rows = %+v, want second row only", list.Rows)
	}
	list.Rows[0].DataJSON[0] = '!'
	if string(store.rows[1].DataJSON) != `{"shield":25}` {
		t.Fatalf("caller mutated stored row data: %s", store.rows[1].DataJSON)
	}
}

func TestContentServiceGetDraftRowFindsRowAndRejectsMissing(t *testing.T) {
	store := &fakeContentDraftStore{
		rows: []content.DraftRow{
			{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(`{"attack_damage":8}`), DisplayJSON: []byte(`{}`)},
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{Drafts: store})

	row, err := service.GetDraftRow(context.Background(), content.ContentTypeModule, "laser_alpha_t1")
	if err != nil {
		t.Fatalf("GetDraftRow() error = %v, want nil", err)
	}
	if row.ContentID != "laser_alpha_t1" || !row.Enabled {
		t.Fatalf("row = %+v, want laser row", row)
	}

	_, err = service.GetDraftRow(context.Background(), content.ContentTypeModule, "missing_module")
	if !errors.Is(err, admin.ErrContentDraftNotFound) {
		t.Fatalf("GetDraftRow(missing) error = %v, want ErrContentDraftNotFound", err)
	}
}

func TestContentServiceUpdateDraftRowValidatesAndWritesServerActor(t *testing.T) {
	store := &fakeContentDraftStore{}
	service := admin.NewContentService(admin.ContentServiceConfig{Drafts: store, Writer: store})

	row, err := service.UpdateDraftRow(context.Background(), content.DraftUpdateInput{
		ContentType: content.ContentTypeModule,
		ContentID:   "laser_alpha_t1",
		Enabled:     true,
		DisplayJSON: []byte(`{"display_name":"Prism Lance I"}`),
		DataJSON:    []byte(`{"attack_damage":8,"cooldown_ms":1200,"range":650}`),
		UpdatedBy:   "account-admin",
	})
	if err != nil {
		t.Fatalf("UpdateDraftRow() error = %v, want nil", err)
	}
	if store.upsertContentType != content.ContentTypeModule || store.upsertRow.ContentID != "laser_alpha_t1" ||
		store.upsertRow.UpdatedBy != "account-admin" || !store.upsertRow.Enabled {
		t.Fatalf("upsert = type %q row %+v, want module laser row by admin", store.upsertContentType, store.upsertRow)
	}
	if row.UpdatedBy != "account-admin" || string(row.DataJSON) != `{"attack_damage":8,"cooldown_ms":1200,"range":650}` {
		t.Fatalf("row = %+v, want updated server actor and stats", row)
	}
	row.DataJSON[0] = '!'
	if string(store.upsertRow.DataJSON) != `{"attack_damage":8,"cooldown_ms":1200,"range":650}` {
		t.Fatalf("caller mutated stored upsert row: %s", store.upsertRow.DataJSON)
	}
}

func TestContentServiceUpdateDraftRowRejectsInvalidContentJSON(t *testing.T) {
	store := &fakeContentDraftStore{}
	service := admin.NewContentService(admin.ContentServiceConfig{Drafts: store, Writer: store})

	_, err := service.UpdateDraftRow(context.Background(), content.DraftUpdateInput{
		ContentType: content.ContentTypeModule,
		ContentID:   "laser_alpha_t1",
		Enabled:     true,
		DataJSON:    []byte(`{"script":"bad"}`),
		UpdatedBy:   "account-admin",
	})
	if !errors.Is(err, content.ErrForbiddenContentField) {
		t.Fatalf("UpdateDraftRow(script) error = %v, want ErrForbiddenContentField", err)
	}
	if store.upsertCalled {
		t.Fatal("UpdateDraftRow(script) wrote invalid draft row")
	}
}

func TestContentServiceValidateDraftBuildsSnapshotAndReportsRuntimeErrors(t *testing.T) {
	now := time.Date(2026, 6, 24, 14, 0, 0, 0, time.UTC)
	validatorErr := errors.New("module laser_alpha_t1: missing item")
	store := &fakeContentDraftStore{
		rowsByType: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(`{"item_id":"laser_alpha_t1"}`), DisplayJSON: []byte(`{}`)},
			},
		},
	}
	validator := &fakeContentDraftValidator{err: validatorErr}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Validator: validator,
		Clock:     testutil.NewFakeClock(now),
	})

	report, err := service.ValidateDraft(context.Background(), content.DraftValidationInput{Version: "draft_candidate_v1"})
	if err != nil {
		t.Fatalf("ValidateDraft() error = %v, want nil", err)
	}
	if report.Valid || report.Version != "draft_candidate_v1" || !report.CheckedAt.Equal(now) {
		t.Fatalf("report = %+v, want invalid report for requested version at fake now", report)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "invalid_runtime_catalog" {
		t.Fatalf("issues = %+v, want runtime catalog issue", report.Issues)
	}
	if validator.snapshot.Version != "draft_candidate_v1" || len(validator.snapshot.Modules) != 1 {
		t.Fatalf("validated snapshot = %+v, want one module row", validator.snapshot)
	}
	if got := store.loadCalls[content.ContentTypeModule]; got != 1 {
		t.Fatalf("module load calls = %d, want 1", got)
	}
}

func TestContentServiceValidateDraftReportsSnapshotErrorsWithoutValidator(t *testing.T) {
	store := &fakeContentDraftStore{
		rowsByType: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(``)},
			},
		},
	}
	validator := &fakeContentDraftValidator{}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Validator: validator,
	})

	report, err := service.ValidateDraft(context.Background(), content.DraftValidationInput{})
	if err != nil {
		t.Fatalf("ValidateDraft(invalid snapshot) error = %v, want nil", err)
	}
	if report.Valid || report.Version != "draft_validation" || len(report.Issues) != 1 ||
		report.Issues[0].Code != "invalid_snapshot" {
		t.Fatalf("report = %+v, want invalid snapshot report", report)
	}
	if validator.called {
		t.Fatal("validator called after structural snapshot error")
	}
}

func TestContentServicePublishDraftValidatesAndWritesImmutableVersion(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	current := snapshotVersionRecordForAdminTest("11111111-1111-5111-8111-111111111111", "content_mvp_seed_v1", moduleSnapshotForAdminTest(8), now.Add(-time.Hour))
	store := &fakeContentDraftStore{
		rowsByType: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(`{"attack_damage":9}`), DisplayJSON: []byte(`{"display_name":"Prism Lance II"}`)},
			},
		},
		currentSnapshot: current,
		publishResult: content.PublishSnapshotResult{
			Record: snapshotVersionRecordForAdminTest("22222222-2222-5222-8222-222222222222", "content_balance_v2", moduleSnapshotForAdminTest(9), now),
		},
	}
	validator := &fakeContentDraftValidator{}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Publisher: store,
		Snapshots: store,
		Validator: validator,
		Clock:     testutil.NewFakeClock(now),
	})

	result, err := service.PublishDraft(context.Background(), content.PublishDraftInput{
		Version:        "content_balance_v2",
		Notes:          "LC1 buff",
		BalanceTag:     "starter_balance",
		ActorAccountID: "account-admin",
	})
	if err != nil {
		t.Fatalf("PublishDraft() error = %v, want nil", err)
	}
	if !result.Published || !result.Validation.Valid || result.RowCount != 1 || result.Version.ID != "22222222-2222-5222-8222-222222222222" {
		t.Fatalf("result = %+v, want published v2", result)
	}
	if store.publishedInput.Version != "content_balance_v2" ||
		store.publishedInput.PublishedBy != "account-admin" ||
		store.publishedInput.CreatedBy != "account-admin" ||
		store.publishedInput.ExpectedCurrentID != current.ID ||
		store.publishedInput.Notes != "LC1 buff" ||
		store.publishedInput.BalanceTag != "starter_balance" ||
		store.publishedInput.IdempotencyKey == "" {
		t.Fatalf("publish input = %+v, want server actor/idempotency/metadata", store.publishedInput)
	}
	if len(store.publishedInput.AuditEntries) != 1 || store.publishedInput.AuditEntries[0].ActorAccountID != "account-admin" ||
		store.publishedInput.AuditEntries[0].ContentType != content.ContentTypeModule ||
		store.publishedInput.AuditEntries[0].Note != "LC1 buff" ||
		store.publishedInput.AuditEntries[0].BalanceTag != "starter_balance" {
		t.Fatalf("audit entries = %+v, want module change with publish metadata by admin", store.publishedInput.AuditEntries)
	}
	if !validator.called || validator.snapshot.Version != "content_balance_v2" {
		t.Fatalf("validator = called %v snapshot %+v, want draft snapshot v2", validator.called, validator.snapshot)
	}
}

func TestContentServicePublishDraftRequiresNotesAndValidatesBalanceTag(t *testing.T) {
	tests := []struct {
		name       string
		notes      string
		balanceTag string
		wantErr    error
	}{
		{name: "empty notes", notes: "  ", balanceTag: "starter_balance", wantErr: admin.ErrMissingContentPublishNotes},
		{name: "uppercase tag", notes: "LC1 buff", balanceTag: "Starter_Balance", wantErr: admin.ErrInvalidContentBalanceTag},
		{name: "space tag", notes: "LC1 buff", balanceTag: "starter balance", wantErr: admin.ErrInvalidContentBalanceTag},
		{name: "long tag", notes: "LC1 buff", balanceTag: strings.Repeat("a", 65), wantErr: admin.ErrInvalidContentBalanceTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeContentDraftStore{}
			service := admin.NewContentService(admin.ContentServiceConfig{
				Drafts:    store,
				Publisher: store,
				Snapshots: store,
				Validator: &fakeContentDraftValidator{},
			})

			_, err := service.PublishDraft(context.Background(), content.PublishDraftInput{
				Version:    "content_balance_v2",
				Notes:      tt.notes,
				BalanceTag: tt.balanceTag,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("PublishDraft() error = %v, want %v", err, tt.wantErr)
			}
			if store.publishCalled || len(store.loadCalls) != 0 {
				t.Fatalf("PublishDraft() touched store for invalid metadata: publish=%v loads=%v", store.publishCalled, store.loadCalls)
			}
		})
	}
}

func TestContentServicePublishDraftInvalidReportDoesNotWrite(t *testing.T) {
	store := &fakeContentDraftStore{
		rowsByType: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{ContentID: "laser_alpha_t1", Enabled: true, DataJSON: []byte(`{"attack_damage":9}`), DisplayJSON: []byte(`{}`)},
			},
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Publisher: store,
		Snapshots: store,
		Validator: &fakeContentDraftValidator{err: errors.New("bad runtime content")},
	})

	result, err := service.PublishDraft(context.Background(), content.PublishDraftInput{Version: "content_bad_v2", Notes: "try bad runtime content"})
	if err != nil {
		t.Fatalf("PublishDraft(invalid) error = %v, want nil report", err)
	}
	if result.Published || result.Validation.Valid || store.publishCalled {
		t.Fatalf("result = %+v publishCalled=%v, want invalid no-write", result, store.publishCalled)
	}
}

func TestContentServicePublishDraftScrubsAuditJSON(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 15, 0, 0, time.UTC)
	current := snapshotVersionRecordForAdminTest("11111111-1111-5111-8111-111111111111", "content_mvp_seed_v1", moduleSnapshotForAdminTest(8), now.Add(-time.Hour))
	store := &fakeContentDraftStore{
		currentSnapshot: current,
		rowsByType: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{
					ContentID:   "laser_alpha_t1",
					Enabled:     true,
					DisplayJSON: []byte(`{"name":"Laser Alpha","session_token":"display-secret"}`),
					DataJSON:    []byte(`{"attack_damage":9,"api_token":"super-secret","nested":{"procedural_seed":"seed-secret"}}`),
				},
			},
		},
		publishResult: content.PublishSnapshotResult{
			Record: snapshotVersionRecordForAdminTest("22222222-2222-5222-8222-222222222222", "content_balance_v2", moduleSnapshotForAdminTest(9), now),
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Publisher: store,
		Snapshots: store,
		Validator: &fakeContentDraftValidator{},
		Clock:     testutil.NewFakeClock(now),
	})

	if _, err := service.PublishDraft(context.Background(), content.PublishDraftInput{Version: "content_balance_v2", Notes: "scrub audit payload"}); err != nil {
		t.Fatalf("PublishDraft() error = %v, want nil", err)
	}
	if len(store.publishedInput.AuditEntries) != 1 {
		t.Fatalf("audit entries = %+v, want one scrubbed entry", store.publishedInput.AuditEntries)
	}
	encoded := string(store.publishedInput.AuditEntries[0].NewValueJSON)
	if strings.Contains(encoded, "super-secret") || strings.Contains(encoded, "seed-secret") || strings.Contains(encoded, "display-secret") {
		t.Fatalf("audit JSON leaked secret values: %s", encoded)
	}
	if !strings.Contains(encoded, "[redacted]") {
		t.Fatalf("audit JSON = %s, want redacted markers", encoded)
	}
}

func TestContentServiceRollbackPublishesTargetSnapshotAsNewVersion(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 30, 0, 0, time.UTC)
	target := snapshotVersionRecordForAdminTest("11111111-1111-5111-8111-111111111111", "content_mvp_seed_v1", moduleSnapshotForAdminTest(8), now.Add(-2*time.Hour))
	current := snapshotVersionRecordForAdminTest("22222222-2222-5222-8222-222222222222", "content_balance_v2", moduleSnapshotForAdminTest(9), now.Add(-time.Hour))
	store := &fakeContentDraftStore{
		currentSnapshot: current,
		targetSnapshots: map[string]content.SnapshotVersionRecord{target.ID: target},
		publishResult: content.PublishSnapshotResult{
			Record: snapshotVersionRecordForAdminTest("33333333-3333-5333-8333-333333333333", "content_rollback_v3", moduleSnapshotForAdminTest(8), now),
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Publisher: store,
		Snapshots: store,
		Validator: &fakeContentDraftValidator{},
		Clock:     testutil.NewFakeClock(now),
	})

	result, err := service.Rollback(context.Background(), content.RollbackInput{
		TargetVersionID: target.ID,
		Version:         "content_rollback_v3",
		Notes:           "restore starter",
		BalanceTag:      "rollback_starter",
		ActorAccountID:  "account-admin",
		IdempotencyKey:  "content_rollback:target:req-1",
	})
	if err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}
	if !result.Published || !result.Validation.Valid || result.Version.ID != "33333333-3333-5333-8333-333333333333" {
		t.Fatalf("rollback result = %+v, want published rollback", result)
	}
	if store.publishedInput.RolledBackFrom != target.ID ||
		store.publishedInput.IdempotencyKey != "content_rollback:target:req-1" ||
		store.publishedInput.ExpectedCurrentID != current.ID ||
		store.publishedInput.PublishedBy != "account-admin" ||
		store.publishedInput.Notes != "restore starter" ||
		store.publishedInput.BalanceTag != "rollback_starter" ||
		store.publishedInput.Snapshot.Version != "content_rollback_v3" {
		t.Fatalf("rollback publish input = %+v, want immutable rollback copy", store.publishedInput)
	}
	if len(store.publishedInput.AuditEntries) != 1 ||
		store.publishedInput.AuditEntries[0].Note != "restore starter" ||
		store.publishedInput.AuditEntries[0].BalanceTag != "rollback_starter" {
		t.Fatalf("rollback audit entries = %+v, want rollback metadata", store.publishedInput.AuditEntries)
	}
}

func TestContentServiceRollbackRequiresNotes(t *testing.T) {
	store := &fakeContentDraftStore{}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Publisher: store,
		Snapshots: store,
		Validator: &fakeContentDraftValidator{},
	})

	_, err := service.Rollback(context.Background(), content.RollbackInput{
		TargetVersionID: "11111111-1111-5111-8111-111111111111",
		Notes:           " ",
	})
	if !errors.Is(err, admin.ErrMissingContentPublishNotes) {
		t.Fatalf("Rollback() error = %v, want ErrMissingContentPublishNotes", err)
	}
	if store.publishCalled {
		t.Fatal("Rollback() published with missing notes")
	}
}

func TestContentServiceAuditLogNormalizesAndUsesStore(t *testing.T) {
	now := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store := &fakeContentDraftStore{
		auditLog: content.AuditLog{
			Total: 1,
			Entries: []content.AuditLogEntry{{
				ID:               "44444444-4444-5444-8444-444444444444",
				ContentVersionID: "22222222-2222-5222-8222-222222222222",
				ContentType:      content.ContentTypeModule,
				ContentID:        "laser_alpha_t1",
				FieldPath:        "$",
				NewValueJSON:     []byte(`{"content_id":"laser_alpha_t1"}`),
				ActorAccountID:   "account-admin",
				CreatedAt:        now,
			}},
		},
	}
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts: store,
		Audit:  store,
		Clock:  testutil.NewFakeClock(now),
	})

	log, err := service.AuditLog(context.Background(), content.AuditLogInput{
		ContentType: content.ContentTypeModule,
		Limit:       999,
		Offset:      -5,
	})
	if err != nil {
		t.Fatalf("AuditLog() error = %v, want nil", err)
	}
	if store.auditInput.Limit != content.MaxAuditLogLimit || store.auditInput.Offset != 0 {
		t.Fatalf("audit input = %+v, want normalized", store.auditInput)
	}
	if log.Total != 1 || log.Limit != content.MaxAuditLogLimit || !log.GeneratedAt.Equal(now) {
		t.Fatalf("audit log = %+v, want generated metadata", log)
	}
}

type fakeContentVersionStore struct {
	input content.VersionListInput
	list  content.VersionList
	err   error
}

func (store *fakeContentVersionStore) ListContentVersions(_ context.Context, input content.VersionListInput) (content.VersionList, error) {
	store.input = input
	if store.err != nil {
		return content.VersionList{}, store.err
	}
	return store.list, nil
}

type fakeContentDraftStore struct {
	contentType       content.ContentType
	rows              []content.DraftRow
	rowsByType        map[content.ContentType][]content.DraftRow
	err               error
	upsertCalled      bool
	upsertContentType content.ContentType
	upsertRow         content.DraftRow
	loadCalls         map[content.ContentType]int
	currentSnapshot   content.SnapshotVersionRecord
	targetSnapshots   map[string]content.SnapshotVersionRecord
	publishCalled     bool
	publishedInput    content.PublishSnapshotInput
	publishResult     content.PublishSnapshotResult
	auditInput        content.AuditLogInput
	auditLog          content.AuditLog
}

func (store *fakeContentDraftStore) LoadDraftRows(_ context.Context, contentType content.ContentType) ([]content.DraftRow, error) {
	store.contentType = contentType
	if store.loadCalls == nil {
		store.loadCalls = make(map[content.ContentType]int)
	}
	store.loadCalls[contentType]++
	if store.err != nil {
		return nil, store.err
	}
	if store.rowsByType != nil {
		return cloneTestDraftRows(store.rowsByType[contentType]), nil
	}
	return store.rows, nil
}

func (store *fakeContentDraftStore) UpsertDraftRow(_ context.Context, contentType content.ContentType, row content.DraftRow) error {
	store.upsertCalled = true
	store.upsertContentType = contentType
	store.upsertRow = cloneTestDraftRow(row)
	return store.err
}

type fakeContentDraftValidator struct {
	called   bool
	snapshot content.Snapshot
	err      error
}

func (validator *fakeContentDraftValidator) ValidateContentSnapshot(_ context.Context, snapshot content.Snapshot) error {
	validator.called = true
	validator.snapshot = snapshot
	return validator.err
}

func (store *fakeContentDraftStore) LoadCurrentContentSnapshot(context.Context) (content.SnapshotVersionRecord, error) {
	if store.currentSnapshot.ID == "" {
		return content.SnapshotVersionRecord{}, errors.New("missing current snapshot")
	}
	return store.currentSnapshot, nil
}

func (store *fakeContentDraftStore) LoadContentSnapshotByID(_ context.Context, id string) (content.SnapshotVersionRecord, error) {
	record, ok := store.targetSnapshots[id]
	if !ok {
		return content.SnapshotVersionRecord{}, errors.New("missing target snapshot")
	}
	return record, nil
}

func (store *fakeContentDraftStore) PublishContentSnapshot(_ context.Context, input content.PublishSnapshotInput) (content.PublishSnapshotResult, error) {
	store.publishCalled = true
	store.publishedInput = input
	if store.publishResult.Record.ID == "" {
		store.publishResult.Record = content.SnapshotVersionRecord{
			ID:             input.ID,
			Version:        input.Version,
			Status:         "published",
			Current:        true,
			Notes:          input.Notes,
			BalanceTag:     input.BalanceTag,
			CreatedBy:      input.CreatedBy,
			CreatedAt:      input.PublishedAt,
			PublishedBy:    input.PublishedBy,
			PublishedAt:    input.PublishedAt,
			RolledBackFrom: input.RolledBackFrom,
			Snapshot:       input.Snapshot,
		}
	}
	return store.publishResult, store.err
}

func (store *fakeContentDraftStore) ListContentAudit(_ context.Context, input content.AuditLogInput) (content.AuditLog, error) {
	store.auditInput = input
	return store.auditLog, store.err
}

func snapshotVersionRecordForAdminTest(id string, version string, snapshot content.Snapshot, publishedAt time.Time) content.SnapshotVersionRecord {
	return content.SnapshotVersionRecord{
		ID:          id,
		Version:     version,
		Status:      "published",
		Current:     true,
		Snapshot:    snapshot,
		PublishedBy: "seed",
		PublishedAt: publishedAt,
		CreatedAt:   publishedAt,
	}
}

func moduleSnapshotForAdminTest(damage int) content.Snapshot {
	data, err := json.Marshal(map[string]any{"attack_damage": damage})
	if err != nil {
		panic(err)
	}
	return content.Snapshot{
		Version: "content_test",
		Modules: []content.SnapshotRow{{
			ContentID:   "laser_alpha_t1",
			Enabled:     true,
			DisplayJSON: []byte(`{}`),
			DataJSON:    data,
		}},
	}
}

func cloneTestDraftRows(rows []content.DraftRow) []content.DraftRow {
	if len(rows) == 0 {
		return nil
	}
	cloned := make([]content.DraftRow, len(rows))
	for index, row := range rows {
		cloned[index] = cloneTestDraftRow(row)
	}
	return cloned
}

func cloneTestDraftRow(row content.DraftRow) content.DraftRow {
	row.DisplayJSON = append([]byte(nil), row.DisplayJSON...)
	row.DataJSON = append([]byte(nil), row.DataJSON...)
	return row
}
