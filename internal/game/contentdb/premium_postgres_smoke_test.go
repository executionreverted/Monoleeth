package contentdb_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/premium"
)

func TestPostgresPremiumStoreTransactionRollsBackClaimRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	playerID := foundation.PlayerID("player-postgres-premium-tx")
	seedPostgresWalletPlayer(t, ctx, store, playerID)
	premiumStore, err := contentdb.NewPremiumEntitlementStore(store)
	if err != nil {
		t.Fatalf("NewPremiumEntitlementStore() error = %v, want nil", err)
	}
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}

	now := time.Date(2026, 6, 25, 23, 0, 0, 0, time.UTC)
	entitlement := postgresPremiumEntitlementForTest("entitlement-postgres-premium-tx", playerID, "event-postgres-premium-tx", now)
	if err := premiumStore.SavePremiumEntitlement(ctx, entitlement); err != nil {
		t.Fatalf("SavePremiumEntitlement() error = %v, want nil", err)
	}
	referenceKey, err := foundation.PremiumWebhookIdempotencyKey("claim." + entitlement.ID.String() + "." + playerID.String())
	if err != nil {
		t.Fatalf("PremiumWebhookIdempotencyKey() error = %v, want nil", err)
	}
	idempotencyRow := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   "premium_claim",
		PlayerID:    playerID,
		RequestHash: "premium-claim-tx-request-hash",
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  []byte(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	rollbackErr := errors.New("injected premium transaction rollback")

	err = premiumStore.WithPremiumEntitlementTransaction(ctx, func(tx premium.PremiumEntitlementTransaction) error {
		locked, ok, err := tx.LoadPremiumEntitlementForUpdate(ctx, entitlement.ID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("locked premium entitlement missing")
		}
		locked.State = premium.EntitlementStateClaimed
		locked.ClaimedAt = now.Add(time.Minute)
		locked.ClaimRequestRef = "claim-postgres-premium-tx"
		if err := tx.SavePremiumEntitlement(ctx, locked); err != nil {
			return err
		}
		if err := tx.CommitWalletMutation(ctx, postgresPremiumWalletCreditCommit(t, playerID, 600, 600, referenceKey, now)); err != nil {
			return err
		}
		claim, err := tx.ClaimIdempotencyKey(ctx, idempotencyRow)
		if err != nil {
			return err
		}
		if claim.Duplicate {
			return errors.New("idempotency claim duplicate before rollback")
		}
		completed := claim.Row.Clone()
		completed.Status = economy.IdempotencyStatusCompleted
		completed.ResultJSON = []byte(`{"entitlement_id":"entitlement-postgres-premium-tx"}`)
		completed.UpdatedAt = now.Add(time.Minute)
		completed.CompletedAt = now.Add(time.Minute)
		if _, err := tx.CompleteIdempotencyKey(ctx, completed); err != nil {
			return err
		}
		if err := tx.InsertOutboxRow(ctx, economy.OutboxRow{
			OutboxID:         "outbox-postgres-premium-tx-rollback",
			Topic:            "economy",
			EventType:        "premium.entitlement_claimed",
			AggregateType:    "premium_entitlement",
			AggregateID:      entitlement.ID.String(),
			IdempotencyScope: economy.IdempotencyScopeEconomy,
			IdempotencyKey:   referenceKey,
			PayloadJSON:      []byte(`{"entitlement_id":"entitlement-postgres-premium-tx"}`),
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithPremiumEntitlementTransaction() error = %v, want rollback sentinel", err)
	}

	loaded, ok, err := premiumStore.LoadPremiumEntitlement(ctx, entitlement.ID)
	if err != nil {
		t.Fatalf("LoadPremiumEntitlement(after rollback) error = %v, want nil", err)
	}
	if !ok || loaded.State != premium.EntitlementStatePending || loaded.ClaimRequestRef != "" {
		t.Fatalf("entitlement after rollback = %+v ok %v, want original pending row", loaded, ok)
	}
	if balance, err := walletStore.WalletBalance(ctx, playerID, economy.CurrencyBucketPremiumPaid); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("WalletBalance(after rollback) = %+v err %v, want sql.ErrNoRows", balance, err)
	}
	ledgerEntries, err := walletStore.LoadCurrencyLedgerEntries(ctx)
	if err != nil {
		t.Fatalf("LoadCurrencyLedgerEntries(after rollback) error = %v, want nil", err)
	}
	if len(ledgerEntries) != 0 {
		t.Fatalf("wallet ledger entries after rollback = %+v, want none", ledgerEntries)
	}
	if _, ok, err := store.LoadOutboxRow(ctx, "outbox-postgres-premium-tx-rollback"); err != nil || ok {
		t.Fatalf("LoadOutboxRow(after rollback) = ok %v err %v, want false nil", ok, err)
	}
	claim, err := store.ClaimIdempotencyKey(ctx, idempotencyRow)
	if err != nil {
		t.Fatalf("ClaimIdempotencyKey(after rollback) error = %v, want nil", err)
	}
	if claim.Duplicate {
		t.Fatal("ClaimIdempotencyKey(after rollback) Duplicate = true, want false")
	}
}

func postgresPremiumEntitlementForTest(
	entitlementID premium.EntitlementID,
	playerID foundation.PlayerID,
	providerReference string,
	now time.Time,
) premium.Entitlement {
	return premium.Entitlement{
		ID:       entitlementID,
		PlayerID: playerID,
		Type:     premium.EntitlementTypePremiumCurrencyPack,
		State:    premium.EntitlementStatePending,
		Provider: premium.ProviderReference{
			Source:    "stripe",
			Reference: providerReference,
		},
		Payload: premium.EntitlementGrantPayload{
			CurrencyBucket: economy.CurrencyBucketPremiumPaid,
			Amount:         600,
		},
		CreatedAt:           now,
		ProviderConfirmedAt: now.Add(-time.Second),
	}
}

func postgresPremiumWalletCreditCommit(
	t *testing.T,
	playerID foundation.PlayerID,
	balanceAfter int64,
	amountValue int64,
	referenceKey foundation.IdempotencyKey,
	now time.Time,
) economy.WalletMutationCommit {
	t.Helper()
	amount, err := foundation.NewMoney(amountValue)
	if err != nil {
		t.Fatalf("NewMoney(%d) error = %v, want nil", amountValue, err)
	}
	entry, err := economy.NewCurrencyLedgerEntry(
		economy.LedgerID("currency-ledger-premium-tx-claim"),
		playerID,
		economy.CurrencyBucketPremiumPaid,
		amount,
		economy.LedgerActionIncrease,
		balanceAfter,
		premium.LedgerReasonPremiumEntitlementClaim,
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewCurrencyLedgerEntry() error = %v, want nil", err)
	}
	entry.CreatedAt = now
	return economy.WalletMutationCommit{
		Balances: []economy.WalletBalance{{
			PlayerID:  playerID,
			Currency:  economy.CurrencyBucketPremiumPaid,
			Balance:   balanceAfter,
			UpdatedAt: now,
		}},
		LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		Reference: economy.WalletMutationReference{
			PlayerID:      playerID,
			Operation:     economy.WalletMutationOperationCredit,
			ReferenceKey:  referenceKey,
			LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		},
		Counters: economy.WalletCounters{LedgerSequence: 1},
	}
}
