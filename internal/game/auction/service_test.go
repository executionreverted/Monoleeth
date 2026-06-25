package auction

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

var auctionTestNow = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

type auctionFixture struct {
	clock       *testutil.FakeClock
	wallet      *economy.WalletService
	idempotency *memoryAuctionIdempotencyStore
	service     *Service
	worldID     foundation.WorldID
	bidderID    foundation.PlayerID
	otherID     foundation.PlayerID
	buyerID     foundation.PlayerID
	payload     LotPayload
}

func TestCreateLotStoresCatalogPayloadAndStatus(t *testing.T) {
	fixture := newAuctionFixture(t)

	result := fixture.createLot(t, "auction-create", 100, int64Pointer(500))

	if result.Lot.Status != LotStatusActive {
		t.Fatalf("lot status = %q, want active", result.Lot.Status)
	}
	if result.Lot.Payload.Type != LotPayloadTypeShipUnlock {
		t.Fatalf("payload type = %q, want ship unlock", result.Lot.Payload.Type)
	}
	if result.Lot.Payload.Source.DefinitionID.String() != "auction-slazar-unlock" {
		t.Fatalf("payload source id = %q, want auction-slazar-unlock", result.Lot.Payload.Source.DefinitionID)
	}
	if result.Lot.BuyNowPrice == nil || *result.Lot.BuyNowPrice != 500 {
		t.Fatalf("buy now price = %v, want 500", result.Lot.BuyNowPrice)
	}

	future := fixture.createLotInput("auction-upcoming", 100, nil)
	future.StartsAt = fixture.clock.Now().Add(time.Minute)
	future.EndsAt = fixture.clock.Now().Add(time.Hour)
	upcoming, err := fixture.service.CreateLot(future)
	if err != nil {
		t.Fatalf("CreateLot future: %v", err)
	}
	if upcoming.Lot.Status != LotStatusUpcoming {
		t.Fatalf("future lot status = %q, want upcoming", upcoming.Lot.Status)
	}
	fixture.clock.Advance(time.Minute)
	activated, ok := fixture.service.Lot(upcoming.Lot.AuctionID)
	if !ok {
		t.Fatal("future lot missing")
	}
	if activated.Status != LotStatusActive {
		t.Fatalf("activated lot status = %q, want active", activated.Status)
	}
}

func TestCreateLotRejectsInvalidInputsWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*auctionFixture, *CreateLotInput)
		wantErr error
	}{
		{
			name: "invalid payload type",
			prepare: func(fixture *auctionFixture, input *CreateLotInput) {
				input.Payload.Type = LotPayloadType("planet_deed")
			},
			wantErr: ErrInvalidLotPayloadType,
		},
		{
			name: "invalid timing",
			prepare: func(fixture *auctionFixture, input *CreateLotInput) {
				input.EndsAt = input.StartsAt
			},
			wantErr: ErrInvalidLotTiming,
		},
		{
			name: "ended before create",
			prepare: func(fixture *auctionFixture, input *CreateLotInput) {
				input.StartsAt = fixture.clock.Now().Add(-2 * time.Hour)
				input.EndsAt = fixture.clock.Now().Add(-time.Hour)
			},
			wantErr: ErrInvalidLotTiming,
		},
		{
			name: "buy now below start",
			prepare: func(fixture *auctionFixture, input *CreateLotInput) {
				input.BuyNowPrice = int64Pointer(input.StartPrice - 1)
			},
			wantErr: ErrBuyNowUnavailable,
		},
		{
			name: "non-positive start",
			prepare: func(fixture *auctionFixture, input *CreateLotInput) {
				input.StartPrice = 0
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAuctionFixture(t)
			input := fixture.createLotInput("auction-"+tc.name, 100, int64Pointer(500))
			tc.prepare(fixture, &input)

			_, err := fixture.service.CreateLot(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CreateLot error = %v, want %v", err, tc.wantErr)
			}
			if got := len(fixture.service.Lots()); got != 0 {
				t.Fatalf("lots len = %d, want 0", got)
			}
		})
	}
}

func TestCreateLotRejectsDuplicateLotID(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.createLot(t, "auction-duplicate", 100, nil)

	_, err := fixture.service.CreateLot(fixture.createLotInput("auction-duplicate", 100, nil))
	if !errors.Is(err, ErrDuplicateLotID) {
		t.Fatalf("duplicate CreateLot error = %v, want ErrDuplicateLotID", err)
	}
	if got := len(fixture.service.Lots()); got != 1 {
		t.Fatalf("lots len = %d, want 1", got)
	}
}

func TestPlaceBidDebitsBidderAndUpdatesCurrentBid(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-bidder")
	lot := fixture.createLot(t, "auction-bid", 100, int64Pointer(500))

	result, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-1",
	})
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}

	if result.Lot.CurrentBid != 120 || result.Lot.CurrentBidderID != fixture.bidderID {
		t.Fatalf("current bid = %d bidder %q, want 120/%q", result.Lot.CurrentBid, result.Lot.CurrentBidderID, fixture.bidderID)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 380 {
		t.Fatalf("bidder balance = %d, want 380", got)
	}
	if result.PreviousRefund != nil {
		t.Fatal("PreviousRefund != nil, want nil on first bid")
	}
}

