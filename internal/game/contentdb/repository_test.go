package contentdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	repositoryTestWorldID          world.WorldID = "world-1"
	repositoryTestPublishedVersion               = "published_runtime_v9"
)

func TestNewRepositoryRejectsNilStore(t *testing.T) {
	_, err := NewRepository(nil)
	if !errors.Is(err, ErrNilDatabase) {
		t.Fatalf("NewRepository(nil) error = %v, want %v", err, ErrNilDatabase)
	}
}

func TestRepositoryNoCurrentPublishedSnapshotReturnsError(t *testing.T) {
	repository := newTestRepository(t, fakePublishedSnapshotLoader{err: ErrCurrentContentNotFound})

	_, err := repository.LoadPublishedContent(context.Background(), repositoryTestWorldID)
	if !errors.Is(err, ErrCurrentContentNotFound) {
		t.Fatalf("LoadPublishedContent() error = %v, want %v", err, ErrCurrentContentNotFound)
	}
}

func TestRepositoryMapsSeedSnapshotToValidGameplayContent(t *testing.T) {
	snapshot := seedSnapshot(t)

	bundle, err := loadSnapshotThroughContent(t, snapshot)
	if err != nil {
		t.Fatalf("LoadPublishedContent(seed) error = %v, want nil", err)
	}
	if got := bundle.Shop.Version; got != catalog.Version(repositoryTestPublishedVersion) {
		t.Fatalf("shop version = %q, want %q", got, repositoryTestPublishedVersion)
	}
	if _, ok := bundle.Modules.Lookup("laser_alpha_t1"); !ok {
		t.Fatal("laser_alpha_t1 missing from mapped modules")
	}
	if _, ok := bundle.Ships.Get("starter"); !ok {
		t.Fatal("starter ship missing from mapped ships")
	}
	if _, ok := bundle.LootTables[content.TrainingDroneSalvageLootTableID]; !ok {
		t.Fatalf("%s missing from mapped loot tables", content.TrainingDroneSalvageLootTableID)
	}
	template, ok := bundle.Quests.Lookup("quest_test_collect_raw_ore")
	if !ok {
		t.Fatal("quest_test_collect_raw_ore missing from mapped quests")
	}
	if template.RewardPayload == nil || len(template.RewardPayload.Grants) != 1 || template.RewardPayload.Grants[0].Amount != 250 {
		t.Fatalf("quest reward payload = %#v, want mapped reward table amount 250", template.RewardPayload)
	}
}

func TestRepositoryChangedDBModuleStatAndItemFieldSurviveAssembly(t *testing.T) {
	snapshot := seedSnapshot(t)
	mutateSnapshotRow[itemRowData](t, snapshot.Items, "raw_ore", func(row *itemRowData) {
		row.Name = "Dense Raw Ore"
	})
	mutateSnapshotRow[modules.ModuleDefinition](t, snapshot.Modules, "laser_alpha_t1", func(row *modules.ModuleDefinition) {
		for index := range row.StatModifiers {
			if row.StatModifiers[index].Stat == modules.StatWeaponDamage {
				row.StatModifiers[index].Value = 99
				return
			}
		}
		t.Fatal("laser_alpha_t1 weapon damage stat missing")
	})

	bundle, err := loadSnapshotThroughContent(t, snapshot)
	if err != nil {
		t.Fatalf("LoadPublishedContent(mutated seed) error = %v, want nil", err)
	}
	if got := bundle.Items["raw_ore"].Name; got != "Dense Raw Ore" {
		t.Fatalf("raw_ore name = %q, want Dense Raw Ore", got)
	}
	module, ok := bundle.Modules.Lookup("laser_alpha_t1")
	if !ok {
		t.Fatal("laser_alpha_t1 missing from mapped modules")
	}
	if got := statValue(t, module, modules.StatWeaponDamage); got != 99 {
		t.Fatalf("laser_alpha_t1 damage = %d, want 99", got)
	}
}

