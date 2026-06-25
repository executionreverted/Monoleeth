package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

type inventorySQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type inventoryInstanceItemQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type inventoryInstanceItemScanner interface {
	Scan(dest ...any) error
}

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

func (store *InventoryStore) LoadInstanceItems(ctx context.Context) ([]economy.InstanceItem, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT item_instance_id, item_id, player_id, location_kind, location_id, quantity, durability_current, bound_state, source_definition_id, source_version, metadata_json, created_at, updated_at
		FROM player_inventory_instance_items
		WHERE quantity > 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]economy.InstanceItem, 0)
	for rows.Next() {
		item, err := scanInventoryInstanceItem(rows)
		if err != nil {
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

func (store *InventoryStore) LoadInstanceItem(ctx context.Context, itemInstanceID foundation.ItemID) (economy.InstanceItem, bool, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return economy.InstanceItem{}, false, ErrNilDatabase
	}
	return loadInventoryInstanceItem(ctx, store.store.db, itemInstanceID)
}

func (store *InventoryStore) UpsertStackableItem(ctx context.Context, item economy.StackableItem) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := item.Validate(); err != nil {
		return err
	}
	return upsertInventoryStackableItem(ctx, store.store.db, item)
}

func (store *InventoryStore) UpsertInstanceItem(ctx context.Context, item economy.InstanceItem) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := item.Validate(); err != nil {
		return err
	}
	return upsertInventoryInstanceItem(ctx, store.store.db, item)
}

