package auction

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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

	auctionBidOperation    = "auction_bid"
	auctionBuyNowOperation = "auction_buy_now"

	auctionOutboxTopic        = "economy"
	auctionAggregateType      = "auction_lot"
	auctionBidPlacedEvent     = "auction.bid_placed"
	auctionBuyNowEvent        = "auction.buy_now"
	auctionBidOutboxPrefix    = "auction_bid:"
	auctionBuyNowOutboxPrefix = "auction_buy_now:"
)

// WalletService is the economy wallet boundary used by auctions.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	Balance(playerID foundation.PlayerID, currency economy.CurrencyBucket) int64
}

type transactionWalletService interface {
	DebitWalletWithoutRepository(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWalletWithoutRepository(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
}

// AuctionLotRepository persists lot snapshots when durable storage is wired.
type AuctionLotRepository interface {
	SaveAuctionLot(ctx context.Context, lot Lot) error
}

// AuctionLotTransactionRepository commits auction settlement rows through one
// durable transaction when contentdb is configured.
type AuctionLotTransactionRepository interface {
	AuctionLotRepository
	WithAuctionLotTransaction(ctx context.Context, fn func(AuctionLotTransaction) error) error
}

// AuctionLotTransaction is the single-transaction seam for auction settlement
// and the economy rows it owns.
type AuctionLotTransaction interface {
	LoadAuctionLotForUpdate(ctx context.Context, auctionID foundation.AuctionID) (Lot, bool, error)
	SaveAuctionLot(ctx context.Context, lot Lot) error
	CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error
	ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error)
	CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error)
	InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error
}

// ServiceConfig wires Service to economy primitives.
type ServiceConfig struct {
	Clock                    foundation.Clock
	Wallet                   WalletService
	LotRepository            AuctionLotRepository
	LotTransactionRepository AuctionLotTransactionRepository
	IdempotencyStore         economy.IdempotencyStore
}

