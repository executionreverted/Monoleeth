package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidContentSnapshot = errors.New("invalid content snapshot")
	ErrDuplicateContentID     = errors.New("duplicate content id")
	ErrInvalidContentJSON     = errors.New("invalid content json")
	ErrForbiddenContentField  = errors.New("forbidden content field")
)

type Snapshot struct {
	Version             string        `json:"version"`
	Items               []SnapshotRow `json:"items"`
	Modules             []SnapshotRow `json:"modules"`
	Ships               []SnapshotRow `json:"ships"`
	ShopProducts        []SnapshotRow `json:"shop_products"`
	NPCTemplates        []SnapshotRow `json:"npc_templates"`
	SpawnAreas          []SnapshotRow `json:"spawn_areas"`
	EnemyPools          []SnapshotRow `json:"enemy_pools"`
	NPCDropProfiles     []SnapshotRow `json:"npc_drop_profiles"`
	NPCAggroProfiles    []SnapshotRow `json:"npc_aggro_profiles"`
	NPCLeashProfiles    []SnapshotRow `json:"npc_leash_profiles"`
	NPCEventSpawns      []SnapshotRow `json:"npc_event_spawns"`
	LootTables          []SnapshotRow `json:"loot_tables"`
	CraftRecipes        []SnapshotRow `json:"craft_recipes"`
	ProductionBuildings []SnapshotRow `json:"production_buildings"`
	QuestTemplates      []SnapshotRow `json:"quest_templates"`
	QuestRewardTables   []SnapshotRow `json:"quest_reward_tables"`
}

type SnapshotRow struct {
	ContentID   ContentID       `json:"content_id"`
	Enabled     bool            `json:"enabled"`
	DisplayJSON json.RawMessage `json:"display_json,omitempty"`
	DataJSON    json.RawMessage `json:"data_json"`
}

type SnapshotGroup struct {
	Type ContentType
	Rows []SnapshotRow
}

func (snapshot Snapshot) Validate() error {
	if err := ValidateContentID("content version", snapshot.Version); err != nil {
		return err
	}
	for _, group := range snapshot.Groups() {
		if err := validateSnapshotRows(group.Type, group.Rows); err != nil {
			return err
		}
	}
	return nil
}

func (snapshot Snapshot) Groups() []SnapshotGroup {
	return []SnapshotGroup{
		{Type: ContentTypeItem, Rows: snapshot.Items},
		{Type: ContentTypeModule, Rows: snapshot.Modules},
		{Type: ContentTypeShip, Rows: snapshot.Ships},
		{Type: ContentTypeShopProduct, Rows: snapshot.ShopProducts},
		{Type: ContentTypeNPCTemplate, Rows: snapshot.NPCTemplates},
		{Type: ContentTypeSpawnArea, Rows: snapshot.SpawnAreas},
		{Type: ContentTypeEnemyPool, Rows: snapshot.EnemyPools},
		{Type: ContentTypeNPCDropProfile, Rows: snapshot.NPCDropProfiles},
		{Type: ContentTypeNPCAggroProfile, Rows: snapshot.NPCAggroProfiles},
		{Type: ContentTypeNPCLeashProfile, Rows: snapshot.NPCLeashProfiles},
		{Type: ContentTypeNPCEventSpawn, Rows: snapshot.NPCEventSpawns},
		{Type: ContentTypeLootTable, Rows: snapshot.LootTables},
		{Type: ContentTypeCraftRecipe, Rows: snapshot.CraftRecipes},
		{Type: ContentTypeProductionBuilding, Rows: snapshot.ProductionBuildings},
		{Type: ContentTypeQuestTemplate, Rows: snapshot.QuestTemplates},
		{Type: ContentTypeQuestRewardTable, Rows: snapshot.QuestRewardTables},
	}
}

func validateSnapshotRows(contentType ContentType, rows []SnapshotRow) error {
	seen := make(map[ContentID]struct{}, len(rows))
	for index, row := range rows {
		path := fmt.Sprintf("%s[%d]", contentType, index)
		if err := ValidateContentID(path, string(row.ContentID)); err != nil {
			return err
		}
		if _, ok := seen[row.ContentID]; ok {
			return fmt.Errorf("%s content_id %q: %w", path, row.ContentID, ErrDuplicateContentID)
		}
		seen[row.ContentID] = struct{}{}
		if err := validateRawJSON(path+".data_json", row.DataJSON, true); err != nil {
			return err
		}
		if err := validateRawJSON(path+".display_json", row.DisplayJSON, false); err != nil {
			return err
		}
	}
	return nil
}

func validateRawJSON(path string, raw json.RawMessage, required bool) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		if required {
			return fmt.Errorf("%s: %w", path, ErrInvalidContentJSON)
		}
		return nil
	}
	if !json.Valid(trimmed) {
		return fmt.Errorf("%s: %w", path, ErrInvalidContentJSON)
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return fmt.Errorf("%s: %w", path, ErrInvalidContentJSON)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: %w", path, ErrInvalidContentJSON)
	}
	if err := rejectForbiddenContentFields(path, object); err != nil {
		return err
	}
	return nil
}

func rejectForbiddenContentFields(path string, object map[string]any) error {
	for key, value := range object {
		lowerKey := strings.ToLower(key)
		if forbiddenSnapshotFields[lowerKey] {
			return fmt.Errorf("%s.%s: %w", path, key, ErrForbiddenContentField)
		}
		nextPath := path + "." + key
		switch typed := value.(type) {
		case map[string]any:
			if err := rejectForbiddenContentFields(nextPath, typed); err != nil {
				return err
			}
		case []any:
			if err := rejectForbiddenContentFieldsInList(nextPath, typed); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectForbiddenContentFieldsInList(path string, values []any) error {
	for index, value := range values {
		nextPath := fmt.Sprintf("%s[%d]", path, index)
		switch typed := value.(type) {
		case map[string]any:
			if err := rejectForbiddenContentFields(nextPath, typed); err != nil {
				return err
			}
		case []any:
			if err := rejectForbiddenContentFieldsInList(nextPath, typed); err != nil {
				return err
			}
		}
	}
	return nil
}

var forbiddenSnapshotFields = map[string]bool{
	"eval":       true,
	"expression": true,
	"formula":    true,
	"script":     true,
}
