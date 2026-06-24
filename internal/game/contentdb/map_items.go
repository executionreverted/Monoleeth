package contentdb

import (
	"encoding/json"
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

type itemRowData struct {
	Source         catalog.VersionedDefinition `json:"source"`
	ItemID         foundation.ItemID           `json:"item_id"`
	Name           string                      `json:"name"`
	Type           economy.ItemType            `json:"item_type"`
	Rarity         economy.ItemRarity          `json:"rarity"`
	MaxStack       int64                       `json:"max_stack"`
	WeightUnits    int64                       `json:"weight_units"`
	TradeFlags     []economy.TradeFlag         `json:"trade_flags,omitempty"`
	BindRules      []economy.BindRule          `json:"bind_rules,omitempty"`
	MetadataSchema json.RawMessage             `json:"metadata_schema,omitempty"`
}

func mapItemRows(snapshot content.Snapshot) (map[foundation.ItemID]economy.ItemDefinition, error) {
	items := make(map[foundation.ItemID]economy.ItemDefinition, len(snapshot.Items))
	for _, row := range snapshot.Items {
		if !row.Enabled {
			continue
		}
		var data itemRowData
		if err := decodeSnapshotRow(content.ContentTypeItem, row, &data); err != nil {
			return nil, err
		}
		if err := requireRowID(content.ContentTypeItem, row, data.ItemID.String()); err != nil {
			return nil, err
		}
		definition, err := data.toDefinition(publishedVersion(snapshot))
		if err != nil {
			return nil, fmt.Errorf("item %q: %w", row.ContentID, err)
		}
		if _, exists := items[definition.ItemID]; exists {
			return nil, fmt.Errorf("item %q: %w", definition.ItemID, content.ErrDuplicateContentID)
		}
		items[definition.ItemID] = definition
	}
	return items, nil
}

func (data itemRowData) toDefinition(version catalog.Version) (economy.ItemDefinition, error) {
	maxStack, err := foundation.NewQuantity(data.MaxStack)
	if err != nil {
		return economy.ItemDefinition{}, fmt.Errorf("max_stack: %w", err)
	}
	weight, err := foundation.NewQuantity(data.WeightUnits)
	if err != nil {
		return economy.ItemDefinition{}, fmt.Errorf("weight_units: %w", err)
	}
	return economy.NewItemDefinition(
		forceSourceVersion(data.Source, version),
		data.ItemID,
		data.Name,
		data.Type,
		data.Rarity,
		maxStack,
		weight,
		data.TradeFlags,
		data.BindRules,
		data.MetadataSchema,
	)
}
