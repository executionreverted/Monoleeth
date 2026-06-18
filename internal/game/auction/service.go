package auction

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const (
	ledgerReasonAuctionBid    economy.LedgerReason = "auction_bid"
	ledgerReasonAuctionRefund economy.LedgerReason = "auction_refund"
	ledgerReasonAuctionBuyNow economy.LedgerReason = "auction_buy_now"
)

// WalletService is the economy wallet boundary used by auctions.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	Balance(playerID foundation.PlayerID, currency economy.CurrencyBucket) int64
}

// ServiceConfig wires Service to economy primitives.
type ServiceConfig struct {
	Clock  foundation.Clock
	Wallet WalletService
}

// Service owns in-memory auction lot state for the Phase 10 MVP.
type Service struct {
	mu     sync.Mutex
	clock  foundation.Clock
	wallet WalletService

	lots          map[foundation.AuctionID]Lot
	bidResults    map[foundation.IdempotencyKey]PlaceBidResult
	buyNowResults map[foundation.IdempotencyKey]BuyNowResult
	closeResults  map[foundation.IdempotencyKey]CloseAuctionResult
	grants        []Grant
}

// NewService returns a concurrency-safe in-memory auction service.
func NewService(config ServiceConfig) (*Service, error) {
	if config.Wallet == nil {
		return nil, ErrMissingWalletService
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &Service{
		clock:         clock,
		wallet:        config.Wallet,
		lots:          make(map[foundation.AuctionID]Lot),
		bidResults:    make(map[foundation.IdempotencyKey]PlaceBidResult),
		buyNowResults: make(map[foundation.IdempotencyKey]BuyNowResult),
		closeResults:  make(map[foundation.IdempotencyKey]CloseAuctionResult),
	}, nil
}

// CreateLot validates and inserts one server-authored lot.
func (service *Service) CreateLot(input CreateLotInput) (CreateLotResult, error) {
	if err := input.validate(service.clock.Now()); err != nil {
		return CreateLotResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if _, exists := service.lots[input.AuctionID]; exists {
		return CreateLotResult{}, fmt.Errorf("auction %q: %w", input.AuctionID, ErrDuplicateLotID)
	}

	now := service.clock.Now()
	lot := Lot{
		AuctionID:   input.AuctionID,
		WorldID:     input.WorldID,
		Payload:     clonePayload(input.Payload),
		Currency:    input.Currency,
		StartPrice:  input.StartPrice,
		BuyNowPrice: cloneInt64Pointer(input.BuyNowPrice),
		Status:      statusForTime(input.StartsAt, input.EndsAt, now),
		StartsAt:    input.StartsAt,
		EndsAt:      input.EndsAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	service.lots[input.AuctionID] = cloneLot(lot)
	return CreateLotResult{Lot: cloneLot(lot)}, nil
}

// PlaceBid debits the bidder, refunds the previous bidder if needed, and updates the current bid.
func (service *Service) PlaceBid(input PlaceBidInput) (PlaceBidResult, error) {
	if err := input.validate(); err != nil {
		return PlaceBidResult{}, err
	}
	referenceKey, err := foundation.AuctionBidIdempotencyKey(input.AuctionID, input.BidderPlayerID, input.RequestID)
	if err != nil {
		return PlaceBidResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.bidResults[referenceKey]; ok {
		if previous.Amount != input.Amount {
			return PlaceBidResult{}, fmt.Errorf("reference %q amount %d want %d: %w", referenceKey, input.Amount, previous.Amount, ErrBidReferenceMismatch)
		}
		result := clonePlaceBidResult(previous)
		result.Duplicate = true
		return result, nil
	}

	lot, ok := service.lots[input.AuctionID]
	if !ok {
		return PlaceBidResult{}, fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound)
	}
	if err := service.prepareActiveLotLocked(&lot); err != nil {
		return PlaceBidResult{}, err
	}
	if lot.CurrentBidderID == input.BidderPlayerID {
		return PlaceBidResult{}, ErrCurrentWinningBidder
	}
	if err := validateBidAmount(lot, input.Amount); err != nil {
		return PlaceBidResult{}, err
	}
	if err := service.validateDebitCapacity(input.BidderPlayerID, lot.Currency, input.Amount); err != nil {
		return PlaceBidResult{}, err
	}

	var refundReference foundation.IdempotencyKey
	var previousRefund *economy.CreditWalletResult
	if !lot.CurrentBidderID.IsZero() {
		refundReference, err = foundation.AuctionRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
		if err != nil {
			return PlaceBidResult{}, err
		}
		if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
			return PlaceBidResult{}, err
		}
	}

	bidderDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.BidderPlayerID,
		Currency:     lot.Currency,
		Amount:       input.Amount,
		Reason:       ledgerReasonAuctionBid,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return PlaceBidResult{}, err
	}
	if !lot.CurrentBidderID.IsZero() {
		refund, err := service.wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     lot.CurrentBidderID,
			Currency:     lot.Currency,
			Amount:       lot.CurrentBid,
			Reason:       ledgerReasonAuctionRefund,
			ReferenceKey: refundReference,
		})
		if err != nil {
			return PlaceBidResult{}, err
		}
		previousRefund = &refund
	}

	lot.CurrentBid = input.Amount
	lot.CurrentBidderID = input.BidderPlayerID
	lot.UpdatedAt = service.clock.Now()
	service.lots[input.AuctionID] = cloneLot(lot)

	result := PlaceBidResult{
		Lot:             cloneLot(lot),
		Amount:          input.Amount,
		BidderDebit:     bidderDebit,
		PreviousRefund:  previousRefund,
		ReferenceKey:    referenceKey,
		RefundReference: refundReference,
	}
	service.bidResults[referenceKey] = clonePlaceBidResult(result)
	return result, nil
}