func TestPlaceBidRefundsPreviousBidder(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-first")
	fixture.seedCredits(t, fixture.otherID, 500, "seed-second")
	lot := fixture.createLot(t, "auction-refund", 100, int64Pointer(500))
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-first")

	result, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.otherID,
		Amount:         150,
		RequestID:      "bid-second",
	})
	if err != nil {
		t.Fatalf("PlaceBid second: %v", err)
	}

	if result.PreviousRefund == nil {
		t.Fatal("PreviousRefund = nil, want refund")
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("first bidder balance = %d, want refunded 500", got)
	}
	if got := fixture.wallet.Balance(fixture.otherID, economy.CurrencyBucketCredits); got != 350 {
		t.Fatalf("second bidder balance = %d, want 350", got)
	}
	if result.Lot.CurrentBid != 150 || result.Lot.CurrentBidderID != fixture.otherID {
		t.Fatalf("current bid = %d bidder %q, want 150/%q", result.Lot.CurrentBid, result.Lot.CurrentBidderID, fixture.otherID)
	}
}

func TestPlaceBidTransactionRepositoryCommitsBidRows(t *testing.T) {
	fixture, repository := newAuctionFixtureWithLotTransactionRepository(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-tx-bidder")
	created := fixture.createLot(t, "auction-tx-bid", 100, int64Pointer(500))

	result, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      created.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-tx-1",
	})
	if err != nil {
		t.Fatalf("PlaceBid() error = %v, want nil", err)
	}

	if got := repository.transactionCount(); got != 1 {
		t.Fatalf("transaction count = %d, want 1", got)
	}
	if got := repository.lockedLotCount(); got != 1 {
		t.Fatalf("locked lot count = %d, want 1", got)
	}
	saved, ok := repository.saved(created.Lot.AuctionID)
	if !ok || saved.CurrentBid != 120 || saved.CurrentBidderID != fixture.bidderID {
		t.Fatalf("saved lot = %+v ok %v, want current bid 120 by %q", saved, ok, fixture.bidderID)
	}
	walletCommits := repository.walletCommitsSnapshot()
	if len(walletCommits) != 1 || walletCommits[0].Reference.Operation != economy.WalletMutationOperationDebit || walletCommits[0].Reference.ReferenceKey != result.ReferenceKey {
		t.Fatalf("wallet commits = %+v, want one bidder debit for %q", walletCommits, result.ReferenceKey)
	}
	row, ok := repository.idempotencyRow(economy.IdempotencyScopeEconomy, result.ReferenceKey)
	if !ok || row.Status != economy.IdempotencyStatusCompleted {
		t.Fatalf("idempotency row = %+v ok %v, want completed", row, ok)
	}
	outbox, ok := repository.outboxRow(auctionBidOutboxPrefix + result.ReferenceKey.String())
	if !ok || outbox.EventType != auctionBidPlacedEvent || outbox.AggregateID != result.Lot.AuctionID.String() {
		t.Fatalf("outbox row = %+v ok %v, want auction bid outbox", outbox, ok)
	}
}

