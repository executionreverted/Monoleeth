package quests

import (
	"encoding/json"
	"fmt"
	"sort"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// QuestCatalogVersion identifies the first static quest template catalog.
const QuestCatalogVersion catalog.Version = "quest_catalog_mvp_v1"

// QuestCatalog indexes immutable static quest templates by template id.
type QuestCatalog struct {
	templates    []QuestTemplate
	byTemplateID map[catalog.DefinitionID]QuestTemplate
}

// NewQuestCatalog validates, clones, sorts, and indexes quest templates.
func NewQuestCatalog(templates []QuestTemplate) (QuestCatalog, error) {
	if len(templates) == 0 {
		return QuestCatalog{}, ErrEmptyQuestCatalog
	}

	cloned := make([]QuestTemplate, 0, len(templates))
	byTemplateID := make(map[catalog.DefinitionID]QuestTemplate, len(templates))
	for _, template := range templates {
		if err := template.Validate(); err != nil {
			return QuestCatalog{}, err
		}
		if _, ok := byTemplateID[template.TemplateID]; ok {
			return QuestCatalog{}, fmt.Errorf("quest template %q: %w", template.TemplateID, ErrDuplicateQuestTemplate)
		}
		clonedTemplate := cloneQuestTemplate(template)
		cloned = append(cloned, clonedTemplate)
		byTemplateID[clonedTemplate.TemplateID] = clonedTemplate
	}
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].TemplateID < cloned[j].TemplateID
	})
	byTemplateID = make(map[catalog.DefinitionID]QuestTemplate, len(cloned))
	for _, template := range cloned {
		byTemplateID[template.TemplateID] = template
	}

	return QuestCatalog{
		templates:    cloned,
		byTemplateID: byTemplateID,
	}, nil
}

// MVPQuestCatalog returns the validated Phase 07 MVP quest template catalog.
func MVPQuestCatalog() (QuestCatalog, error) {
	return NewQuestCatalog(MVPQuestTemplates())
}

// MustMVPQuestCatalog returns the validated MVP quest catalog or panics if the
// checked-in catalog data is invalid.
func MustMVPQuestCatalog() QuestCatalog {
	questCatalog, err := MVPQuestCatalog()
	if err != nil {
		panic(err)
	}
	return questCatalog
}

// Validate reports whether the catalog has at least one valid indexed template.
func (questCatalog QuestCatalog) Validate() error {
	if len(questCatalog.templates) == 0 || len(questCatalog.byTemplateID) == 0 {
		return ErrEmptyQuestCatalog
	}
	for _, template := range questCatalog.templates {
		if err := template.Validate(); err != nil {
			return err
		}
		indexed, ok := questCatalog.byTemplateID[template.TemplateID]
		if !ok || indexed.TemplateID != template.TemplateID {
			return fmt.Errorf("quest template %q: %w", template.TemplateID, ErrUnknownQuestTemplate)
		}
	}
	return nil
}

// Templates returns all templates in deterministic catalog order.
func (questCatalog QuestCatalog) Templates() []QuestTemplate {
	templates := make([]QuestTemplate, 0, len(questCatalog.templates))
	for _, template := range questCatalog.templates {
		templates = append(templates, cloneQuestTemplate(template))
	}
	return templates
}

// Lookup returns one template by id.
func (questCatalog QuestCatalog) Lookup(templateID catalog.DefinitionID) (QuestTemplate, bool) {
	template, ok := questCatalog.byTemplateID[templateID]
	if !ok {
		return QuestTemplate{}, false
	}
	return cloneQuestTemplate(template), true
}

// EligibleTemplates returns templates whose rank and role requirements are met
// by the player snapshot. Filtering is done before board selection.
func (questCatalog QuestCatalog) EligibleTemplates(snapshot PlayerQuestBoardSnapshot) ([]QuestTemplate, error) {
	if err := questCatalog.Validate(); err != nil {
		return nil, err
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}

	eligible := make([]QuestTemplate, 0, len(questCatalog.templates))
	for _, template := range questCatalog.templates {
		if snapshot.MeetsRequirements(template.Requirements) {
			eligible = append(eligible, cloneQuestTemplate(template))
		}
	}
	return eligible, nil
}

