package contentseed

import (
	"bytes"
	"encoding/json"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/world"
)

func TestBuildMVPSnapshotIncludesQuestTemplatesAndRewardTables(t *testing.T) {
	snapshot := buildTestSnapshot(t)
	want := len(quests.MustMVPQuestCatalog().Templates())

	if got := len(snapshot.QuestTemplates); got != want {
		t.Fatalf("quest templates = %d, want %d", got, want)
	}
	if got := len(snapshot.QuestRewardTables); got != want {
		t.Fatalf("quest reward tables = %d, want %d", got, want)
	}
	if len(snapshot.Items) == 0 || len(snapshot.CraftRecipes) == 0 || len(snapshot.ProductionBuildings) == 0 || len(snapshot.NPCTemplates) == 0 {
		t.Fatalf("seed snapshot missing core refs: items=%d recipes=%d production_buildings=%d npc_templates=%d",
			len(snapshot.Items), len(snapshot.CraftRecipes), len(snapshot.ProductionBuildings), len(snapshot.NPCTemplates))
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot.Validate() error = %v, want nil", err)
	}
}

func TestMVPQuestSeedRowsRoundTripIDs(t *testing.T) {
	snapshot := buildTestSnapshot(t)
	templateIDs := map[catalog.DefinitionID]struct{}{}

	for _, row := range snapshot.QuestTemplates {
		data := decodeQuestTemplateRow(t, row)
		if got, want := row.ContentID, content.ContentID(data.TemplateID.String()); got != want {
			t.Fatalf("template row content_id = %q, want %q", got, want)
		}
		if data.Source.DefinitionID != data.TemplateID {
			t.Fatalf("template source = %q, want template %q", data.Source.DefinitionID, data.TemplateID)
		}
		if err := content.ValidateContentID("template", data.TemplateID.String()); err != nil {
			t.Fatalf("ValidateContentID(%q) error = %v", data.TemplateID, err)
		}
		template, err := data.Template()
		if err != nil {
			t.Fatalf("Template() error = %v, want nil", err)
		}
		if template.TemplateID != data.TemplateID || template.Source != data.Source {
			t.Fatalf("template round trip = %+v, want id=%q source=%+v", template, data.TemplateID, data.Source)
		}
		templateIDs[data.TemplateID] = struct{}{}
	}

	for _, row := range snapshot.QuestRewardTables {
		data := decodeQuestRewardTableRow(t, row)
		if got, want := row.ContentID, content.ContentID(data.RewardTableID.String()); got != want {
			t.Fatalf("reward row content_id = %q, want %q", got, want)
		}
		if data.Source.DefinitionID != data.RewardTableID {
			t.Fatalf("reward source = %q, want table %q", data.Source.DefinitionID, data.RewardTableID)
		}
		if _, ok := templateIDs[data.TemplateID]; !ok {
			t.Fatalf("reward table %q references missing template %q", data.RewardTableID, data.TemplateID)
		}
		if err := content.ValidateContentID("reward table", data.RewardTableID.String()); err != nil {
			t.Fatalf("ValidateContentID(%q) error = %v", data.RewardTableID, err)
		}
		if err := data.RewardPayload.Validate(); err != nil {
			t.Fatalf("reward payload %q Validate() error = %v", data.RewardTableID, err)
		}
	}
}

func TestMVPQuestSeedRowsValidateAgainstSeedResolver(t *testing.T) {
	snapshot := buildTestSnapshot(t)
	bundle, err := content.DefaultGameplayContent(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultGameplayContent() error = %v, want nil", err)
	}

	err = content.ValidateQuestContentRows(
		snapshot.QuestTemplates,
		snapshot.QuestRewardTables,
		questReferenceResolverFromSeed(t, snapshot, bundle),
	)
	if err != nil {
		t.Fatalf("ValidateQuestContentRows() error = %v, want nil", err)
	}
}