func (store *InventoryStore) LoadItemLedgerEntries(ctx context.Context) ([]economy.ItemLedgerEntry, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT ledger_id, player_id, item_id, item_instance_id, quantity, action, balance_after, location_kind, location_id, reason, reference_key, created_at
		FROM player_inventory_item_ledger
		ORDER BY created_at, ledger_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]economy.ItemLedgerEntry, 0)
	for rows.Next() {
		var ledgerID string
		var playerID string
		var itemID string
		var itemInstanceID string
		var quantityAmount int64
		var action string
		var balanceAfter int64
		var locationKind string
		var locationID string
		var reason string
		var referenceKey string
		var createdAt time.Time
		if err := rows.Scan(&ledgerID, &playerID, &itemID, &itemInstanceID, &quantityAmount, &action, &balanceAfter, &locationKind, &locationID, &reason, &referenceKey, &createdAt); err != nil {
			return nil, err
		}
		quantity, err := foundation.NewQuantity(quantityAmount)
		if err != nil {
			return nil, err
		}
		entry, err := economy.NewItemLedgerEntry(
			economy.LedgerID(ledgerID),
			foundation.PlayerID(playerID),
			foundation.ItemID(itemID),
			foundation.ItemID(itemInstanceID),
			quantity,
			economy.LedgerAction(action),
			balanceAfter,
			economy.ItemLocation{Kind: economy.LocationKind(locationKind), ID: economy.LocationID(locationID)},
			economy.LedgerReason(reason),
			foundation.IdempotencyKey(referenceKey),
		)
		if err != nil {
			return nil, err
		}
		entry.CreatedAt = createdAt.UTC()
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (store *InventoryStore) LoadAddItemReferences(ctx context.Context) ([]economy.AddItemReference, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}

	ledgerEntries, err := store.LoadItemLedgerEntries(ctx)
	if err != nil {
		return nil, err
	}
	ledgerByID := make(map[economy.LedgerID]economy.ItemLedgerEntry, len(ledgerEntries))
	for _, entry := range ledgerEntries {
		ledgerByID[entry.LedgerID] = entry
	}
	stackables, err := store.LoadStackableItems(ctx)
	if err != nil {
		return nil, err
	}
	stackableByID := make(map[foundation.ItemID]economy.StackableItem, len(stackables))
	for _, item := range stackables {
		stackableByID[item.ItemInstanceID] = item
	}
	instances, err := store.LoadInstanceItems(ctx)
	if err != nil {
		return nil, err
	}
	instanceByID := make(map[foundation.ItemID]economy.InstanceItem, len(instances))
	for _, item := range instances {
		instanceByID[item.ItemInstanceID] = item
	}

	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, reference_key, ledger_id, item_instance_ids
		FROM player_inventory_add_item_references
		ORDER BY player_id, reference_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	references := make([]economy.AddItemReference, 0)
	for rows.Next() {
		var playerID string
		var referenceKey string
		var ledgerID string
		var itemInstanceIDsJSON []byte
		if err := rows.Scan(&playerID, &referenceKey, &ledgerID, &itemInstanceIDsJSON); err != nil {
			return nil, err
		}
		ledgerEntry, ok := ledgerByID[economy.LedgerID(ledgerID)]
		if !ok {
			return nil, economy.ErrEmptyLedgerID
		}
		itemInstanceIDs, err := parseInventoryItemInstanceIDs(itemInstanceIDsJSON)
		if err != nil {
			return nil, err
		}
		result := economy.AddItemResult{LedgerEntry: ledgerEntry}
		for _, itemInstanceID := range itemInstanceIDs {
			if item, ok := stackableByID[itemInstanceID]; ok {
				result.StackableItems = append(result.StackableItems, item)
			}
			if item, ok := instanceByID[itemInstanceID]; ok {
				result.InstanceItems = append(result.InstanceItems, item)
			}
		}
		reference := economy.AddItemReference{
			PlayerID:     foundation.PlayerID(playerID),
			ReferenceKey: foundation.IdempotencyKey(referenceKey),
			Result:       result,
		}
		if err := reference.Validate(); err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return references, nil
}

func (store *InventoryStore) LoadMoveItemReferences(ctx context.Context) ([]economy.MoveItemReference, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, reference_key, result_json
		FROM player_inventory_move_item_references
		ORDER BY player_id, reference_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	references := make([]economy.MoveItemReference, 0)
	for rows.Next() {
		var playerID string
		var referenceKey string
		var resultJSON []byte
		if err := rows.Scan(&playerID, &referenceKey, &resultJSON); err != nil {
			return nil, err
		}
		result, err := unmarshalInventoryMoveItemResult(resultJSON)
		if err != nil {
			return nil, err
		}
		reference := economy.MoveItemReference{
			PlayerID:     foundation.PlayerID(playerID),
			ReferenceKey: foundation.IdempotencyKey(referenceKey),
			Result:       result,
		}
		if err := reference.Validate(); err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return references, nil
}

func (store *InventoryStore) LoadRemoveItemReferences(ctx context.Context) ([]economy.RemoveItemReference, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, reference_key, result_json
		FROM player_inventory_remove_item_references
		ORDER BY player_id, reference_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	references := make([]economy.RemoveItemReference, 0)
	for rows.Next() {
		var playerID string
		var referenceKey string
		var resultJSON []byte
		if err := rows.Scan(&playerID, &referenceKey, &resultJSON); err != nil {
			return nil, err
		}
		result, err := unmarshalInventoryRemoveItemResult(resultJSON)
		if err != nil {
			return nil, err
		}
		reference := economy.RemoveItemReference{
			PlayerID:     foundation.PlayerID(playerID),
			ReferenceKey: foundation.IdempotencyKey(referenceKey),
			Result:       result,
		}
		if err := reference.Validate(); err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return references, nil
}

func (store *InventoryStore) LoadInventoryCounters(ctx context.Context) (economy.InventoryCounters, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return economy.InventoryCounters{}, ErrNilDatabase
	}
	var counters economy.InventoryCounters
	err := store.store.db.QueryRowContext(ctx, `
		SELECT item_sequence, ledger_sequence
		FROM player_inventory_counters
		WHERE counter_id = 'inventory'
	`).Scan(&counters.ItemSequence, &counters.LedgerSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return economy.InventoryCounters{}, nil
	}
	if err != nil {
		return economy.InventoryCounters{}, err
	}
	if err := counters.Validate(); err != nil {
		return economy.InventoryCounters{}, err
	}
	return counters, nil
}

func (store *InventoryStore) CommitAddItem(ctx context.Context, commit economy.InventoryAddItemCommit) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := commitInventoryAddItem(ctx, tx, commit); err != nil {
		return err
	}
	return tx.Commit()
}

func commitInventoryAddItem(ctx context.Context, execer inventorySQLExecer, commit economy.InventoryAddItemCommit) error {
	if execer == nil {
		return ErrNilDatabase
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	for _, item := range commit.StackableItems {
		if err := upsertInventoryStackableItem(ctx, execer, item); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := upsertInventoryInstanceItem(ctx, execer, item); err != nil {
			return err
		}
	}
	if err := insertInventoryItemLedgerEntry(ctx, execer, commit.LedgerEntry); err != nil {
		return err
	}
	if err := insertInventoryAddItemReference(ctx, execer, commit.Reference); err != nil {
		return err
	}
	if err := upsertInventoryCounters(ctx, execer, commit.Counters); err != nil {
		return err
	}
	return nil
}

func (store *InventoryStore) CommitMoveItem(ctx context.Context, commit economy.InventoryMoveItemCommit) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := commitInventoryMoveItem(ctx, tx, commit); err != nil {
		return err
	}
	return tx.Commit()
}

func commitInventoryMoveItem(ctx context.Context, execer inventorySQLExecer, commit economy.InventoryMoveItemCommit) error {
	if execer == nil {
		return ErrNilDatabase
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	for _, item := range commit.DeletedStackableItems {
		if err := deleteInventoryStackableItem(ctx, execer, item); err != nil {
			return err
		}
	}
	for _, item := range commit.StackableItems {
		if err := upsertInventoryStackableItem(ctx, execer, item); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := upsertInventoryInstanceItem(ctx, execer, item); err != nil {
			return err
		}
	}
	for _, entry := range commit.LedgerEntries {
		if err := insertInventoryItemLedgerEntry(ctx, execer, entry); err != nil {
			return err
		}
	}
	if err := insertInventoryMoveItemReference(ctx, execer, commit.Reference); err != nil {
		return err
	}
	if err := upsertInventoryCounters(ctx, execer, commit.Counters); err != nil {
		return err
	}
	return nil
}

func (store *InventoryStore) CommitRemoveItem(ctx context.Context, commit economy.InventoryRemoveItemCommit) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range commit.DeletedStackableItems {
		if err := deleteInventoryStackableItem(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range commit.StackableItems {
		if err := upsertInventoryStackableItem(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, item := range commit.DeletedInstanceItems {
		if err := deleteInventoryInstanceItem(ctx, tx, item); err != nil {
			return err
		}
	}
	for _, entry := range commit.LedgerEntries {
		if err := insertInventoryItemLedgerEntry(ctx, tx, entry); err != nil {
			return err
		}
	}
	if err := insertInventoryRemoveItemReference(ctx, tx, commit.Reference); err != nil {
		return err
	}
	if err := upsertInventoryCounters(ctx, tx, commit.Counters); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertInventoryStackableItem(ctx context.Context, execer inventorySQLExecer, item economy.StackableItem) error {
	_, err := execer.ExecContext(ctx, `
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

func upsertInventoryInstanceItem(ctx context.Context, execer inventorySQLExecer, item economy.InstanceItem) error {
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_inventory_instance_items(item_instance_id, player_id, item_id, location, location_kind, location_id, quantity, durability_current, bound_state, source_definition_id, source_version, metadata_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (item_instance_id) DO UPDATE
		SET player_id = EXCLUDED.player_id,
			item_id = EXCLUDED.item_id,
			location = EXCLUDED.location,
			location_kind = EXCLUDED.location_kind,
			location_id = EXCLUDED.location_id,
			quantity = EXCLUDED.quantity,
			durability_current = EXCLUDED.durability_current,
			bound_state = EXCLUDED.bound_state,
			source_definition_id = EXCLUDED.source_definition_id,
			source_version = EXCLUDED.source_version,
			metadata_json = EXCLUDED.metadata_json,
			updated_at = EXCLUDED.updated_at
	`, item.ItemInstanceID.String(), item.OwnerPlayerID.String(), item.ItemID.String(), item.Location.String(), item.Location.Kind.String(), item.Location.ID.String(), item.Quantity.Int64(), item.DurabilityCurrent, item.BoundState.String(), item.Source.DefinitionID.String(), item.Source.Version.String(), nullableRawJSON(item.MetadataJSON), item.CreatedAt.UTC(), item.UpdatedAt.UTC())
	return err
}

func deleteInventoryStackableItem(ctx context.Context, execer inventorySQLExecer, item economy.StackableItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		DELETE FROM player_inventory_items
		WHERE player_id = $1
			AND location = $2
			AND item_id = $3
	`, item.OwnerPlayerID.String(), item.Location.String(), item.ItemID.String())
	return err
}

func deleteInventoryInstanceItem(ctx context.Context, execer inventorySQLExecer, item economy.InstanceItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		DELETE FROM player_inventory_instance_items
		WHERE item_instance_id = $1
	`, item.ItemInstanceID.String())
	return err
}

func loadInventoryInstanceItem(ctx context.Context, querier inventoryInstanceItemQuerier, itemInstanceID foundation.ItemID) (economy.InstanceItem, bool, error) {
	if querier == nil {
		return economy.InstanceItem{}, false, ErrNilDatabase
	}
	if err := itemInstanceID.Validate(); err != nil {
		return economy.InstanceItem{}, false, err
	}
	item, err := scanInventoryInstanceItem(querier.QueryRowContext(ctx, `
		SELECT item_instance_id, item_id, player_id, location_kind, location_id, quantity, durability_current, bound_state, source_definition_id, source_version, metadata_json, created_at, updated_at
		FROM player_inventory_instance_items
		WHERE item_instance_id = $1
			AND quantity > 0
	`, itemInstanceID.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return economy.InstanceItem{}, false, nil
	}
	if err != nil {
		return economy.InstanceItem{}, false, err
	}
	return item, true, nil
}

func scanInventoryInstanceItem(scanner inventoryInstanceItemScanner) (economy.InstanceItem, error) {
	var itemInstanceID string
	var itemID string
	var playerID string
	var locationKind string
	var locationID string
	var quantityAmount int64
	var durabilityCurrent int64
	var boundState string
	var sourceDefinitionID string
	var sourceVersion string
	var metadataJSON sql.NullString
	var createdAt time.Time
	var updatedAt time.Time
	if err := scanner.Scan(&itemInstanceID, &itemID, &playerID, &locationKind, &locationID, &quantityAmount, &durabilityCurrent, &boundState, &sourceDefinitionID, &sourceVersion, &metadataJSON, &createdAt, &updatedAt); err != nil {
		return economy.InstanceItem{}, err
	}
	quantity, err := foundation.NewQuantity(quantityAmount)
	if err != nil {
		return economy.InstanceItem{}, err
	}
	item, err := economy.NewInstanceItem(
		catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID(sourceDefinitionID), Version: catalog.Version(sourceVersion)},
		foundation.ItemID(itemInstanceID),
		foundation.ItemID(itemID),
		foundation.PlayerID(playerID),
		economy.ItemLocation{Kind: economy.LocationKind(locationKind), ID: economy.LocationID(locationID)},
		quantity,
	)
	if err != nil {
		return economy.InstanceItem{}, err
	}
	item.DurabilityCurrent = durabilityCurrent
	item.BoundState = economy.BoundState(boundState)
	if metadataJSON.Valid && metadataJSON.String != "" {
		item.MetadataJSON = json.RawMessage(metadataJSON.String)
	}
	item.CreatedAt = createdAt.UTC()
	item.UpdatedAt = updatedAt.UTC()
	if err := item.Validate(); err != nil {
		return economy.InstanceItem{}, err
	}
	return item, nil
}

func insertInventoryItemLedgerEntry(ctx context.Context, execer inventorySQLExecer, entry economy.ItemLedgerEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_inventory_item_ledger(ledger_id, player_id, item_id, item_instance_id, quantity, action, balance_after, location, location_kind, location_id, reason, reference_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, entry.LedgerID.String(), entry.PlayerID.String(), entry.ItemID.String(), entry.ItemInstanceID.String(), entry.Quantity.Int64(), entry.Action.String(), entry.BalanceAfter, entry.Location.String(), entry.Location.Kind.String(), entry.Location.ID.String(), entry.Reason.String(), entry.ReferenceKey.String(), entry.CreatedAt.UTC())
	return err
}

func insertInventoryAddItemReference(ctx context.Context, execer inventorySQLExecer, reference economy.AddItemReference) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	itemInstanceIDsJSON, err := json.Marshal(addItemReferenceItemInstanceIDs(reference.Result))
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO player_inventory_add_item_references(player_id, reference_key, ledger_id, item_instance_ids, created_at)
		VALUES ($1, $2, $3, $4::jsonb, $5)
	`, reference.PlayerID.String(), reference.ReferenceKey.String(), reference.Result.LedgerEntry.LedgerID.String(), string(itemInstanceIDsJSON), reference.Result.LedgerEntry.CreatedAt.UTC())
	return err
}

func insertInventoryMoveItemReference(ctx context.Context, execer inventorySQLExecer, reference economy.MoveItemReference) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	ledgerIDsJSON, err := json.Marshal(inventoryReferenceLedgerIDs(reference.Result.LedgerEntries))
	if err != nil {
		return err
	}
	resultJSON, err := marshalInventoryMoveItemResult(reference.Result)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO player_inventory_move_item_references(player_id, reference_key, primary_ledger_id, ledger_ids, result_json, created_at)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6)
	`, reference.PlayerID.String(), reference.ReferenceKey.String(), reference.Result.LedgerEntries[0].LedgerID.String(), string(ledgerIDsJSON), string(resultJSON), reference.Result.LedgerEntries[0].CreatedAt.UTC())
	return err
}

func insertInventoryRemoveItemReference(ctx context.Context, execer inventorySQLExecer, reference economy.RemoveItemReference) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	ledgerIDsJSON, err := json.Marshal(inventoryReferenceLedgerIDs(reference.Result.LedgerEntries))
	if err != nil {
		return err
	}
	resultJSON, err := marshalInventoryRemoveItemResult(reference.Result)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO player_inventory_remove_item_references(player_id, reference_key, primary_ledger_id, ledger_ids, result_json, created_at)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6)
	`, reference.PlayerID.String(), reference.ReferenceKey.String(), reference.Result.LedgerEntries[0].LedgerID.String(), string(ledgerIDsJSON), string(resultJSON), reference.Result.LedgerEntries[0].CreatedAt.UTC())
	return err
}

func upsertInventoryCounters(ctx context.Context, execer inventorySQLExecer, counters economy.InventoryCounters) error {
	if err := counters.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_inventory_counters(counter_id, item_sequence, ledger_sequence, updated_at)
		VALUES ('inventory', $1, $2, now())
		ON CONFLICT (counter_id) DO UPDATE
		SET item_sequence = GREATEST(player_inventory_counters.item_sequence, EXCLUDED.item_sequence),
			ledger_sequence = GREATEST(player_inventory_counters.ledger_sequence, EXCLUDED.ledger_sequence),
			updated_at = EXCLUDED.updated_at
	`, counters.ItemSequence, counters.LedgerSequence)
	return err
}