func TestRepositoryAdminPublishItemShipShopDraftSurvivesRuntimeReload(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)
	store := newAdminPublishRuntimeProofStore(t, seedSnapshot(t))
	service := admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    store,
		Writer:    store,
		Publisher: store,
		Snapshots: store,
		Validator: SnapshotValidator{WorldID: repositoryTestWorldID},
		Clock:     testutil.NewFakeClock(now),
	})

	updateDraftRowFromSeed(t, ctx, service, content.ContentTypeItem, store.current.Snapshot.Items, "raw_ore", json.RawMessage(`{"display_name":"Auric Ore","category":"resources","rarity":"uncommon"}`), func(row *itemRowData) {
		row.Name = "Auric Ore"
		row.Rarity = "uncommon"
		row.WeightUnits = 6
	})
	updateDraftRowFromSeed(t, ctx, service, content.ContentTypeShip, store.current.Snapshot.Ships, "starter", nil, func(row *ships.ShipDefinition) {
		row.Name = "Proof Skiff"
		row.BaseStats.HP = 321
		row.BaseStats.CargoCapacity = 88
		row.Slots.Offensive = 2
	})
	updateDraftRowFromSeed(t, ctx, service, content.ContentTypeShopProduct, store.current.Snapshot.ShopProducts, "product_laser_lens", nil, func(row *catalog.ShopProductDefinition) {
		row.Display.DisplayName = "Auric Lens Bundle"
		row.Display.Description = "Published runtime proof lens pack."
		row.Display.Rarity = "uncommon"
		row.GrantTarget.RefID = "laser_lens"
		row.GrantTarget.Quantity = 7
		row.Price.Amount = 123
		row.Availability = catalog.AvailabilityRule{Available: true}
	})

	published, err := service.PublishDraft(ctx, content.PublishDraftInput{
		Version:        "content_admin_runtime_proof_v1",
		Notes:          "item ship shop runtime proof",
		BalanceTag:     "runtime_proof",
		ActorAccountID: "admin-runtime-proof",
	})
	if err != nil {
		t.Fatalf("PublishDraft() error = %v, want nil", err)
	}
	if !published.Published || !published.Validation.Valid {
		t.Fatalf("PublishDraft() = %+v, want published valid", published)
	}
	if store.publishInput.ExpectedCurrentID != "11111111-1111-5111-8111-111111111111" ||
		store.publishInput.PublishedBy != "admin-runtime-proof" {
		t.Fatalf("publish input = %+v, want current guard and server actor", store.publishInput)
	}

	repository, err := newRepository(store)
	if err != nil {
		t.Fatalf("newRepository(admin store) error = %v, want nil", err)
	}
	bundle, err := content.LoadPublishedContent(ctx, repository, repositoryTestWorldID)
	if err != nil {
		t.Fatalf("LoadPublishedContent(after admin publish) error = %v, want nil", err)
	}

	item := bundle.Items["raw_ore"]
	if item.Name != "Auric Ore" || item.WeightUnits.Int64() != 6 || item.Rarity != "uncommon" {
		t.Fatalf("runtime item = %+v, want published name/weight/rarity", item)
	}
	ship, ok := bundle.Ships.Get("starter")
	if !ok {
		t.Fatal("starter ship missing after runtime reload")
	}
	if ship.Name != "Proof Skiff" || ship.BaseStats.HP != 321 || ship.BaseStats.CargoCapacity != 88 || ship.Slots.Offensive != 2 {
		t.Fatalf("runtime ship = %+v, want published hull/cargo/slot values", ship)
	}
	product, ok := bundle.Shop.ShopProduct("product_laser_lens")
	if !ok {
		t.Fatal("product_laser_lens missing after runtime reload")
	}
	if product.Price.Amount != 123 || product.Price.Amount == 40 ||
		product.GrantTarget.RefID != "laser_lens" || product.GrantTarget.Quantity != 7 || product.GrantTarget.Quantity == 2 ||
		!product.Availability.Available {
		t.Fatalf("runtime shop product = %+v, want published price/grant/enabled values", product)
	}

	projection, err := content.ProjectSnapshotForPlayers(store.current.Snapshot)
	if err != nil {
		t.Fatalf("ProjectSnapshotForPlayers() error = %v, want nil", err)
	}
	projectedItem := requireProjectedItem(t, projection, "raw_ore")
	if projectedItem.Display.DisplayName != "Auric Ore" || projectedItem.WeightUnits != 6 || projectedItem.Display.Category != "resources" {
		t.Fatalf("projected item = %+v, want client-safe published display/weight/category", projectedItem)
	}
	projectedProduct := requireProjectedShopProduct(t, projection, "product_laser_lens")
	if projectedProduct.Price.Amount != 123 || projectedProduct.GrantTarget.Quantity != 7 || !projectedProduct.Availability.Available {
		t.Fatalf("projected product = %+v, want client-safe published shop fields", projectedProduct)
	}
	rawProjection, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	for _, forbidden := range []string{`"metadata_schema"`, `"source"`, `"loot_table"`, `"spawn_area"`, `"enemy_pool"`, `"procedural_seed"`, `"snapshot_json"`} {
		if strings.Contains(string(rawProjection), forbidden) {
			t.Fatalf("content.catalog projection leaked %q in %s", forbidden, rawProjection)
		}
	}
}

func TestRepositoryMappedQuestRewardTableControlsGeneratedBoardReward(t *testing.T) {
	snapshot := seedSnapshot(t)
	targetTemplateID := catalog.DefinitionID("quest_test_collect_raw_ore")
	mutateSnapshotRow[content.QuestRewardTableRow](t, snapshot.QuestRewardTables, "quest_rewards."+targetTemplateID.String(), func(row *content.QuestRewardTableRow) {
		row.RewardPayload = quests.RewardPayload{Grants: []quests.RewardGrant{{
			Kind:     quests.RewardKindCredits,
			Currency: "credits",
			Amount:   4321,
		}}}
	})

	bundle, err := loadSnapshotThroughContent(t, snapshot)
	if err != nil {
		t.Fatalf("LoadPublishedContent(quest rewards) error = %v, want nil", err)
	}
	input := quests.BoardGenerationInput{
		Player: quests.PlayerQuestBoardSnapshot{
			PlayerID:  "player_quest_reward_cms",
			Rank:      1,
			MainLevel: 1,
		},
		Seed:      42,
		Catalog:   bundle.Quests,
		CreatedAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
		WeightHook: func(_ quests.PlayerQuestBoardSnapshot, template quests.QuestTemplate) int {
			if template.TemplateID == targetTemplateID {
				return 1 << 60
			}
			return 1
		},
	}
	offers, err := quests.GenerateBoard(input)
	if err != nil {
		t.Fatalf("GenerateBoard() error = %v, want nil", err)
	}
	for _, offer := range offers {
		if offer.TemplateID != targetTemplateID {
			continue
		}
		if len(offer.RewardPayload.Grants) != 1 || offer.RewardPayload.Grants[0].Amount != 4321 {
			t.Fatalf("offer reward payload = %#v, want CMS amount 4321", offer.RewardPayload)
		}
		return
	}
	t.Fatalf("target template %q not generated in offers %#v", targetTemplateID, offers)
}

