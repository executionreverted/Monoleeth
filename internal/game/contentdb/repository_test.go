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

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
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
				mutateSnapshotRow[lootTableRowData](t, snapshot.LootTables, content.TrainingDroneSalvageLootTableID, func(row *lootTableRowData) {
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
				mutateSnapshotRow[catalog.ShopProductDefinition](t, snapshot.ShopProducts, "product_ferrite_ore", func(row *catalog.ShopProductDefinition) {
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
					ContentID: "map_1_1.extra_spawn_area",
					Enabled:   true,
					DataJSON:  json.RawMessage(`{"map_id":"map_1_1","spawn_area_id":"extra_spawn_area"}`),
				})
			},
		},
		{
			name: "mismatch",
			mutate: func(snapshot *content.Snapshot) {
				mutateSnapshotRow[npcDropProfileMapRow](t, snapshot.NPCDropProfiles, string(snapshot.NPCDropProfiles[0].ContentID), func(row *npcDropProfileMapRow) {
					row.LootTableID = "wrong_loot_table"
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
		snapshot.LootTables = append(snapshot.LootTables, testSnapshotRow(t, tableID, bundle.LootTables[tableID]))
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
	appendSeedQuestRows(t, snapshot)
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
		snapshot.NPCTemplates = append(snapshot.NPCTemplates, testSnapshotRow(t, template.StatTemplateID.String(), npcTemplateMapRow{
			MapID:   definition.InternalMapID,
			NPCType: template.NPCType,
		}))
	}
	for _, area := range definition.SpawnAreas {
		snapshot.SpawnAreas = append(snapshot.SpawnAreas, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, area.SpawnAreaID.String()), spawnAreaMapRow{
			MapID:       definition.InternalMapID,
			SpawnAreaID: area.SpawnAreaID,
		}))
	}
	for _, pool := range definition.EnemyPools {
		snapshot.EnemyPools = append(snapshot.EnemyPools, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, pool.EnemyPoolID.String()), enemyPoolMapRow{
			MapID:       definition.InternalMapID,
			EnemyPoolID: pool.EnemyPoolID,
			NPCType:     pool.NPCType,
		}))
	}
	for _, profile := range definition.NPCDropProfiles {
		snapshot.NPCDropProfiles = append(snapshot.NPCDropProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.DropProfileID.String()), npcDropProfileMapRow{
			MapID:         definition.InternalMapID,
			DropProfileID: profile.DropProfileID,
			NPCType:       profile.NPCType,
			LootTableID:   profile.LootTableID,
		}))
	}
	for _, profile := range definition.NPCAggroProfiles {
		snapshot.NPCAggroProfiles = append(snapshot.NPCAggroProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.AggroProfileID.String()), npcAggroProfileMapRow{
			MapID:          definition.InternalMapID,
			AggroProfileID: profile.AggroProfileID,
		}))
	}
	for _, profile := range definition.NPCLeashProfiles {
		snapshot.NPCLeashProfiles = append(snapshot.NPCLeashProfiles, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, profile.LeashProfileID.String()), npcLeashProfileMapRow{
			MapID:          definition.InternalMapID,
			LeashProfileID: profile.LeashProfileID,
		}))
	}
	for _, eventSpawn := range definition.NPCEventSpawns {
		snapshot.NPCEventSpawns = append(snapshot.NPCEventSpawns, testSnapshotRow(t, qualifiedMapContentID(definition.InternalMapID, eventSpawn.EventSpawnID.String()), npcEventSpawnMapRow{
			MapID:        definition.InternalMapID,
			EventSpawnID: eventSpawn.EventSpawnID,
		}))
	}
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
