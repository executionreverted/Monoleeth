package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/loot"
)

type LootPickupStore struct {
	store *Store
}

var _ loot.LootPickupTransactionRepository = (*LootPickupStore)(nil)

type LootPickupTx struct {
	tx *sql.Tx
}

var _ loot.LootPickupTransaction = (*LootPickupTx)(nil)

func NewLootPickupStore(store *Store) (*LootPickupStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &LootPickupStore{store: store}, nil
}

func (store *LootPickupStore) WithLootPickupTransaction(ctx context.Context, fn func(loot.LootPickupTransaction) error) (err error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if fn == nil {
		return errors.New("nil loot pickup transaction function")
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx, &err)
	if err = fn(&LootPickupTx{tx: tx}); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (tx *LootPickupTx) SaveLootDropClaim(ctx context.Context, drop loot.Drop) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	if drop.ClaimedAt == nil || drop.ClaimedBy.IsZero() {
		return loot.ErrDropClaimed
	}
	payload, err := json.Marshal(drop)
	if err != nil {
		return err
	}
	_, err = tx.tx.ExecContext(ctx, `
		INSERT INTO loot_drop_claims(drop_id, player_id, item_id, quantity, source_type, source_id, claimed_at, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, drop.ID.String(), drop.ClaimedBy.String(), drop.ItemDefinition.ItemID.String(), drop.Quantity, drop.SourceType.String(), drop.SourceID.String(), drop.ClaimedAt.UTC(), string(payload))
	return err
}

func (tx *LootPickupTx) CommitInventoryAddItem(ctx context.Context, commit economy.InventoryAddItemCommit) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return commitInventoryAddItem(ctx, tx.tx, commit)
}

func (tx *LootPickupTx) InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if tx == nil || tx.tx == nil {
		return ErrNilDatabase
	}
	return insertOutboxRow(ctx, tx.tx, row)
}