func TestRepositoryMapNPCSnapshotRowsControlRuntimeCatalog(t *testing.T) {
	snapshot := seedSnapshot(t)
	mutateSnapshotRow[npcTemplateMapRow](t, snapshot.NPCTemplates, "training_drone_level_1", func(row *npcTemplateMapRow) {
		row.HPMax = 77
	})
	mutateSnapshotRow[enemyPoolMapRow](t, snapshot.EnemyPools, "map_1_1.starter_training_drone_pool", func(row *enemyPoolMapRow) {
		row.InitialAlive = 0
	})
	mutateSnapshotRow[spawnAreaMapRow](t, snapshot.SpawnAreas, "map_1_1.starter_training_drone_area", func(row *spawnAreaMapRow) {
		row.Radius = 190
	})

	bundle, err := loadSnapshotThroughContent(t, snapshot)
	if err != nil {
		t.Fatalf("LoadPublishedContent(map npc mutation) error = %v, want nil", err)
	}
	definition, ok := bundle.Maps.Get(worldmaps.StarterMapID)
	if !ok {
		t.Fatal("starter map missing from mapped catalog")
	}
	template := findNPCStatTemplate(t, definition, "training_drone_level_1")
	if template.HPMax != 77 {
		t.Fatalf("training drone hp = %v, want CMS value 77", template.HPMax)
	}
	pool := findEnemyPool(t, definition, "starter_training_drone_pool")
	if pool.InitialAlive != 0 {
		t.Fatalf("training drone initial alive = %d, want CMS value 0", pool.InitialAlive)
	}
	area := findSpawnArea(t, definition, "starter_training_drone_area")
	if area.Radius != 190 {
		t.Fatalf("training drone spawn radius = %v, want CMS value 190", area.Radius)
	}
}

func TestRepositoryForcesPublishedVersionOntoMappedDefinitions(t *testing.T) {
	snapshot := seedSnapshot(t)
	snapshot.Version = "stale_snapshot_v1"
	publishedVersion := "db_published_v42"

	bundle, err := loadSnapshotThroughContentWithVersion(t, snapshot, publishedVersion)
	if err != nil {
		t.Fatalf("LoadPublishedContent(versioned seed) error = %v, want nil", err)
	}
	want := catalog.Version(publishedVersion)
	if got := bundle.Items["raw_ore"].Source.Version; got != want {
		t.Fatalf("item source version = %q, want %q", got, want)
	}
	module, _ := bundle.Modules.Lookup("laser_alpha_t1")
	if got := module.Source.Version; got != want {
		t.Fatalf("module source version = %q, want %q", got, want)
	}
	ship, _ := bundle.Ships.Get("starter")
	if got := ship.Source.Version; got != want {
		t.Fatalf("ship source version = %q, want %q", got, want)
	}
	if got := bundle.LootTables[content.TrainingDroneSalvageLootTableID].Source.Version; got != want {
		t.Fatalf("loot source version = %q, want %q", got, want)
	}
	recipe, _ := bundle.Recipes.Get(crafting.RecipeIDRefinedAlloy)
	if got := recipe.Source.Version; got != want {
		t.Fatalf("recipe source version = %q, want %q", got, want)
	}
	productionDefinition, _ := bundle.Production.Get(production.ProductionDefinitionIDIronExtractorL1)
	if got := productionDefinition.Source.Version; got != want {
		t.Fatalf("production source version = %q, want %q", got, want)
	}
	questTemplate, _ := bundle.Quests.Lookup("quest_test_collect_raw_ore")
	if got := questTemplate.Source.Version; got != want {
		t.Fatalf("quest source version = %q, want %q", got, want)
	}
	if got := bundle.Shop.Version; got != want {
		t.Fatalf("shop version = %q, want %q", got, want)
	}
}

func TestRepositoryMissingItemReferencesFail(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*content.Snapshot)
	}{
		{
			name: "loot",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[content.LootTableSnapshotData](t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, func(row *content.LootTableSnapshotData) {
					row.Rows[0].ItemDefinition.ItemID = "missing_item"
				})
			},
		},
		{
			name: "recipe",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[recipeRowData](t, snapshot.CraftRecipes, crafting.RecipeIDRefinedAlloy.String(), func(row *recipeRowData) {
					row.Inputs[0].ItemID = "missing_item"
				})
			},
		},
		{
			name: "production",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[production.BuildingProductionDefinition](t, snapshot.ProductionBuildings, production.ProductionDefinitionIDIronExtractorL1.String(), func(row *production.BuildingProductionDefinition) {
					row.Outputs[0].ItemID = "missing_item"
				})
			},
		},
		{
			name: "shop",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[catalog.ShopProductDefinition](t, snapshot.ShopProducts, "product_laser_lens", func(row *catalog.ShopProductDefinition) {
					row.GrantTarget.RefID = "missing_item"
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := seedSnapshot(t)
			test.mutate(&snapshot)

			_, err := loadSnapshotThroughContent(t, snapshot)
			if err == nil {
				t.Fatal("LoadPublishedContent() error = nil, want missing reference error")
			}
		})
	}
}

func TestRepositoryLootTableRowsRejectLegacyWeightedShape(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*content.Snapshot)
	}{
		{
			name: "row weight",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRowData(t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, func(data map[string]any) {
					rows := data["Rows"].([]any)
					row := rows[0].(map[string]any)
					row["Weight"] = 100
				})
			},
		},
		{
			name: "top level weighted pool",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRowData(t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, func(data map[string]any) {
					data["WeightedRows"] = []any{map[string]any{"item_id": "raw_ore", "weight": 100}}
				})
			},
		},
		{
			name: "expanded item definition",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRowData(t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, func(data map[string]any) {
					rows := data["Rows"].([]any)
					row := rows[0].(map[string]any)
					item := row["ItemDefinition"].(map[string]any)
					item["name"] = "Raw Ore"
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := seedSnapshot(t)
			test.mutate(&snapshot)

			_, err := loadSnapshotThroughContent(t, snapshot)
			if err == nil {
				t.Fatal("LoadPublishedContent() error = nil, want strict loot DTO rejection")
			}
			if !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("LoadPublishedContent() error = %v, want unknown field rejection", err)
			}
		})
	}
}