func addItemReferenceItemInstanceIDs(result economy.AddItemResult) []string {
	itemInstanceIDs := make([]string, 0, len(result.StackableItems)+len(result.InstanceItems))
	for _, item := range result.StackableItems {
		itemInstanceIDs = append(itemInstanceIDs, item.ItemInstanceID.String())
	}
	for _, item := range result.InstanceItems {
		itemInstanceIDs = append(itemInstanceIDs, item.ItemInstanceID.String())
	}
	return itemInstanceIDs
}

func parseInventoryItemInstanceIDs(raw []byte) ([]foundation.ItemID, error) {
	var values []string
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	}
	itemInstanceIDs := make([]foundation.ItemID, 0, len(values))
	for _, value := range values {
		itemInstanceID := foundation.ItemID(value)
		if err := itemInstanceID.Validate(); err != nil {
			return nil, err
		}
		itemInstanceIDs = append(itemInstanceIDs, itemInstanceID)
	}
	return itemInstanceIDs, nil
}

func inventoryReferenceLedgerIDs(entries []economy.ItemLedgerEntry) []string {
	ledgerIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		ledgerIDs = append(ledgerIDs, entry.LedgerID.String())
	}
	return ledgerIDs
}

func marshalInventoryMoveItemResult(result economy.MoveItemResult) ([]byte, error) {
	return json.Marshal(inventoryMoveItemResultSnapshot{
		StackableItems:        snapshotInventoryStackableItems(result.StackableItems),
		DeletedStackableItems: snapshotInventoryStackableItems(result.DeletedStackableItems),
		InstanceItems:         snapshotInventoryInstanceItems(result.InstanceItems),
		LedgerEntries:         snapshotInventoryItemLedgerEntries(result.LedgerEntries),
		Duplicate:             result.Duplicate,
	})
}

