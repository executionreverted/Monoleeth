package admin_test

import (
	"context"
	"errors"
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