// Service owns in-memory auction lot state for the Phase 10 MVP.
type Service struct {
	mu               sync.Mutex
	clock            foundation.Clock
	wallet           WalletService
	lotRepository    AuctionLotRepository
	lotTransactions  AuctionLotTransactionRepository
	idempotencyStore economy.IdempotencyStore

	lots                  map[foundation.AuctionID]Lot
	bidResults            map[foundation.IdempotencyKey]PlaceBidResult
	buyNowResults         map[foundation.IdempotencyKey]BuyNowResult
	closeResults          map[foundation.IdempotencyKey]CloseAuctionResult
	bidIdempotencyRows    map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	buyNowIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	grants                []Grant
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
	lotRepository := config.LotRepository
	lotTransactions := config.LotTransactionRepository
	if lotTransactions == nil {
		if configured, ok := lotRepository.(AuctionLotTransactionRepository); ok {
			lotTransactions = configured
		}
	}
	if lotRepository == nil && lotTransactions != nil {
		lotRepository = lotTransactions
	}
	return &Service{
		clock:                 clock,
		wallet:                config.Wallet,
		lotRepository:         lotRepository,
		lotTransactions:       lotTransactions,
		idempotencyStore:      config.IdempotencyStore,
		lots:                  make(map[foundation.AuctionID]Lot),
		bidResults:            make(map[foundation.IdempotencyKey]PlaceBidResult),
		buyNowResults:         make(map[foundation.IdempotencyKey]BuyNowResult),
		closeResults:          make(map[foundation.IdempotencyKey]CloseAuctionResult),
		bidIdempotencyRows:    make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
		buyNowIdempotencyRows: make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
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
	if err := service.saveLotSnapshot(lot); err != nil {
		return CreateLotResult{}, err
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
	requestHash := auctionBidRequestHash(input)

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
	if service.auctionLotTransactionRepository() != nil {
		return service.placeBidWithTransactionLocked(input, referenceKey, requestHash)
	}
	idempotencyRow, duplicateResult, duplicate, err := service.claimAuctionBidIdempotency(input, referenceKey, requestHash)
	if err != nil {
		return PlaceBidResult{}, err
	}
	if duplicate {
		return duplicateResult, nil
	}

	lot, ok := service.lots[input.AuctionID]
	if !ok {
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound))
	}
	if err := service.prepareActiveLotLocked(&lot); err != nil {
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
	}
	if lot.CurrentBidderID == input.BidderPlayerID {
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, ErrCurrentWinningBidder)
	}
	if err := validateBidAmount(lot, input.Amount); err != nil {
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
	}
	if err := service.validateDebitCapacity(input.BidderPlayerID, lot.Currency, input.Amount); err != nil {
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
	}

	var refundReference foundation.IdempotencyKey
	var previousRefund *economy.CreditWalletResult
	if !lot.CurrentBidderID.IsZero() {
		refundReference, err = foundation.AuctionRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
		if err != nil {
			return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
		}
		if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
			return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
		}
	}

	walletSnapshot, restoreWallet, hasWalletSnapshot := service.snapshotWalletMutationState()
	bidderDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.BidderPlayerID,
		Currency:     lot.Currency,
		Amount:       input.Amount,
		Reason:       ledgerReasonAuctionBid,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		if hasWalletSnapshot {
			restoreWallet(walletSnapshot)
		}
		return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
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
			if hasWalletSnapshot {
				restoreWallet(walletSnapshot)
			}
			return PlaceBidResult{}, service.failAuctionBidIdempotency(idempotencyRow, err)
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
	if err := service.completeAuctionBidIdempotency(idempotencyRow, result); err != nil {
		return PlaceBidResult{}, err
	}
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
	requestHash := auctionBuyNowRequestHash(input)

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
	if service.auctionLotTransactionRepository() != nil {
		return service.buyNowWithTransactionLocked(input, referenceKey, requestHash)
	}
	idempotencyRow, duplicateResult, duplicate, err := service.claimAuctionBuyNowIdempotency(input, referenceKey, requestHash)
	if err != nil {
		return BuyNowResult{}, err
	}
	if duplicate {
		return duplicateResult, nil
	}

	lot, ok := service.lots[input.AuctionID]
	if !ok {
		return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound))
	}
	if err := service.prepareActiveLotLocked(&lot); err != nil {
		return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
	}
	if lot.BuyNowPrice == nil {
		return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, ErrBuyNowUnavailable)
	}
	price := *lot.BuyNowPrice
	if err := service.validateDebitCapacity(input.BuyerPlayerID, lot.Currency, price); err != nil {
		return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
	}

	var refundReference foundation.IdempotencyKey
	var currentRefund *economy.CreditWalletResult
	if !lot.CurrentBidderID.IsZero() {
		refundReference, err = foundation.AuctionBuyNowRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
		if err != nil {
			return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
		}
		if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
			return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
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
		return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
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
			return BuyNowResult{}, service.failAuctionBuyNowIdempotency(idempotencyRow, err)
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
	if err := service.completeAuctionBuyNowIdempotency(idempotencyRow, result); err != nil {
		return BuyNowResult{}, err
	}
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
	if lot.Status != LotStatusUpcoming || now.Before(lot.StartsAt) || !now.Before(lot.EndsAt) {
		return
	}
	lot.Status = LotStatusActive
	lot.UpdatedAt = now
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

type walletMutationSnapshotter interface {
	SnapshotMutationState() economy.WalletMutationSnapshot
	RestoreMutationState(economy.WalletMutationSnapshot)
}

func (service *Service) snapshotWalletMutationState() (economy.WalletMutationSnapshot, func(economy.WalletMutationSnapshot), bool) {
	snapshotter, ok := service.wallet.(walletMutationSnapshotter)
	if !ok {
		return economy.WalletMutationSnapshot{}, nil, false
	}
	return snapshotter.SnapshotMutationState(), snapshotter.RestoreMutationState, true
}

func (service *Service) saveLotSnapshot(lot Lot) error {
	if service == nil || service.lotRepository == nil {
		return nil
	}
	return service.lotRepository.SaveAuctionLot(context.Background(), cloneLot(lot))
}

func (service *Service) auctionLotTransactionRepository() AuctionLotTransactionRepository {
	if service == nil {
		return nil
	}
	return service.lotTransactions
}

func (service *Service) transactionWallet() (transactionWalletService, error) {
	wallet, ok := service.wallet.(transactionWalletService)
	if !ok {
		return nil, errors.New("auction transaction repository requires wallet no-repository mutation methods")
	}
	return wallet, nil
}

func (service *Service) placeBidWithTransactionLocked(
	input PlaceBidInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (PlaceBidResult, error) {
	repository := service.auctionLotTransactionRepository()
	if repository == nil {
		return PlaceBidResult{}, errors.New("auction lot transaction repository missing")
	}
	wallet, err := service.transactionWallet()
	if err != nil {
		return PlaceBidResult{}, err
	}
	walletSnapshot, restoreWallet, hasWalletSnapshot := service.snapshotWalletMutationState()
	if !hasWalletSnapshot {
		return PlaceBidResult{}, errors.New("auction transaction repository requires wallet mutation snapshots")
	}
	idempotencyRow := service.auctionBidIdempotencyCandidate(input, referenceKey, requestHash)

	var result PlaceBidResult
	var completedRow economy.IdempotencyKeyRow
	var outboxRow economy.OutboxRow
	var duplicate bool
	err = repository.WithAuctionLotTransaction(context.Background(), func(tx AuctionLotTransaction) error {
		locked, ok, err := tx.LoadAuctionLotForUpdate(context.Background(), input.AuctionID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound)
		}
		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return auctionBidIdempotencyClaimError(referenceKey, err)
		}
		claimedRow, duplicateResult, isDuplicate, err := auctionBidResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			result = clonePlaceBidResult(duplicateResult)
			duplicate = true
			return nil
		}

		lot := cloneLot(locked)
		if err := service.prepareActiveLotLocked(&lot); err != nil {
			return err
		}
		if lot.CurrentBidderID == input.BidderPlayerID {
			return ErrCurrentWinningBidder
		}
		if err := validateBidAmount(lot, input.Amount); err != nil {
			return err
		}
		if err := service.validateDebitCapacity(input.BidderPlayerID, lot.Currency, input.Amount); err != nil {
			return err
		}

		var refundReference foundation.IdempotencyKey
		var previousRefund *economy.CreditWalletResult
		if !lot.CurrentBidderID.IsZero() {
			refundReference, err = foundation.AuctionRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
			if err != nil {
				return err
			}
			if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
				return err
			}
		}

		bidderDebit, err := wallet.DebitWalletWithoutRepository(economy.DebitWalletInput{
			PlayerID:     input.BidderPlayerID,
			Currency:     lot.Currency,
			Amount:       input.Amount,
			Reason:       ledgerReasonAuctionBid,
			ReferenceKey: referenceKey,
		})
		if err != nil {
			return err
		}
		if !lot.CurrentBidderID.IsZero() {
			refund, err := wallet.CreditWalletWithoutRepository(economy.CreditWalletInput{
				PlayerID:     lot.CurrentBidderID,
				Currency:     lot.Currency,
				Amount:       lot.CurrentBid,
				Reason:       ledgerReasonAuctionRefund,
				ReferenceKey: refundReference,
			})
			if err != nil {
				return err
			}
			previousRefund = &refund
		}

		lot.CurrentBid = input.Amount
		lot.CurrentBidderID = input.BidderPlayerID
		lot.UpdatedAt = service.clock.Now()
		result = PlaceBidResult{
			Lot:             cloneLot(lot),
			Amount:          input.Amount,
			BidderDebit:     bidderDebit,
			PreviousRefund:  previousRefund,
			ReferenceKey:    referenceKey,
			RefundReference: refundReference,
		}
		completedRow, err = service.completedAuctionBidIdempotencyRow(claimedRow, result)
		if err != nil {
			return err
		}
		outboxRow, err = service.auctionBidOutboxRow(result, input)
		if err != nil {
			return err
		}
		if err := tx.SaveAuctionLot(context.Background(), result.Lot); err != nil {
			return err
		}
		for _, commit := range auctionBidWalletCommits(result) {
			if err := tx.CommitWalletMutation(context.Background(), commit); err != nil {
				return err
			}
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil {
		restoreWallet(walletSnapshot)
		return PlaceBidResult{}, err
	}
	if duplicate {
		return result, nil
	}
	service.lots[input.AuctionID] = cloneLot(result.Lot)
	service.bidResults[referenceKey] = clonePlaceBidResult(result)
	if err := service.recordCompletedAuctionBidIdempotencyRow(completedRow); err != nil {
		return PlaceBidResult{}, err
	}
	return result, nil
}

func (service *Service) buyNowWithTransactionLocked(
	input BuyNowInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (BuyNowResult, error) {
	repository := service.auctionLotTransactionRepository()
	if repository == nil {
		return BuyNowResult{}, errors.New("auction lot transaction repository missing")
	}
	wallet, err := service.transactionWallet()
	if err != nil {
		return BuyNowResult{}, err
	}
	walletSnapshot, restoreWallet, hasWalletSnapshot := service.snapshotWalletMutationState()
	if !hasWalletSnapshot {
		return BuyNowResult{}, errors.New("auction transaction repository requires wallet mutation snapshots")
	}
	idempotencyRow := service.auctionBuyNowIdempotencyCandidate(input, referenceKey, requestHash)

	var result BuyNowResult
	var completedRow economy.IdempotencyKeyRow
	var outboxRow economy.OutboxRow
	var duplicate bool
	err = repository.WithAuctionLotTransaction(context.Background(), func(tx AuctionLotTransaction) error {
		locked, ok, err := tx.LoadAuctionLotForUpdate(context.Background(), input.AuctionID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("auction %q: %w", input.AuctionID, ErrLotNotFound)
		}
		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return err
		}
		claimedRow, duplicateResult, isDuplicate, err := auctionBuyNowResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			result = cloneBuyNowResult(duplicateResult)
			duplicate = true
			return nil
		}

		lot := cloneLot(locked)
		if err := service.prepareActiveLotLocked(&lot); err != nil {
			return err
		}
		if lot.BuyNowPrice == nil {
			return ErrBuyNowUnavailable
		}
		price := *lot.BuyNowPrice
		if err := service.validateDebitCapacity(input.BuyerPlayerID, lot.Currency, price); err != nil {
			return err
		}

		var refundReference foundation.IdempotencyKey
		var currentRefund *economy.CreditWalletResult
		if !lot.CurrentBidderID.IsZero() {
			refundReference, err = foundation.AuctionBuyNowRefundIdempotencyKey(input.AuctionID, lot.CurrentBidderID, input.RequestID)
			if err != nil {
				return err
			}
			if err := service.validateCreditCapacity(lot.CurrentBidderID, lot.Currency, lot.CurrentBid); err != nil {
				return err
			}
		}

		buyerDebit, err := wallet.DebitWalletWithoutRepository(economy.DebitWalletInput{
			PlayerID:     input.BuyerPlayerID,
			Currency:     lot.Currency,
			Amount:       price,
			Reason:       ledgerReasonAuctionBuyNow,
			ReferenceKey: referenceKey,
		})
		if err != nil {
			return err
		}
		if !lot.CurrentBidderID.IsZero() {
			refund, err := wallet.CreditWalletWithoutRepository(economy.CreditWalletInput{
				PlayerID:     lot.CurrentBidderID,
				Currency:     lot.Currency,
				Amount:       lot.CurrentBid,
				Reason:       ledgerReasonAuctionRefund,
				ReferenceKey: refundReference,
			})
			if err != nil {
				return err
			}
			currentRefund = &refund
		}

		grant := service.newGrantPayload(input.AuctionID, input.BuyerPlayerID, lot.Payload, CloseReasonBuyNow)
		now := service.clock.Now()
		lot.Status = LotStatusClosed
		lot.WinningPlayerID = input.BuyerPlayerID
		lot.CloseReason = CloseReasonBuyNow
		lot.ClosedAt = &now
		lot.UpdatedAt = now
		result = BuyNowResult{
			Lot:             cloneLot(lot),
			Price:           price,
			BuyerDebit:      buyerDebit,
			CurrentRefund:   currentRefund,
			Grant:           cloneGrant(grant),
			ReferenceKey:    referenceKey,
			RefundReference: refundReference,
		}
		completedRow, err = service.completedAuctionBuyNowIdempotencyRow(claimedRow, result)
		if err != nil {
			return err
		}
		outboxRow, err = service.auctionBuyNowOutboxRow(result, input)
		if err != nil {
			return err
		}
		if err := tx.SaveAuctionLot(context.Background(), result.Lot); err != nil {
			return err
		}
		for _, commit := range auctionBuyNowWalletCommits(result) {
			if err := tx.CommitWalletMutation(context.Background(), commit); err != nil {
				return err
			}
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil {
		restoreWallet(walletSnapshot)
		return BuyNowResult{}, err
	}
	if duplicate {
		return result, nil
	}
	service.grants = append(service.grants, cloneGrant(result.Grant))
	service.lots[input.AuctionID] = cloneLot(result.Lot)
	service.buyNowResults[referenceKey] = cloneBuyNowResult(result)
	if err := service.recordCompletedAuctionBuyNowIdempotencyRow(completedRow); err != nil {
		return BuyNowResult{}, err
	}
	return result, nil
}

type bidIdempotencyResult struct {
	Lot             Lot                       `json:"lot"`
	Amount          int64                     `json:"amount"`
	ReferenceKey    foundation.IdempotencyKey `json:"reference_id"`
	RefundReference foundation.IdempotencyKey `json:"refund_reference_id,omitempty"`
}

type buyNowIdempotencyResult struct {
	Lot             Lot                       `json:"lot"`
	Price           int64                     `json:"price"`
	Grant           Grant                     `json:"grant"`
	ReferenceKey    foundation.IdempotencyKey `json:"reference_id"`
	RefundReference foundation.IdempotencyKey `json:"refund_reference_id,omitempty"`
}

type auctionBidOutboxPayload struct {
	AuctionID       foundation.AuctionID      `json:"auction_id"`
	BidderPlayerID  foundation.PlayerID       `json:"bidder_player_id"`
	Amount          int64                     `json:"amount"`
	Currency        economy.CurrencyBucket    `json:"currency_type"`
	CurrentBid      int64                     `json:"current_bid"`
	CurrentBidderID foundation.PlayerID       `json:"current_bidder_id"`
	ReferenceKey    foundation.IdempotencyKey `json:"reference_id"`
	RefundReference foundation.IdempotencyKey `json:"refund_reference_id,omitempty"`
}

type auctionBuyNowOutboxPayload struct {
	AuctionID       foundation.AuctionID      `json:"auction_id"`
	BuyerPlayerID   foundation.PlayerID       `json:"buyer_player_id"`
	Price           int64                     `json:"price"`
	Currency        economy.CurrencyBucket    `json:"currency_type"`
	WinningPlayerID foundation.PlayerID       `json:"winning_player_id"`
	ReferenceKey    foundation.IdempotencyKey `json:"reference_id"`
	RefundReference foundation.IdempotencyKey `json:"refund_reference_id,omitempty"`
	Grant           Grant                     `json:"grant"`
}

func auctionBidWalletCommits(result PlaceBidResult) []economy.WalletMutationCommit {
	commits := []economy.WalletMutationCommit{
		auctionWalletMutationCommit(
			result.BidderDebit.Balance.PlayerID,
			economy.WalletMutationOperationDebit,
			result.ReferenceKey,
			result.BidderDebit.Balance,
			result.BidderDebit.LedgerEntry,
		),
	}
	if result.PreviousRefund != nil {
		commits = append(commits, auctionWalletMutationCommit(
			result.PreviousRefund.Balance.PlayerID,
			economy.WalletMutationOperationCredit,
			result.RefundReference,
			result.PreviousRefund.Balance,
			result.PreviousRefund.LedgerEntry,
		))
	}
	return commits
}

func auctionBuyNowWalletCommits(result BuyNowResult) []economy.WalletMutationCommit {
	commits := []economy.WalletMutationCommit{
		auctionWalletMutationCommit(
			result.BuyerDebit.Balance.PlayerID,
			economy.WalletMutationOperationDebit,
			result.ReferenceKey,
			result.BuyerDebit.Balance,
			result.BuyerDebit.LedgerEntry,
		),
	}
	if result.CurrentRefund != nil {
		commits = append(commits, auctionWalletMutationCommit(
			result.CurrentRefund.Balance.PlayerID,
			economy.WalletMutationOperationCredit,
			result.RefundReference,
			result.CurrentRefund.Balance,
			result.CurrentRefund.LedgerEntry,
		))
	}
	return commits
}

func auctionWalletMutationCommit(
	playerID foundation.PlayerID,
	operation economy.WalletMutationOperation,
	referenceKey foundation.IdempotencyKey,
	balance economy.WalletBalance,
	ledgerEntry economy.CurrencyLedgerEntry,
) economy.WalletMutationCommit {
	return economy.WalletMutationCommit{
		Balances:      []economy.WalletBalance{balance},
		LedgerEntries: []economy.CurrencyLedgerEntry{ledgerEntry},
		Reference: economy.WalletMutationReference{
			PlayerID:      playerID,
			Operation:     operation,
			ReferenceKey:  referenceKey,
			LedgerEntries: []economy.CurrencyLedgerEntry{ledgerEntry},
		},
		Counters: economy.WalletCounters{LedgerSequence: walletLedgerSequence(ledgerEntry.LedgerID)},
	}
}

func walletLedgerSequence(ledgerID economy.LedgerID) int64 {
	var sequence int64
	if _, err := fmt.Sscanf(ledgerID.String(), "currency-ledger-%d", &sequence); err != nil || sequence < 0 {
		return 0
	}
	return sequence
}

func (service *Service) auctionBidOutboxRow(result PlaceBidResult, input PlaceBidInput) (economy.OutboxRow, error) {
	payload, err := json.Marshal(auctionBidOutboxPayload{
		AuctionID:       result.Lot.AuctionID,
		BidderPlayerID:  input.BidderPlayerID,
		Amount:          result.Amount,
		Currency:        result.Lot.Currency,
		CurrentBid:      result.Lot.CurrentBid,
		CurrentBidderID: result.Lot.CurrentBidderID,
		ReferenceKey:    result.ReferenceKey,
		RefundReference: result.RefundReference,
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	now := service.clock.Now()
	return economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         auctionBidOutboxPrefix + result.ReferenceKey.String(),
		Topic:            auctionOutboxTopic,
		EventType:        auctionBidPlacedEvent,
		AggregateType:    auctionAggregateType,
		AggregateID:      result.Lot.AuctionID.String(),
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   result.ReferenceKey,
		PayloadJSON:      payload,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (service *Service) auctionBuyNowOutboxRow(result BuyNowResult, input BuyNowInput) (economy.OutboxRow, error) {
	payload, err := json.Marshal(auctionBuyNowOutboxPayload{
		AuctionID:       result.Lot.AuctionID,
		BuyerPlayerID:   input.BuyerPlayerID,
		Price:           result.Price,
		Currency:        result.Lot.Currency,
		WinningPlayerID: result.Lot.WinningPlayerID,
		ReferenceKey:    result.ReferenceKey,
		RefundReference: result.RefundReference,
		Grant:           cloneGrant(result.Grant),
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	now := service.clock.Now()
	return economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         auctionBuyNowOutboxPrefix + result.ReferenceKey.String(),
		Topic:            auctionOutboxTopic,
		EventType:        auctionBuyNowEvent,
		AggregateType:    auctionAggregateType,
		AggregateID:      result.Lot.AuctionID.String(),
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   result.ReferenceKey,
		PayloadJSON:      payload,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (service *Service) claimAuctionBidIdempotency(
	input PlaceBidInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, PlaceBidResult, bool, error) {
	candidate := service.auctionBidIdempotencyCandidate(input, referenceKey, requestHash)
	if service.idempotencyStore == nil {
		service.ensureBidIdempotencyRowsLocked()
		existing, ok := service.bidIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, PlaceBidResult{}, false, err
			}
			service.bidIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), PlaceBidResult{}, false, nil
		}
		return resolveAuctionBidIdempotencyClaim(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, PlaceBidResult{}, false, auctionBidIdempotencyClaimError(referenceKey, err)
	}
	return auctionBidResultFromClaim(claim)
}

func (service *Service) auctionBidIdempotencyCandidate(
	input PlaceBidInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) economy.IdempotencyKeyRow {
	now := service.clock.Now()
	return economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   auctionBidOperation,
		PlayerID:    input.BidderPlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func resolveAuctionBidIdempotencyClaim(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, PlaceBidResult, bool, error) {
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, PlaceBidResult{}, false, auctionBidIdempotencyClaimError(candidate.Key, err)
	}
	return auctionBidResultFromClaim(claim)
}

func auctionBidResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, PlaceBidResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, PlaceBidResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := auctionBidResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, PlaceBidResult{}, false, err
		}
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, PlaceBidResult{}, false, nil
	default:
		return claim.Row, PlaceBidResult{}, false, ErrBidInProgress
	}
}

func (service *Service) completeAuctionBidIdempotency(row economy.IdempotencyKeyRow, result PlaceBidResult) error {
	if row.Key.IsZero() {
		return nil
	}
	completed, err := service.completedAuctionBidIdempotencyRow(row, result)
	if err != nil {
		return err
	}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), completed); err != nil {
			return err
		}
	}
	return service.recordCompletedAuctionBidIdempotencyRow(completed)
}

func (service *Service) completedAuctionBidIdempotencyRow(row economy.IdempotencyKeyRow, result PlaceBidResult) (economy.IdempotencyKeyRow, error) {
	payload, err := auctionBidIdempotencyResultJSON(result)
	if err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	return row.Clone(), nil
}

func (service *Service) recordCompletedAuctionBidIdempotencyRow(row economy.IdempotencyKeyRow) error {
	if row.Key.IsZero() {
		return nil
	}
	service.ensureBidIdempotencyRowsLocked()
	if existing, ok := service.bidIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return auctionBidIdempotencyClaimError(row.Key, err)
		}
	}
	service.bidIdempotencyRows[row.Key] = row.Clone()
	return nil
}

