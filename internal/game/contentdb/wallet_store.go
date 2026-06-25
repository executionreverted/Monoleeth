package contentdb

import (
	"context"
	"database/sql"
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

func (store *WalletStore) UpsertWalletBalance(ctx context.Context, balance economy.WalletBalance) error {
	if store == nil || store.store == nil || store.store.db == nil {
		return ErrNilDatabase
	}
	if err := balance.Validate(); err != nil {
		return err
	}
	_, err := store.store.db.ExecContext(ctx, `
		INSERT INTO player_wallets(player_id, currency_type, balance, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (player_id, currency_type) DO UPDATE
		SET balance = EXCLUDED.balance,
			updated_at = EXCLUDED.updated_at
	`, balance.PlayerID.String(), balance.Currency.String(), balance.Balance, balance.UpdatedAt.UTC())
	if err != nil {
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
