package contentdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

type WalletStore struct {
	store *Store
}

var _ economy.WalletRepository = (*WalletStore)(nil)

type walletSQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewWalletStore(store *Store) (*WalletStore, error) {
	if store == nil || store.db == nil {
		return nil, ErrNilDatabase
	}
	return &WalletStore{store: store}, nil
}

func (store *WalletStore) LoadWalletBalances(ctx context.Context) ([]economy.WalletBalance, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, currency_type, balance, updated_at
		FROM player_wallets
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	balances := make([]economy.WalletBalance, 0)
	for rows.Next() {
		var playerID string
		var currency string
		var balance int64
		var updatedAt time.Time
		if err := rows.Scan(&playerID, &currency, &balance, &updatedAt); err != nil {
			return nil, err
		}
		walletBalance := economy.WalletBalance{
			PlayerID:  foundation.PlayerID(playerID),
			Currency:  economy.CurrencyBucket(currency),
			Balance:   balance,
			UpdatedAt: updatedAt.UTC(),
		}
		if err := walletBalance.Validate(); err != nil {
			return nil, err
		}
		balances = append(balances, walletBalance)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(balances, func(i, j int) bool {
		if balances[i].PlayerID != balances[j].PlayerID {
			return balances[i].PlayerID < balances[j].PlayerID
		}
		return balances[i].Currency < balances[j].Currency
	})
	return balances, nil
}

func (store *WalletStore) LoadCurrencyLedgerEntries(ctx context.Context) ([]economy.CurrencyLedgerEntry, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT ledger_id, player_id, currency_type, amount, action, balance_after, reason, reference_key, created_at
		FROM player_wallet_ledger
		ORDER BY created_at, ledger_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]economy.CurrencyLedgerEntry, 0)
	for rows.Next() {
		var ledgerID string
		var playerID string
		var currency string
		var amountValue int64
		var action string
		var balanceAfter int64
		var reason string
		var referenceKey string
		var createdAt time.Time
		if err := rows.Scan(&ledgerID, &playerID, &currency, &amountValue, &action, &balanceAfter, &reason, &referenceKey, &createdAt); err != nil {
			return nil, err
		}
		amount, err := foundation.NewMoney(amountValue)
		if err != nil {
			return nil, err
		}
		entry, err := economy.NewCurrencyLedgerEntry(
			economy.LedgerID(ledgerID),
			foundation.PlayerID(playerID),
			economy.CurrencyBucket(currency),
			amount,
			economy.LedgerAction(action),
			balanceAfter,
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

func (store *WalletStore) LoadWalletMutationReferences(ctx context.Context) ([]economy.WalletMutationReference, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return nil, ErrNilDatabase
	}
	ledgerEntries, err := store.LoadCurrencyLedgerEntries(ctx)
	if err != nil {
		return nil, err
	}
	ledgerByID := make(map[economy.LedgerID]economy.CurrencyLedgerEntry, len(ledgerEntries))
	for _, entry := range ledgerEntries {
		ledgerByID[entry.LedgerID] = entry
	}
	rows, err := store.store.db.QueryContext(ctx, `
		SELECT player_id, operation, reference_key, ledger_ids
		FROM player_wallet_references
		ORDER BY player_id, operation, reference_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	references := make([]economy.WalletMutationReference, 0)
	for rows.Next() {
		var playerID string
		var operation string
		var referenceKey string
		var ledgerIDsJSON []byte
		if err := rows.Scan(&playerID, &operation, &referenceKey, &ledgerIDsJSON); err != nil {
			return nil, err
		}
		ledgerIDs, err := parseWalletLedgerIDs(ledgerIDsJSON)
		if err != nil {
			return nil, err
		}
		entries := make([]economy.CurrencyLedgerEntry, 0, len(ledgerIDs))
		for _, ledgerID := range ledgerIDs {
			entry, ok := ledgerByID[ledgerID]
			if !ok {
				return nil, economy.ErrEmptyLedgerID
			}
			entries = append(entries, entry)
		}
		reference := economy.WalletMutationReference{
			PlayerID:      foundation.PlayerID(playerID),
			Operation:     economy.WalletMutationOperation(operation),
			ReferenceKey:  foundation.IdempotencyKey(referenceKey),
			LedgerEntries: entries,
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

func (store *WalletStore) LoadWalletCounters(ctx context.Context) (economy.WalletCounters, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return economy.WalletCounters{}, ErrNilDatabase
	}
	var counters economy.WalletCounters
	err := store.store.db.QueryRowContext(ctx, `
		SELECT ledger_sequence
		FROM player_wallet_counters
		WHERE counter_id = 'wallet'
	`).Scan(&counters.LedgerSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return economy.WalletCounters{}, nil
	}
	if err != nil {
		return economy.WalletCounters{}, err
	}
	if err := counters.Validate(); err != nil {
		return economy.WalletCounters{}, err
	}
	return counters, nil
}

func (store *WalletStore) UpsertWalletBalance(ctx context.Context, balance economy.WalletBalance) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	return upsertWalletBalance(ctx, store.store.db, balance)
}

func (store *WalletStore) CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	tx, err := store.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := commitWalletMutation(ctx, tx, commit); err != nil {
		return err
	}
	return tx.Commit()
}

func commitWalletMutation(ctx context.Context, execer walletSQLExecer, commit economy.WalletMutationCommit) error {
	if execer == nil {
		return ErrNilDatabase
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	for _, balance := range commit.Balances {
		if err := upsertWalletBalance(ctx, execer, balance); err != nil {
			return err
		}
	}
	for _, entry := range commit.LedgerEntries {
		if err := insertWalletCurrencyLedgerEntry(ctx, execer, entry); err != nil {
			return err
		}
	}
	if err := insertWalletMutationReference(ctx, execer, commit.Reference); err != nil {
		return err
	}
	if err := upsertWalletCounters(ctx, execer, commit.Counters); err != nil {
		return err
	}
	return nil
}

func (store *WalletStore) WalletBalance(ctx context.Context, playerID foundation.PlayerID, currency economy.CurrencyBucket) (economy.WalletBalance, error) {
	if store == nil || store.store == nil || store.store.db == nil {
		return economy.WalletBalance{}, ErrNilDatabase
	}
	row := store.store.db.QueryRowContext(ctx, `
		SELECT player_id, currency_type, balance, updated_at
		FROM player_wallets
		WHERE player_id = $1 AND currency_type = $2
	`, playerID.String(), currency.String())
	var storedPlayerID string
	var storedCurrency string
	var balance int64
	var updatedAt time.Time
	if err := row.Scan(&storedPlayerID, &storedCurrency, &balance, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return economy.WalletBalance{}, sql.ErrNoRows
		}
		return economy.WalletBalance{}, err
	}
	return economy.WalletBalance{
		PlayerID:  foundation.PlayerID(storedPlayerID),
		Currency:  economy.CurrencyBucket(storedCurrency),
		Balance:   balance,
		UpdatedAt: updatedAt.UTC(),
	}, nil
}

func upsertWalletBalance(ctx context.Context, execer walletSQLExecer, balance economy.WalletBalance) error {
	if err := balance.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_wallets(player_id, currency_type, balance, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (player_id, currency_type) DO UPDATE
		SET balance = EXCLUDED.balance,
			updated_at = EXCLUDED.updated_at
	`, balance.PlayerID.String(), balance.Currency.String(), balance.Balance, balance.UpdatedAt.UTC())
	return err
}

func insertWalletCurrencyLedgerEntry(ctx context.Context, execer walletSQLExecer, entry economy.CurrencyLedgerEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_wallet_ledger(ledger_id, player_id, currency_type, amount, action, balance_after, reason, reference_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, entry.LedgerID.String(), entry.PlayerID.String(), entry.Currency.String(), entry.Amount.Int64(), entry.Action.String(), entry.BalanceAfter, entry.Reason.String(), entry.ReferenceKey.String(), entry.CreatedAt.UTC())
	return err
}

func insertWalletMutationReference(ctx context.Context, execer walletSQLExecer, reference economy.WalletMutationReference) error {
	if err := reference.Validate(); err != nil {
		return err
	}
	ledgerIDsJSON, err := json.Marshal(walletReferenceLedgerIDs(reference.LedgerEntries))
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
		INSERT INTO player_wallet_references(player_id, operation, reference_key, primary_ledger_id, ledger_ids, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
	`, reference.PlayerID.String(), reference.Operation.String(), reference.ReferenceKey.String(), reference.LedgerEntries[0].LedgerID.String(), string(ledgerIDsJSON), reference.LedgerEntries[0].CreatedAt.UTC())
	return err
}

func upsertWalletCounters(ctx context.Context, execer walletSQLExecer, counters economy.WalletCounters) error {
	if err := counters.Validate(); err != nil {
		return err
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO player_wallet_counters(counter_id, ledger_sequence, updated_at)
		VALUES ('wallet', $1, now())
		ON CONFLICT (counter_id) DO UPDATE
		SET ledger_sequence = GREATEST(player_wallet_counters.ledger_sequence, EXCLUDED.ledger_sequence),
			updated_at = EXCLUDED.updated_at
	`, counters.LedgerSequence)
	return err
}

func walletReferenceLedgerIDs(entries []economy.CurrencyLedgerEntry) []string {
	ledgerIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		ledgerIDs = append(ledgerIDs, entry.LedgerID.String())
	}
	return ledgerIDs
}

func parseWalletLedgerIDs(raw []byte) ([]economy.LedgerID, error) {
	var values []string
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	}
	ledgerIDs := make([]economy.LedgerID, 0, len(values))
	for _, value := range values {
		ledgerID := economy.LedgerID(value)
		if err := ledgerID.Validate(); err != nil {
			return nil, err
		}
		ledgerIDs = append(ledgerIDs, ledgerID)
	}
	return ledgerIDs, nil
}