func TestRepositoryLootTableRowsValidateChanceAndQuantity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*content.LootTableSnapshotData)
	}{
		{
			name: "chance below zero",
			mutate: func(row *content.LootTableSnapshotData) {
				row.Rows[0].Chance = -0.01
			},
		},
		{
			name: "chance above one",
			mutate: func(row *content.LootTableSnapshotData) {
				row.Rows[0].Chance = 1.01
			},
		},
		{
			name: "zero min quantity",
			mutate: func(row *content.LootTableSnapshotData) {
				row.Rows[0].MinQuantity = 0
			},
		},
		{
			name: "max below min quantity",
			mutate: func(row *content.LootTableSnapshotData) {
				row.Rows[0].MinQuantity = 3
				row.Rows[0].MaxQuantity = 2
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := seedSnapshot(t)
			mutateSnapshotRow[content.LootTableSnapshotData](t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, test.mutate)

			_, err := loadSnapshotThroughContent(t, snapshot)
			if !errors.Is(err, content.ErrInvalidContentLootRow) {
				t.Fatalf("LoadPublishedContent() error = %v, want %v", err, content.ErrInvalidContentLootRow)
			}
		})
	}
}

func TestRepositoryModuleWithoutMatchingItemFails(t *testing.T) {
	snapshot := seedSnapshot(t)
	snapshot.Items = removeSnapshotRow(snapshot.Items, "scanner_t1")

	_, err := loadSnapshotThroughContent(t, snapshot)
	if !errors.Is(err, content.ErrUnknownContentItem) {
		t.Fatalf("LoadPublishedContent() error = %v, want %v", err, content.ErrUnknownContentItem)
	}
}

func TestLoadPublishedContentWrapsDisabledRequiredRowValidationFailure(t *testing.T) {
	snapshot := seedSnapshot(t)
	setSnapshotRowEnabled(t, snapshot.Modules, "scanner_t1", false)

	_, err := loadSnapshotThroughContent(t, snapshot)
	if err == nil {
		t.Fatal("LoadPublishedContent() error = nil, want validation failure")
	}
	if !strings.Contains(err.Error(), "published content:") {
		t.Fatalf("LoadPublishedContent() error = %v, want published content wrapper", err)
	}
}

func TestLoadPublishedContentWrapsMapperFailure(t *testing.T) {
	snapshot := seedSnapshot(t)
	mutateSnapshotRow[itemRowData](t, snapshot.Items, "raw_ore", func(row *itemRowData) {
		row.ItemID = "raw_ore_mismatch"
	})

	_, err := loadSnapshotThroughContent(t, snapshot)
	if !errors.Is(err, ErrContentRowIDMismatch) {
		t.Fatalf("LoadPublishedContent() error = %v, want %v", err, ErrContentRowIDMismatch)
	}
	if !strings.Contains(err.Error(), "published content:") {
		t.Fatalf("LoadPublishedContent() error = %v, want published content wrapper", err)
	}
}

func TestRepositoryMapNPCRowsRejectDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*content.Snapshot)
	}{
		{
			name: "missing",
			mutate: func(snapshot *content.Snapshot) {
				snapshot.NPCTemplates = removeSnapshotRow(snapshot.NPCTemplates, string(snapshot.NPCTemplates[0].ContentID))
			},
		},
		{
			name: "extra",
			mutate: func(snapshot *content.Snapshot) {
				snapshot.SpawnAreas = append(snapshot.SpawnAreas, content.SnapshotRow{
					ContentID: "missing_map.extra_spawn_area",
					Enabled:   true,
					DataJSON:  json.RawMessage(`{"map_id":"missing_map","spawn_area_id":"extra_spawn_area","shape":"circle","center":{"x":100,"y":100},"radius":50}`),
				})
			},
		},
		{
			name: "mismatch",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[npcDropProfileMapRow](t, snapshot.NPCDropProfiles, string(snapshot.NPCDropProfiles[0].ContentID), func(row *npcDropProfileMapRow) {
					row.NPCType = "wrong_npc_type"
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := seedSnapshot(t)
			test.mutate(&snapshot)
			repository := newTestRepository(t, fakePublishedSnapshotLoader{snapshot: snapshot})

			_, err := repository.LoadPublishedContent(context.Background(), repositoryTestWorldID)
			if err == nil {
				t.Fatal("Repository.LoadPublishedContent() error = nil, want map/NPC drift error")
			}
		})
	}
}

type fakePublishedSnapshotLoader struct {
	snapshot content.Snapshot
	version  string
	err      error
}

func (loader fakePublishedSnapshotLoader) LoadCurrentPublishedSnapshot(ctx context.Context) (PublishedSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PublishedSnapshot{}, err
	}
	if loader.err != nil {
		return PublishedSnapshot{}, loader.err
	}
	version := loader.version
	if version == "" {
		version = loader.snapshot.Version
	}
	return PublishedSnapshot{
		Version:  version,
		Snapshot: loader.snapshot,
	}, nil
}

func seedSnapshot(t *testing.T) content.Snapshot {
	t.Helper()
	bundle, err := content.DefaultGameplayContent(repositoryTestWorldID)
	if err != nil {
		t.Fatalf("DefaultGameplayContent() error = %v, want nil", err)
	}
	snapshot := content.Snapshot{Version: "content_mvp_seed_v1"}
	appendSeedCoreRows(t, &snapshot, bundle)
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("seed snapshot Validate() error = %v, want nil", err)
	}
	return snapshot
}