// MVPQuestTemplates returns the initial board template set. It intentionally
// has more than ten rank-1 eligible templates so selection can vary by seed
// while still covering every MVP quest type.
func MVPQuestTemplates() []QuestTemplate {
	return []QuestTemplate{
		newMVPQuestTemplate(
			"quest_build_extractor_r1",
			QuestTypeBuild,
			"quest.build_extractor.title",
			"quest.build_extractor.description",
			buildObjective("build_1", "iron_extractor", 1),
			nil,
		),
		newMVPQuestTemplate(
			"quest_build_storage_r1",
			QuestTypeBuild,
			"quest.build_storage.title",
			"quest.build_storage.description",
			buildObjective("build_1", "alloy_foundry", 1),
			nil,
		),
		newMVPQuestTemplate(
			"quest_collect_carbon_shards_r1",
			QuestTypeCollect,
			"quest.collect_carbon_shards.title",
			"quest.collect_carbon_shards.description",
			collectObjective("collect_1", "carbon_shards", 18),
			nil,
		),
		newMVPQuestTemplate(
			"quest_collect_iron_ore_r1",
			QuestTypeCollect,
			"quest.collect_iron_ore.title",
			"quest.collect_iron_ore.description",
			collectObjective("collect_1", "iron_ore", 25),
			nil,
		),
		newMVPQuestTemplate(
			"quest_craft_energy_cells_r1",
			QuestTypeCraft,
			"quest.craft_energy_cells.title",
			"quest.craft_energy_cells.description",
			craftObjective("craft_1", "refined_alloy_batch", "", 1),
			nil,
		),
		newMVPQuestTemplate(
			"quest_craft_refined_alloy_r1",
			QuestTypeCraft,
			"quest.craft_refined_alloy.title",
			"quest.craft_refined_alloy.description",
			craftObjective("craft_1", "refined_alloy_batch", "", 1),
			nil,
		),
		newMVPQuestTemplate(
			"quest_deliver_energy_cells_r1",
			QuestTypeDeliver,
			"quest.deliver_energy_cells.title",
			"quest.deliver_energy_cells.description",
			deliverObjective("deliver_1", "energy_cell", 8, DeliveryTargetStation, "station_frontier"),
			nil,
		),
		newMVPQuestTemplate(
			"quest_deliver_iron_ore_r1",
			QuestTypeDeliver,
			"quest.deliver_iron_ore.title",
			"quest.deliver_iron_ore.description",
			deliverObjective("deliver_1", "iron_ore", 20, DeliveryTargetStation, "station_frontier"),
			nil,
		),
		newMVPQuestTemplate(
			"quest_kill_pirates_r1",
			QuestTypeKill,
			"quest.kill_pirates.title",
			"quest.kill_pirates.description",
			killObjective("kill_1", "training_drone", 5),
			nil,
		),
		newMVPQuestTemplate(
			"quest_kill_raiders_r1",
			QuestTypeKill,
			"quest.kill_raiders.title",
			"quest.kill_raiders.description",
			killObjective("kill_1", "border_raider_drone", 4),
			nil,
		),
		newMVPQuestTemplate(
			"quest_scan_planets_r1",
			QuestTypeScan,
			"quest.scan_planets.title",
			"quest.scan_planets.description",
			scanObjective("scan_1", ScanTargetPlanet, 1),
			nil,
		),
		newMVPQuestTemplate(
			"quest_scan_signals_r1",
			QuestTypeScan,
			"quest.scan_signals.title",
			"quest.scan_signals.description",
			scanObjective("scan_1", ScanTargetSignal, 3),
			nil,
		),
		newMVPQuestTemplate(
			"quest_craft_laser_alpha_r2",
			QuestTypeCraft,
			"quest.craft_laser_alpha.title",
			"quest.craft_laser_alpha.description",
			craftObjective("craft_1", "laser_alpha_t1", "", 1),
			[]QuestRequirement{{MinRank: 2, Role: progression.RoleTypeCrafting, RoleLevel: 2}},
		),
		newMVPQuestTemplate(
			"quest_kill_void_raiders_r3",
			QuestTypeKill,
			"quest.kill_void_raiders.title",
			"quest.kill_void_raiders.description",
			killObjective("kill_1", "outer_ring_scout_drone", 8),
			[]QuestRequirement{{MinRank: 3, Role: progression.RoleTypeCombat, RoleLevel: 2}},
		),
	}
}

func newMVPQuestTemplate(
	templateID catalog.DefinitionID,
	questType QuestType,
	titleKey string,
	descriptionKey string,
	objective ObjectiveSchema,
	requirements []QuestRequirement,
) QuestTemplate {
	return QuestTemplate{
		Source:          mustQuestSource(templateID),
		TemplateID:      templateID,
		Type:            questType,
		TitleKey:        titleKey,
		DescriptionKey:  descriptionKey,
		ObjectiveSchema: objective,
		Requirements:    append([]QuestRequirement(nil), requirements...),
	}
}

func killObjective(id, npcType string, requiredCount int64) ObjectiveSchema {
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindKill,
		Kill: &KillObjective{
			TargetNPCType: npcType,
			RequiredCount: mustQuestQuantity(requiredCount),
		},
	}}}
}