func TestBuyNowTransactionRepositoryCommitsSettlementRows(t *testing.T) {
	fixture, repository := newAuctionFixtureWithLotTransactionRepository(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-tx-buy-now-bidder")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "seed-tx-buy-now-buyer")
	created := fixture.createLot(t, "auction-tx-buy-now", 100, int64Pointer(300))
	fixture.placeBid(t, created.Lot.AuctionID, fixture.bidderID, 120, "bid-before-tx-buy-now")
	walletCommitsBefore := len(repository.walletCommitsSnapshot())

	result, err := fixture.service.BuyNow(BuyNowInput{
		AuctionID:     created.Lot.AuctionID,
		BuyerPlayerID: fixture.buyerID,
		RequestID:     "buy-now-tx-1",
	})
	if err != nil {
		t.Fatalf("BuyNow() error = %v, want nil", err)
	}

	saved, ok := repository.saved(created.Lot.AuctionID)
	if !ok || saved.Status != LotStatusClosed || saved.WinningPlayerID != fixture.buyerID {
		t.Fatalf("saved lot = %+v ok %v, want closed winner %q", saved, ok, fixture.buyerID)
	}
	walletCommits := repository.walletCommitsSnapshot()
	if got := len(walletCommits) - walletCommitsBefore; got != 2 {
		t.Fatalf("buy-now wallet commit delta = %d, want buyer debit and current-bid refund", got)
	}
	row, ok := repository.idempotencyRow(economy.IdempotencyScopeEconomy, result.ReferenceKey)
	if !ok || row.Status != economy.IdempotencyStatusCompleted {
		t.Fatalf("idempotency row = %+v ok %v, want completed", row, ok)
	}
	outbox, ok := repository.outboxRow(auctionBuyNowOutboxPrefix + result.ReferenceKey.String())
	if !ok || outbox.EventType != auctionBuyNowEvent || outbox.AggregateID != result.Lot.AuctionID.String() {
		t.Fatalf("outbox row = %+v ok %v, want auction buy-now outbox", outbox, ok)
	}
	if grants := fixture.service.Grants(); len(grants) != 1 || grants[0].PlayerID != fixture.buyerID {
		t.Fatalf("grants = %+v, want one buyer grant", grants)
	}
}

func TestAuctionTransactionOutboxFailureRollsBackRepositoryAndWalletState(t *testing.T) {
	fixture, repository := newAuctionFixtureWithLotTransactionRepository(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-tx-rollback-bidder")
	created := fixture.createLot(t, "auction-tx-rollback", 100, nil)
	repository.failOutboxWith = errors.New("injected auction outbox failure")
	ledgerBefore := len(fixture.wallet.CurrencyLedgerEntries())

	_, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      created.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-tx-rollback",
	})
	if !errors.Is(err, repository.failOutboxWith) {
		t.Fatalf("PlaceBid() error = %v, want injected outbox failure", err)
	}

	saved, ok := repository.saved(created.Lot.AuctionID)
	if !ok || saved.CurrentBid != 0 || !saved.CurrentBidderID.IsZero() {
		t.Fatalf("saved lot after rollback = %+v ok %v, want original no-bid lot", saved, ok)
	}
	if got := len(repository.walletCommitsSnapshot()); got != 0 {
		t.Fatalf("repository wallet commits after rollback = %d, want 0", got)
	}
	referenceKey, keyErr := foundation.AuctionBidIdempotencyKey(created.Lot.AuctionID, fixture.bidderID, "bid-tx-rollback")
	if keyErr != nil {
		t.Fatalf("AuctionBidIdempotencyKey() error = %v, want nil", keyErr)
	}
	if row, ok := repository.idempotencyRow(economy.IdempotencyScopeEconomy, referenceKey); ok {
		t.Fatalf("idempotency row after rollback = %+v, want none", row)
	}
	if outbox, ok := repository.outboxRow(auctionBidOutboxPrefix + referenceKey.String()); ok {
		t.Fatalf("outbox row after rollback = %+v, want none", outbox)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance after rollback = %d, want 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerBefore {
		t.Fatalf("wallet ledger len after rollback = %d, want %d", got, ledgerBefore)
	}
	if lot, ok := fixture.service.Lot(created.Lot.AuctionID); !ok || lot.CurrentBid != 0 || !lot.CurrentBidderID.IsZero() {
		t.Fatalf("service lot after rollback = %+v ok %v, want original no-bid lot", lot, ok)
	}
}

func TestPlaceBidDuplicateRetryDoesNotDebitOrRefundTwice(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-duplicate")
	lot := fixture.createLot(t, "auction-duplicate-bid", 100, nil)
	input := PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-duplicate",
	}

	first, err := fixture.service.PlaceBid(input)
	if err != nil {
		t.Fatalf("first PlaceBid: %v", err)
	}
	ledgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	second, err := fixture.service.PlaceBid(input)
	if err != nil {
		t.Fatalf("duplicate PlaceBid: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 380 {
		t.Fatalf("bidder balance = %d, want 380", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerAfterFirst {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerAfterFirst)
	}

	input.Amount = 121
	_, err = fixture.service.PlaceBid(input)
	if !errors.Is(err, ErrBidReferenceMismatch) {
		t.Fatalf("mismatched retry error = %v, want ErrBidReferenceMismatch", err)
	}
}

func TestPlaceBidDuplicateIdempotencyRowDoesNotDebitTwice(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-bid-row")
	lot := fixture.createLot(t, "auction-bid-row", 100, nil)
	input := PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-row",
	}

	first, err := fixture.service.PlaceBid(input)
	if err != nil {
		t.Fatalf("first PlaceBid: %v", err)
	}
	ledgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	fixture.service.bidResults = make(map[foundation.IdempotencyKey]PlaceBidResult)
	second, err := fixture.service.PlaceBid(input)
	if err != nil {
		t.Fatalf("idempotency-row duplicate PlaceBid: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 380 {
		t.Fatalf("bidder balance = %d, want 380", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerAfterFirst {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerAfterFirst)
	}
	if second.Amount != 120 || second.Lot.CurrentBidderID != fixture.bidderID {
		t.Fatalf("duplicate result = %+v, want cached winning bid", second)
	}
}

func TestPlaceBidRejectsTooLowAndEndedWithoutDebit(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-too-low")
	lowLot := fixture.createLot(t, "auction-too-low", 100, nil)

	_, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      lowLot.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         99,
		RequestID:      "bid-too-low",
	})
	if !errors.Is(err, ErrBidTooLow) {
		t.Fatalf("too low bid error = %v, want ErrBidTooLow", err)
	}

	endedInput := fixture.createLotInput("auction-ended", 100, nil)
	endedInput.EndsAt = fixture.clock.Now().Add(time.Minute)
	endedLot, err := fixture.service.CreateLot(endedInput)
	if err != nil {
		t.Fatalf("CreateLot ended fixture: %v", err)
	}
	ledgerBeforeEndedBid := len(fixture.wallet.CurrencyLedgerEntries())
	fixture.clock.Advance(2 * time.Minute)
	_, err = fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      endedLot.Lot.AuctionID,
		BidderPlayerID: fixture.bidderID,
		Amount:         120,
		RequestID:      "bid-ended",
	})
	if !errors.Is(err, ErrLotEnded) {
		t.Fatalf("ended bid error = %v, want ErrLotEnded", err)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerBeforeEndedBid {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerBeforeEndedBid)
	}
}

func TestPlaceBidDebitFailureLeavesCurrentBidUnchanged(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-debit-failure")
	lot := fixture.createLot(t, "auction-debit-failure", 100, nil)
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-before-debit-failure")
	ledgerBeforeFailedBid := len(fixture.wallet.CurrencyLedgerEntries())

	_, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.otherID,
		Amount:         150,
		RequestID:      "bid-debit-failure",
	})
	if !errors.Is(err, economy.ErrInsufficientWalletFunds) {
		t.Fatalf("debit failure error = %v, want ErrInsufficientWalletFunds", err)
	}

	finalLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("final lot missing")
	}
	if finalLot.CurrentBid != 120 || finalLot.CurrentBidderID != fixture.bidderID {
		t.Fatalf("final bid/winner = %d/%q, want unchanged 120/%q", finalLot.CurrentBid, finalLot.CurrentBidderID, fixture.bidderID)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerBeforeFailedBid {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerBeforeFailedBid)
	}
}

