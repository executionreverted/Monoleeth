package contentdb

import (
	"context"
	"fmt"
	"reflect"

	"gameproject/internal/game/content"
	"gameproject/internal/game/world"
)

type publishedSnapshotLoader interface {
	LoadCurrentPublishedSnapshot(ctx context.Context) (PublishedSnapshot, error)
}

type Repository struct {
	loader publishedSnapshotLoader
}

var _ content.Repository = (*Repository)(nil)

type SnapshotValidator struct {
	WorldID world.WorldID
}

func NewRepository(store *Store) (*Repository, error) {
	if store == nil {
		return nil, ErrNilDatabase
	}
	return newRepository(store)
}

func NewSnapshotValidator(worldID world.WorldID) SnapshotValidator {
	return SnapshotValidator{WorldID: worldID}
}

func (validator SnapshotValidator) ValidateContentSnapshot(ctx context.Context, snapshot content.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ValidateSnapshot(snapshot, validator.WorldID)
}

func ValidateSnapshot(snapshot content.Snapshot, worldID world.WorldID) error {
	_, err := mapPublishedSnapshot(snapshot, worldID)
	return err
}

func newRepository(loader publishedSnapshotLoader) (*Repository, error) {
	if isNilSnapshotLoader(loader) {
		return nil, ErrNilDatabase
	}
	return &Repository{loader: loader}, nil
}

func (repository *Repository) LoadPublishedContent(ctx context.Context, worldID world.WorldID) (content.GameplayContent, error) {
	if repository == nil || isNilSnapshotLoader(repository.loader) {
		return content.GameplayContent{}, ErrNilDatabase
	}
	if err := ctx.Err(); err != nil {
		return content.GameplayContent{}, err
	}
	record, err := repository.loader.LoadCurrentPublishedSnapshot(ctx)
	if err != nil {
		return content.GameplayContent{}, err
	}
	snapshot := record.Snapshot
	if record.Version != "" {
		snapshot.Version = record.Version
	}
	bundle, err := mapPublishedSnapshot(snapshot, worldID)
	if err != nil {
		return content.GameplayContent{}, fmt.Errorf("map published content %q: %w", record.Version, err)
	}
	return bundle, nil
}

func mapPublishedSnapshot(snapshot content.Snapshot, worldID world.WorldID) (content.GameplayContent, error) {
	if err := snapshot.Validate(); err != nil {
		return content.GameplayContent{}, err
	}

	items, err := mapItemRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	moduleCatalog, err := mapModuleRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	shipCatalog, err := mapShipRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	lootTables, err := mapLootTableRows(snapshot, items)
	if err != nil {
		return content.GameplayContent{}, err
	}
	recipeCatalog, err := mapCraftRecipeRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	productionCatalog, err := mapProductionRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	shopContent, err := mapShopProductRows(snapshot)
	if err != nil {
		return content.GameplayContent{}, err
	}
	mapCatalog, err := mapAndVerifyWorldMaps(snapshot, worldID)
	if err != nil {
		return content.GameplayContent{}, err
	}
	questCatalog, err := mapQuestRows(snapshot, items, shipCatalog, recipeCatalog, productionCatalog, mapCatalog)
	if err != nil {
		return content.GameplayContent{}, err
	}

	return content.GameplayContent{
		Items:      items,
		LootTables: lootTables,
		Modules:    moduleCatalog,
		Ships:      shipCatalog,
		Recipes:    recipeCatalog,
		Production: productionCatalog,
		Quests:     questCatalog,
		Maps:       mapCatalog,
		Scanner:    content.DefaultScannerContent(),
		Starter:    content.DefaultStarterContent(),
		Shop:       shopContent,
		Route:      content.DefaultRouteContent(),
		Rules:      content.DefaultProductionRulesContent(),
		Combat:     content.DefaultCombatRulesContent(),
	}, nil
}

func isNilSnapshotLoader(loader publishedSnapshotLoader) bool {
	if loader == nil {
		return true
	}
	value := reflect.ValueOf(loader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
