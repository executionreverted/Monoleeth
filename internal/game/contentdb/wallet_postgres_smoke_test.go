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

func TestPostgresWalletStorePersistsLedgerReferenceAcrossServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-wallet-ledger-ref")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}
	service, err := economy.NewWalletServiceWithRepository(foundation.RealClock{}, walletStore)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository() error = %v, want nil", err)
	}
	referenceKey := postgresWalletReferenceKey(t, "quest_reward:postgres-wallet-ledger-ref")
	input := economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       300,
		Reason:       economy.LedgerReason("postgres_wallet_ledger_ref"),
		ReferenceKey: referenceKey,
	}
	first, err := service.CreditWallet(input)
	if err != nil {
		t.Fatalf("CreditWallet() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewWalletServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository(reopen) error = %v, want nil", err)
	}

	entries := reloaded.CurrencyLedgerEntries()
	if len(entries) != 1 || entries[0].LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("CurrencyLedgerEntries() = %+v, want persisted ledger %q", entries, first.LedgerEntry.LedgerID)
	}
	duplicate, err := reloaded.CreditWallet(input)
	if err != nil {
		t.Fatalf("duplicate CreditWallet after reload error = %v, want nil", err)
	}
	if !duplicate.Duplicate || duplicate.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate after reload = %+v, want same persisted ledger duplicate", duplicate)
	}
	if got := reloaded.Balance(playerID, economy.CurrencyBucketCredits); got != 300 {
		t.Fatalf("Balance() after duplicate credit reload = %d, want 300", got)
	}
}

func TestPostgresWalletStoreDuplicateDebitReferenceAfterServiceReloadDoesNotDoubleDebit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-wallet-debit-ref")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}
	service, err := economy.NewWalletServiceWithRepository(foundation.RealClock{}, walletStore)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository() error = %v, want nil", err)
	}
	creditInput := economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       500,
		Reason:       economy.LedgerReason("postgres_wallet_debit_seed"),
		ReferenceKey: postgresWalletReferenceKey(t, "quest_reward:postgres-wallet-debit-seed"),
	}
	if _, err := service.CreditWallet(creditInput); err != nil {
		t.Fatalf("seed CreditWallet() error = %v, want nil", err)
	}
	debitInput := economy.DebitWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       125,
		Reason:       economy.LedgerReason("postgres_wallet_debit_ref"),
		ReferenceKey: postgresWalletReferenceKey(t, "market_buy:listing-postgres-wallet-debit-ref:player-postgres-wallet-debit-ref:request-postgres-wallet-debit-ref"),
	}
	first, err := service.DebitWallet(debitInput)
	if err != nil {
		t.Fatalf("DebitWallet() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore(reopen) error = %v, want nil", err)
	}
	reloaded, err := economy.NewWalletServiceWithRepository(foundation.RealClock{}, reopened)
	if err != nil {
		t.Fatalf("NewWalletServiceWithRepository(reopen) error = %v, want nil", err)
	}
	duplicate, err := reloaded.DebitWallet(debitInput)
	if err != nil {
		t.Fatalf("duplicate DebitWallet after reload error = %v, want nil", err)
	}

	if !duplicate.Duplicate || duplicate.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate debit after reload = %+v, want same persisted ledger duplicate", duplicate)
	}
	if got := reloaded.Balance(playerID, economy.CurrencyBucketCredits); got != 375 {
		t.Fatalf("Balance() after duplicate debit reload = %d, want 375", got)
	}
	if got := len(reloaded.CurrencyLedgerEntries()); got != 2 {
		t.Fatalf("CurrencyLedgerEntries() len after duplicate debit reload = %d, want 2", got)
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

func postgresWalletReferenceKey(t *testing.T, value string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.ParseIdempotencyKey(value)
	if err != nil {
		t.Fatalf("ParseIdempotencyKey(%q) error = %v, want nil", value, err)
	}
	return key
}
