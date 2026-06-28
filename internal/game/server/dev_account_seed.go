package server

import (
	"context"
	"fmt"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const devAccountLedgerReason economy.LedgerReason = "dev_account_seed"

type devAccountSeedIdentity struct {
	Email    string
	Callsign string
}

var defaultDevAccountSeeds = []devAccountSeedIdentity{
	{Email: "pilot1@example.com", Callsign: "Dev Pilot 1"},
	{Email: "pilot2@example.com", Callsign: "Dev Pilot 2"},
}

func (runtime *Runtime) seedDevAccounts(ctx context.Context, password string, targetCredits int64) error {
	if runtime == nil || runtime.Auth == nil || runtime.Wallet == nil {
		return nil
	}
	if targetCredits <= 0 {
		targetCredits = defaultDevAccountCredits
	}
	for _, account := range defaultDevAccountSeeds {
		result, err := runtime.seedDevAuthAccount(ctx, account, password)
		if err != nil {
			return err
		}
		if err := runtime.seedStarterWallet(result.Session.PlayerID); err != nil {
			return fmt.Errorf("seed starter wallet for dev account %q: %w", account.Email, err)
		}
		if err := runtime.ensureDevAccountCredits(result.Session.PlayerID, targetCredits); err != nil {
			return fmt.Errorf("seed wallet for dev account %q: %w", account.Email, err)
		}
	}
	return nil
}

func (runtime *Runtime) seedDevAuthAccount(ctx context.Context, account devAccountSeedIdentity, password string) (auth.AuthResult, error) {
	if password == "" {
		password = defaultDevAccountPassword
	}
	result, err := runtime.Auth.Login(ctx, auth.LoginInput{
		Email:    account.Email,
		Password: password,
	})
	if err == nil {
		return result, nil
	}
	if !foundation.IsCode(err, foundation.CodeUnauthenticated) {
		return auth.AuthResult{}, fmt.Errorf("login dev account %q: %w", account.Email, err)
	}
	result, err = runtime.Auth.Register(ctx, auth.RegisterInput{
		Email:    account.Email,
		Password: password,
		Callsign: account.Callsign,
	})
	if err != nil {
		return auth.AuthResult{}, fmt.Errorf("register dev account %q: %w", account.Email, err)
	}
	return result, nil
}

func (runtime *Runtime) ensureDevAccountCredits(playerID foundation.PlayerID, targetCredits int64) error {
	current := runtime.Wallet.Balance(playerID, economy.CurrencyBucketCredits)
	if current >= targetCredits {
		return nil
	}
	reference, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), "dev-account-credits")
	if err != nil {
		return err
	}
	_, err = runtime.Wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       targetCredits - current,
		Reason:       devAccountLedgerReason,
		ReferenceKey: reference,
	})
	return err
}
