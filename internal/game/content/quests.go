package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/quests"
)

var (
	ErrInvalidQuestContent       = errors.New("invalid quest content")
	ErrInvalidQuestRewardTable   = errors.New("invalid quest reward table")
	ErrDuplicateQuestRewardTable = errors.New("duplicate quest reward table")
	ErrUnknownContentRecipe      = errors.New("unknown content recipe")
	ErrUnknownContentNPC         = errors.New("unknown content npc")
	ErrUnknownContentBuilding    = errors.New("unknown content building")
	ErrInvalidQuestRowConversion = errors.New("invalid quest row conversion")
)

// QuestReferenceResolver lets CMS validation prove quest content references
// current authoritative catalogs without importing those catalogs here.
type QuestReferenceResolver struct {
	HasTemplate   func(catalog.DefinitionID) bool
	HasItem       func(foundation.ItemID) bool
	HasShip       func(foundation.ShipID) bool
	HasRecipe     func(catalog.DefinitionID) bool
	HasProduction func(catalog.DefinitionID) bool
	HasBuilding   func(string) bool
	HasNPC        func(string) bool
}

// QuestTemplateRow is the CMS JSON row for one quest template.
type QuestTemplateRow struct {
	Source           catalog.VersionedDefinition `json:"source"`
	TemplateID       catalog.DefinitionID        `json:"template_id"`
	Type             quests.QuestType            `json:"quest_type"`
	TitleKey         string                      `json:"title_key"`
	DescriptionKey   string                      `json:"description_key"`
	DifficultyRules  json.RawMessage             `json:"difficulty_rules_json,omitempty"`
	ObjectiveSchema  QuestObjectiveSchemaRow     `json:"objective_schema_json"`
	RewardRules      json.RawMessage             `json:"reward_rules_json,omitempty"`
	ExpirationRules  json.RawMessage             `json:"expiration_rules_json,omitempty"`
	Requirements     []quests.QuestRequirement   `json:"requirements_json,omitempty"`
	BoardWeight      int                         `json:"board_weight,omitempty"`
	BoardProbability float64                     `json:"board_probability,omitempty"`
}

// QuestRewardTableRow is the CMS JSON row for one template reward table.
type QuestRewardTableRow struct {
	Source        catalog.VersionedDefinition `json:"source"`
	RewardTableID catalog.DefinitionID        `json:"reward_table_id"`
	TemplateID    catalog.DefinitionID        `json:"template_id"`
	RewardPayload quests.RewardPayload        `json:"reward_payload_json"`
	Weight        int                         `json:"weight,omitempty"`
	Probability   float64                     `json:"probability,omitempty"`
}

// ValidateQuestContentRows validates enabled quest template and reward rows
// together so reward tables can reference templates from the same snapshot.
func ValidateQuestContentRows(templateRows []SnapshotRow, rewardRows []SnapshotRow, resolver QuestReferenceResolver) error {
	templates, err := validateQuestTemplateRows(templateRows, resolver)
	if err != nil {
		return err
	}
	baseHasTemplate := resolver.HasTemplate
	resolver.HasTemplate = func(templateID catalog.DefinitionID) bool {
		if _, ok := templates[templateID]; ok {
			return true
		}
		return baseHasTemplate != nil && baseHasTemplate(templateID)
	}
	return ValidateQuestRewardTableRows(rewardRows, resolver)
}

// ValidateQuestTemplateRows validates enabled CMS quest template rows.
func ValidateQuestTemplateRows(rows []SnapshotRow, resolver QuestReferenceResolver) error {
	_, err := validateQuestTemplateRows(rows, resolver)
	return err
}

// ValidateQuestRewardTableRows validates enabled CMS quest reward table rows.
func ValidateQuestRewardTableRows(rows []SnapshotRow, resolver QuestReferenceResolver) error {
	if err := validateSnapshotRows(ContentTypeQuestRewardTable, rows); err != nil {
		return err
	}
	seenTables := make(map[catalog.DefinitionID]struct{}, len(rows))
	for index, row := range rows {
		if !row.Enabled {
			continue
		}
		var data QuestRewardTableRow
		if err := decodeQuestRowData(ContentTypeQuestRewardTable, row, &data); err != nil {
			return err
		}
		if err := validateQuestRewardTableRow(row, data, resolver); err != nil {
			return fmt.Errorf("quest reward table[%d] %q: %w", index, row.ContentID, err)
		}
		if _, ok := seenTables[data.RewardTableID]; ok {
			return fmt.Errorf("quest reward table %q: %w", data.RewardTableID, ErrDuplicateQuestRewardTable)
		}
		seenTables[data.RewardTableID] = struct{}{}
	}
	return nil
}