func TestPlaceBidRefundFailureLeavesCurrentBidUnchanged(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-refund-failure-first")
	fixture.seedCredits(t, fixture.otherID, 500, "seed-refund-failure-second")
	refundErr := errors.New("forced refund failure")
	failingWallet := &failingAuctionWallet{
		WalletService:       fixture.wallet,
		failCreditPlayerID:  fixture.bidderID,
		failCreditReason:    ledgerReasonAuctionRefund,
		failCreditWithError: refundErr,
	}
	service, err := NewService(ServiceConfig{
		Clock:            fixture.clock,
		Wallet:           failingWallet,
		IdempotencyStore: fixture.idempotency,
	})
	if err != nil {
		t.Fatalf("NewService failing wallet: %v", err)
	}
	fixture.service = service
	lot := fixture.createLot(t, "auction-refund-failure", 100, nil)
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-before-refund-failure")
	ledgerBeforeFailedBid := len(fixture.wallet.CurrencyLedgerEntries())

	_, err = fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      lot.Lot.AuctionID,
		BidderPlayerID: fixture.otherID,
		Amount:         150,
		RequestID:      "bid-refund-failure",
	})
	if !errors.Is(err, refundErr) {
		t.Fatalf("refund failure error = %v, want forced refund failure", err)
	}

	finalLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("final lot missing")
	}
	if finalLot.CurrentBid != 120 || finalLot.CurrentBidderID != fixture.bidderID {
		t.Fatalf("final bid/winner = %d/%q, want unchanged 120/%q", finalLot.CurrentBid, finalLot.CurrentBidderID, fixture.bidderID)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 380 {
		t.Fatalf("previous bidder balance = %d, want still escrowed 380", got)
	}
	if got := fixture.wallet.Balance(fixture.otherID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("failed bidder balance = %d, want rollback to 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerBeforeFailedBid {
		t.Fatalf("wallet ledger len = %d, want rollback to %d", got, ledgerBeforeFailedBid)
	}
}

func TestBuyNowDebitsRefundsGrantsAndClosesOnce(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-bidder-buy-now")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "seed-buyer-buy-now")
	lot := fixture.createLot(t, "auction-buy-now", 100, int64Pointer(300))
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-before-buy-now")
	input := BuyNowInput{
		AuctionID:     lot.Lot.AuctionID,
		BuyerPlayerID: fixture.buyerID,
		RequestID:     "buy-now-1",
	}

	first, err := fixture.service.BuyNow(input)
	if err != nil {
		t.Fatalf("BuyNow: %v", err)
	}
	ledgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	second, err := fixture.service.BuyNow(input)
	if err != nil {
		t.Fatalf("duplicate BuyNow: %v", err)
	}

	if first.Lot.Status != LotStatusClosed || first.Lot.WinningPlayerID != fixture.buyerID {
		t.Fatalf("lot status/winner = %q/%q, want closed/%q", first.Lot.Status, first.Lot.WinningPlayerID, fixture.buyerID)
	}
	if first.CurrentRefund == nil {
		t.Fatal("CurrentRefund = nil, want previous bidder refund")
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 700 {
		t.Fatalf("buyer balance = %d, want 700", got)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want refunded 500", got)
	}
	if got := fixture.service.Grants(); len(got) != 1 || got[0].PlayerID != fixture.buyerID {
		t.Fatalf("grants = %+v, want one buyer grant", got)
	}
	if !second.Duplicate {
		t.Fatal("duplicate BuyNow Duplicate = false, want true")
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerAfterFirst {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerAfterFirst)
	}
	if got := len(fixture.service.Grants()); got != 1 {
		t.Fatalf("grants len = %d, want 1", got)
	}
}

func TestBuyNowDuplicateIdempotencyRowDoesNotMutateTwice(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-bidder-buy-now-row")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "seed-buyer-buy-now-row")
	lot := fixture.createLot(t, "auction-buy-now-row", 100, int64Pointer(300))
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-before-buy-now-row")
	input := BuyNowInput{
		AuctionID:     lot.Lot.AuctionID,
		BuyerPlayerID: fixture.buyerID,
		RequestID:     "buy-now-row",
	}

	first, err := fixture.service.BuyNow(input)
	if err != nil {
		t.Fatalf("first BuyNow: %v", err)
	}
	ledgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	fixture.service.buyNowResults = make(map[foundation.IdempotencyKey]BuyNowResult)
	second, err := fixture.service.BuyNow(input)
	if err != nil {
		t.Fatalf("idempotency-row duplicate BuyNow: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 700 {
		t.Fatalf("buyer balance = %d, want 700", got)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want refunded once to 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerAfterFirst {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerAfterFirst)
	}
	if got := len(fixture.service.Grants()); got != 1 {
		t.Fatalf("grants len = %d, want 1", got)
	}
}

func TestConcurrentBuyNowSameKeyClosesLotExactlyOnce(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-bidder-concurrent-buy-now")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "seed-buyer-concurrent-buy-now")
	lot := fixture.createLot(t, "auction-concurrent-buy-now", 100, int64Pointer(300))
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 120, "bid-before-concurrent-buy-now")
	ledgerBeforeBuyNow := len(fixture.wallet.CurrencyLedgerEntries())
	input := BuyNowInput{
		AuctionID:     lot.Lot.AuctionID,
		BuyerPlayerID: fixture.buyerID,
		RequestID:     "buy-now-concurrent",
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan BuyNowResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := fixture.service.BuyNow(input)
			results <- result
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent BuyNow error = %v, want nil", err)
		}
	}
	duplicates := 0
	nonDuplicates := 0
	for result := range results {
		if result.Duplicate {
			duplicates++
			continue
		}
		nonDuplicates++
		if result.Lot.Status != LotStatusClosed || result.Lot.WinningPlayerID != fixture.buyerID {
			t.Fatalf("non-duplicate lot status/winner = %q/%q, want closed/%q", result.Lot.Status, result.Lot.WinningPlayerID, fixture.buyerID)
		}
		if result.CurrentRefund == nil {
			t.Fatal("non-duplicate CurrentRefund = nil, want previous bidder refund")
		}
	}
	if nonDuplicates != 1 || duplicates != 1 {
		t.Fatalf("non-duplicates/duplicates = %d/%d, want 1/1", nonDuplicates, duplicates)
	}
	if got := fixture.wallet.Balance(fixture.buyerID, economy.CurrencyBucketCredits); got != 700 {
		t.Fatalf("buyer balance = %d, want 700", got)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want refunded 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != ledgerBeforeBuyNow+2 {
		t.Fatalf("wallet ledger len = %d, want %d", got, ledgerBeforeBuyNow+2)
	}
	if got := fixture.service.Grants(); len(got) != 1 || got[0].PlayerID != fixture.buyerID {
		t.Fatalf("grants = %+v, want one buyer grant", got)
	}
	finalLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("final lot missing")
	}
	if finalLot.Status != LotStatusClosed || finalLot.CloseReason != CloseReasonBuyNow || finalLot.WinningPlayerID != fixture.buyerID {
		t.Fatalf("final lot = %+v, want closed buy-now winner %q", finalLot, fixture.buyerID)
	}
}

func TestBidRacingBuyNowCannotCreateTwoWinners(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-race-bidder")
	fixture.seedCredits(t, fixture.buyerID, 1_000, "seed-race-buyer")
	lot := fixture.createLot(t, "auction-bid-buy-race", 100, int64Pointer(300))

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := fixture.service.PlaceBid(PlaceBidInput{
			AuctionID:      lot.Lot.AuctionID,
			BidderPlayerID: fixture.bidderID,
			Amount:         120,
			RequestID:      "bid-race",
		})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := fixture.service.BuyNow(BuyNowInput{
			AuctionID:     lot.Lot.AuctionID,
			BuyerPlayerID: fixture.buyerID,
			RequestID:     "buy-race",
		})
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrLotNotActive) {
			t.Fatalf("race loser error = %v, want ErrLotNotActive", err)
		}
	}
	if successes < 1 || successes > 2 {
		t.Fatalf("successful operations = %d, want one or two serialized operations", successes)
	}

	finalLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("final lot missing")
	}
	if finalLot.Status != LotStatusClosed || finalLot.WinningPlayerID != fixture.buyerID {
		t.Fatalf("final status/winner = %q/%q, want closed/%q", finalLot.Status, finalLot.WinningPlayerID, fixture.buyerID)
	}
	if got := fixture.service.Grants(); len(got) != 1 || got[0].PlayerID != fixture.buyerID {
		t.Fatalf("grants = %+v, want one buyer grant", got)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("bidder balance = %d, want no stuck bid 500", got)
	}
}