func unmarshalInventoryMoveItemResult(raw []byte) (economy.MoveItemResult, error) {
	var snapshot inventoryMoveItemResultSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return economy.MoveItemResult{}, err
	}
	stackableItems, err := snapshot.stackableItems()
	if err != nil {
		return economy.MoveItemResult{}, err
	}
	deletedStackableItems, err := snapshot.deletedStackableItems()
	if err != nil {
		return economy.MoveItemResult{}, err
	}
	instanceItems, err := snapshot.instanceItems()
	if err != nil {
		return economy.MoveItemResult{}, err
	}
	ledgerEntries, err := snapshot.ledgerEntries()
	if err != nil {
		return economy.MoveItemResult{}, err
	}
	return economy.MoveItemResult{
		StackableItems:        stackableItems,
		DeletedStackableItems: deletedStackableItems,
		InstanceItems:         instanceItems,
		LedgerEntries:         ledgerEntries,
		Duplicate:             snapshot.Duplicate,
	}, nil
}

func marshalInventoryRemoveItemResult(result economy.RemoveItemResult) ([]byte, error) {
	return json.Marshal(inventoryRemoveItemResultSnapshot{
		StackableItems:        snapshotInventoryStackableItems(result.StackableItems),
		DeletedStackableItems: snapshotInventoryStackableItems(result.DeletedStackableItems),
		InstanceItems:         snapshotInventoryInstanceItems(result.InstanceItems),
		LedgerEntries:         snapshotInventoryItemLedgerEntries(result.LedgerEntries),
		Duplicate:             result.Duplicate,
	})
}

