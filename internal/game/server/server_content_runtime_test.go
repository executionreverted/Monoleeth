package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/world"
)

func TestNewRuntimeUsesInjectedContentRepository(t *testing.T) {
	bundle := runtimeTestBundleWithLaserDamage(t, 77)
	repository := &fakeRuntimeRepository{bundle: bundle}

	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: repository,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}

	assertRuntimeLaserDamage(t, runtime, 77)
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
}

func TestNewRuntimeInjectedRepositoryErrorDoesNotFallBackToStatic(t *testing.T) {
	loadErr := errors.New("injected repository failed")

	_, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: &fakeRuntimeRepository{err: loadErr},
	})
	if !errors.Is(err, loadErr) {
		t.Fatalf("NewRuntime() error = %v, want %v", err, loadErr)
	}
}

func TestNewRuntimeContentDBOffUsesStaticRepository(t *testing.T) {
	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:   foundation.WorldID("world-1"),
		ContentDB: contentdb.Config{Mode: contentdb.ContentModeOff},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}

	assertRuntimeLaserDamage(t, runtime, 12)
}

func TestNewRuntimeContentDBDevFallbackWithoutURLUsesStaticRepository(t *testing.T) {
	runtime, err := NewRuntime(RuntimeConfig{
		WorldID: foundation.WorldID("world-1"),
		ContentDB: contentdb.Config{
			Mode: contentdb.ContentModeDevFallback,
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}

	assertRuntimeLaserDamage(t, runtime, 12)
}

func TestLoadRuntimeContentFromDBSeedsEmptyStoreThenLoadsRepository(t *testing.T) {
	store := &fakeRuntimeContentStore{}
	repository := &fakeRuntimeRepository{bundle: runtimeTestBundle(t)}

	bundle, err := loadRuntimeContent(context.Background(), RuntimeConfig{
		WorldID:   foundation.WorldID("world-1"),
		ContentDB: runtimeContentDBConfig(),
		contentDBOpen: func(context.Context, contentdb.Config) (runtimeContentStore, error) {
			return store, nil
		},
		contentSeedSnapshot: func(world.WorldID) (gamecontent.Snapshot, error) {
			return runtimeTestSeedSnapshot(), nil
		},
		contentRepositoryStore: func(got runtimeContentStore) (gamecontent.Repository, error) {
			if got != store {
				t.Fatalf("repository store = %T, want fake store", got)
			}
			return repository, nil
		},
	})
	if err != nil {
		t.Fatalf("loadRuntimeContent() error = %v, want nil", err)
	}

	assertBundleLaserDamage(t, bundle, 12)
	if got, want := store.migrations, []contentdb.MigrationMode{contentdb.MigrationModeVerify}; !equalMigrationModes(got, want) {
		t.Fatalf("migrations = %v, want %v", got, want)
	}
	if store.hasAnyCalls != 2 {
		t.Fatalf("HasAnyContent calls = %d, want 2", store.hasAnyCalls)
	}
	if countRuntimeSeedRows(store.upserts) == 0 || len(store.published) != 1 {
		t.Fatalf("seed writes = upsert rows %d published %d, want seed", countRuntimeSeedRows(store.upserts), len(store.published))
	}
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
	if !store.closed {
		t.Fatal("store closed = false, want true")
	}
}

func TestLoadRuntimeContentFromDBDoesNotOverwriteExistingContent(t *testing.T) {
	store := &fakeRuntimeContentStore{hasAny: true}
	repository := &fakeRuntimeRepository{bundle: runtimeTestBundle(t)}

	_, err := loadRuntimeContent(context.Background(), RuntimeConfig{
		WorldID:   foundation.WorldID("world-1"),
		ContentDB: runtimeContentDBConfig(),
		contentDBOpen: func(context.Context, contentdb.Config) (runtimeContentStore, error) {
			return store, nil
		},
		contentSeedSnapshot: func(world.WorldID) (gamecontent.Snapshot, error) {
			t.Fatal("content seed snapshot builder called for non-empty content DB")
			return gamecontent.Snapshot{}, nil
		},
		contentRepositoryStore: func(runtimeContentStore) (gamecontent.Repository, error) {
			return repository, nil
		},
	})
	if err != nil {
		t.Fatalf("loadRuntimeContent() error = %v, want nil", err)
	}

	if len(store.upserts) != 0 || len(store.published) != 0 {
		t.Fatalf("seed writes = upserts %d published %d, want none", len(store.upserts), len(store.published))
	}
	if store.hasAnyCalls != 1 {
		t.Fatalf("HasAnyContent calls = %d, want 1", store.hasAnyCalls)
	}
	if repository.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repository.calls)
	}
}

func TestNewRuntimeContentDBOpenErrorDoesNotFallBackToStatic(t *testing.T) {
	openErr := errors.New("open content db failed")

	_, err := NewRuntime(RuntimeConfig{
		WorldID:   foundation.WorldID("world-1"),
		ContentDB: runtimeContentDBConfig(),
		contentDBOpen: func(context.Context, contentdb.Config) (runtimeContentStore, error) {
			return nil, openErr
		},
	})
	if !errors.Is(err, openErr) {
		t.Fatalf("NewRuntime() error = %v, want %v", err, openErr)
	}
}

func TestNewRuntimeContentDBLoadErrorDoesNotFallBackToStatic(t *testing.T) {
	loadErr := errors.New("db repository failed")
	store := &fakeRuntimeContentStore{}

	_, err := NewRuntime(RuntimeConfig{
		WorldID:   foundation.WorldID("world-1"),
		ContentDB: runtimeContentDBConfig(),
		contentDBOpen: func(context.Context, contentdb.Config) (runtimeContentStore, error) {
			return store, nil
		},
		contentSeedSnapshot: func(world.WorldID) (gamecontent.Snapshot, error) {
			return runtimeTestSeedSnapshot(), nil
		},
		contentRepositoryStore: func(runtimeContentStore) (gamecontent.Repository, error) {
			return &fakeRuntimeRepository{err: loadErr}, nil
		},
	})
	if !errors.Is(err, loadErr) {
		t.Fatalf("NewRuntime() error = %v, want %v", err, loadErr)
	}
	if len(store.published) != 1 {
		t.Fatalf("published seed writes = %d, want 1 before load failure", len(store.published))
	}
}