func TestCloseAuctionGrantsPayloadOnce(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-close")
	lot := fixture.createLot(t, "auction-close", 100, nil)
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 125, "bid-close")
	fixture.clock.Advance(time.Hour + time.Second)
	input := CloseAuctionInput{AuctionID: lot.Lot.AuctionID}

	first, err := fixture.service.CloseAuction(input)
	if err != nil {
		t.Fatalf("CloseAuction: %v", err)
	}
	second, err := fixture.service.CloseAuction(input)
	if err != nil {
		t.Fatalf("duplicate CloseAuction: %v", err)
	}

	if first.Lot.Status != LotStatusClosed || first.Lot.WinningPlayerID != fixture.bidderID {
		t.Fatalf("closed lot status/winner = %q/%q, want closed/%q", first.Lot.Status, first.Lot.WinningPlayerID, fixture.bidderID)
	}
	if first.Grant == nil || first.Grant.PlayerID != fixture.bidderID {
		t.Fatalf("close grant = %+v, want bidder grant", first.Grant)
	}
	if !second.Duplicate {
		t.Fatal("duplicate close Duplicate = false, want true")
	}
	if got := len(fixture.service.Grants()); got != 1 {
		t.Fatalf("grants len = %d, want 1", got)
	}
	if got := fixture.wallet.Balance(fixture.bidderID, economy.CurrencyBucketCredits); got != 375 {
		t.Fatalf("winner balance = %d, want settled 375", got)
	}
}

func TestCloseAuctionAfterEndedReadStillGrantsWinner(t *testing.T) {
	fixture := newAuctionFixture(t)
	fixture.seedCredits(t, fixture.bidderID, 500, "seed-close-after-read")
	lot := fixture.createLot(t, "auction-close-after-read", 100, nil)
	fixture.placeBid(t, lot.Lot.AuctionID, fixture.bidderID, 125, "bid-close-after-read")
	fixture.clock.Advance(time.Hour + time.Second)

	readLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("read lot missing")
	}
	if readLot.Status != LotStatusActive {
		t.Fatalf("read-ended lot status = %q, want active until close command settles", readLot.Status)
	}
	if got := fixture.service.Lots(); len(got) != 1 || got[0].Status != LotStatusActive {
		t.Fatalf("Lots after end = %+v, want one active-unsettled lot", got)
	}

	result, err := fixture.service.CloseAuction(CloseAuctionInput{AuctionID: lot.Lot.AuctionID})
	if err != nil {
		t.Fatalf("CloseAuction after read: %v", err)
	}
	if result.Lot.Status != LotStatusClosed || result.Grant == nil || result.Grant.PlayerID != fixture.bidderID {
		t.Fatalf("close result = %+v, want closed with bidder grant", result)
	}
	if got := len(fixture.service.Grants()); got != 1 {
		t.Fatalf("grants len = %d, want 1", got)
	}
}