func unmarshalInventoryRemoveItemResult(raw []byte) (economy.RemoveItemResult, error) {
	var snapshot inventoryRemoveItemResultSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return economy.RemoveItemResult{}, err
	}
	stackableItems, err := snapshot.stackableItems()
	if err != nil {
		return economy.RemoveItemResult{}, err
	}
	deletedStackableItems, err := snapshot.deletedStackableItems()
	if err != nil {
		return economy.RemoveItemResult{}, err
	}
	instanceItems, err := snapshot.instanceItems()
	if err != nil {
		return economy.RemoveItemResult{}, err
	}
	ledgerEntries, err := snapshot.ledgerEntries()
	if err != nil {
		return economy.RemoveItemResult{}, err
	}
	return economy.RemoveItemResult{
		StackableItems:        stackableItems,
		DeletedStackableItems: deletedStackableItems,
		InstanceItems:         instanceItems,
		LedgerEntries:         ledgerEntries,
		Duplicate:             snapshot.Duplicate,
	}, nil
}

type inventoryMoveItemResultSnapshot struct {
	StackableItems        []inventoryStackableItemSnapshot `json:"stackable_items,omitempty"`
	DeletedStackableItems []inventoryStackableItemSnapshot `json:"deleted_stackable_items,omitempty"`
	InstanceItems         []inventoryInstanceItemSnapshot  `json:"instance_items,omitempty"`
	LedgerEntries         []inventoryItemLedgerSnapshot    `json:"ledger_entries"`
	Duplicate             bool                             `json:"duplicate,omitempty"`
}