func (service *Service) failAuctionBidIdempotency(row economy.IdempotencyKeyRow, cause error) error {
	if row.Key.IsZero() {
		return cause
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	payload, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return errors.Join(cause, err)
	}
	row.Status = economy.IdempotencyStatusFailed
	row.ResultJSON = payload
	row.UpdatedAt = service.clock.Now()
	row.CompletedAt = time.Time{}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensureBidIdempotencyRowsLocked()
	service.bidIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func (service *Service) ensureBidIdempotencyRowsLocked() {
	if service.bidIdempotencyRows == nil {
		service.bidIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
}

func (service *Service) claimAuctionBuyNowIdempotency(
	input BuyNowInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, BuyNowResult, bool, error) {
	candidate := service.auctionBuyNowIdempotencyCandidate(input, referenceKey, requestHash)
	if service.idempotencyStore == nil {
		service.ensureBuyNowIdempotencyRowsLocked()
		existing, ok := service.buyNowIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, BuyNowResult{}, false, err
			}
			service.buyNowIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), BuyNowResult{}, false, nil
		}
		return resolveAuctionBuyNowIdempotencyClaim(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, BuyNowResult{}, false, err
	}
	return auctionBuyNowResultFromClaim(claim)
}

func (service *Service) auctionBuyNowIdempotencyCandidate(
	input BuyNowInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) economy.IdempotencyKeyRow {
	now := service.clock.Now()
	return economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   auctionBuyNowOperation,
		PlayerID:    input.BuyerPlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func resolveAuctionBuyNowIdempotencyClaim(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, BuyNowResult, bool, error) {
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, BuyNowResult{}, false, err
	}
	return auctionBuyNowResultFromClaim(claim)
}

func auctionBuyNowResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, BuyNowResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, BuyNowResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := auctionBuyNowResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, BuyNowResult{}, false, err
		}
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, BuyNowResult{}, false, nil
	default:
		return claim.Row, BuyNowResult{}, false, ErrBuyNowInProgress
	}
}

func (service *Service) completeAuctionBuyNowIdempotency(row economy.IdempotencyKeyRow, result BuyNowResult) error {
	if row.Key.IsZero() {
		return nil
	}
	completed, err := service.completedAuctionBuyNowIdempotencyRow(row, result)
	if err != nil {
		return err
	}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), completed); err != nil {
			return err
		}
	}
	return service.recordCompletedAuctionBuyNowIdempotencyRow(completed)
}

func (service *Service) completedAuctionBuyNowIdempotencyRow(row economy.IdempotencyKeyRow, result BuyNowResult) (economy.IdempotencyKeyRow, error) {
	payload, err := auctionBuyNowIdempotencyResultJSON(result)
	if err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	return row.Clone(), nil
}

func (service *Service) recordCompletedAuctionBuyNowIdempotencyRow(row economy.IdempotencyKeyRow) error {
	if row.Key.IsZero() {
		return nil
	}
	service.ensureBuyNowIdempotencyRowsLocked()
	if existing, ok := service.buyNowIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.buyNowIdempotencyRows[row.Key] = row.Clone()
	return nil
}