// Template converts a CMS row into the runtime quest domain template.
func (row QuestTemplateRow) Template() (quests.QuestTemplate, error) {
	objectiveSchema, err := row.ObjectiveSchema.ObjectiveSchema()
	if err != nil {
		return quests.QuestTemplate{}, err
	}
	return quests.QuestTemplate{
		Source:          row.Source,
		TemplateID:      row.TemplateID,
		Type:            row.Type,
		TitleKey:        row.TitleKey,
		DescriptionKey:  row.DescriptionKey,
		DifficultyRules: cloneRawMessage(row.DifficultyRules),
		ObjectiveSchema: objectiveSchema,
		RewardRules:     cloneRawMessage(row.RewardRules),
		ExpirationRules: cloneRawMessage(row.ExpirationRules),
		Requirements:    append([]quests.QuestRequirement(nil), row.Requirements...),
	}, nil
}

func validateQuestTemplateRows(rows []SnapshotRow, resolver QuestReferenceResolver) (map[catalog.DefinitionID]struct{}, error) {
	if err := validateSnapshotRows(ContentTypeQuestTemplate, rows); err != nil {
		return nil, err
	}
	seen := make(map[catalog.DefinitionID]struct{}, len(rows))
	for index, row := range rows {
		if !row.Enabled {
			continue
		}
		var data QuestTemplateRow
		if err := decodeQuestRowData(ContentTypeQuestTemplate, row, &data); err != nil {
			return nil, err
		}
		if err := validateQuestTemplateRow(row, data, resolver); err != nil {
			return nil, fmt.Errorf("quest template[%d] %q: %w", index, row.ContentID, err)
		}
		if _, ok := seen[data.TemplateID]; ok {
			return nil, fmt.Errorf("quest template %q: %w", data.TemplateID, quests.ErrDuplicateQuestTemplate)
		}
		seen[data.TemplateID] = struct{}{}
	}
	return seen, nil
}

func validateQuestTemplateRow(row SnapshotRow, data QuestTemplateRow, resolver QuestReferenceResolver) error {
	if err := validateQuestDefinitionID("quest template", data.TemplateID); err != nil {
		return err
	}
	if string(row.ContentID) != data.TemplateID.String() {
		return fmt.Errorf("row %q template %q: %w", row.ContentID, data.TemplateID, ErrInvalidContentSnapshot)
	}
	if err := validateQuestRowSelection("quest template", data.BoardWeight, data.BoardProbability); err != nil {
		return err
	}
	template, err := data.Template()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidQuestRowConversion, err)
	}
	if err := template.Validate(); err != nil {
		return err
	}
	return validateQuestObjectiveReferences("quest template "+data.TemplateID.String(), template.ObjectiveSchema, resolver)
}

func validateQuestRewardTableRow(row SnapshotRow, data QuestRewardTableRow, resolver QuestReferenceResolver) error {
	if err := validateQuestDefinitionID("quest reward table", data.RewardTableID); err != nil {
		return err
	}
	if string(row.ContentID) != data.RewardTableID.String() {
		return fmt.Errorf("row %q reward table %q: %w", row.ContentID, data.RewardTableID, ErrInvalidContentSnapshot)
	}
	if err := data.Source.Validate(); err != nil {
		return err
	}
	if data.Source.DefinitionID != data.RewardTableID {
		return fmt.Errorf("source %q reward table %q: %w", data.Source.DefinitionID, data.RewardTableID, ErrInvalidQuestRewardTable)
	}
	if err := validateQuestDefinitionID("quest reward table template", data.TemplateID); err != nil {
		return err
	}
	if resolver.HasTemplate != nil && !resolver.HasTemplate(data.TemplateID) {
		return fmt.Errorf("template %q: %w", data.TemplateID, quests.ErrUnknownQuestTemplate)
	}
	if err := validateQuestRowSelection("quest reward table", data.Weight, data.Probability); err != nil {
		return err
	}
	if err := data.RewardPayload.Validate(); err != nil {
		return err
	}
	return validateQuestRewardReferences("quest reward table "+data.RewardTableID.String(), data.RewardPayload, resolver)
}