func (snapshot inventoryMoveItemResultSnapshot) stackableItems() ([]economy.StackableItem, error) {
	return restoreInventoryStackableItems(snapshot.StackableItems)
}

func (snapshot inventoryMoveItemResultSnapshot) deletedStackableItems() ([]economy.StackableItem, error) {
	return restoreInventoryStackableItems(snapshot.DeletedStackableItems)
}

func (snapshot inventoryMoveItemResultSnapshot) instanceItems() ([]economy.InstanceItem, error) {
	return restoreInventoryInstanceItems(snapshot.InstanceItems)
}

func (snapshot inventoryMoveItemResultSnapshot) ledgerEntries() ([]economy.ItemLedgerEntry, error) {
	return restoreInventoryItemLedgerEntries(snapshot.LedgerEntries)
}

type inventoryRemoveItemResultSnapshot struct {
	StackableItems        []inventoryStackableItemSnapshot `json:"stackable_items,omitempty"`
	DeletedStackableItems []inventoryStackableItemSnapshot `json:"deleted_stackable_items,omitempty"`
	InstanceItems         []inventoryInstanceItemSnapshot  `json:"instance_items,omitempty"`
	LedgerEntries         []inventoryItemLedgerSnapshot    `json:"ledger_entries"`
	Duplicate             bool                             `json:"duplicate,omitempty"`
}