func TestCloseAuctionWithoutBidsExpiresWithoutGrant(t *testing.T) {
	fixture := newAuctionFixture(t)
	lot := fixture.createLot(t, "auction-no-bids", 100, nil)
	fixture.clock.Advance(time.Hour + time.Second)

	result, err := fixture.service.CloseAuction(CloseAuctionInput{AuctionID: lot.Lot.AuctionID})
	if err != nil {
		t.Fatalf("CloseAuction no bids: %v", err)
	}

	if result.Lot.Status != LotStatusExpired || result.Lot.CloseReason != CloseReasonNoBids {
		t.Fatalf("lot status/reason = %q/%q, want expired/no_bids", result.Lot.Status, result.Lot.CloseReason)
	}
	if result.Grant != nil {
		t.Fatalf("Grant = %+v, want nil", result.Grant)
	}
	if got := len(fixture.service.Grants()); got != 0 {
		t.Fatalf("grants len = %d, want 0", got)
	}
}

func TestConcurrentFinalBidsKeepSingleWinnerAndRefundLosers(t *testing.T) {
	fixture := newAuctionFixture(t)
	lot := fixture.createLot(t, "auction-final-bids", 100, nil)
	bidders := []foundation.PlayerID{
		"bidder-0",
		"bidder-1",
		"bidder-2",
		"bidder-3",
		"bidder-4",
		"bidder-5",
		"bidder-6",
		"bidder-7",
	}
	for _, bidderID := range bidders {
		fixture.seedCredits(t, bidderID, 1_000, "seed-"+bidderID.String())
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, len(bidders))
	for index, bidderID := range bidders {
		amount := int64(100 + index)
		wg.Add(1)
		go func(bidderID foundation.PlayerID, amount int64) {
			defer wg.Done()
			<-start
			_, err := fixture.service.PlaceBid(PlaceBidInput{
				AuctionID:      lot.Lot.AuctionID,
				BidderPlayerID: bidderID,
				Amount:         amount,
				RequestID:      foundation.RequestID("bid-" + bidderID.String()),
			})
			errs <- err
		}(bidderID, amount)
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrBidTooLow) {
			t.Fatalf("concurrent bid loser error = %v, want ErrBidTooLow", err)
		}
	}
	if successes == 0 {
		t.Fatal("successful bids = 0, want at least one")
	}

	finalLot, ok := fixture.service.Lot(lot.Lot.AuctionID)
	if !ok {
		t.Fatal("final lot missing")
	}
	wantWinner := bidders[len(bidders)-1]
	if finalLot.CurrentBid != 107 || finalLot.CurrentBidderID != wantWinner {
		t.Fatalf("final bid/winner = %d/%q, want 107/%q", finalLot.CurrentBid, finalLot.CurrentBidderID, wantWinner)
	}
	for _, bidderID := range bidders[:len(bidders)-1] {
		if got := fixture.wallet.Balance(bidderID, economy.CurrencyBucketCredits); got != 1_000 {
			t.Fatalf("loser %q balance = %d, want refunded/unchanged 1000", bidderID, got)
		}
		bids := countAuctionLedgerEntries(fixture.wallet.CurrencyLedgerEntries(), bidderID, ledgerReasonAuctionBid, economy.LedgerActionDecrease)
		refunds := countAuctionLedgerEntries(fixture.wallet.CurrencyLedgerEntries(), bidderID, ledgerReasonAuctionRefund, economy.LedgerActionIncrease)
		if bids != refunds {
			t.Fatalf("loser %q bid/refund ledger counts = %d/%d, want paired exactly once", bidderID, bids, refunds)
		}
		if bids > 1 {
			t.Fatalf("loser %q bid ledger count = %d, want at most 1", bidderID, bids)
		}
	}
	if got := fixture.wallet.Balance(wantWinner, economy.CurrencyBucketCredits); got != 893 {
		t.Fatalf("winner balance = %d, want 893", got)
	}
	if bids := countAuctionLedgerEntries(fixture.wallet.CurrencyLedgerEntries(), wantWinner, ledgerReasonAuctionBid, economy.LedgerActionDecrease); bids != 1 {
		t.Fatalf("winner bid ledger count = %d, want 1", bids)
	}
	if refunds := countAuctionLedgerEntries(fixture.wallet.CurrencyLedgerEntries(), wantWinner, ledgerReasonAuctionRefund, economy.LedgerActionIncrease); refunds != 0 {
		t.Fatalf("winner refund ledger count = %d, want 0", refunds)
	}
	if got := len(fixture.service.Grants()); got != 0 {
		t.Fatalf("grants len before close = %d, want 0", got)
	}
}