func appendSeedCoreRows(t *testing.T, snapshot *content.Snapshot, bundle content.GameplayContent) {
	t.Helper()
	itemIDs := make([]foundation.ItemID, 0, len(bundle.Items))
	for itemID := range bundle.Items {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })
	for _, itemID := range itemIDs {
		snapshot.Items = append(snapshot.Items, testSnapshotRow(t, itemID.String(), bundle.Items[itemID]))
	}

	for _, definition := range bundle.Modules.Definitions() {
		snapshot.Modules = append(snapshot.Modules, testSnapshotRow(t, definition.ItemID.String(), definition))
	}
	for _, definition := range bundle.Ships.All() {
		snapshot.Ships = append(snapshot.Ships, testSnapshotRow(t, definition.ShipID.String(), definition))
	}
	for _, product := range bundle.Shop.SortedShopProducts() {
		snapshot.ShopProducts = append(snapshot.ShopProducts, testSnapshotRow(t, string(product.ProductID), product))
	}

	tableIDs := make([]string, 0, len(bundle.LootTables))
	for tableID := range bundle.LootTables {
		tableIDs = append(tableIDs, tableID)
	}
	sort.Strings(tableIDs)
	for _, tableID := range tableIDs {
		snapshot.LootTables = append(snapshot.LootTables, testSnapshotRow(t, tableID, content.SnapshotDataForLootTable(bundle.LootTables[tableID])))
	}
	for _, definition := range bundle.Recipes.Definitions() {
		snapshot.CraftRecipes = append(snapshot.CraftRecipes, testSnapshotRow(t, definition.RecipeID.String(), definition))
	}
	for _, definition := range bundle.Production.Definitions() {
		snapshot.ProductionBuildings = append(snapshot.ProductionBuildings, testSnapshotRow(t, definition.DefinitionID.String(), definition))
	}
	for _, definition := range bundle.Maps.Definitions() {
		appendSeedMapRows(t, snapshot, definition)
	}
	appendSeedServerRuleRows(t, snapshot, bundle)
	appendSeedQuestRows(t, snapshot)
}

func appendSeedServerRuleRows(t *testing.T, snapshot *content.Snapshot, bundle content.GameplayContent) {
	t.Helper()
	snapshot.ScannerConfigs = append(snapshot.ScannerConfigs, testSnapshotRow(t, "scanner_config", bundle.Scanner))
	snapshot.StarterConfigs = append(snapshot.StarterConfigs, testSnapshotRow(t, "starter_config", bundle.Starter))
	snapshot.RoutePolicies = append(snapshot.RoutePolicies, testSnapshotRow(t, "route_policy", bundle.Route))
	snapshot.ProductionRules = append(snapshot.ProductionRules, testSnapshotRow(t, "production_rules", bundle.Rules))
	snapshot.CombatRules = append(snapshot.CombatRules, testSnapshotRow(t, "combat_rules", bundle.Combat))
}

func appendSeedQuestRows(t *testing.T, snapshot *content.Snapshot) {
	t.Helper()
	templateIDs := []catalog.DefinitionID{"quest_test_collect_raw_ore"}
	for index := 1; index < quests.BoardOfferCount; index++ {
		templateIDs = append(templateIDs, catalog.DefinitionID(fmt.Sprintf("quest_test_collect_raw_ore_%02d", index)))
	}
	for index, templateID := range templateIDs {
		appendSeedQuestRow(t, snapshot, templateID, int64(250+index))
	}
}

func appendSeedQuestRow(t *testing.T, snapshot *content.Snapshot, templateID catalog.DefinitionID, rewardAmount int64) {
	t.Helper()
	source, err := catalog.NewQuestSource(templateID.String(), "quest_seed_test_v1")
	if err != nil {
		t.Fatalf("NewQuestSource() error = %v, want nil", err)
	}
	row := content.QuestTemplateRow{
		Source:         source,
		TemplateID:     templateID,
		Type:           quests.QuestTypeCollect,
		TitleKey:       "quest." + templateID.String() + ".title",
		DescriptionKey: "quest." + templateID.String() + ".description",
		ObjectiveSchema: content.QuestObjectiveSchemaRow{Objectives: []content.QuestObjectiveRow{{
			ID:   "collect_raw_ore",
			Kind: quests.ObjectiveKindCollect,
			Collect: &content.QuestCollectObjectiveRow{
				ItemID:   "raw_ore",
				Quantity: 1,
			},
		}}},
		BoardWeight: 100,
	}
	snapshot.QuestTemplates = append(snapshot.QuestTemplates, testSnapshotRow(t, templateID.String(), row))
	rewardTableID := catalog.DefinitionID("quest_rewards." + templateID.String())
	rewardSource, err := catalog.NewQuestSource(rewardTableID.String(), "quest_seed_test_v1")
	if err != nil {
		t.Fatalf("NewQuestSource(reward) error = %v, want nil", err)
	}
	reward := content.QuestRewardTableRow{
		Source:        rewardSource,
		RewardTableID: rewardTableID,
		TemplateID:    templateID,
		RewardPayload: quests.RewardPayload{Grants: []quests.RewardGrant{{
			Kind:     quests.RewardKindCredits,
			Currency: "credits",
			Amount:   rewardAmount,
		}}},
		Weight:      100,
		Probability: 1,
	}
	snapshot.QuestRewardTables = append(snapshot.QuestRewardTables, testSnapshotRow(t, rewardTableID.String(), reward))
}