func TestMVPQuestSeedRowsUseCurrentContentReferences(t *testing.T) {
	snapshot := buildTestSnapshot(t)

	assertNoSeedRow(t, snapshot.CraftRecipes, "energy_cell_batch")
	assertNoSeedRow(t, snapshot.ProductionBuildings, "extractor_t1")
	assertNoSeedRow(t, snapshot.ProductionBuildings, "storage_t1")
	assertNoSeedRow(t, snapshot.NPCTemplates, "pirate")
	assertNoSeedRow(t, snapshot.NPCTemplates, "raider")
	assertNoSeedRow(t, snapshot.NPCTemplates, "void_raider")

	killPirates := requireQuestTemplateRow(t, snapshot, "quest_kill_pirates_r1")
	if got, want := killPirates.ObjectiveSchema.Objectives[0].Kill.TargetNPCType, "streuner"; got != want {
		t.Fatalf("pirate quest target = %q, want %q", got, want)
	}
	killRaiders := requireQuestTemplateRow(t, snapshot, "quest_kill_raiders_r1")
	if got, want := killRaiders.ObjectiveSchema.Objectives[0].Kill.TargetNPCType, "saimon"; got != want {
		t.Fatalf("raider quest target = %q, want %q", got, want)
	}
	craftEnergy := requireQuestTemplateRow(t, snapshot, "quest_craft_energy_cells_r1")
	if got, want := craftEnergy.ObjectiveSchema.Objectives[0].Craft.RecipeID, crafting.RecipeIDRefinedAlloy; got != want {
		t.Fatalf("energy craft quest recipe = %q, want %q", got, want)
	}
	buildExtractor := requireQuestTemplateRow(t, snapshot, "quest_build_extractor_r1")
	if got, want := buildExtractor.ObjectiveSchema.Objectives[0].Build.BuildingType, production.BuildingTypeIronExtractor.String(); got != want {
		t.Fatalf("extractor quest building = %q, want %q", got, want)
	}
	buildStorage := requireQuestTemplateRow(t, snapshot, "quest_build_storage_r1")
	if got, want := buildStorage.ObjectiveSchema.Objectives[0].Build.BuildingType, production.BuildingTypeAlloyFoundry.String(); got != want {
		t.Fatalf("storage quest building = %q, want %q", got, want)
	}
}

func TestMVPQuestRewardRowsDoNotLeakPlayerHookKeys(t *testing.T) {
	snapshot := buildTestSnapshot(t)

	for _, row := range snapshot.QuestRewardTables {
		if bytes.Contains(row.DataJSON, []byte("rare_cap")) ||
			bytes.Contains(row.DataJSON, []byte("player")) ||
			bytes.Contains(row.DataJSON, []byte("offer_seed")) {
			t.Fatalf("reward row %q contains runtime/player hook data: %s", row.ContentID, row.DataJSON)
		}
		data := decodeQuestRewardTableRow(t, row)
		if len(data.RewardPayload.RareCapHooks) != 0 || len(data.RewardPayload.Hooks) != 0 {
			t.Fatalf("reward row %q hooks = %+v %+v, want none", row.ContentID, data.RewardPayload.RareCapHooks, data.RewardPayload.Hooks)
		}
	}
}

func requireQuestTemplateRow(t *testing.T, snapshot content.Snapshot, templateID catalog.DefinitionID) content.QuestTemplateRow {
	t.Helper()
	for _, row := range snapshot.QuestTemplates {
		if row.ContentID == content.ContentID(templateID.String()) {
			return decodeQuestTemplateRow(t, row)
		}
	}
	t.Fatalf("missing quest template row %q", templateID)
	return content.QuestTemplateRow{}
}

func assertNoSeedRow(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) {
	t.Helper()
	for _, row := range rows {
		if row.ContentID == contentID {
			t.Fatalf("unexpected synthetic seed row %q", contentID)
		}
	}
}

func buildTestSnapshot(t *testing.T) content.Snapshot {
	t.Helper()
	snapshot, err := BuildMVPSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildMVPSnapshot() error = %v, want nil", err)
	}
	return snapshot
}

func decodeQuestTemplateRow(t *testing.T, row content.SnapshotRow) content.QuestTemplateRow {
	t.Helper()
	var data content.QuestTemplateRow
	if err := json.Unmarshal(row.DataJSON, &data); err != nil {
		t.Fatalf("Unmarshal template row %q error = %v", row.ContentID, err)
	}
	return data
}

