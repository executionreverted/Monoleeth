package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
)

func TestPostgresAuthStorePersistsAccountAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	authStore, err := contentdb.NewAuthStore(store)
	if err != nil {
		t.Fatalf("NewAuthStore() error = %v, want nil", err)
	}
	account := auth.Account{
		ID:           foundation.AccountID("acc-postgres-auth-smoke"),
		Email:        auth.Email("pilot@example.com"),
		PasswordHash: auth.PasswordHash("pbkdf2-sha256$v1$1$c2FsdA$a2V5"),
		CreatedAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 25, 12, 0, 1, 0, time.UTC),
	}
	player := auth.PlayerProfile{
		ID:        foundation.PlayerID("player-postgres-auth-smoke"),
		AccountID: account.ID,
		Callsign:  "Frontier-01",
		CreatedAt: account.CreatedAt,
		UpdatedAt: account.UpdatedAt,
	}
	if err := authStore.InsertAccount(ctx, account, player); err != nil {
		t.Fatalf("InsertAccount() error = %v, want nil", err)
	}
	reopened, err := contentdb.NewAuthStore(store)
	if err != nil {
		t.Fatalf("NewAuthStore(reopen) error = %v, want nil", err)
	}
	storedAccount, storedPlayer, err := reopened.AccountByEmail(ctx, account.Email)
	if err != nil {
		t.Fatalf("AccountByEmail() error = %v, want nil", err)
	}
	if storedAccount.ID != account.ID || storedPlayer.ID != player.ID || storedPlayer.AccountID != account.ID {
		t.Fatalf("stored account/player = %+v / %+v, want persisted linked rows", storedAccount, storedPlayer)
	}
}