func appendSeedMapRows(t *testing.T, snapshot *content.Snapshot, definition worldmaps.MapDefinition) {
	t.Helper()
	for _, template := range definition.NPCStatTemplates {
		snapshot.NPCTemplates = append(snapshot.NPCTemplates, testSnapshotRow(t, template.StatTemplateID.String(), npcTemplateMapRowFromDefinition(definition.InternalMapID, template)))
	}
	for _, area := range definition.SpawnAreas {
		snapshot.SpawnAreas = append(snapshot.SpawnAreas, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, area.SpawnAreaID.String()), spawnAreaMapRowFromDefinition(definition.InternalMapID, area)))
	}
	for _, pool := range definition.EnemyPools {
		snapshot.EnemyPools = append(snapshot.EnemyPools, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, pool.EnemyPoolID.String()), enemyPoolMapRowFromDefinition(definition.InternalMapID, pool)))
	}
	for _, profile := range definition.NPCDropProfiles {
		snapshot.NPCDropProfiles = append(snapshot.NPCDropProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.DropProfileID.String()), npcDropProfileMapRowFromDefinition(definition.InternalMapID, profile)))
	}
	for _, profile := range definition.NPCAggroProfiles {
		snapshot.NPCAggroProfiles = append(snapshot.NPCAggroProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.AggroProfileID.String()), npcAggroProfileMapRowFromDefinition(definition.InternalMapID, profile)))
	}
	for _, profile := range definition.NPCLeashProfiles {
		snapshot.NPCLeashProfiles = append(snapshot.NPCLeashProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.LeashProfileID.String()), npcLeashProfileMapRowFromDefinition(definition.InternalMapID, profile)))
	}
	for _, eventSpawn := range definition.NPCEventSpawns {
		snapshot.NPCEventSpawns = append(snapshot.NPCEventSpawns, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, eventSpawn.EventSpawnID.String()), npcEventSpawnMapRowFromDefinition(definition.InternalMapID, eventSpawn)))
	}
}

func npcTemplateMapRowFromDefinition(mapID worldmaps.MapID, template worldmaps.NPCStatTemplate) npcTemplateMapRow {
	return npcTemplateMapRow{
		MapID:          mapID,
		StatTemplateID: template.StatTemplateID,
		NPCType:        template.NPCType,
		MinLevel:       template.MinLevel,
		MaxLevel:       template.MaxLevel,
		LabelKey:       template.LabelKey,
		HPMax:          template.HPMax,
		ShieldMax:      template.ShieldMax,
		EnergyMax:      template.EnergyMax,
		WeaponRange:    template.WeaponRange,
		WeaponDamage:   template.WeaponDamage,
		WeaponCooldown: template.WeaponCooldown,
		Accuracy:       template.Accuracy,
		RadarSignature: template.RadarSignature,
		Speed:          template.Speed,
		XPValue:        template.XPValue,
	}
}

func spawnAreaMapRowFromDefinition(mapID worldmaps.MapID, area worldmaps.MapSpawnAreaDefinition) spawnAreaMapRow {
	return spawnAreaMapRow{
		MapID:                 mapID,
		SpawnAreaID:           area.SpawnAreaID,
		Shape:                 area.Shape,
		Center:                area.Center,
		Radius:                area.Radius,
		SafeZoneExcluded:      area.SafeZoneExcluded,
		PortalExclusionRadius: area.PortalExclusionRadius,
	}
}

func enemyPoolMapRowFromDefinition(mapID worldmaps.MapID, pool worldmaps.MapEnemyPoolDefinition) enemyPoolMapRow {
	return enemyPoolMapRow{
		MapID:            mapID,
		EnemyPoolID:      pool.EnemyPoolID,
		NPCType:          pool.NPCType,
		MinLevel:         pool.MinLevel,
		MaxLevel:         pool.MaxLevel,
		SpawnAreaIDs:     append([]worldmaps.SpawnAreaID(nil), pool.SpawnAreaIDs...),
		MapMaxAlive:      pool.MapMaxAlive,
		PoolMaxAlive:     pool.PoolMaxAlive,
		InitialAlive:     pool.InitialAlive,
		SpawnInterval:    pool.SpawnInterval,
		KillRespawnDelay: pool.KillRespawnDelay,
		SpawnJitter:      pool.SpawnJitter,
		SpawnMode:        pool.SpawnMode,
		StatTemplateID:   pool.StatTemplateID,
		DropProfileID:    pool.DropProfileID,
		AggroProfileID:   pool.AggroProfileID,
		LeashProfileID:   pool.LeashProfileID,
		Enabled:          pool.Enabled,
	}
}

func npcDropProfileMapRowFromDefinition(mapID worldmaps.MapID, profile worldmaps.NPCDropProfile) npcDropProfileMapRow {
	return npcDropProfileMapRow{
		MapID:         mapID,
		DropProfileID: profile.DropProfileID,
		NPCType:       profile.NPCType,
		MinLevel:      profile.MinLevel,
		MaxLevel:      profile.MaxLevel,
		RiskBand:      profile.RiskBand,
		LootTableID:   profile.LootTableID,
	}
}

func npcAggroProfileMapRowFromDefinition(mapID worldmaps.MapID, profile worldmaps.NPCAggroProfile) npcAggroProfileMapRow {
	return npcAggroProfileMapRow{
		MapID:                mapID,
		AggroProfileID:       profile.AggroProfileID,
		AggroRadius:          profile.AggroRadius,
		AssistRadius:         profile.AssistRadius,
		TargetMemory:         profile.TargetMemory,
		SafeZoneAttackPolicy: profile.SafeZoneAttackPolicy,
	}
}

func npcLeashProfileMapRowFromDefinition(mapID worldmaps.MapID, profile worldmaps.NPCLeashProfile) npcLeashProfileMapRow {
	return npcLeashProfileMapRow{
		MapID:          mapID,
		LeashProfileID: profile.LeashProfileID,
		LeashDistance:  profile.LeashDistance,
		ResetOnBreak:   profile.ResetOnBreak,
	}
}