// BuyNow closes a lot immediately, debits the buyer, refunds any current bidder, and grants the payload.
func (service *Service) BuyNow(input BuyNowInput) (BuyNowResult, error) {
	if err := input.validate(); err != nil {
		return BuyNowResult{}, err
	}
	referenceKey, err := foundation.AuctionBuyNowIdempotencyKey(input.AuctionID, input.BuyerPlayerID, input.RequestID)
	if err != nil {
		return BuyNowResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.buyNowResults[referenceKey]; ok {
		if previous.Lot.AuctionID != input.AuctionID || previous.Grant.PlayerID != input.BuyerPlayerID {
			return BuyNowResult{}, fmt.Errorf("reference %q: %w", referenceKey, ErrBuyNowReferenceMismatch)
		}
		result := cloneBuyNowResult(previous)
		result.Duplicate = true
		return result, nil
	}

	lot, ok := service.lots[input.AuctionID]
	if !ok {
		return BuyNowResult{}, fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound)
	}
	if err := service.prepareActiveLotLocked(&lot); err != nil {
		return BuyNowResult{}, err
	}
	if lot.BuyNowPrice == nil {
		return BuyNowResult{}, ErrBuyNowUnavailable
	}
	price := *lot.BuyNowPrice
	if err := service.validateDebitCapacity(input.BuyerPlayerID, lot.Currency, price); err != nil {
		return BuyNowResult{}, err
	}

	var refundReference foundation.IdempotencyKey
	var currentRefund *economy.CreditWalletResult
	if !lot.CurrentBidderID.IsZero() {
		refundReference, err = foundation.AuctionBuyNowRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
		if err != nil {
			return BuyNowResult{}, err
		}
		if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
			return BuyNowResult{}, err
		}
	}

	buyerDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.BuyerPlayerID,
		Currency:     lot.Currency,
		Amount:       price,
		Reason:       ledgerReasonAuctionBuyNow,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return BuyNowResult{}, err
	}
	if !lot.CurrentBidderID.IsZero() {
		refund, err := service.wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     lot.CurrentBidderID,
			Currency:     lot.Currency,
			Amount:       lot.CurrentBid,
			Reason:       ledgerReasonAuctionRefund,
			ReferenceKey: refundReference,
		})
		if err != nil {
			return BuyNowResult{}, err
		}
		currentRefund = &refund
	}

	grant := service.grantPayloadLocked(input.AuctionID, input.BuyerPlayerID, lot.Payload, CloseReasonBuyNow)
	now := service.clock.Now()
	lot.Status = LotStatusClosed
	lot.WinningPlayerID = input.BuyerPlayerID
	lot.CloseReason = CloseReasonBuyNow
	lot.ClosedAt = &now
	lot.UpdatedAt = now
	service.lots[input.AuctionID] = cloneLot(lot)

	result := BuyNowResult{
		Lot:             cloneLot(lot),
		Price:           price,
		BuyerDebit:      buyerDebit,
		CurrentRefund:   currentRefund,
		Grant:           cloneGrant(grant),
		ReferenceKey:    referenceKey,
		RefundReference: refundReference,
	}
	service.buyNowResults[referenceKey] = cloneBuyNowResult(result)
	return result, nil
}