func validateQuestObjectiveReferences(label string, schema quests.ObjectiveSchema, resolver QuestReferenceResolver) error {
	for _, objective := range schema.Objectives {
		if err := validateQuestObjectiveReference(label+" objective "+objective.ID, objective, resolver); err != nil {
			return err
		}
	}
	if schema.Kill != nil {
		if err := requireKnownQuestNPC(label+" kill", schema.Kill.NPCType, resolver); err != nil {
			return err
		}
	}
	if schema.Collect != nil {
		if err := requireKnownQuestItem(label+" collect", schema.Collect.ItemID, resolver); err != nil {
			return err
		}
	}
	if schema.Craft != nil {
		if err := validateQuestCraftReferences(label+" craft", schema.Craft.RecipeID, schema.Craft.ItemID, resolver); err != nil {
			return err
		}
	}
	if schema.Build != nil {
		if err := requireKnownQuestBuilding(label+" build", schema.Build.BuildingID, resolver); err != nil {
			return err
		}
	}
	if schema.Deliver != nil {
		if err := requireKnownQuestItem(label+" deliver", schema.Deliver.ItemID, resolver); err != nil {
			return err
		}
	}
	return nil
}

func validateQuestObjectiveReference(label string, objective quests.Objective, resolver QuestReferenceResolver) error {
	if objective.Kill != nil {
		return requireKnownQuestNPC(label+" kill", objective.Kill.TargetNPCType, resolver)
	}
	if objective.Collect != nil {
		return requireKnownQuestItem(label+" collect", objective.Collect.ItemID, resolver)
	}
	if objective.Craft != nil {
		return validateQuestCraftReferences(label+" craft", objective.Craft.RecipeID, objective.Craft.ItemID, resolver)
	}
	if objective.Build != nil {
		return requireKnownQuestBuilding(label+" build", objective.Build.BuildingType, resolver)
	}
	if objective.Deliver != nil {
		return requireKnownQuestItem(label+" deliver", objective.Deliver.ItemID, resolver)
	}
	return nil
}

func validateQuestCraftReferences(label string, recipeID catalog.DefinitionID, itemID foundation.ItemID, resolver QuestReferenceResolver) error {
	if !recipeID.IsZero() {
		if err := requireKnownQuestRecipe(label, recipeID, resolver); err != nil {
			return err
		}
	}
	if !itemID.IsZero() {
		if err := requireKnownQuestItem(label, itemID, resolver); err != nil {
			return err
		}
	}
	return nil
}

func validateQuestRewardReferences(label string, payload quests.RewardPayload, resolver QuestReferenceResolver) error {
	for index, grant := range payload.Grants {
		if grant.Kind == quests.RewardKindItem {
			if err := requireKnownQuestItem(fmt.Sprintf("%s grant %d", label, index), grant.ItemID, resolver); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireKnownQuestItem(label string, itemID foundation.ItemID, resolver QuestReferenceResolver) error {
	if resolver.HasItem != nil && !resolver.HasItem(itemID) {
		return fmt.Errorf("%s item %q: %w", label, itemID, ErrUnknownContentItem)
	}
	return nil
}

func requireKnownQuestRecipe(label string, recipeID catalog.DefinitionID, resolver QuestReferenceResolver) error {
	if resolver.HasRecipe != nil && !resolver.HasRecipe(recipeID) {
		return fmt.Errorf("%s recipe %q: %w", label, recipeID, ErrUnknownContentRecipe)
	}
	return nil
}

func requireKnownQuestNPC(label string, npcType string, resolver QuestReferenceResolver) error {
	if resolver.HasNPC != nil && !resolver.HasNPC(npcType) {
		return fmt.Errorf("%s npc %q: %w", label, npcType, ErrUnknownContentNPC)
	}
	return nil
}

func requireKnownQuestBuilding(label string, buildingID string, resolver QuestReferenceResolver) error {
	checked := false
	if resolver.HasBuilding != nil {
		checked = true
		if resolver.HasBuilding(buildingID) {
			return nil
		}
	}
	if resolver.HasProduction != nil {
		checked = true
		if resolver.HasProduction(catalog.DefinitionID(buildingID)) {
			return nil
		}
	}
	if checked {
		return fmt.Errorf("%s building %q: %w", label, buildingID, ErrUnknownContentBuilding)
	}
	return nil
}

func validateQuestDefinitionID(kind string, id catalog.DefinitionID) error {
	if err := id.Validate(); err != nil {
		return err
	}
	return ValidateContentID(kind, id.String())
}

func validateQuestRowSelection(kind string, weight int, probability float64) error {
	if weight < 0 {
		return fmt.Errorf("%s weight %d: %w", kind, weight, ErrInvalidQuestContent)
	}
	if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
		return fmt.Errorf("%s probability %v: %w", kind, probability, ErrInvalidQuestContent)
	}
	return nil
}

func decodeQuestRowData(contentType ContentType, row SnapshotRow, target any) error {
	if err := json.Unmarshal(row.DataJSON, target); err != nil {
		return fmt.Errorf("%s %q data: %w", contentType, row.ContentID, err)
	}
	return nil
}

func cloneRawMessage(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), payload...)
}