func decodeQuestRewardTableRow(t *testing.T, row content.SnapshotRow) content.QuestRewardTableRow {
	t.Helper()
	var data content.QuestRewardTableRow
	if err := json.Unmarshal(row.DataJSON, &data); err != nil {
		t.Fatalf("Unmarshal reward row %q error = %v", row.ContentID, err)
	}
	return data
}

func questReferenceResolverFromSeed(t *testing.T, snapshot content.Snapshot, bundle content.GameplayContent) content.QuestReferenceResolver {
	t.Helper()
	templates := map[catalog.DefinitionID]struct{}{}
	items := map[foundation.ItemID]struct{}{}
	ships := map[foundation.ShipID]struct{}{}
	recipes := map[catalog.DefinitionID]struct{}{}
	productionRows := map[catalog.DefinitionID]struct{}{}
	buildings := map[string]struct{}{}
	npcs := map[string]struct{}{}

	for _, row := range snapshot.QuestTemplates {
		data := decodeQuestTemplateRow(t, row)
		templates[data.TemplateID] = struct{}{}
	}
	for itemID := range bundle.Items {
		items[itemID] = struct{}{}
	}
	for _, row := range snapshot.Items {
		items[foundation.ItemID(row.ContentID)] = struct{}{}
	}
	for _, definition := range bundle.Ships.All() {
		ships[definition.ShipID] = struct{}{}
	}
	for _, definition := range bundle.Recipes.Definitions() {
		recipes[definition.RecipeID] = struct{}{}
	}
	for _, row := range snapshot.CraftRecipes {
		recipes[catalog.DefinitionID(row.ContentID)] = struct{}{}
	}
	for _, definition := range bundle.Production.Definitions() {
		productionRows[definition.DefinitionID] = struct{}{}
		buildings[definition.BuildingType.String()] = struct{}{}
	}
	for _, row := range snapshot.ProductionBuildings {
		productionRows[catalog.DefinitionID(row.ContentID)] = struct{}{}
		buildings[string(row.ContentID)] = struct{}{}
		var payload struct {
			BuildingID   string `json:"building_id"`
			BuildingType string `json:"building_type"`
		}
		if err := json.Unmarshal(row.DataJSON, &payload); err != nil {
			t.Fatalf("Unmarshal production/building row %q error = %v", row.ContentID, err)
		}
		if payload.BuildingID != "" {
			buildings[payload.BuildingID] = struct{}{}
		}
		if payload.BuildingType != "" {
			buildings[payload.BuildingType] = struct{}{}
		}
	}
	for _, definition := range bundle.Maps.Definitions() {
		for _, template := range definition.NPCStatTemplates {
			npcs[template.NPCType] = struct{}{}
		}
	}
	for _, row := range snapshot.NPCTemplates {
		npcs[string(row.ContentID)] = struct{}{}
		var payload struct {
			NPCType string `json:"npc_type"`
		}
		if err := json.Unmarshal(row.DataJSON, &payload); err != nil {
			t.Fatalf("Unmarshal npc template row %q error = %v", row.ContentID, err)
		}
		if payload.NPCType != "" {
			npcs[payload.NPCType] = struct{}{}
		}
	}

	return content.QuestReferenceResolver{
		HasTemplate: func(id catalog.DefinitionID) bool {
			_, ok := templates[id]
			return ok
		},
		HasItem: func(id foundation.ItemID) bool {
			_, ok := items[id]
			return ok
		},
		HasShip: func(id foundation.ShipID) bool {
			_, ok := ships[id]
			return ok
		},
		HasRecipe: func(id catalog.DefinitionID) bool {
			_, ok := recipes[id]
			return ok
		},
		HasProduction: func(id catalog.DefinitionID) bool {
			_, ok := productionRows[id]
			return ok
		},
		HasBuilding: func(id string) bool {
			_, ok := buildings[id]
			return ok
		},
		HasNPC: func(id string) bool {
			_, ok := npcs[id]
			return ok
		},
	}
}