func (snapshot inventoryRemoveItemResultSnapshot) stackableItems() ([]economy.StackableItem, error) {
	return restoreInventoryStackableItems(snapshot.StackableItems)
}

func (snapshot inventoryRemoveItemResultSnapshot) deletedStackableItems() ([]economy.StackableItem, error) {
	return restoreInventoryStackableItems(snapshot.DeletedStackableItems)
}

func (snapshot inventoryRemoveItemResultSnapshot) instanceItems() ([]economy.InstanceItem, error) {
	return restoreInventoryInstanceItems(snapshot.InstanceItems)
}

func (snapshot inventoryRemoveItemResultSnapshot) ledgerEntries() ([]economy.ItemLedgerEntry, error) {
	return restoreInventoryItemLedgerEntries(snapshot.LedgerEntries)
}

type inventoryStackableItemSnapshot struct {
	Source         catalog.VersionedDefinition `json:"source"`
	ItemInstanceID foundation.ItemID           `json:"item_instance_id"`
	ItemID         foundation.ItemID           `json:"item_id"`
	OwnerPlayerID  foundation.PlayerID         `json:"owner_player_id"`
	Location       economy.ItemLocation        `json:"location"`
	Quantity       int64                       `json:"quantity"`
	MetadataJSON   json.RawMessage             `json:"metadata_json,omitempty"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

type inventoryInstanceItemSnapshot struct {
	Source            catalog.VersionedDefinition `json:"source"`
	ItemInstanceID    foundation.ItemID           `json:"item_instance_id"`
	ItemID            foundation.ItemID           `json:"item_id"`
	OwnerPlayerID     foundation.PlayerID         `json:"owner_player_id"`
	Location          economy.ItemLocation        `json:"location"`
	Quantity          int64                       `json:"quantity"`
	DurabilityCurrent int64                       `json:"durability_current,omitempty"`
	BoundState        economy.BoundState          `json:"bound_state"`
	MetadataJSON      json.RawMessage             `json:"metadata_json,omitempty"`
	CreatedAt         time.Time                   `json:"created_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
}

type inventoryItemLedgerSnapshot struct {
	LedgerID       economy.LedgerID          `json:"ledger_id"`
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemID         foundation.ItemID         `json:"item_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Quantity       int64                     `json:"quantity"`
	Action         economy.LedgerAction      `json:"action"`
	BalanceAfter   int64                     `json:"balance_after"`
	Location       economy.ItemLocation      `json:"location"`
	Reason         economy.LedgerReason      `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	CreatedAt      time.Time                 `json:"created_at"`
}

func snapshotInventoryStackableItems(items []economy.StackableItem) []inventoryStackableItemSnapshot {
	if len(items) == 0 {
		return nil
	}
	snapshots := make([]inventoryStackableItemSnapshot, 0, len(items))
	for _, item := range items {
		metadata := append(json.RawMessage(nil), item.MetadataJSON...)
		if len(metadata) == 0 {
			metadata = nil
		}
		snapshots = append(snapshots, inventoryStackableItemSnapshot{
			Source:         item.Source,
			ItemInstanceID: item.ItemInstanceID,
			ItemID:         item.ItemID,
			OwnerPlayerID:  item.OwnerPlayerID,
			Location:       item.Location,
			Quantity:       item.Quantity.Int64(),
			MetadataJSON:   metadata,
			CreatedAt:      item.CreatedAt.UTC(),
			UpdatedAt:      item.UpdatedAt.UTC(),
		})
	}
	return snapshots
}