type fakeRuntimeRepository struct {
	bundle gamecontent.GameplayContent
	err    error
	calls  int
}

func (repository *fakeRuntimeRepository) LoadPublishedContent(ctx context.Context, worldID world.WorldID) (gamecontent.GameplayContent, error) {
	repository.calls++
	if err := ctx.Err(); err != nil {
		return gamecontent.GameplayContent{}, err
	}
	if repository.err != nil {
		return gamecontent.GameplayContent{}, repository.err
	}
	return repository.bundle, nil
}

type fakeRuntimeContentStore struct {
	hasAny      bool
	hasAnyCalls int
	migrations  []contentdb.MigrationMode
	upserts     []runtimeSeedUpsert
	published   []contentdb.PublishedSnapshotInput
	closed      bool
}

type runtimeSeedUpsert struct {
	contentType  gamecontent.ContentType
	draftVersion string
	rows         []gamecontent.SnapshotRow
	updatedBy    string
}

func (store *fakeRuntimeContentStore) Migrate(_ context.Context, mode contentdb.MigrationMode) error {
	store.migrations = append(store.migrations, mode)
	return nil
}

func (store *fakeRuntimeContentStore) Close() error {
	store.closed = true
	return nil
}

func (store *fakeRuntimeContentStore) HasAnyContent(context.Context) (bool, error) {
	store.hasAnyCalls++
	return store.hasAny, nil
}

func (store *fakeRuntimeContentStore) UpsertDraftRows(_ context.Context, contentType gamecontent.ContentType, draftVersion string, rows []gamecontent.SnapshotRow, updatedBy string) error {
	store.upserts = append(store.upserts, runtimeSeedUpsert{
		contentType:  contentType,
		draftVersion: draftVersion,
		rows:         append([]gamecontent.SnapshotRow(nil), rows...),
		updatedBy:    updatedBy,
	})
	return nil
}

func (store *fakeRuntimeContentStore) InsertPublishedSnapshot(_ context.Context, input contentdb.PublishedSnapshotInput) error {
	store.published = append(store.published, input)
	store.hasAny = true
	return nil
}

func runtimeContentDBConfig() contentdb.Config {
	return contentdb.Config{
		DatabaseURL: "postgres://gameproject:pw@localhost:5432/gameproject?sslmode=disable",
		Mode:        contentdb.ContentModeRequired,
		Migrations:  contentdb.MigrationModeVerify,
	}
}

func runtimeTestBundle(t *testing.T) gamecontent.GameplayContent {
	t.Helper()
	bundle, err := gamecontent.DefaultGameplayContent(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultGameplayContent() error = %v", err)
	}
	return bundle
}

func runtimeTestBundleWithLaserDamage(t *testing.T, damage int64) gamecontent.GameplayContent {
	t.Helper()
	bundle := runtimeTestBundle(t)
	definitions := bundle.Modules.Definitions()
	found := false
	for defIndex := range definitions {
		if definitions[defIndex].ItemID != foundation.ItemID("laser_alpha_t1") {
			continue
		}
		for statIndex := range definitions[defIndex].StatModifiers {
			if definitions[defIndex].StatModifiers[statIndex].Stat == modules.StatWeaponDamage {
				definitions[defIndex].StatModifiers[statIndex].Value = damage
				found = true
			}
		}
	}
	if !found {
		t.Fatal("laser_alpha_t1 weapon damage stat missing")
	}
	moduleCatalog, err := modules.NewCatalog(definitions)
	if err != nil {
		t.Fatalf("NewCatalog(mutated modules) error = %v, want nil", err)
	}
	bundle.Modules = moduleCatalog
	if err := bundle.Validate(); err != nil {
		t.Fatalf("mutated bundle Validate() error = %v, want nil", err)
	}
	return bundle
}

func runtimeTestSeedSnapshot() gamecontent.Snapshot {
	return gamecontent.Snapshot{
		Version: "runtime_seed_test_v1",
		Items: []gamecontent.SnapshotRow{{
			ContentID: "raw_ore",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"source":"runtime-test"}`),
		}},
	}
}

func assertRuntimeLaserDamage(t *testing.T, runtime *Runtime, want int64) {
	t.Helper()
	if runtime == nil {
		t.Fatal("runtime = nil")
	}
	assertBundleLaserDamage(t, gamecontent.GameplayContent{Modules: runtime.ModuleCatalog}, want)
}

func assertBundleLaserDamage(t *testing.T, bundle gamecontent.GameplayContent, want int64) {
	t.Helper()
	definition, ok := bundle.Modules.Lookup("laser_alpha_t1")
	if !ok {
		t.Fatal("laser_alpha_t1 missing from module catalog")
	}
	for _, modifier := range definition.StatModifiers {
		if modifier.Stat == modules.StatWeaponDamage {
			if modifier.Value != want {
				t.Fatalf("laser weapon damage = %d, want %d", modifier.Value, want)
			}
			return
		}
	}
	t.Fatal("laser weapon damage stat missing")
}

func countRuntimeSeedRows(upserts []runtimeSeedUpsert) int {
	count := 0
	for _, upsert := range upserts {
		count += len(upsert.rows)
	}
	return count
}

func equalMigrationModes(left, right []contentdb.MigrationMode) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
