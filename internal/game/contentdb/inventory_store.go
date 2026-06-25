package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

type InventoryStore struct {
	store *Store
}

var _ economy.InventoryRepository = (*InventoryStore)(nil)

func NewInventoryStore(store *Store) (*InventoryStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &InventoryStore{store: store}, nil
}

func (store *InventoryStore) LoadStackableItems(ctx context.Context) ([]economy.StackableItem, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT item_instance_id, item_id, player_id, location_kind, location_id, quantity, source_definition_id, source_version, metadata_json, created_at, updated_at
		FROM player_inventory_items
		WHERE quantity > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]economy.StackableItem, 0)
	for rows.Next() {
		var itemInstanceID string
		var itemID string
		var playerID string
		var locationKind string
		var locationID string
		var quantityAmount int64
		var sourceDefinitionID string
		var sourceVersion string
		var metadataJSON sql.NullString
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(&itemInstanceID, &itemID, &playerID, &locationKind, &locationID, &quantityAmount, &sourceDefinitionID, &sourceVersion, &metadataJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		quantity, err := foundation.NewQuantity(quantityAmount)
		if err != nil {
			return nil, err
		}
		item, err := economy.NewStackableItem(
			catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID(sourceDefinitionID), Version: catalog.Version(sourceVersion)},
			foundation.ItemID(itemInstanceID),
			foundation.ItemID(itemID),
			foundation.PlayerID(playerID),
			economy.ItemLocation{Kind: economy.LocationKind(locationKind), ID: economy.LocationID(locationID)},
			quantity,
		)
		if err != nil {
			return nil, err
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			item.MetadataJSON = json.RawMessage(metadataJSON.String)
		}
		item.CreatedAt = createdAt.UTC()
		item.UpdatedAt = updatedAt.UTC()
		if err := item.Validate(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OwnerPlayerID != items[j].OwnerPlayerID {
			return items[i].OwnerPlayerID < items[j].OwnerPlayerID
		}
		if items[i].Location != items[j].Location {
			return items[i].Location.String() < items[j].Location.String()
		}
		if items[i].ItemID != items[j].ItemID {
			return items[i].ItemID < items[j].ItemID
		}
		return items[i].ItemInstanceID < items[j].ItemInstanceID
	})
	return items, nil
}

func (store *InventoryStore) UpsertStackableItem(ctx context.Context, item economy.StackableItem) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := item.Validate(); err != nil {
		return err
	}
	_, err := store.store.db.ExecContext(ctx, `
		INSERT INTO player_inventory_items(player_id, location, item_id, quantity, updated_at, item_instance_id, source_definition_id, source_version, location_kind, location_id, metadata_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (player_id, location, item_id) DO UPDATE
		SET quantity = EXCLUDED.quantity,
			updated_at = EXCLUDED.updated_at,
			item_instance_id = EXCLUDED.item_instance_id,
			source_definition_id = EXCLUDED.source_definition_id,
			source_version = EXCLUDED.source_version,
			location_kind = EXCLUDED.location_kind,
			location_id = EXCLUDED.location_id,
			metadata_json = EXCLUDED.metadata_json
	`, item.OwnerPlayerID.String(), item.Location.String(), item.ItemID.String(), item.Quantity.Int64(), item.UpdatedAt.UTC(), item.ItemInstanceID.String(), item.Source.DefinitionID.String(), item.Source.Version.String(), item.Location.Kind.String(), item.Location.ID.String(), nullableRawJSON(item.MetadataJSON), item.CreatedAt.UTC())
	return err
}

func nullableRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}