func restoreInventoryStackableItems(snapshots []inventoryStackableItemSnapshot) ([]economy.StackableItem, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	items := make([]economy.StackableItem, 0, len(snapshots))
	for _, snapshot := range snapshots {
		quantity, err := foundation.NewQuantity(snapshot.Quantity)
		if err != nil {
			return nil, err
		}
		item := economy.StackableItem{
			Source:         snapshot.Source,
			ItemInstanceID: snapshot.ItemInstanceID,
			ItemID:         snapshot.ItemID,
			OwnerPlayerID:  snapshot.OwnerPlayerID,
			Location:       snapshot.Location,
			Quantity:       quantity,
			MetadataJSON:   append(json.RawMessage(nil), snapshot.MetadataJSON...),
			CreatedAt:      snapshot.CreatedAt.UTC(),
			UpdatedAt:      snapshot.UpdatedAt.UTC(),
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func snapshotInventoryInstanceItems(items []economy.InstanceItem) []inventoryInstanceItemSnapshot {
	if len(items) == 0 {
		return nil
	}
	snapshots := make([]inventoryInstanceItemSnapshot, 0, len(items))
	for _, item := range items {
		metadata := append(json.RawMessage(nil), item.MetadataJSON...)
		if len(metadata) == 0 {
			metadata = nil
		}
		snapshots = append(snapshots, inventoryInstanceItemSnapshot{
			Source:            item.Source,
			ItemInstanceID:    item.ItemInstanceID,
			ItemID:            item.ItemID,
			OwnerPlayerID:     item.OwnerPlayerID,
			Location:          item.Location,
			Quantity:          item.Quantity.Int64(),
			DurabilityCurrent: item.DurabilityCurrent,
			BoundState:        item.BoundState,
			MetadataJSON:      metadata,
			CreatedAt:         item.CreatedAt.UTC(),
			UpdatedAt:         item.UpdatedAt.UTC(),
		})
	}
	return snapshots
}

func restoreInventoryInstanceItems(snapshots []inventoryInstanceItemSnapshot) ([]economy.InstanceItem, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	items := make([]economy.InstanceItem, 0, len(snapshots))
	for _, snapshot := range snapshots {
		quantity, err := foundation.NewQuantity(snapshot.Quantity)
		if err != nil {
			return nil, err
		}
		item := economy.InstanceItem{
			Source:            snapshot.Source,
			ItemInstanceID:    snapshot.ItemInstanceID,
			ItemID:            snapshot.ItemID,
			OwnerPlayerID:     snapshot.OwnerPlayerID,
			Location:          snapshot.Location,
			Quantity:          quantity,
			DurabilityCurrent: snapshot.DurabilityCurrent,
			BoundState:        snapshot.BoundState,
			MetadataJSON:      append(json.RawMessage(nil), snapshot.MetadataJSON...),
			CreatedAt:         snapshot.CreatedAt.UTC(),
			UpdatedAt:         snapshot.UpdatedAt.UTC(),
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func snapshotInventoryItemLedgerEntries(entries []economy.ItemLedgerEntry) []inventoryItemLedgerSnapshot {
	if len(entries) == 0 {
		return nil
	}
	snapshots := make([]inventoryItemLedgerSnapshot, 0, len(entries))
	for _, entry := range entries {
		snapshots = append(snapshots, inventoryItemLedgerSnapshot{
			LedgerID:       entry.LedgerID,
			PlayerID:       entry.PlayerID,
			ItemID:         entry.ItemID,
			ItemInstanceID: entry.ItemInstanceID,
			Quantity:       entry.Quantity.Int64(),
			Action:         entry.Action,
			BalanceAfter:   entry.BalanceAfter,
			Location:       entry.Location,
			Reason:         entry.Reason,
			ReferenceKey:   entry.ReferenceKey,
			CreatedAt:      entry.CreatedAt.UTC(),
		})
	}
	return snapshots
}

func restoreInventoryItemLedgerEntries(snapshots []inventoryItemLedgerSnapshot) ([]economy.ItemLedgerEntry, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	entries := make([]economy.ItemLedgerEntry, 0, len(snapshots))
	for _, snapshot := range snapshots {
		quantity, err := foundation.NewQuantity(snapshot.Quantity)
		if err != nil {
			return nil, err
		}
		entry, err := economy.NewItemLedgerEntry(
			snapshot.LedgerID,
			snapshot.PlayerID,
			snapshot.ItemID,
			snapshot.ItemInstanceID,
			quantity,
			snapshot.Action,
			snapshot.BalanceAfter,
			snapshot.Location,
			snapshot.Reason,
			snapshot.ReferenceKey,
		)
		if err != nil {
			return nil, err
		}
		entry.CreatedAt = snapshot.CreatedAt.UTC()
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func nullableRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}