func newAuctionFixture(t *testing.T) *auctionFixture {
	t.Helper()

	clock := testutil.NewFakeClock(auctionTestNow)
	wallet := economy.NewWalletService(clock)
	idempotency := newMemoryAuctionIdempotencyStore()
	service, err := NewService(ServiceConfig{
		Clock:            clock,
		Wallet:           wallet,
		IdempotencyStore: idempotency,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &auctionFixture{
		clock:       clock,
		wallet:      wallet,
		idempotency: idempotency,
		service:     service,
		worldID:     "world-1",
		bidderID:    "bidder-1",
		otherID:     "bidder-2",
		buyerID:     "buyer-1",
		payload: LotPayload{
			Type:     LotPayloadTypeShipUnlock,
			Source:   mustAuctionSource(t, "auction-slazar-unlock", "v1"),
			Quantity: 1,
		},
	}
}

func newAuctionFixtureWithLotTransactionRepository(t *testing.T) (*auctionFixture, *fakeAuctionLotTransactionRepository) {
	t.Helper()

	fixture := newAuctionFixture(t)
	repository := newFakeAuctionLotTransactionRepository()
	service, err := NewService(ServiceConfig{
		Clock:         fixture.clock,
		Wallet:        fixture.wallet,
		LotRepository: repository,
	})
	if err != nil {
		t.Fatalf("NewService(transaction repository): %v", err)
	}
	fixture.service = service
	return fixture, repository
}

func (fixture *auctionFixture) createLotInput(auctionID string, startPrice int64, buyNowPrice *int64) CreateLotInput {
	return CreateLotInput{
		AuctionID:   foundation.AuctionID(auctionID),
		WorldID:     fixture.worldID,
		Payload:     fixture.payload,
		Currency:    economy.CurrencyBucketCredits,
		StartPrice:  startPrice,
		BuyNowPrice: buyNowPrice,
		StartsAt:    fixture.clock.Now().Add(-time.Minute),
		EndsAt:      fixture.clock.Now().Add(time.Hour),
	}
}

func (fixture *auctionFixture) createLot(t *testing.T, auctionID string, startPrice int64, buyNowPrice *int64) CreateLotResult {
	t.Helper()

	result, err := fixture.service.CreateLot(fixture.createLotInput(auctionID, startPrice, buyNowPrice))
	if err != nil {
		t.Fatalf("CreateLot: %v", err)
	}
	return result
}

func (fixture *auctionFixture) placeBid(
	t *testing.T,
	auctionID foundation.AuctionID,
	bidderID foundation.PlayerID,
	amount int64,
	requestID foundation.RequestID,
) PlaceBidResult {
	t.Helper()

	result, err := fixture.service.PlaceBid(PlaceBidInput{
		AuctionID:      auctionID,
		BidderPlayerID: bidderID,
		Amount:         amount,
		RequestID:      requestID,
	})
	if err != nil {
		t.Fatalf("PlaceBid: %v", err)
	}
	return result
}

func (fixture *auctionFixture) seedCredits(t *testing.T, playerID foundation.PlayerID, amount int64, reference string) {
	t.Helper()

	_, err := fixture.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       "test_seed",
		ReferenceKey: mustQuestRewardKey(t, reference),
	})
	if err != nil {
		t.Fatalf("seed credits: %v", err)
	}
}

func mustAuctionSource(t *testing.T, lotID string, version string) catalog.VersionedDefinition {
	t.Helper()

	source, err := catalog.NewAuctionLotSource(lotID, version)
	if err != nil {
		t.Fatalf("NewAuctionLotSource: %v", err)
	}
	return source
}

func mustQuestRewardKey(t *testing.T, reference string) foundation.IdempotencyKey {
	t.Helper()

	key, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(reference))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey: %v", err)
	}
	return key
}

func int64Pointer(value int64) *int64 {
	return &value
}

type memoryAuctionIdempotencyStore struct {
	mu   sync.Mutex
	rows map[string]economy.IdempotencyKeyRow
}

func newMemoryAuctionIdempotencyStore() *memoryAuctionIdempotencyStore {
	return &memoryAuctionIdempotencyStore{rows: make(map[string]economy.IdempotencyKeyRow)}
}

func (store *memoryAuctionIdempotencyStore) ClaimIdempotencyKey(_ context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	key := auctionMemoryIdempotencyKey(row)
	existing, ok := store.rows[key]
	if !ok {
		if err := row.Validate(); err != nil {
			return economy.IdempotencyClaimResult{}, err
		}
		store.rows[key] = row.Clone()
		return economy.IdempotencyClaimResult{Row: row.Clone()}, nil
	}
	return economy.ResolveIdempotencyClaim(&existing, row)
}

func (store *memoryAuctionIdempotencyStore) CompleteIdempotencyKey(_ context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	key := auctionMemoryIdempotencyKey(row)
	existing, ok := store.rows[key]
	if ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return economy.IdempotencyKeyRow{}, err
		}
	}
	store.rows[key] = row.Clone()
	return row.Clone(), nil
}

func auctionMemoryIdempotencyKey(row economy.IdempotencyKeyRow) string {
	return row.Scope + ":" + row.Key.String()
}

type fakeAuctionLotTransactionRepository struct {
	mu               sync.Mutex
	lots             map[foundation.AuctionID]Lot
	transactionTotal int
	lockedLotTotal   int
	walletCommits    []economy.WalletMutationCommit
	idempotencyRows  map[string]economy.IdempotencyKeyRow
	outboxRows       map[string]economy.OutboxRow
	failOutboxWith   error
}

func newFakeAuctionLotTransactionRepository() *fakeAuctionLotTransactionRepository {
	return &fakeAuctionLotTransactionRepository{
		lots:            make(map[foundation.AuctionID]Lot),
		idempotencyRows: make(map[string]economy.IdempotencyKeyRow),
		outboxRows:      make(map[string]economy.OutboxRow),
	}
}

func (repository *fakeAuctionLotTransactionRepository) SaveAuctionLot(_ context.Context, lot Lot) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.lots[lot.AuctionID] = cloneLot(lot)
	return nil
}