func npcEventSpawnMapRowFromDefinition(mapID worldmaps.MapID, eventSpawn worldmaps.NPCEventSpawnDefinition) npcEventSpawnMapRow {
	return npcEventSpawnMapRow{
		MapID:         mapID,
		EventSpawnID:  eventSpawn.EventSpawnID,
		EnemyPoolID:   eventSpawn.EnemyPoolID,
		DropProfileID: eventSpawn.DropProfileID,
		Enabled:       eventSpawn.Enabled,
		StartsAfter:   eventSpawn.StartsAfter,
		MaxAlive:      eventSpawn.MaxAlive,
		MapPolicy:     eventSpawn.MapPolicy,
	}
}

func findNPCStatTemplate(t *testing.T, definition worldmaps.MapDefinition, templateID worldmaps.NPCStatTemplateID) worldmaps.NPCStatTemplate {
	t.Helper()
	for _, template := range definition.NPCStatTemplates {
		if template.StatTemplateID == templateID {
			return template
		}
	}
	t.Fatalf("npc stat template %q missing", templateID)
	return worldmaps.NPCStatTemplate{}
}

func findEnemyPool(t *testing.T, definition worldmaps.MapDefinition, poolID worldmaps.EnemyPoolID) worldmaps.MapEnemyPoolDefinition {
	t.Helper()
	for _, pool := range definition.EnemyPools {
		if pool.EnemyPoolID == poolID {
			return pool
		}
	}
	t.Fatalf("enemy pool %q missing", poolID)
	return worldmaps.MapEnemyPoolDefinition{}
}

func findSpawnArea(t *testing.T, definition worldmaps.MapDefinition, areaID worldmaps.SpawnAreaID) worldmaps.MapSpawnAreaDefinition {
	t.Helper()
	for _, area := range definition.SpawnAreas {
		if area.SpawnAreaID == areaID {
			return area
		}
	}
	t.Fatalf("spawn area %q missing", areaID)
	return worldmaps.MapSpawnAreaDefinition{}
}

func testSnapshotRow(t *testing.T, contentID string, data any) content.SnapshotRow {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal row %q: %v", contentID, err)
	}
	return content.SnapshotRow{
		ContentID: content.ContentID(contentID),
		Enabled:   true,
		DataJSON:  raw,
	}
}

func newTestRepository(t *testing.T, loader fakePublishedSnapshotLoader) *Repository {
	t.Helper()
	repository, err := newRepository(loader)
	if err != nil {
		t.Fatalf("newRepository() error = %v, want nil", err)
	}
	return repository
}

func loadSnapshotThroughContent(t *testing.T, snapshot content.Snapshot) (content.GameplayContent, error) {
	t.Helper()
	return loadSnapshotThroughContentWithVersion(t, snapshot, repositoryTestPublishedVersion)
}

func loadSnapshotThroughContentWithVersion(t *testing.T, snapshot content.Snapshot, version string) (content.GameplayContent, error) {
	t.Helper()
	repository := newTestRepository(t, fakePublishedSnapshotLoader{snapshot: snapshot, version: version})
	return content.LoadPublishedContent(context.Background(), repository, repositoryTestWorldID)
}

func mutateSnapshotRow[T any](t *testing.T, rows []content.SnapshotRow, contentID string, mutate func(*T)) {
	t.Helper()
	for index := range rows {
		if string(rows[index].ContentID) != contentID {
			continue
		}
		var decoded T
		if err := json.Unmarshal(rows[index].DataJSON, &decoded); err != nil {
			t.Fatalf("decode row %q: %v", contentID, err)
		}
		mutate(&decoded)
		raw, err := json.Marshal(decoded)
		if err != nil {
			t.Fatalf("encode row %q: %v", contentID, err)
		}
		rows[index].DataJSON = raw
		return
	}
	t.Fatalf("row %q missing", contentID)
}

func mutateSnapshotRowData(t *testing.T, rows []content.SnapshotRow, contentID string, mutate func(map[string]any)) {
	t.Helper()
	for index := range rows {
		if string(rows[index].ContentID) != contentID {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal(rows[index].DataJSON, &decoded); err != nil {
			t.Fatalf("decode row %q: %v", contentID, err)
		}
		mutate(decoded)
		raw, err := json.Marshal(decoded)
		if err != nil {
			t.Fatalf("encode row %q: %v", contentID, err)
		}
		rows[index].DataJSON = raw
		return
	}
	t.Fatalf("row %q missing", contentID)
}

func setSnapshotRowEnabled(t *testing.T, rows []content.SnapshotRow, contentID string, enabled bool) {
	t.Helper()
	for index := range rows {
		if string(rows[index].ContentID) == contentID {
			rows[index].Enabled = enabled
			return
		}
	}
	t.Fatalf("row %q missing", contentID)
}

func removeSnapshotRow(rows []content.SnapshotRow, contentID string) []content.SnapshotRow {
	for index := range rows {
		if string(rows[index].ContentID) == contentID {
			return append(rows[:index], rows[index+1:]...)
		}
	}
	return rows
}

func statValue(t *testing.T, definition modules.ModuleDefinition, stat modules.StatKey) int64 {
	t.Helper()
	for _, modifier := range definition.StatModifiers {
		if modifier.Stat == stat {
			return modifier.Value
		}
	}
	t.Fatalf("stat %q missing from %q", stat, definition.ItemID)
	return 0
}

type adminPublishRuntimeProofStore struct {
	drafts       map[content.ContentType][]content.DraftRow
	current      content.SnapshotVersionRecord
	publishInput content.PublishSnapshotInput
}

func newAdminPublishRuntimeProofStore(t *testing.T, snapshot content.Snapshot) *adminPublishRuntimeProofStore {
	t.Helper()
	return &adminPublishRuntimeProofStore{
		drafts: draftRowsFromSnapshot(snapshot),
		current: content.SnapshotVersionRecord{
			ID:          "11111111-1111-5111-8111-111111111111",
			Version:     snapshot.Version,
			Status:      "published",
			Current:     true,
			Snapshot:    snapshot,
			CreatedAt:   time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC),
			PublishedAt: time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC),
		},
	}
}

