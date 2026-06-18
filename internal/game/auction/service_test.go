package auction

import (
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
	clock    *testutil.FakeClock
	wallet   *economy.WalletService
	service  *Service
	worldID  foundation.WorldID
	bidderID foundation.PlayerID
	otherID  foundation.PlayerID
	buyerID  foundation.PlayerID
	payload  LotPayload
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
	}
	if got := fixture.wallet.Balance(wantWinner, economy.CurrencyBucketCredits); got != 893 {
		t.Fatalf("winner balance = %d, want 893", got)
	}
	if got := len(fixture.service.Grants()); got != 0 {
		t.Fatalf("grants len before close = %d, want 0", got)
	}
}

func newAuctionFixture(t *testing.T) *auctionFixture {
	t.Helper()

	clock := testutil.NewFakeClock(auctionTestNow)
	wallet := economy.NewWalletService(clock)
	service, err := NewService(ServiceConfig{
		Clock:  clock,
		Wallet: wallet,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return &auctionFixture{
		clock:    clock,
		wallet:   wallet,
		service:  service,
		worldID:  "world-1",
		bidderID: "bidder-1",
		otherID:  "bidder-2",
		buyerID:  "buyer-1",
		payload: LotPayload{
			Type:     LotPayloadTypeShipUnlock,
			Source:   mustAuctionSource(t, "auction-slazar-unlock", "v1"),
			Quantity: 1,
		},
	}
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
