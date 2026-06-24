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