// CloseAuction settles an ended lot or a trusted server-forced close.
func (service *Service) CloseAuction(input CloseAuctionInput) (CloseAuctionResult, error) {
	if err := input.validate(); err != nil {
		return CloseAuctionResult{}, err
	}
	referenceKey, err := foundation.AuctionCloseIdempotencyKey(input.AuctionID)
	if err != nil {
		return CloseAuctionResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.closeResults[referenceKey]; ok {
		if previous.Lot.AuctionID != input.AuctionID {
			return CloseAuctionResult{}, fmt.Errorf("reference %q: %w", referenceKey, ErrCloseReferenceMismatch)
		}
		result := cloneCloseAuctionResult(previous)
		result.Duplicate = true
		return result, nil
	}

	lot, ok := service.lots[input.AuctionID]
	if !ok {
		return CloseAuctionResult{}, fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound)
	}
	now := service.clock.Now()
	if lot.Status.IsTerminal() {
		return CloseAuctionResult{}, fmt.Errorf("auction %q status %q: %w", input.AuctionID, lot.Status, ErrLotNotActive)
	}
	if !input.Force && now.Before(lot.EndsAt) {
		return CloseAuctionResult{}, ErrCloseTooEarly
	}

	var grant *Grant
	if lot.CurrentBidderID.IsZero() {
		lot.Status = LotStatusExpired
		lot.CloseReason = CloseReasonNoBids
	} else {
		createdGrant := service.grantPayloadLocked(input.AuctionID, lot.CurrentBidderID, lot.Payload, CloseReasonEnded)
		grant = &createdGrant
		lot.Status = LotStatusClosed
		lot.WinningPlayerID = lot.CurrentBidderID
		lot.CloseReason = CloseReasonEnded
	}
	lot.ClosedAt = &now
	lot.UpdatedAt = now
	service.lots[input.AuctionID] = cloneLot(lot)

	result := CloseAuctionResult{
		Lot:          cloneLot(lot),
		Grant:        grant,
		ReferenceKey: referenceKey,
	}
	service.closeResults[referenceKey] = cloneCloseAuctionResult(result)
	return result, nil
}

// Lot returns one lot snapshot.
func (service *Service) Lot(auctionID foundation.AuctionID) (Lot, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	lot, ok := service.lots[auctionID]
	if !ok {
		return Lot{}, false
	}
	service.refreshLotStatusLocked(&lot)
	service.lots[auctionID] = cloneLot(lot)
	return cloneLot(lot), true
}

// Lots returns all lot snapshots sorted by id.
func (service *Service) Lots() []Lot {
	service.mu.Lock()
	defer service.mu.Unlock()

	lots := make([]Lot, 0, len(service.lots))
	for id, lot := range service.lots {
		service.refreshLotStatusLocked(&lot)
		service.lots[id] = cloneLot(lot)
		lots = append(lots, cloneLot(lot))
	}
	sort.Slice(lots, func(i, j int) bool {
		return lots[i].AuctionID < lots[j].AuctionID
	})
	return lots
}

// Grants returns all skeleton grant snapshots sorted by auction and player.
func (service *Service) Grants() []Grant {
	service.mu.Lock()
	defer service.mu.Unlock()

	grants := make([]Grant, 0, len(service.grants))
	for _, grant := range service.grants {
		grants = append(grants, cloneGrant(grant))
	}
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].AuctionID != grants[j].AuctionID {
			return grants[i].AuctionID < grants[j].AuctionID
		}
		return grants[i].PlayerID < grants[j].PlayerID
	})
	return grants
}

func (input CreateLotInput) validate(now time.Time) error {
	if err := input.AuctionID.Validate(); err != nil {
		return err
	}
	if err := input.WorldID.Validate(); err != nil {
		return err
	}
	if err := input.Payload.validate(); err != nil {
		return err
	}
	if err := input.Currency.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewMoney(input.StartPrice); err != nil {
		return err
	}
	if input.BuyNowPrice != nil {
		if _, err := foundation.NewMoney(*input.BuyNowPrice); err != nil {
			return err
		}
		if *input.BuyNowPrice < input.StartPrice {
			return fmt.Errorf("buy now %d start %d: %w", *input.BuyNowPrice, input.StartPrice, ErrBuyNowUnavailable)
		}
	}
	if input.StartsAt.IsZero() || input.EndsAt.IsZero() || !input.EndsAt.After(input.StartsAt) {
		return ErrInvalidLotTiming
	}
	if !input.EndsAt.After(now) {
		return fmt.Errorf("ends_at %s: %w", input.EndsAt, ErrInvalidLotTiming)
	}
	return nil
}

