package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
)

// LootTableSnapshotData is the strict CMS DTO for server-owned loot tables.
// Rows use independent chance rolls; legacy weighted row shapes are rejected by
// DecodeLootTableSnapshotData through json.Decoder.DisallowUnknownFields.
type LootTableSnapshotData struct {
	Source catalog.VersionedDefinition `json:"Source"`
	Rows   []LootRowSnapshotData       `json:"Rows"`
}

type LootRowSnapshotData struct {
	ItemDefinition LootItemSnapshotReference `json:"ItemDefinition"`
	MinQuantity    int64                     `json:"MinQuantity"`
	MaxQuantity    int64                     `json:"MaxQuantity"`
	Chance         float64                   `json:"Chance"`
}

type LootItemSnapshotReference struct {
	ItemID foundation.ItemID `json:"item_id"`
}

func SnapshotDataForLootTable(table loot.LootTable) LootTableSnapshotData {
	rows := make([]LootRowSnapshotData, 0, len(table.Rows))
	for _, row := range table.Rows {
		rows = append(rows, LootRowSnapshotData{
			ItemDefinition: LootItemSnapshotReference{ItemID: row.ItemDefinition.ItemID},
			MinQuantity:    row.MinQuantity,
			MaxQuantity:    row.MaxQuantity,
			Chance:         row.Chance,
		})
	}
	return LootTableSnapshotData{
		Source: table.Source,
		Rows:   rows,
	}
}

func DecodeLootTableSnapshotData(raw json.RawMessage) (LootTableSnapshotData, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return LootTableSnapshotData{}, ErrInvalidContentJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var data LootTableSnapshotData
	if err := decoder.Decode(&data); err != nil {
		return LootTableSnapshotData{}, err
	}
	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = ErrInvalidContentJSON
		}
		return LootTableSnapshotData{}, fmt.Errorf("multiple json values: %w", err)
	}
	return data, nil
}