func (store *adminPublishRuntimeProofStore) LoadDraftRows(_ context.Context, contentType content.ContentType) ([]content.DraftRow, error) {
	return cloneDraftProofRows(store.drafts[contentType]), nil
}

func (store *adminPublishRuntimeProofStore) UpsertDraftRow(_ context.Context, contentType content.ContentType, row content.DraftRow) error {
	rows := cloneDraftProofRows(store.drafts[contentType])
	replaced := false
	for index := range rows {
		if rows[index].ContentID == row.ContentID {
			rows[index] = cloneDraftProofRow(row)
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append(rows, cloneDraftProofRow(row))
	}
	store.drafts[contentType] = rows
	return nil
}

func (store *adminPublishRuntimeProofStore) LoadCurrentContentSnapshot(context.Context) (content.SnapshotVersionRecord, error) {
	return store.current, nil
}

func (store *adminPublishRuntimeProofStore) LoadContentSnapshotByID(_ context.Context, id string) (content.SnapshotVersionRecord, error) {
	if store.current.ID == id {
		return store.current, nil
	}
	return content.SnapshotVersionRecord{}, ErrCurrentContentNotFound
}

func (store *adminPublishRuntimeProofStore) PublishContentSnapshot(_ context.Context, input content.PublishSnapshotInput) (content.PublishSnapshotResult, error) {
	store.publishInput = input
	store.current = content.SnapshotVersionRecord{
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
	return content.PublishSnapshotResult{Record: store.current}, nil
}

func (store *adminPublishRuntimeProofStore) LoadCurrentPublishedSnapshot(ctx context.Context) (PublishedSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PublishedSnapshot{}, err
	}
	return PublishedSnapshot{
		ID:          store.current.ID,
		Version:     store.current.Version,
		Snapshot:    store.current.Snapshot,
		Notes:       store.current.Notes,
		BalanceTag:  store.current.BalanceTag,
		PublishedAt: store.current.PublishedAt,
	}, nil
}

func draftRowsFromSnapshot(snapshot content.Snapshot) map[content.ContentType][]content.DraftRow {
	out := make(map[content.ContentType][]content.DraftRow)
	for _, group := range snapshot.Groups() {
		for _, row := range group.Rows {
			out[group.Type] = append(out[group.Type], content.DraftRow{
				ContentID:    row.ContentID,
				DraftVersion: snapshot.Version,
				Enabled:      row.Enabled,
				DisplayJSON:  append(json.RawMessage(nil), row.DisplayJSON...),
				DataJSON:     append(json.RawMessage(nil), row.DataJSON...),
				UpdatedBy:    "seed",
			})
		}
	}
	return out
}

func updateDraftRowFromSeed[T any](t *testing.T, ctx context.Context, service *admin.ContentService, contentType content.ContentType, rows []content.SnapshotRow, contentID string, displayJSON json.RawMessage, mutate func(*T)) {
	t.Helper()
	for _, row := range rows {
		if string(row.ContentID) != contentID {
			continue
		}
		var decoded T
		if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
			t.Fatalf("decode seed row %q: %v", contentID, err)
		}
		mutate(&decoded)
		dataJSON, err := json.Marshal(decoded)
		if err != nil {
			t.Fatalf("encode seed row %q: %v", contentID, err)
		}
		if displayJSON == nil {
			displayJSON = row.DisplayJSON
		}
		if _, err := service.UpdateDraftRow(ctx, content.DraftUpdateInput{
			ContentType: contentType,
			ContentID:   content.ContentID(contentID),
			Enabled:     row.Enabled,
			DisplayJSON: displayJSON,
			DataJSON:    dataJSON,
			UpdatedBy:   "admin-runtime-proof",
		}); err != nil {
			t.Fatalf("UpdateDraftRow(%s %q) error = %v, want nil", contentType, contentID, err)
		}
		return
	}
	t.Fatalf("seed row %q missing", contentID)
}

func requireProjectedItem(t *testing.T, projection content.PlayerContentProjection, itemID string) content.PlayerItemProjection {
	t.Helper()
	for _, item := range projection.Items {
		if item.ItemID == itemID {
			return item
		}
	}
	t.Fatalf("projected item %q missing", itemID)
	return content.PlayerItemProjection{}
}

func requireProjectedShopProduct(t *testing.T, projection content.PlayerContentProjection, productID string) content.PlayerShopProductProjection {
	t.Helper()
	for _, product := range projection.ShopProducts {
		if product.ProductID == productID {
			return product
		}
	}
	t.Fatalf("projected shop product %q missing", productID)
	return content.PlayerShopProductProjection{}
}

func cloneDraftProofRows(rows []content.DraftRow) []content.DraftRow {
	out := make([]content.DraftRow, len(rows))
	for index := range rows {
		out[index] = cloneDraftProofRow(rows[index])
	}
	return out
}

func cloneDraftProofRow(row content.DraftRow) content.DraftRow {
	row.DisplayJSON = append(json.RawMessage(nil), row.DisplayJSON...)
	row.DataJSON = append(json.RawMessage(nil), row.DataJSON...)
	return row
}