func (input PlaceBidInput) validate() error {
	if err := input.AuctionID.Validate(); err != nil {
		return err
	}
	if err := input.BidderPlayerID.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewMoney(input.Amount); err != nil {
		return err
	}
	if err := input.RequestID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input BuyNowInput) validate() error {
	if err := input.AuctionID.Validate(); err != nil {
		return err
	}
	if err := input.BuyerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.RequestID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input CloseAuctionInput) validate() error {
	return input.AuctionID.Validate()
}

func (service *Service) prepareActiveLotLocked(lot *Lot) error {
	now := service.clock.Now()
	service.refreshLotStatusLocked(lot)
	if now.Before(lot.StartsAt) {
		return ErrLotNotStarted
	}
	if !now.Before(lot.EndsAt) {
		return ErrLotEnded
	}
	if lot.Status != LotStatusActive {
		return fmt.Errorf("auction %q status %q: %w", lot.AuctionID, lot.Status, ErrLotNotActive)
	}
	return nil
}

func (service *Service) refreshLotStatusLocked(lot *Lot) {
	if lot.Status.IsTerminal() {
		return
	}
	now := service.clock.Now()
	next := statusForTime(lot.StartsAt, lot.EndsAt, now)
	if lot.Status == next {
		return
	}
	if lot.Status.CanTransitionTo(next) {
		lot.Status = next
		lot.UpdatedAt = now
	}
}

func (service *Service) validateDebitCapacity(playerID foundation.PlayerID, currency economy.CurrencyBucket, amount int64) error {
	current := service.wallet.Balance(playerID, currency)
	if current < amount {
		return fmt.Errorf("have %d need %d: %w", current, amount, economy.ErrInsufficientWalletFunds)
	}
	return nil
}

func (service *Service) validateCreditCapacity(playerID foundation.PlayerID, currency economy.CurrencyBucket, amount int64) error {
	if amount <= 0 {
		return nil
	}
	current := service.wallet.Balance(playerID, currency)
	if amount > math.MaxInt64-current {
		return ErrAuctionAmountOverflow
	}
	return nil
}

func validateBidAmount(lot Lot, amount int64) error {
	if amount < lot.StartPrice {
		return fmt.Errorf("amount %d start %d: %w", amount, lot.StartPrice, ErrBidTooLow)
	}
	if !lot.CurrentBidderID.IsZero() && amount <= lot.CurrentBid {
		return fmt.Errorf("amount %d current %d: %w", amount, lot.CurrentBid, ErrBidTooLow)
	}
	if lot.BuyNowPrice != nil && amount >= *lot.BuyNowPrice {
		return fmt.Errorf("amount %d buy now %d: %w", amount, *lot.BuyNowPrice, ErrBidReachesBuyNow)
	}
	return nil
}

func (service *Service) grantPayloadLocked(auctionID foundation.AuctionID, playerID foundation.PlayerID, payload LotPayload, reason CloseReason) Grant {
	grant := Grant{
		AuctionID: auctionID,
		PlayerID:  playerID,
		Payload:   clonePayload(payload),
		Reason:    reason,
		GrantedAt: service.clock.Now(),
	}
	service.grants = append(service.grants, cloneGrant(grant))
	return grant
}

func statusForTime(startsAt time.Time, endsAt time.Time, now time.Time) LotStatus {
	if now.Before(startsAt) {
		return LotStatusUpcoming
	}
	if !now.Before(endsAt) {
		return LotStatusExpired
	}
	return LotStatusActive
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func clonePlaceBidResult(result PlaceBidResult) PlaceBidResult {
	result.Lot = cloneLot(result.Lot)
	if result.PreviousRefund != nil {
		refund := *result.PreviousRefund
		result.PreviousRefund = &refund
	}
	return result
}

func cloneBuyNowResult(result BuyNowResult) BuyNowResult {
	result.Lot = cloneLot(result.Lot)
	if result.CurrentRefund != nil {
		refund := *result.CurrentRefund
		result.CurrentRefund = &refund
	}
	result.Grant = cloneGrant(result.Grant)
	return result
}

func cloneCloseAuctionResult(result CloseAuctionResult) CloseAuctionResult {
	result.Lot = cloneLot(result.Lot)
	if result.Grant != nil {
		grant := cloneGrant(*result.Grant)
		result.Grant = &grant
	}
	return result
}
