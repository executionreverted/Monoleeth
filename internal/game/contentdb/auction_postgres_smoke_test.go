package contentdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestPostgresAuctionStorePersistsLotAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	auctionStore, err := contentdb.NewAuctionLotStore(store)
	if err != nil {
		t.Fatalf("NewAuctionLotStore() error = %v, want nil", err)
	}
	lot := postgresAuctionLotForTest(t, "auction-postgres-roundtrip", time.Date(2026, 6, 25, 21, 0, 0, 0, time.UTC))
	if err := auctionStore.SaveAuctionLot(ctx, lot); err != nil {
		t.Fatalf("SaveAuctionLot() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewAuctionLotStore(store)
	if err != nil {
		t.Fatalf("NewAuctionLotStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.LoadAuctionLot(ctx, lot.AuctionID)
	if err != nil {
		t.Fatalf("LoadAuctionLot() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("LoadAuctionLot(%q) ok = false, want true", lot.AuctionID)
	}
	if loaded.AuctionID != lot.AuctionID ||
		loaded.CurrentBid != lot.CurrentBid ||
		loaded.CurrentBidderID != lot.CurrentBidderID ||
		loaded.Payload.Source.DefinitionID != lot.Payload.Source.DefinitionID {
		t.Fatalf("loaded lot = %+v, want persisted bid/bidder/payload from %+v", loaded, lot)
	}
}

func TestPostgresAuctionStoreTransactionRollsBackSettlementRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	bidderID := foundation.PlayerID("player-postgres-auction-tx-bidder")
	seedPostgresWalletPlayer(t, ctx, store, bidderID)
	auctionStore, err := contentdb.NewAuctionLotStore(store)
	if err != nil {
		t.Fatalf("NewAuctionLotStore() error = %v, want nil", err)
	}
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}

	now := time.Date(2026, 6, 25, 22, 0, 0, 0, time.UTC)
	lot := postgresAuctionLotForTest(t, "auction-postgres-tx-rollback", now)
	if err := auctionStore.SaveAuctionLot(ctx, lot); err != nil {
		t.Fatalf("SaveAuctionLot() error = %v, want nil", err)
	}
	if err := walletStore.UpsertWalletBalance(ctx, economy.WalletBalance{PlayerID: bidderID, Currency: economy.CurrencyBucketCredits, Balance: 1000, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertWalletBalance() error = %v, want nil", err)
	}
	requestID := foundation.RequestID("request-postgres-auction-tx-rollback")
	referenceKey, err := foundation.AuctionBidIdempotencyKey(lot.AuctionID, bidderID, requestID)
	if err != nil {
		t.Fatalf("AuctionBidIdempotencyKey() error = %v, want nil", err)
	}
	idempotencyRow := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   "auction_bid",
		PlayerID:    bidderID,
		RequestHash: "auction-tx-request-hash",
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  []byte(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	rollbackErr := errors.New("injected auction transaction rollback")

	err = auctionStore.WithAuctionLotTransaction(ctx, func(tx auction.AuctionLotTransaction) error {
		locked, ok, err := tx.LoadAuctionLotForUpdate(ctx, lot.AuctionID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("locked auction lot missing")
		}
		locked.CurrentBid = 120
		locked.CurrentBidderID = bidderID
		locked.UpdatedAt = now.Add(time.Minute)
		if err := tx.SaveAuctionLot(ctx, locked); err != nil {
			return err
		}
		if err := tx.CommitWalletMutation(ctx, postgresAuctionWalletCommit(t, bidderID, 880, 120, referenceKey, now)); err != nil {
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
		completed.ResultJSON = []byte(`{"auction_id":"auction-postgres-tx-rollback"}`)
		completed.UpdatedAt = now.Add(time.Minute)
		completed.CompletedAt = now.Add(time.Minute)
		if _, err := tx.CompleteIdempotencyKey(ctx, completed); err != nil {
			return err
		}
		if err := tx.InsertOutboxRow(ctx, economy.OutboxRow{
			OutboxID:         "outbox-postgres-auction-tx-rollback",
			Topic:            "economy",
			EventType:        "auction.bid_placed",
			AggregateType:    "auction_lot",
			AggregateID:      lot.AuctionID.String(),
			IdempotencyScope: economy.IdempotencyScopeEconomy,
			IdempotencyKey:   referenceKey,
			PayloadJSON:      []byte(`{"auction_id":"auction-postgres-tx-rollback"}`),
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithAuctionLotTransaction() error = %v, want rollback sentinel", err)
	}

	loaded, ok, err := auctionStore.LoadAuctionLot(ctx, lot.AuctionID)
	if err != nil {
		t.Fatalf("LoadAuctionLot(after rollback) error = %v, want nil", err)
	}
	if !ok || loaded.CurrentBid != 0 || !loaded.CurrentBidderID.IsZero() {
		t.Fatalf("lot after rollback = %+v ok %v, want original no-bid lot", loaded, ok)
	}
	balance, err := walletStore.WalletBalance(ctx, bidderID, economy.CurrencyBucketCredits)
	if err != nil {
		t.Fatalf("WalletBalance(after rollback) error = %v, want nil", err)
	}
	if balance.Balance != 1000 {
		t.Fatalf("wallet balance after rollback = %d, want 1000", balance.Balance)
	}
	ledgerEntries, err := walletStore.LoadCurrencyLedgerEntries(ctx)
	if err != nil {
		t.Fatalf("LoadCurrencyLedgerEntries(after rollback) error = %v, want nil", err)
	}
	if len(ledgerEntries) != 0 {
		t.Fatalf("wallet ledger entries after rollback = %+v, want none", ledgerEntries)
	}
	if _, ok, err := store.LoadOutboxRow(ctx, "outbox-postgres-auction-tx-rollback"); err != nil || ok {
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

func postgresAuctionLotForTest(t *testing.T, auctionID string, now time.Time) auction.Lot {
	t.Helper()
	buyNowPrice := int64(300)
	return auction.Lot{
		AuctionID: foundation.AuctionID(auctionID),
		WorldID:   foundation.WorldID("world-postgres-auction"),
		Payload: auction.LotPayload{
			Type:     auction.LotPayloadTypeXCoreFragmentBundle,
			Source:   catalogAuctionSourceForPostgresTest(t, auctionID),
			Quantity: 2,
		},
		Currency:    economy.CurrencyBucketCredits,
		StartPrice:  100,
		BuyNowPrice: &buyNowPrice,
		Status:      auction.LotStatusActive,
		StartsAt:    now.Add(-time.Minute),
		EndsAt:      now.Add(time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func catalogAuctionSourceForPostgresTest(t *testing.T, definitionID string) catalog.VersionedDefinition {
	t.Helper()
	source, err := catalog.NewAuctionLotSource(definitionID, "v1")
	if err != nil {
		t.Fatalf("NewAuctionLotSource() error = %v, want nil", err)
	}
	return source
}

func postgresAuctionWalletCommit(
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
		economy.LedgerID("currency-ledger-auction-tx-bid"),
		playerID,
		economy.CurrencyBucketCredits,
		amount,
		economy.LedgerActionDecrease,
		balanceAfter,
		economy.LedgerReason("auction_bid_tx_test"),
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewCurrencyLedgerEntry() error = %v, want nil", err)
	}
	entry.CreatedAt = now
	return economy.WalletMutationCommit{
		Balances: []economy.WalletBalance{{
			PlayerID:  playerID,
			Currency:  economy.CurrencyBucketCredits,
			Balance:   balanceAfter,
			UpdatedAt: now,
		}},
		LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		Reference: economy.WalletMutationReference{
			PlayerID:      playerID,
			Operation:     economy.WalletMutationOperationDebit,
			ReferenceKey:  referenceKey,
			LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		},
		Counters: economy.WalletCounters{LedgerSequence: 2},
	}
}