func (repository *fakeAuctionLotTransactionRepository) WithAuctionLotTransaction(
	_ context.Context,
	fn func(AuctionLotTransaction) error,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.transactionTotal++
	lotsSnapshot := cloneAuctionLotMap(repository.lots)
	walletSnapshot := append([]economy.WalletMutationCommit(nil), repository.walletCommits...)
	idempotencySnapshot := cloneAuctionIdempotencyRows(repository.idempotencyRows)
	outboxSnapshot := cloneAuctionOutboxRows(repository.outboxRows)

	tx := &fakeAuctionLotTransaction{repository: repository}
	if err := fn(tx); err != nil {
		repository.lots = lotsSnapshot
		repository.walletCommits = walletSnapshot
		repository.idempotencyRows = idempotencySnapshot
		repository.outboxRows = outboxSnapshot
		return err
	}
	return nil
}

func (repository *fakeAuctionLotTransactionRepository) saved(auctionID foundation.AuctionID) (Lot, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	lot, ok := repository.lots[auctionID]
	return cloneLot(lot), ok
}

func (repository *fakeAuctionLotTransactionRepository) transactionCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return repository.transactionTotal
}

func (repository *fakeAuctionLotTransactionRepository) lockedLotCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return repository.lockedLotTotal
}

func (repository *fakeAuctionLotTransactionRepository) walletCommitsSnapshot() []economy.WalletMutationCommit {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return append([]economy.WalletMutationCommit(nil), repository.walletCommits...)
}

func (repository *fakeAuctionLotTransactionRepository) idempotencyRow(scope string, key foundation.IdempotencyKey) (economy.IdempotencyKeyRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.idempotencyRows[auctionFakeIdempotencyKey(scope, key)]
	return row.Clone(), ok
}

func (repository *fakeAuctionLotTransactionRepository) outboxRow(outboxID string) (economy.OutboxRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.outboxRows[outboxID]
	return row.Clone(), ok
}

type fakeAuctionLotTransaction struct {
	repository *fakeAuctionLotTransactionRepository
}

func (tx *fakeAuctionLotTransaction) LoadAuctionLotForUpdate(_ context.Context, auctionID foundation.AuctionID) (Lot, bool, error) {
	tx.repository.lockedLotTotal++
	lot, ok := tx.repository.lots[auctionID]
	return cloneLot(lot), ok, nil
}

func (tx *fakeAuctionLotTransaction) SaveAuctionLot(_ context.Context, lot Lot) error {
	tx.repository.lots[lot.AuctionID] = cloneLot(lot)
	return nil
}

func (tx *fakeAuctionLotTransaction) CommitWalletMutation(_ context.Context, commit economy.WalletMutationCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx.repository.walletCommits = append(tx.repository.walletCommits, commit)
	return nil
}

func (tx *fakeAuctionLotTransaction) ClaimIdempotencyKey(
	_ context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	key := auctionFakeIdempotencyKey(row.Scope, row.Key)
	existing, ok := tx.repository.idempotencyRows[key]
	if !ok {
		if err := row.Validate(); err != nil {
			return economy.IdempotencyClaimResult{}, err
		}
		tx.repository.idempotencyRows[key] = row.Clone()
		return economy.IdempotencyClaimResult{Row: row.Clone()}, nil
	}
	return economy.ResolveIdempotencyClaim(&existing, row)
}

func (tx *fakeAuctionLotTransaction) CompleteIdempotencyKey(
	_ context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	key := auctionFakeIdempotencyKey(row.Scope, row.Key)
	if existing, ok := tx.repository.idempotencyRows[key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return economy.IdempotencyKeyRow{}, err
		}
	}
	tx.repository.idempotencyRows[key] = row.Clone()
	return row.Clone(), nil
}

func (tx *fakeAuctionLotTransaction) InsertOutboxRow(_ context.Context, row economy.OutboxRow) error {
	if tx.repository.failOutboxWith != nil {
		return tx.repository.failOutboxWith
	}
	if err := row.Validate(); err != nil {
		return err
	}
	tx.repository.outboxRows[row.OutboxID] = row.Clone()
	return nil
}

func auctionFakeIdempotencyKey(scope string, key foundation.IdempotencyKey) string {
	return scope + ":" + key.String()
}

func cloneAuctionLotMap(input map[foundation.AuctionID]Lot) map[foundation.AuctionID]Lot {
	out := make(map[foundation.AuctionID]Lot, len(input))
	for key, value := range input {
		out[key] = cloneLot(value)
	}
	return out
}

func cloneAuctionIdempotencyRows(input map[string]economy.IdempotencyKeyRow) map[string]economy.IdempotencyKeyRow {
	out := make(map[string]economy.IdempotencyKeyRow, len(input))
	for key, value := range input {
		out[key] = value.Clone()
	}
	return out
}

func cloneAuctionOutboxRows(input map[string]economy.OutboxRow) map[string]economy.OutboxRow {
	out := make(map[string]economy.OutboxRow, len(input))
	for key, value := range input {
		out[key] = value.Clone()
	}
	return out
}

type failingAuctionWallet struct {
	*economy.WalletService
	failCreditPlayerID  foundation.PlayerID
	failCreditReason    economy.LedgerReason
	failCreditWithError error
}

func (wallet *failingAuctionWallet) CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error) {
	if input.PlayerID == wallet.failCreditPlayerID && input.Reason == wallet.failCreditReason {
		return economy.CreditWalletResult{}, wallet.failCreditWithError
	}
	return wallet.WalletService.CreditWallet(input)
}

func countAuctionLedgerEntries(
	entries []economy.CurrencyLedgerEntry,
	playerID foundation.PlayerID,
	reason economy.LedgerReason,
	action economy.LedgerAction,
) int {
	count := 0
	for _, entry := range entries {
		if entry.PlayerID == playerID && entry.Reason == reason && entry.Action == action {
			count++
		}
	}
	return count
}
