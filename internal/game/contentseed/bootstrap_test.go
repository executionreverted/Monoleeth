package contentseed

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/content"
)

func TestEnsurePublishedSeedSeedsEmptyStoreOnceAndWritesAllGroups(t *testing.T) {
	store := &fakeSeedStore{}
	snapshot := bootstrapTestSnapshot()

	result, err := EnsurePublishedSeed(context.Background(), store, snapshot, SeedOptions{})
	if err != nil {
		t.Fatalf("EnsurePublishedSeed() error = %v, want nil", err)
	}
	if !result.Seeded {
		t.Fatalf("result.Seeded = false, want true")
	}
	if result.Version != snapshot.Version {
		t.Fatalf("result.Version = %q, want %q", result.Version, snapshot.Version)
	}
	if result.RowCount != countSnapshotRows(snapshot) {
		t.Fatalf("result.RowCount = %d, want %d", result.RowCount, countSnapshotRows(snapshot))
	}
	if len(store.upserts) != len(snapshot.Groups()) {
		t.Fatalf("draft upserts = %d, want %d", len(store.upserts), len(snapshot.Groups()))
	}
	for index, group := range snapshot.Groups() {
		call := store.upserts[index]
		if call.contentType != group.Type {
			t.Fatalf("upsert[%d].contentType = %q, want %q", index, call.contentType, group.Type)
		}
		if len(call.rows) != len(group.Rows) {
			t.Fatalf("upsert[%d].rows = %d, want %d", index, len(call.rows), len(group.Rows))
		}
	}
	if len(store.published) != 1 {
		t.Fatalf("published inserts = %d, want 1", len(store.published))
	}
	if err := validateSeedUUID("default version id", store.published[0].ID); err != nil {
		t.Fatalf("default version id invalid: %v", err)
	}
	if got, want := store.published[0].ID, deterministicSeedVersionID(snapshot.Version); got != want {
		t.Fatalf("default version id = %q, want %q", got, want)
	}
	if got, want := store.published[0].IdempotencyKey, "contentseed:"+snapshot.Version; got != want {
		t.Fatalf("idempotency key = %q, want %q", got, want)
	}

	second, err := EnsurePublishedSeed(context.Background(), store, snapshot, SeedOptions{})
	if err != nil {
		t.Fatalf("second EnsurePublishedSeed() error = %v, want nil", err)
	}
	if second.Seeded {
		t.Fatalf("second result.Seeded = true, want false")
	}
	if len(store.published) != 1 {
		t.Fatalf("published inserts after second call = %d, want 1", len(store.published))
	}
}

func TestEnsurePublishedSeedNoopsWhenContentExists(t *testing.T) {
	store := &fakeSeedStore{hasAny: true}

	result, err := EnsurePublishedSeed(context.Background(), store, bootstrapTestSnapshot(), SeedOptions{})
	if err != nil {
		t.Fatalf("EnsurePublishedSeed() error = %v, want nil", err)
	}
	if result.Seeded || result.RowCount != 0 || result.Version != "" {
		t.Fatalf("result = %+v, want zero no-op", result)
	}
	if len(store.upserts) != 0 || len(store.published) != 0 {
		t.Fatalf("writes = upserts:%d published:%d, want none", len(store.upserts), len(store.published))
	}
}

func TestEnsurePublishedSeedInvalidSnapshotFailsBeforeWrites(t *testing.T) {
	store := &fakeSeedStore{}
	snapshot := bootstrapTestSnapshot()
	snapshot.Items[0].DataJSON = json.RawMessage(`[]`)

	_, err := EnsurePublishedSeed(context.Background(), store, snapshot, SeedOptions{})
	if !errors.Is(err, content.ErrInvalidContentJSON) {
		t.Fatalf("EnsurePublishedSeed() error = %v, want ErrInvalidContentJSON", err)
	}
	if len(store.upserts) != 0 || len(store.published) != 0 {
		t.Fatalf("writes = upserts:%d published:%d, want none", len(store.upserts), len(store.published))
	}
}

func TestEnsurePublishedSeedNilStoreFails(t *testing.T) {
	_, err := EnsurePublishedSeed(context.Background(), nil, bootstrapTestSnapshot(), SeedOptions{})
	if !errors.Is(err, ErrNilSeedStore) {
		t.Fatalf("EnsurePublishedSeed(nil store) error = %v, want ErrNilSeedStore", err)
	}

	var store *fakeSeedStore
	_, err = EnsurePublishedSeed(context.Background(), store, bootstrapTestSnapshot(), SeedOptions{})
	if !errors.Is(err, ErrNilSeedStore) {
		t.Fatalf("EnsurePublishedSeed(typed nil store) error = %v, want ErrNilSeedStore", err)
	}
}