func (service *Service) failAuctionBuyNowIdempotency(row economy.IdempotencyKeyRow, cause error) error {
	if row.Key.IsZero() {
		return cause
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	payload, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return errors.Join(cause, err)
	}
	row.Status = economy.IdempotencyStatusFailed
	row.ResultJSON = payload
	row.UpdatedAt = service.clock.Now()
	row.CompletedAt = time.Time{}
	if service.idempotencyStore != nil {
		if _, err := service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row); err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensureBuyNowIdempotencyRowsLocked()
	service.buyNowIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func (service *Service) ensureBuyNowIdempotencyRowsLocked() {
	if service.buyNowIdempotencyRows == nil {
		service.buyNowIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
}

func auctionBidRequestHash(input PlaceBidInput) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"auction_bid|auction=%s|bidder=%s|amount=%d|request=%s",
		input.AuctionID,
		input.BidderPlayerID,
		input.Amount,
		input.RequestID,
	)))
	return fmt.Sprintf("sha256:%x", hash[:])
}

func auctionBuyNowRequestHash(input BuyNowInput) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"auction_buy_now|auction=%s|buyer=%s|request=%s",
		input.AuctionID,
		input.BuyerPlayerID,
		input.RequestID,
	)))
	return fmt.Sprintf("sha256:%x", hash[:])
}

