package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestPostgresWalletStorePersistsBalanceAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	seedPostgresWalletPlayer(t, ctx, store, "player-postgres-wallet-smoke")
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}
	balance := economy.WalletBalance{
		PlayerID:  foundation.PlayerID("player-postgres-wallet-smoke"),
		Currency:  economy.CurrencyBucketCredits,
		Balance:   750,
		UpdatedAt: time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC),
	}
	if err := walletStore.UpsertWalletBalance(ctx, balance); err != nil {
		t.Fatalf("UpsertWalletBalance() error = %v, want nil", err)
	}
	reopened, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore(reopen) error = %v, want nil", err)
	}
	balances, err := reopened.LoadWalletBalances(ctx)
	if err != nil {
		t.Fatalf("LoadWalletBalances() error = %v, want nil", err)
	}
	if len(balances) != 1 || balances[0].PlayerID != balance.PlayerID || balances[0].Currency != balance.Currency || balances[0].Balance != balance.Balance {
		t.Fatalf("balances = %+v, want persisted balance %+v", balances, balance)
	}
}

func seedPostgresWalletPlayer(t *testing.T, ctx context.Context, store *contentdb.Store, playerID foundation.PlayerID) {
	t.Helper()
	authStore, err := contentdb.NewAuthStore(store)
	if err != nil {
		t.Fatalf("NewAuthStore() error = %v, want nil", err)
	}
	accountID := foundation.AccountID("acc-" + playerID.String())
	account := auth.Account{
		ID:           accountID,
		Email:        auth.Email(playerID.String() + "@example.com"),
		PasswordHash: auth.PasswordHash("pbkdf2-sha256$v1$1$c2FsdA$a2V5"),
		CreatedAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
	}
	player := auth.PlayerProfile{
		ID:        playerID,
		AccountID: accountID,
		Callsign:  playerID.String(),
		CreatedAt: account.CreatedAt,
		UpdatedAt: account.UpdatedAt,
	}
	if err := authStore.InsertAccount(ctx, account, player); err != nil {
		t.Fatalf("InsertAccount(seed wallet player) error = %v, want nil", err)
	}
}