func TestEnsurePublishedSeedUsesOptionMetadata(t *testing.T) {
	publishedAt := time.Unix(1700000000, 0).UTC()
	store := &fakeSeedStore{}
	snapshot := bootstrapTestSnapshot()
	options := SeedOptions{
		VersionID:            "11111111-1111-5111-8111-111111111111",
		DraftVersionID:       "22222222-2222-5222-8222-222222222222",
		IdempotencyKey:       "cms-seed-bootstrap",
		Actor:                "acct.admin",
		CreatedBy:            "acct.creator",
		PublishedBy:          "acct.publisher",
		PublishedAt:          publishedAt,
		Notes:                "seed mvp content",
		BalanceTag:           "mvp",
		ValidationReportJSON: json.RawMessage(`{"valid":true}`),
	}

	_, err := EnsurePublishedSeed(context.Background(), store, snapshot, options)
	if err != nil {
		t.Fatalf("EnsurePublishedSeed() error = %v, want nil", err)
	}
	for index, call := range store.upserts {
		if call.draftVersion != options.DraftVersionID {
			t.Fatalf("upsert[%d].draftVersion = %q, want %q", index, call.draftVersion, options.DraftVersionID)
		}
		if call.updatedBy != options.Actor {
			t.Fatalf("upsert[%d].updatedBy = %q, want %q", index, call.updatedBy, options.Actor)
		}
	}
	if len(store.published) != 1 {
		t.Fatalf("published inserts = %d, want 1", len(store.published))
	}
	input := store.published[0]
	if input.ID != options.VersionID ||
		input.Version != snapshot.Version ||
		input.IdempotencyKey != options.IdempotencyKey ||
		input.CreatedBy != options.CreatedBy ||
		input.PublishedBy != options.PublishedBy ||
		!input.PublishedAt.Equal(publishedAt) ||
		input.Notes != options.Notes ||
		input.BalanceTag != options.BalanceTag ||
		string(input.ValidationReportJSON) != string(options.ValidationReportJSON) {
		t.Fatalf("published input = %+v, want options metadata", input)
	}
}

type fakeSeedStore struct {
	hasAny    bool
	upserts   []draftUpsertCall
	published []PublishedSnapshotInput
}

type draftUpsertCall struct {
	contentType  content.ContentType
	draftVersion string
	rows         []content.SnapshotRow
	updatedBy    string
}

func (store *fakeSeedStore) HasAnyContent(context.Context) (bool, error) {
	return store.hasAny, nil
}

func (store *fakeSeedStore) UpsertDraftRows(_ context.Context, contentType content.ContentType, draftVersion string, rows []content.SnapshotRow, updatedBy string) error {
	store.upserts = append(store.upserts, draftUpsertCall{
		contentType:  contentType,
		draftVersion: draftVersion,
		rows:         append([]content.SnapshotRow(nil), rows...),
		updatedBy:    updatedBy,
	})
	return nil
}

func (store *fakeSeedStore) InsertPublishedSnapshot(_ context.Context, input PublishedSnapshotInput) error {
	store.published = append(store.published, input)
	store.hasAny = true
	return nil
}

func bootstrapTestSnapshot() content.Snapshot {
	return content.Snapshot{
		Version: "content_mvp_seed_v1",
		Items: []content.SnapshotRow{
			bootstrapTestRow("raw_ore"),
			bootstrapTestRow("iron_ore"),
		},
		Modules: []content.SnapshotRow{
			bootstrapTestRow("laser_alpha_t1"),
		},
		QuestRewardTables: []content.SnapshotRow{
			bootstrapTestRow("quest_rewards.quest_kill_pirates_r1"),
		},
	}
}

func bootstrapTestRow(contentID string) content.SnapshotRow {
	return content.SnapshotRow{
		ContentID: content.ContentID(contentID),
		Enabled:   true,
		DataJSON:  json.RawMessage(`{"source":"test"}`),
	}
}

func countSnapshotRows(snapshot content.Snapshot) int {
	count := 0
	for _, group := range snapshot.Groups() {
		count += len(group.Rows)
	}
	return count
}