func auctionBuyNowIdempotencyResultJSON(result BuyNowResult) (json.RawMessage, error) {
	payload, err := json.Marshal(buyNowIdempotencyResult{
		Lot:             cloneLot(result.Lot),
		Price:           result.Price,
		Grant:           cloneGrant(result.Grant),
		ReferenceKey:    result.ReferenceKey,
		RefundReference: result.RefundReference,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func auctionBidIdempotencyResultJSON(result PlaceBidResult) (json.RawMessage, error) {
	payload, err := json.Marshal(bidIdempotencyResult{
		Lot:             cloneLot(result.Lot),
		Amount:          result.Amount,
		ReferenceKey:    result.ReferenceKey,
		RefundReference: result.RefundReference,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func auctionBidResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (PlaceBidResult, error) {
	var payload bidIdempotencyResult
	if err := json.Unmarshal(row.ResultJSON, &payload); err != nil {
		return PlaceBidResult{}, err
	}
	if payload.ReferenceKey.IsZero() {
		return PlaceBidResult{}, ErrBidIdempotencyResult
	}
	return PlaceBidResult{
		Lot:             cloneLot(payload.Lot),
		Amount:          payload.Amount,
		ReferenceKey:    payload.ReferenceKey,
		RefundReference: payload.RefundReference,
		Duplicate:       true,
	}, nil
}

func auctionBidIdempotencyClaimError(referenceKey foundation.IdempotencyKey, err error) error {
	if errors.Is(err, economy.ErrIdempotencyKeyConflict) {
		return fmt.Errorf("reference %q: %w", referenceKey, ErrBidReferenceMismatch)
	}
	return err
}

func auctionBuyNowResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (BuyNowResult, error) {
	var payload buyNowIdempotencyResult
	if err := json.Unmarshal(row.ResultJSON, &payload); err != nil {
		return BuyNowResult{}, err
	}
	if payload.ReferenceKey.IsZero() {
		return BuyNowResult{}, ErrBuyNowIdempotencyResult
	}
	return BuyNowResult{
		Lot:             cloneLot(payload.Lot),
		Price:           payload.Price,
		Grant:           cloneGrant(payload.Grant),
		ReferenceKey:    payload.ReferenceKey,
		RefundReference: payload.RefundReference,
		Duplicate:       true,
	}, nil
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
	grant := service.newGrantPayload(auctionID, playerID, payload, reason)
	service.grants = append(service.grants, cloneGrant(grant))
	return grant
}

func (service *Service) newGrantPayload(auctionID foundation.AuctionID, playerID foundation.PlayerID, payload LotPayload, reason CloseReason) Grant {
	return Grant{
		AuctionID: auctionID,
		PlayerID:  playerID,
		Payload:   clonePayload(payload),
		Reason:    reason,
		GrantedAt: service.clock.Now(),
	}
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
	result.PreviousRefund = cloneCreditWalletResultPointer(result.PreviousRefund)
	return result
}

func cloneBuyNowResult(result BuyNowResult) BuyNowResult {
	result.Lot = cloneLot(result.Lot)
	result.CurrentRefund = cloneCreditWalletResultPointer(result.CurrentRefund)
	result.Grant = cloneGrant(result.Grant)
	return result
}

func cloneCreditWalletResultPointer(result *economy.CreditWalletResult) *economy.CreditWalletResult {
	if result == nil {
		return nil
	}
	cloned := *result
	return &cloned
}

func cloneCloseAuctionResult(result CloseAuctionResult) CloseAuctionResult {
	result.Lot = cloneLot(result.Lot)
	if result.Grant != nil {
		grant := cloneGrant(*result.Grant)
		result.Grant = &grant
	}
	return result
}