func collectObjective(id string, itemID foundation.ItemID, quantity int64) ObjectiveSchema {
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindCollect,
		Collect: &CollectObjective{
			ItemID:   itemID,
			Quantity: mustQuestQuantity(quantity),
		},
	}}}
}

func craftObjective(id string, recipeID catalog.DefinitionID, itemID foundation.ItemID, quantity int64) ObjectiveSchema {
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindCraft,
		Craft: &CraftObjective{
			RecipeID: recipeID,
			ItemID:   itemID,
			Quantity: mustQuestQuantity(quantity),
		},
	}}}
}

func scanObjective(id string, targetKind ScanTargetKind, requiredCount int64) ObjectiveSchema {
	targetSignalType := "planet_signal"
	if targetKind == ScanTargetPlanet {
		targetSignalType = "planet"
	}
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindScan,
		Scan: &ScanObjective{
			TargetSignalType: targetSignalType,
			RequiredCount:    mustQuestQuantity(requiredCount),
		},
	}}}
}

func buildObjective(id, buildingType string, requiredCount int64) ObjectiveSchema {
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindBuild,
		Build: &BuildObjective{
			BuildingType:  buildingType,
			RequiredCount: mustQuestQuantity(requiredCount),
		},
	}}}
}

func deliverObjective(
	id string,
	itemID foundation.ItemID,
	quantity int64,
	destinationKind DeliveryTargetKind,
	destinationID string,
) ObjectiveSchema {
	return ObjectiveSchema{Objectives: []Objective{{
		ID:   id,
		Kind: ObjectiveKindDeliver,
		Deliver: &DeliverObjective{
			ItemID:          itemID,
			Quantity:        mustQuestQuantity(quantity),
			DestinationType: destinationKind.String(),
			DestinationID:   destinationID,
		},
	}}}
}

func mustQuestSource(templateID catalog.DefinitionID) catalog.VersionedDefinition {
	source, err := catalog.NewQuestSource(templateID.String(), QuestCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	return source
}

func mustQuestQuantity(amount int64) foundation.Quantity {
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		panic(err)
	}
	return quantity
}

func cloneQuestTemplate(template QuestTemplate) QuestTemplate {
	template.DifficultyRules = cloneRawMessage(template.DifficultyRules)
	template.ObjectiveSchema = cloneObjectiveSchema(template.ObjectiveSchema)
	template.RewardRules = cloneRawMessage(template.RewardRules)
	template.RewardPayload = cloneRewardPayloadPtr(template.RewardPayload)
	template.ExpirationRules = cloneRawMessage(template.ExpirationRules)
	template.Requirements = append([]QuestRequirement(nil), template.Requirements...)
	return template
}

func cloneRewardPayloadPtr(payload *RewardPayload) *RewardPayload {
	if payload == nil {
		return nil
	}
	cloned := cloneRewardPayload(*payload)
	return &cloned
}

func cloneObjectiveSchema(schema ObjectiveSchema) ObjectiveSchema {
	schema.Objectives = append([]Objective(nil), schema.Objectives...)
	for index := range schema.Objectives {
		schema.Objectives[index] = cloneObjective(schema.Objectives[index])
	}
	if schema.Kill != nil {
		value := *schema.Kill
		schema.Kill = &value
	}
	if schema.Collect != nil {
		value := *schema.Collect
		schema.Collect = &value
	}
	if schema.Craft != nil {
		value := *schema.Craft
		schema.Craft = &value
	}
	if schema.Scan != nil {
		value := *schema.Scan
		schema.Scan = &value
	}
	if schema.Build != nil {
		value := *schema.Build
		schema.Build = &value
	}
	if schema.Deliver != nil {
		value := *schema.Deliver
		schema.Deliver = &value
	}
	return schema
}

func cloneObjective(objective Objective) Objective {
	if objective.Kill != nil {
		value := *objective.Kill
		objective.Kill = &value
	}
	if objective.Collect != nil {
		value := *objective.Collect
		objective.Collect = &value
	}
	if objective.Craft != nil {
		value := *objective.Craft
		objective.Craft = &value
	}
	if objective.Scan != nil {
		value := *objective.Scan
		objective.Scan = &value
	}
	if objective.Build != nil {
		value := *objective.Build
		objective.Build = &value
	}
	if objective.Deliver != nil {
		value := *objective.Deliver
		objective.Deliver = &value
	}
	return objective
}

func cloneRawMessage(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), payload...)
}

func (questCatalog QuestCatalog) fingerprint() string {
	payload, err := json.Marshal(questCatalog.templates)
	if err != nil {
		panic(err)
	}
	return stableHex(payload, 16)
}
