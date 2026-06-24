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
	contentType content.ContentType
	rows        []content.DraftRow
	err         error
}

func (store *fakeContentDraftStore) LoadDraftRows(_ context.Context, contentType content.ContentType) ([]content.DraftRow, error) {
	store.contentType = contentType
	if store.err != nil {
		return nil, store.err
	}
	return store.rows, nil
}
