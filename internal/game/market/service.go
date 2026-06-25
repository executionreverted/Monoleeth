package market

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
)

const (
	marketListingReason economy.LedgerReason = "market_listing"
	marketBuyReason     economy.LedgerReason = "market_buy"
	marketSaleReason    economy.LedgerReason = "market_sale"
	marketFeeReason     economy.LedgerReason = "market_fee"
	marketCancelReason  economy.LedgerReason = "market_cancel"
	marketExpireReason  economy.LedgerReason = "market_expire"

	marketBuyOperation          = "market_buy"
	marketCancelOperation       = "market_cancel"
	marketBuySettlementOp       = "market.buy"
	marketCancelSettlementOp    = "market.cancel"
	marketBuyOutboxTopic        = "economy"
	marketBuyCompletedEventType = "market.buy_completed"
	marketCancelEventType       = "market.listing_cancelled"
	marketBuyAggregateType      = "market_listing"
	marketCancelAggregateType   = "market_listing"

	defaultSystemFeePlayerID             foundation.PlayerID = "market-fee-sink"
	defaultHighValueSaleThresholdCredits int64               = 100
)

// InventoryService is the economy inventory boundary used by market escrow.
type InventoryService interface {
	SystemMoveItem(input economy.MoveItemInput) (economy.MoveItemResult, error)
	TotalItemQuantity(playerID foundation.PlayerID, itemID foundation.ItemID, location economy.ItemLocation) int64
	SnapshotMutationState() economy.InventoryMutationSnapshot
	RestoreMutationState(snapshot economy.InventoryMutationSnapshot)
}

type transactionInventoryService interface {
	SystemMoveItemWithoutRepository(input economy.MoveItemInput) (economy.MoveItemResult, error)
}

// WalletService is the economy wallet boundary used by market settlement.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	Balance(playerID foundation.PlayerID, currency economy.CurrencyBucket) int64
	SnapshotMutationState() economy.WalletMutationSnapshot
	RestoreMutationState(snapshot economy.WalletMutationSnapshot)
}

type transactionWalletService interface {
	DebitWalletWithoutRepository(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWalletWithoutRepository(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
}

// MarketListingRepository persists listing snapshots when durable storage is wired.
type MarketListingRepository interface {
	SaveMarketListing(ctx context.Context, listing Listing) error
}

// MarketListingTransactionRepository commits market settlement rows through one
// durable transaction when contentdb is configured.
type MarketListingTransactionRepository interface {
	MarketListingRepository
	WithMarketListingTransaction(ctx context.Context, fn func(MarketListingTransaction) error) error
}

// MarketListingTransaction is the single-transaction seam for market listing
// settlement and the economy rows it owns.
type MarketListingTransaction interface {
	LoadMarketListingForUpdate(ctx context.Context, listingID foundation.ListingID) (Listing, bool, error)
	SaveMarketListing(ctx context.Context, listing Listing) error
	CommitWalletMutation(ctx context.Context, commit economy.WalletMutationCommit) error
	CommitInventoryMoveItem(ctx context.Context, commit economy.InventoryMoveItemCommit) error
	ClaimIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyClaimResult, error)
	CompleteIdempotencyKey(ctx context.Context, row economy.IdempotencyKeyRow) (economy.IdempotencyKeyRow, error)
	InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error
}

// MarketServiceConfig wires MarketService to economy primitives.
type MarketServiceConfig struct {
	Clock                        foundation.Clock
	Inventory                    InventoryService
	Wallet                       WalletService
	ListingRepository            MarketListingRepository
	ListingTransactionRepository MarketListingTransactionRepository
	IdempotencyStore             economy.IdempotencyStore
	OutboxStore                  economy.OutboxStore
	SettlementLogger             observability.SettlementLogger
	FeePolicy                    FeePolicy
	SuspiciousPolicy             SuspiciousTradePolicy
	SystemFeePlayerID            foundation.PlayerID
}

// MarketService owns in-memory fixed-price listing state for the MVP.
type MarketService struct {
	mu    sync.Mutex
	clock foundation.Clock

	inventory           InventoryService
	wallet              WalletService
	listingRepository   MarketListingRepository
	listingTransactions MarketListingTransactionRepository
	idempotencyStore    economy.IdempotencyStore
	outboxStore         economy.OutboxStore
	settlementLogger    observability.SettlementLogger
	feePolicy           FeePolicy
	suspiciousPolicy    SuspiciousTradePolicy
	systemFeePlayerID   foundation.PlayerID

	listings              map[foundation.ListingID]Listing
	createResults         map[foundation.IdempotencyKey]CreateListingResult
	buyResults            map[foundation.IdempotencyKey]BuyListingResult
	cancelResults         map[foundation.ListingID]CancelListingResult
	expireResults         map[foundation.ListingID]ExpireListingResult
	staleResults          map[foundation.ListingID]MarkListingStaleResult
	buyIdempotencyRows    map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	cancelIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	buyOutboxRows         map[string]economy.OutboxRow
	cancelOutboxRows      map[string]economy.OutboxRow

	suspiciousTradeLogs []SuspiciousTradeLog
	nextSuspiciousLogID int64
}

// NewMarketService returns an in-memory fixed-price market service.
func NewMarketService(config MarketServiceConfig) (*MarketService, error) {
	if config.Inventory == nil {
		return nil, ErrMissingInventoryService
	}
	if config.Wallet == nil {
		return nil, ErrMissingWalletService
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	feePolicy := config.FeePolicy
	if feePolicy.SaleFeeBasisPoints == 0 {
		feePolicy = DefaultFeePolicy()
	}
	if err := feePolicy.validate(); err != nil {
		return nil, err
	}
	suspiciousPolicy := config.SuspiciousPolicy
	if suspiciousPolicy.HighValueSaleThreshold == 0 {
		suspiciousPolicy = DefaultSuspiciousTradePolicy()
	}
	if err := suspiciousPolicy.validate(); err != nil {
		return nil, err
	}
	systemFeePlayerID := config.SystemFeePlayerID
	if systemFeePlayerID.IsZero() {
		systemFeePlayerID = defaultSystemFeePlayerID
	}
	if err := systemFeePlayerID.Validate(); err != nil {
		return nil, err
	}
	listingRepository := config.ListingRepository
	listingTransactions := config.ListingTransactionRepository
	if listingTransactions == nil {
		if configured, ok := listingRepository.(MarketListingTransactionRepository); ok {
			listingTransactions = configured
		}
	}
	if listingRepository == nil && listingTransactions != nil {
		listingRepository = listingTransactions
	}

	return &MarketService{
		clock:                 clock,
		inventory:             config.Inventory,
		wallet:                config.Wallet,
		listingRepository:     listingRepository,
		listingTransactions:   listingTransactions,
		idempotencyStore:      config.IdempotencyStore,
		outboxStore:           config.OutboxStore,
		settlementLogger:      config.SettlementLogger,
		feePolicy:             feePolicy,
		suspiciousPolicy:      suspiciousPolicy,
		systemFeePlayerID:     systemFeePlayerID,
		listings:              make(map[foundation.ListingID]Listing),
		createResults:         make(map[foundation.IdempotencyKey]CreateListingResult),
		buyResults:            make(map[foundation.IdempotencyKey]BuyListingResult),
		cancelResults:         make(map[foundation.ListingID]CancelListingResult),
		expireResults:         make(map[foundation.ListingID]ExpireListingResult),
		staleResults:          make(map[foundation.ListingID]MarkListingStaleResult),
		buyIdempotencyRows:    make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
		cancelIdempotencyRows: make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow),
		buyOutboxRows:         make(map[string]economy.OutboxRow),
		cancelOutboxRows:      make(map[string]economy.OutboxRow),
	}, nil
}

// CreateListing validates a seller-owned source stack and moves it into market escrow.
func (service *MarketService) CreateListing(input CreateListingInput) (CreateListingResult, error) {
	if err := input.validate(service.clock.Now()); err != nil {
		return CreateListingResult{}, err
	}
	referenceKey, err := foundation.MarketListingIdempotencyKey(input.ListingID)
	if err != nil {
		return CreateListingResult{}, err
	}
	escrowLocation, err := marketEscrowLocation(input.ListingID)
	if err != nil {
		return CreateListingResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if _, exists := service.listings[input.ListingID]; exists {
		if previous, ok := service.createResults[referenceKey]; ok {
			if err := createListingInputMatchesResult(input, previous); err != nil {
				return CreateListingResult{}, err
			}
			result := cloneCreateListingResult(previous)
			result.Duplicate = true
			return result, nil
		}
		return CreateListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrDuplicateListingID)
	}
	if err := service.validateSourceQuantity(input.SellerPlayerID, input.ItemRef.Definition, input.SourceLocation, input.Quantity); err != nil {
		return CreateListingResult{}, err
	}

	var inventorySnapshot economy.InventoryMutationSnapshot
	if service.listingRepository != nil {
		inventorySnapshot = service.inventory.SnapshotMutationState()
	}
	escrowMove, err := service.inventory.SystemMoveItem(economy.MoveItemInput{
		PlayerID:     input.SellerPlayerID,
		ItemRef:      input.ItemRef,
		FromLocation: input.SourceLocation,
		ToLocation:   escrowLocation,
		Quantity:     input.Quantity,
		Reason:       marketListingReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return CreateListingResult{}, err
	}

	now := service.clock.Now()
	listing := Listing{
		ListingID:            input.ListingID,
		SellerPlayerID:       input.SellerPlayerID,
		ItemDefinition:       cloneItemDefinition(input.ItemRef.Definition),
		ItemInstanceID:       input.ItemRef.ItemInstanceID,
		ItemID:               input.ItemRef.Definition.ItemID,
		OriginalQuantity:     input.Quantity,
		RemainingQuantity:    input.Quantity,
		UnitPrice:            input.UnitPrice,
		Currency:             input.Currency,
		Status:               ListingStatusActive,
		SourceReturnLocation: input.SourceLocation,
		EscrowLocation:       escrowLocation,
		CreatedAt:            now,
		UpdatedAt:            now,
		ExpiresAt:            cloneTimePointer(input.ExpiresAt),
	}
	if err := service.saveListingSnapshot(listing); err != nil {
		if service.listingRepository != nil {
			service.inventory.RestoreMutationState(inventorySnapshot)
		}
		return CreateListingResult{}, err
	}
	service.listings[input.ListingID] = cloneListing(listing)

	result := CreateListingResult{
		Listing:      cloneListing(listing),
		EscrowMove:   escrowMove,
		ReferenceKey: referenceKey,
	}
	service.createResults[referenceKey] = cloneCreateListingResult(result)
	return result, nil
}

// BuyListing settles a buyer purchase against active escrowed listing quantity.
func (service *MarketService) BuyListing(input BuyListingInput) (result BuyListingResult, err error) {
	if err := input.validate(); err != nil {
		return BuyListingResult{}, err
	}
	referenceKey, err := foundation.MarketBuyIdempotencyKey(input.ListingID, input.BuyerPlayerID, input.RequestID)
	if err != nil {
		return BuyListingResult{}, err
	}
	saleReference, err := foundation.MarketSaleIdempotencyKey(input.ListingID, input.BuyerPlayerID, input.RequestID)
	if err != nil {
		return BuyListingResult{}, err
	}
	feeReference, err := foundation.MarketFeeIdempotencyKey(input.ListingID, input.BuyerPlayerID, input.RequestID)
	if err != nil {
		return BuyListingResult{}, err
	}
	startedAt := service.nowUTC()
	defer func() {
		service.recordSettlementLog(
			observability.Operation(marketBuySettlementOp),
			input.RequestID,
			input.BuyerPlayerID,
			referenceKey,
			[]foundation.IdempotencyKey{referenceKey, saleReference, feeReference},
			startedAt,
			err,
		)
	}()
	requestHash := marketBuyRequestHash(input)

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.buyResults[referenceKey]; ok {
		if previous.Quantity != input.Quantity {
			return BuyListingResult{}, fmt.Errorf("reference %q quantity %d want %d: %w", referenceKey, input.Quantity, previous.Quantity, ErrBuyReferenceMismatch)
		}
		result := cloneBuyListingResult(previous)
		result.Duplicate = true
		return result, nil
	}

	listing, ok := service.listings[input.ListingID]
	if !ok {
		return BuyListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingNotFound)
	}
	if listing.Status != ListingStatusActive {
		return BuyListingResult{}, fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrListingNotActive)
	}
	if listing.isExpired(service.clock.Now()) {
		return BuyListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingExpired)
	}
	if listing.SellerPlayerID == input.BuyerPlayerID {
		return BuyListingResult{}, ErrSellerCannotBuyOwnListing
	}
	if input.Quantity > listing.RemainingQuantity {
		return BuyListingResult{}, fmt.Errorf("listing remaining %d need %d: %w", listing.RemainingQuantity, input.Quantity, economy.ErrInsufficientItemQuantity)
	}

	total, fee, sellerProceeds, err := service.calculateSettlement(listing.UnitPrice, input.Quantity)
	if err != nil {
		return BuyListingResult{}, err
	}
	if err := service.validateBuyerFunds(input.BuyerPlayerID, listing.Currency, total); err != nil {
		return BuyListingResult{}, err
	}
	if err := service.validateCreditCapacity(listing.SellerPlayerID, listing.Currency, sellerProceeds); err != nil {
		return BuyListingResult{}, err
	}
	if fee > 0 {
		if err := service.validateCreditCapacity(service.systemFeePlayerID, listing.Currency, fee); err != nil {
			return BuyListingResult{}, err
		}
	}
	if err := service.validateEscrowQuantity(listing, input.Quantity); err != nil {
		return BuyListingResult{}, err
	}
	idempotencyRow, duplicateResult, duplicate, err := service.claimMarketBuyIdempotency(input, referenceKey, requestHash)
	if err != nil {
		return BuyListingResult{}, err
	}
	if duplicate {
		return duplicateResult, nil
	}
	snapshot := service.snapshotMarketBuyMutationLocked()
	rollback := func(cause error) (BuyListingResult, error) {
		service.restoreMarketBuyMutationLocked(snapshot)
		return BuyListingResult{}, service.failMarketBuyIdempotency(idempotencyRow, cause)
	}

	// Cross-service calls stay under the market lock so listing state, wallet
	// mutations, and escrow movement are serialized until durable transactions exist.
	buyerDebit, err := service.debitWalletForSettlement(economy.DebitWalletInput{
		PlayerID:     input.BuyerPlayerID,
		Currency:     listing.Currency,
		Amount:       total,
		Reason:       marketBuyReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return BuyListingResult{}, service.failMarketBuyIdempotency(idempotencyRow, err)
	}
	sellerCredit, err := service.creditWalletForSettlement(economy.CreditWalletInput{
		PlayerID:     listing.SellerPlayerID,
		Currency:     listing.Currency,
		Amount:       sellerProceeds,
		Reason:       marketSaleReason,
		ReferenceKey: saleReference,
	})
	if err != nil {
		return rollback(err)
	}
	var feeCredit *economy.CreditWalletResult
	if fee > 0 {
		credit, err := service.creditWalletForSettlement(economy.CreditWalletInput{
			PlayerID:     service.systemFeePlayerID,
			Currency:     listing.Currency,
			Amount:       fee,
			Reason:       marketFeeReason,
			ReferenceKey: feeReference,
		})
		if err != nil {
			return rollback(err)
		}
		feeCredit = &credit
	}

	buyerLocation, err := accountInventoryLocation(input.BuyerPlayerID)
	if err != nil {
		return rollback(err)
	}
	itemMove, err := service.moveInventoryForSettlement(economy.MoveItemInput{
		PlayerID:     listing.SellerPlayerID,
		ToPlayerID:   input.BuyerPlayerID,
		ItemRef:      economy.MoveItemRef{Definition: listing.ItemDefinition, ItemInstanceID: listing.ItemInstanceID},
		FromLocation: listing.EscrowLocation,
		ToLocation:   buyerLocation,
		Quantity:     input.Quantity,
		Reason:       marketBuyReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return rollback(err)
	}

	listing.RemainingQuantity -= input.Quantity
	if listing.RemainingQuantity == 0 {
		if err := listing.transitionTo(ListingStatusSold); err != nil {
			return rollback(err)
		}
	}
	listing.UpdatedAt = service.clock.Now()

	result = BuyListingResult{
		Listing:        cloneListing(listing),
		Quantity:       input.Quantity,
		TotalAmount:    total,
		FeeAmount:      fee,
		SellerProceeds: sellerProceeds,
		BuyerDebit:     buyerDebit,
		SellerCredit:   sellerCredit,
		FeeCredit:      feeCredit,
		ItemMove:       itemMove,
		ReferenceKey:   referenceKey,
		SaleReference:  saleReference,
		FeeReference:   feeReference,
	}
	txDuplicateResult, txDuplicate, err := service.commitMarketBuySettlement(listing, result, input, idempotencyRow)
	if txDuplicate {
		service.restoreMarketBuyMutationLocked(snapshot)
		service.buyResults[referenceKey] = cloneBuyListingResult(txDuplicateResult)
		return txDuplicateResult, nil
	}
	if err != nil {
		return rollback(err)
	}
	service.listings[input.ListingID] = cloneListing(listing)
	service.recordSuspiciousTradeLocked(listing, input, total, referenceKey)
	service.buyResults[referenceKey] = cloneBuyListingResult(result)
	return result, nil
}

// CancelListing returns remaining active escrow to the seller's recorded source location.
func (service *MarketService) CancelListing(input CancelListingInput) (result CancelListingResult, err error) {
	if err := input.validate(); err != nil {
		return CancelListingResult{}, err
	}
	referenceKey, err := foundation.MarketCancelIdempotencyKey(input.ListingID)
	if err != nil {
		return CancelListingResult{}, err
	}
	startedAt := service.nowUTC()
	defer func() {
		service.recordSettlementLog(
			observability.Operation(marketCancelSettlementOp),
			input.RequestID,
			input.SellerPlayerID,
			referenceKey,
			[]foundation.IdempotencyKey{referenceKey},
			startedAt,
			err,
		)
	}()
	requestHash := marketCancelRequestHash(input)

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.cancelResults[input.ListingID]; ok {
		if previous.Listing.SellerPlayerID != input.SellerPlayerID {
			return CancelListingResult{}, ErrListingOwnership
		}
		result := cloneCancelListingResult(previous)
		result.Duplicate = true
		return result, nil
	}

	listing, ok := service.listings[input.ListingID]
	if !ok {
		return CancelListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingNotFound)
	}
	if listing.SellerPlayerID != input.SellerPlayerID {
		return CancelListingResult{}, ErrListingOwnership
	}
	idempotencyRow, duplicateResult, duplicate, err := service.claimMarketCancelIdempotency(input, referenceKey, requestHash)
	if err != nil {
		return CancelListingResult{}, err
	}
	if duplicate {
		return duplicateResult, nil
	}
	snapshot := service.snapshotMarketCancelMutationLocked()
	rollback := func(cause error) (CancelListingResult, error) {
		service.restoreMarketCancelMutationLocked(snapshot)
		return CancelListingResult{}, service.failMarketCancelIdempotency(idempotencyRow, cause)
	}

	if listing.Status != ListingStatusActive && listing.Status != ListingStatusStale {
		return rollback(fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrListingNotActive))
	}
	if listing.RemainingQuantity <= 0 {
		return rollback(fmt.Errorf("listing remaining %d: %w", listing.RemainingQuantity, ErrListingNotActive))
	}
	if err := service.validateEscrowQuantity(listing, listing.RemainingQuantity); err != nil {
		return rollback(err)
	}

	returnMove, err := service.moveInventoryForSettlement(economy.MoveItemInput{
		PlayerID:     listing.SellerPlayerID,
		ItemRef:      economy.MoveItemRef{Definition: listing.ItemDefinition, ItemInstanceID: listing.ItemInstanceID},
		FromLocation: listing.EscrowLocation,
		ToLocation:   listing.SourceReturnLocation,
		Quantity:     listing.RemainingQuantity,
		Reason:       marketCancelReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return rollback(err)
	}

	returnedQuantity := listing.RemainingQuantity
	if err := listing.transitionTo(ListingStatusCancelled); err != nil {
		return rollback(err)
	}
	listing.UpdatedAt = service.clock.Now()

	result = CancelListingResult{
		Listing:          cloneListing(listing),
		ReturnedQuantity: returnedQuantity,
		ReturnMove:       returnMove,
		ReferenceKey:     referenceKey,
	}
	txDuplicateResult, txDuplicate, err := service.commitMarketCancelSettlement(listing, result, input, idempotencyRow)
	if txDuplicate {
		service.restoreMarketCancelMutationLocked(snapshot)
		service.cancelResults[input.ListingID] = cloneCancelListingResult(txDuplicateResult)
		return txDuplicateResult, nil
	}
	if err != nil {
		return rollback(err)
	}
	service.listings[input.ListingID] = cloneListing(listing)
	service.cancelResults[input.ListingID] = cloneCancelListingResult(result)
	return result, nil
}

// ExpireListing returns remaining escrow for a listing whose expiration time has passed.
func (service *MarketService) ExpireListing(input ExpireListingInput) (ExpireListingResult, error) {
	if err := input.validate(); err != nil {
		return ExpireListingResult{}, err
	}
	referenceKey, err := foundation.MarketExpireIdempotencyKey(input.ListingID)
	if err != nil {
		return ExpireListingResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.expireResults[input.ListingID]; ok {
		result := cloneExpireListingResult(previous)
		result.Duplicate = true
		return result, nil
	}

	listing, ok := service.listings[input.ListingID]
	if !ok {
		return ExpireListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingNotFound)
	}
	if listing.Status != ListingStatusActive && listing.Status != ListingStatusStale {
		return ExpireListingResult{}, fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrListingNotActive)
	}
	if !listing.isExpired(service.clock.Now()) {
		return ExpireListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingNotExpired)
	}
	if listing.RemainingQuantity <= 0 {
		return ExpireListingResult{}, fmt.Errorf("listing remaining %d: %w", listing.RemainingQuantity, ErrListingNotActive)
	}
	if err := service.validateEscrowQuantity(listing, listing.RemainingQuantity); err != nil {
		return ExpireListingResult{}, err
	}

	returnMove, err := service.inventory.SystemMoveItem(economy.MoveItemInput{
		PlayerID:     listing.SellerPlayerID,
		ItemRef:      economy.MoveItemRef{Definition: listing.ItemDefinition, ItemInstanceID: listing.ItemInstanceID},
		FromLocation: listing.EscrowLocation,
		ToLocation:   listing.SourceReturnLocation,
		Quantity:     listing.RemainingQuantity,
		Reason:       marketExpireReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return ExpireListingResult{}, err
	}

	returnedQuantity := listing.RemainingQuantity
	if err := listing.transitionTo(ListingStatusExpired); err != nil {
		return ExpireListingResult{}, err
	}
	listing.UpdatedAt = service.clock.Now()
	service.listings[input.ListingID] = cloneListing(listing)

	result := ExpireListingResult{
		Listing:          cloneListing(listing),
		ReturnedQuantity: returnedQuantity,
		ReturnMove:       returnMove,
		ReferenceKey:     referenceKey,
	}
	service.expireResults[input.ListingID] = cloneExpireListingResult(result)
	return result, nil
}

// MarkListingStale marks an active listing as stale after an owning intel fact changes.
func (service *MarketService) MarkListingStale(input MarkListingStaleInput) (MarkListingStaleResult, error) {
	if err := input.validate(); err != nil {
		return MarkListingStaleResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if previous, ok := service.staleResults[input.ListingID]; ok {
		result := cloneMarkListingStaleResult(previous)
		result.Duplicate = true
		return result, nil
	}

	listing, ok := service.listings[input.ListingID]
	if !ok {
		return MarkListingStaleResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingNotFound)
	}
	if listing.Status == ListingStatusStale {
		result := MarkListingStaleResult{Listing: cloneListing(listing), Duplicate: true}
		service.staleResults[input.ListingID] = cloneMarkListingStaleResult(result)
		return result, nil
	}
	if listing.Status != ListingStatusActive {
		return MarkListingStaleResult{}, fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrListingNotActive)
	}
	if listing.isExpired(service.clock.Now()) {
		return MarkListingStaleResult{}, fmt.Errorf("listing %q: %w", input.ListingID, ErrListingExpired)
	}

	now := service.clock.Now()
	if err := listing.transitionTo(ListingStatusStale); err != nil {
		return MarkListingStaleResult{}, err
	}
	listing.StaleAt = &now
	listing.StaleReason = strings.TrimSpace(input.Reason)
	listing.UpdatedAt = now
	service.listings[input.ListingID] = cloneListing(listing)

	result := MarkListingStaleResult{Listing: cloneListing(listing)}
	service.staleResults[input.ListingID] = cloneMarkListingStaleResult(result)
	return result, nil
}

// Listing returns a listing snapshot.
func (service *MarketService) Listing(listingID foundation.ListingID) (Listing, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	listing, ok := service.listings[listingID]
	if !ok {
		return Listing{}, false
	}
	return cloneListing(listing), true
}

// Listings returns all listing snapshots sorted by id.
func (service *MarketService) Listings() []Listing {
	service.mu.Lock()
	defer service.mu.Unlock()

	listings := make([]Listing, 0, len(service.listings))
	for _, listing := range service.listings {
		listings = append(listings, cloneListing(listing))
	}
	sort.Slice(listings, func(i, j int) bool {
		return listings[i].ListingID < listings[j].ListingID
	})
	return listings
}

// SuspiciousTradeLogs returns a stable snapshot of market fraud-review logs.
func (service *MarketService) SuspiciousTradeLogs() []SuspiciousTradeLog {
	service.mu.Lock()
	defer service.mu.Unlock()

	return append([]SuspiciousTradeLog(nil), service.suspiciousTradeLogs...)
}

// MarketBuyOutboxRows returns pending market-buy outbox rows recorded by this
// in-memory slice.
func (service *MarketService) MarketBuyOutboxRows() []economy.OutboxRow {
	service.mu.Lock()
	defer service.mu.Unlock()

	rows := make([]economy.OutboxRow, 0, len(service.buyOutboxRows))
	for _, row := range service.buyOutboxRows {
		rows = append(rows, row.Clone())
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].OutboxID < rows[j].OutboxID
	})
	return rows
}

func (service *MarketService) recordSettlementLog(
	operation observability.Operation,
	requestID foundation.RequestID,
	playerID foundation.PlayerID,
	idempotencyKey foundation.IdempotencyKey,
	referenceIDs []foundation.IdempotencyKey,
	startedAt time.Time,
	transitionErr error,
) {
	if service == nil || service.settlementLogger == nil {
		return
	}
	finishedAt := service.nowUTC()
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}

	status := observability.CommandStatusOK
	var code foundation.Code
	if transitionErr != nil {
		status = observability.CommandStatusError
		code = foundation.CodeInternal
		if domainCode, ok := foundation.CodeOf(transitionErr); ok {
			code = domainCode
		}
	}

	_ = service.settlementLogger.RecordSettlement(observability.SettlementLogEntry{
		RequestID:      requestID,
		PlayerID:       playerID,
		Operation:      operation,
		ErrorCode:      code,
		IdempotencyKey: idempotencyKey,
		ReferenceIDs:   referenceIDs,
		Duration:       duration,
		Status:         status,
		Timestamp:      startedAt,
	})
}

func (service *MarketService) nowUTC() time.Time {
	if service == nil || service.clock == nil {
		return foundation.RealClock{}.Now().UTC()
	}
	return service.clock.Now().UTC()
}

func (service *MarketService) saveListingSnapshot(listing Listing) error {
	if service == nil || service.listingRepository == nil {
		return nil
	}
	return service.listingRepository.SaveMarketListing(context.Background(), cloneListing(listing))
}

func (service *MarketService) marketListingTransactionRepository() MarketListingTransactionRepository {
	if service == nil {
		return nil
	}
	return service.listingTransactions
}

func (service *MarketService) debitWalletForSettlement(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	if service.marketListingTransactionRepository() != nil {
		if wallet, ok := service.wallet.(transactionWalletService); ok {
			return wallet.DebitWalletWithoutRepository(input)
		}
	}
	return service.wallet.DebitWallet(input)
}

func (service *MarketService) creditWalletForSettlement(input economy.CreditWalletInput) (economy.CreditWalletResult, error) {
	if service.marketListingTransactionRepository() != nil {
		if wallet, ok := service.wallet.(transactionWalletService); ok {
			return wallet.CreditWalletWithoutRepository(input)
		}
	}
	return service.wallet.CreditWallet(input)
}

func (service *MarketService) moveInventoryForSettlement(input economy.MoveItemInput) (economy.MoveItemResult, error) {
	if service.marketListingTransactionRepository() != nil {
		if inventory, ok := service.inventory.(transactionInventoryService); ok {
			return inventory.SystemMoveItemWithoutRepository(input)
		}
	}
	return service.inventory.SystemMoveItem(input)
}

func (service *MarketService) commitMarketBuySettlement(
	listing Listing,
	result BuyListingResult,
	input BuyListingInput,
	idempotencyRow economy.IdempotencyKeyRow,
) (BuyListingResult, bool, error) {
	repository := service.marketListingTransactionRepository()
	if repository == nil {
		if err := service.saveListingSnapshot(listing); err != nil {
			return BuyListingResult{}, false, err
		}
		if err := service.insertMarketBuyOutbox(result, input); err != nil {
			return BuyListingResult{}, false, err
		}
		if err := service.completeMarketBuyIdempotency(idempotencyRow, result, input); err != nil {
			return BuyListingResult{}, false, err
		}
		return BuyListingResult{}, false, nil
	}

	completedRow, err := service.completedMarketBuyIdempotencyRow(idempotencyRow, result, input)
	if err != nil {
		return BuyListingResult{}, false, err
	}
	outboxRow, err := service.marketBuyOutboxRow(result, input)
	if err != nil {
		return BuyListingResult{}, false, err
	}
	walletCommits := marketBuyWalletCommits(result)
	inventoryCommit := marketInventoryMoveCommit(listing.SellerPlayerID, result.ReferenceKey, result.ItemMove)

	var duplicateResult BuyListingResult
	var duplicate bool
	err = repository.WithMarketListingTransaction(context.Background(), func(tx MarketListingTransaction) error {
		locked, ok, err := tx.LoadMarketListingForUpdate(context.Background(), listing.ListingID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("listing %q: %w", listing.ListingID, ErrListingNotFound)
		}
		if locked.SellerPlayerID != listing.SellerPlayerID {
			return ErrListingOwnership
		}
		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return err
		}
		_, cached, isDuplicate, err := marketBuyResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			duplicateResult = cached
			duplicate = true
			return nil
		}
		if locked.Status != ListingStatusActive {
			return fmt.Errorf("listing %q status %q: %w", listing.ListingID, locked.Status, ErrListingNotActive)
		}
		if locked.isExpired(service.clock.Now()) {
			return fmt.Errorf("listing %q: %w", listing.ListingID, ErrListingExpired)
		}
		if locked.RemainingQuantity != result.Listing.RemainingQuantity+result.Quantity {
			return fmt.Errorf("listing %q locked remaining %d want %d: %w", listing.ListingID, locked.RemainingQuantity, result.Listing.RemainingQuantity+result.Quantity, economy.ErrInsufficientItemQuantity)
		}
		if err := tx.SaveMarketListing(context.Background(), cloneListing(listing)); err != nil {
			return err
		}
		for _, commit := range walletCommits {
			if err := tx.CommitWalletMutation(context.Background(), commit); err != nil {
				return err
			}
		}
		if err := tx.CommitInventoryMoveItem(context.Background(), inventoryCommit); err != nil {
			return err
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil || duplicate {
		return duplicateResult, duplicate, err
	}
	if err := service.recordCompletedMarketBuyRows(completedRow, outboxRow); err != nil {
		return BuyListingResult{}, false, err
	}
	return BuyListingResult{}, false, nil
}

func (service *MarketService) commitMarketCancelSettlement(
	listing Listing,
	result CancelListingResult,
	input CancelListingInput,
	idempotencyRow economy.IdempotencyKeyRow,
) (CancelListingResult, bool, error) {
	repository := service.marketListingTransactionRepository()
	if repository == nil {
		if err := service.saveListingSnapshot(listing); err != nil {
			return CancelListingResult{}, false, err
		}
		if err := service.insertMarketCancelOutbox(result, input); err != nil {
			return CancelListingResult{}, false, err
		}
		if err := service.completeMarketCancelIdempotency(idempotencyRow, result, input); err != nil {
			return CancelListingResult{}, false, err
		}
		return CancelListingResult{}, false, nil
	}

	completedRow, err := service.completedMarketCancelIdempotencyRow(idempotencyRow, result, input)
	if err != nil {
		return CancelListingResult{}, false, err
	}
	outboxRow, err := service.marketCancelOutboxRow(result, input)
	if err != nil {
		return CancelListingResult{}, false, err
	}
	inventoryCommit := marketInventoryMoveCommit(listing.SellerPlayerID, result.ReferenceKey, result.ReturnMove)

	var duplicateResult CancelListingResult
	var duplicate bool
	err = repository.WithMarketListingTransaction(context.Background(), func(tx MarketListingTransaction) error {
		locked, ok, err := tx.LoadMarketListingForUpdate(context.Background(), listing.ListingID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("listing %q: %w", listing.ListingID, ErrListingNotFound)
		}
		if locked.SellerPlayerID != listing.SellerPlayerID {
			return ErrListingOwnership
		}
		claim, err := tx.ClaimIdempotencyKey(context.Background(), idempotencyRow)
		if err != nil {
			return err
		}
		_, cached, isDuplicate, err := marketCancelResultFromClaim(claim)
		if err != nil {
			return err
		}
		if isDuplicate {
			duplicateResult = cached
			duplicate = true
			return nil
		}
		if locked.Status != ListingStatusActive && locked.Status != ListingStatusStale {
			return fmt.Errorf("listing %q status %q: %w", listing.ListingID, locked.Status, ErrListingNotActive)
		}
		if locked.RemainingQuantity != result.ReturnedQuantity {
			return fmt.Errorf("listing %q locked remaining %d want %d: %w", listing.ListingID, locked.RemainingQuantity, result.ReturnedQuantity, ErrListingNotActive)
		}
		if err := tx.SaveMarketListing(context.Background(), cloneListing(listing)); err != nil {
			return err
		}
		if err := tx.CommitInventoryMoveItem(context.Background(), inventoryCommit); err != nil {
			return err
		}
		if _, err := tx.CompleteIdempotencyKey(context.Background(), completedRow); err != nil {
			return err
		}
		return tx.InsertOutboxRow(context.Background(), outboxRow)
	})
	if err != nil || duplicate {
		return duplicateResult, duplicate, err
	}
	if err := service.recordCompletedMarketCancelRows(completedRow, outboxRow); err != nil {
		return CancelListingResult{}, false, err
	}
	return CancelListingResult{}, false, nil
}

func marketBuyWalletCommits(result BuyListingResult) []economy.WalletMutationCommit {
	commits := []economy.WalletMutationCommit{
		marketWalletMutationCommit(
			result.BuyerDebit.Balance.PlayerID,
			economy.WalletMutationOperationDebit,
			result.ReferenceKey,
			result.BuyerDebit.Balance,
			result.BuyerDebit.LedgerEntry,
		),
		marketWalletMutationCommit(
			result.SellerCredit.Balance.PlayerID,
			economy.WalletMutationOperationCredit,
			result.SaleReference,
			result.SellerCredit.Balance,
			result.SellerCredit.LedgerEntry,
		),
	}
	if result.FeeCredit != nil {
		commits = append(commits, marketWalletMutationCommit(
			result.FeeCredit.Balance.PlayerID,
			economy.WalletMutationOperationCredit,
			result.FeeReference,
			result.FeeCredit.Balance,
			result.FeeCredit.LedgerEntry,
		))
	}
	return commits
}

func marketWalletMutationCommit(
	playerID foundation.PlayerID,
	operation economy.WalletMutationOperation,
	referenceKey foundation.IdempotencyKey,
	balance economy.WalletBalance,
	ledgerEntry economy.CurrencyLedgerEntry,
) economy.WalletMutationCommit {
	ledgerEntries := []economy.CurrencyLedgerEntry{ledgerEntry}
	return economy.WalletMutationCommit{
		Balances:      []economy.WalletBalance{balance},
		LedgerEntries: ledgerEntries,
		Reference: economy.WalletMutationReference{
			PlayerID:      playerID,
			Operation:     operation,
			ReferenceKey:  referenceKey,
			LedgerEntries: append([]economy.CurrencyLedgerEntry(nil), ledgerEntries...),
		},
		Counters: economy.WalletCounters{LedgerSequence: maxCurrencyLedgerSequence(ledgerEntries)},
	}
}

func marketInventoryMoveCommit(
	playerID foundation.PlayerID,
	referenceKey foundation.IdempotencyKey,
	result economy.MoveItemResult,
) economy.InventoryMoveItemCommit {
	cloned := cloneMoveItemResult(result)
	return economy.InventoryMoveItemCommit{
		StackableItems:        append([]economy.StackableItem(nil), cloned.StackableItems...),
		DeletedStackableItems: append([]economy.StackableItem(nil), cloned.DeletedStackableItems...),
		InstanceItems:         append([]economy.InstanceItem(nil), cloned.InstanceItems...),
		LedgerEntries:         append([]economy.ItemLedgerEntry(nil), cloned.LedgerEntries...),
		Reference: economy.MoveItemReference{
			PlayerID:     playerID,
			ReferenceKey: referenceKey,
			Result:       cloned,
		},
		Counters: economy.InventoryCounters{
			ItemSequence:   maxInventoryItemSequence(cloned),
			LedgerSequence: maxItemLedgerSequence(cloned.LedgerEntries),
		},
	}
}

func maxCurrencyLedgerSequence(entries []economy.CurrencyLedgerEntry) int64 {
	var sequence int64
	for _, entry := range entries {
		sequence = max(sequence, ledgerIDSequence(entry.LedgerID, "currency-ledger-"))
	}
	return sequence
}

func maxItemLedgerSequence(entries []economy.ItemLedgerEntry) int64 {
	var sequence int64
	for _, entry := range entries {
		sequence = max(sequence, ledgerIDSequence(entry.LedgerID, "item-ledger-"))
	}
	return sequence
}

func maxInventoryItemSequence(result economy.MoveItemResult) int64 {
	var sequence int64
	for _, item := range result.StackableItems {
		sequence = max(sequence, itemInstanceSequence(item.ItemInstanceID))
	}
	for _, item := range result.DeletedStackableItems {
		sequence = max(sequence, itemInstanceSequence(item.ItemInstanceID))
	}
	for _, item := range result.InstanceItems {
		sequence = max(sequence, itemInstanceSequence(item.ItemInstanceID))
	}
	return sequence
}

func ledgerIDSequence(id economy.LedgerID, prefix string) int64 {
	value := strings.TrimPrefix(id.String(), prefix)
	if value == id.String() {
		return 0
	}
	sequence, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
}

func itemInstanceSequence(itemID foundation.ItemID) int64 {
	value := itemID.String()
	index := strings.LastIndex(value, "-")
	if index < 0 || index == len(value)-1 {
		return 0
	}
	sequence, err := strconv.ParseInt(value[index+1:], 10, 64)
	if err != nil || sequence < 0 {
		return 0
	}
	return sequence
}

func (input CreateListingInput) validate(now time.Time) error {
	if err := input.SellerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ListingID.Validate(); err != nil {
		return err
	}
	if err := input.ItemRef.Validate(); err != nil {
		return err
	}
	if err := economy.ValidateMarketListingTradeFlags(input.ItemRef.Definition.TradeFlags); err != nil {
		return err
	}
	if err := input.SourceLocation.Validate(); err != nil {
		return err
	}
	if err := ValidateListingSourceLocation(input.SellerPlayerID, input.SourceLocation); err != nil {
		return err
	}
	if _, err := foundation.NewQuantity(input.Quantity); err != nil {
		return err
	}
	if _, err := foundation.NewMoney(input.UnitPrice); err != nil {
		return err
	}
	if err := input.Currency.Validate(); err != nil {
		return err
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return fmt.Errorf("expires_at %s: %w", input.ExpiresAt, ErrListingExpired)
	}
	return nil
}

func (input BuyListingInput) validate() error {
	if err := input.BuyerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ListingID.Validate(); err != nil {
		return err
	}
	if _, err := foundation.NewQuantity(input.Quantity); err != nil {
		return err
	}
	if err := input.RequestID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input CancelListingInput) validate() error {
	if err := input.SellerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.ListingID.Validate(); err != nil {
		return err
	}
	if err := input.RequestID.Validate(); err != nil {
		return err
	}
	return nil
}

func (input ExpireListingInput) validate() error {
	return input.ListingID.Validate()
}

func (input MarkListingStaleInput) validate() error {
	if err := input.ListingID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return ErrInvalidStaleReason
	}
	return nil
}

func (policy FeePolicy) validate() error {
	if policy.SaleFeeBasisPoints < 0 || policy.SaleFeeBasisPoints >= 10_000 {
		return fmt.Errorf("sale fee basis points %d: %w", policy.SaleFeeBasisPoints, ErrInvalidFeePolicy)
	}
	return nil
}

func (policy SuspiciousTradePolicy) validate() error {
	if policy.HighValueSaleThreshold < 0 {
		return fmt.Errorf("high value sale threshold %d: %w", policy.HighValueSaleThreshold, ErrInvalidFeePolicy)
	}
	if policy.HighValueSaleThreshold > foundation.MaxAmount {
		return fmt.Errorf("high value sale threshold %d: %w", policy.HighValueSaleThreshold, ErrMarketAmountOverflow)
	}
	return nil
}

func (service *MarketService) validateSourceQuantity(
	playerID foundation.PlayerID,
	definition economy.ItemDefinition,
	location economy.ItemLocation,
	quantity int64,
) error {
	available := service.inventory.TotalItemQuantity(playerID, definition.ItemID, location)
	if available < quantity {
		return fmt.Errorf("have %d need %d: %w", available, quantity, economy.ErrInsufficientItemQuantity)
	}
	return nil
}

func (service *MarketService) validateEscrowQuantity(listing Listing, quantity int64) error {
	available := service.inventory.TotalItemQuantity(listing.SellerPlayerID, listing.ItemID, listing.EscrowLocation)
	if available < quantity {
		return fmt.Errorf("escrow have %d need %d: %w", available, quantity, ErrMarketEscrowQuantityMissing)
	}
	return nil
}

func (service *MarketService) validateBuyerFunds(playerID foundation.PlayerID, currency economy.CurrencyBucket, total int64) error {
	current := service.wallet.Balance(playerID, currency)
	if current < total {
		return fmt.Errorf("have %d need %d: %w", current, total, economy.ErrInsufficientWalletFunds)
	}
	return nil
}

func (service *MarketService) validateCreditCapacity(playerID foundation.PlayerID, currency economy.CurrencyBucket, amount int64) error {
	if amount <= 0 {
		return nil
	}
	current := service.wallet.Balance(playerID, currency)
	if amount > math.MaxInt64-current {
		return ErrMarketAmountOverflow
	}
	return nil
}

func (service *MarketService) calculateSettlement(unitPrice int64, quantity int64) (int64, int64, int64, error) {
	if unitPrice > math.MaxInt64/quantity {
		return 0, 0, 0, ErrMarketAmountOverflow
	}
	total := unitPrice * quantity
	if total > foundation.MaxAmount {
		return 0, 0, 0, fmt.Errorf("total %d: %w", total, ErrMarketAmountOverflow)
	}
	fee := (total * service.feePolicy.SaleFeeBasisPoints) / 10_000
	sellerProceeds := total - fee
	if sellerProceeds <= 0 {
		return 0, 0, 0, ErrMarketAmountOverflow
	}
	return total, fee, sellerProceeds, nil
}

type marketBuyMutationSnapshot struct {
	listings            map[foundation.ListingID]Listing
	buyResults          map[foundation.IdempotencyKey]BuyListingResult
	buyIdempotencyRows  map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	buyOutboxRows       map[string]economy.OutboxRow
	suspiciousLogs      []SuspiciousTradeLog
	nextSuspiciousLogID int64
	inventory           economy.InventoryMutationSnapshot
	wallet              economy.WalletMutationSnapshot
}

func (service *MarketService) snapshotMarketBuyMutationLocked() marketBuyMutationSnapshot {
	return marketBuyMutationSnapshot{
		listings:            cloneListingMap(service.listings),
		buyResults:          cloneBuyListingResultMap(service.buyResults),
		buyIdempotencyRows:  cloneIdempotencyKeyRowMap(service.buyIdempotencyRows),
		buyOutboxRows:       cloneOutboxRowMap(service.buyOutboxRows),
		suspiciousLogs:      append([]SuspiciousTradeLog(nil), service.suspiciousTradeLogs...),
		nextSuspiciousLogID: service.nextSuspiciousLogID,
		inventory:           service.inventory.SnapshotMutationState(),
		wallet:              service.wallet.SnapshotMutationState(),
	}
}

func (service *MarketService) restoreMarketBuyMutationLocked(snapshot marketBuyMutationSnapshot) {
	service.listings = cloneListingMap(snapshot.listings)
	service.buyResults = cloneBuyListingResultMap(snapshot.buyResults)
	service.buyIdempotencyRows = cloneIdempotencyKeyRowMap(snapshot.buyIdempotencyRows)
	service.buyOutboxRows = cloneOutboxRowMap(snapshot.buyOutboxRows)
	service.suspiciousTradeLogs = append([]SuspiciousTradeLog(nil), snapshot.suspiciousLogs...)
	service.nextSuspiciousLogID = snapshot.nextSuspiciousLogID
	service.inventory.RestoreMutationState(snapshot.inventory)
	service.wallet.RestoreMutationState(snapshot.wallet)
}

func (service *MarketService) ensureMarketBuyDurabilityMapsLocked() {
	if service.buyIdempotencyRows == nil {
		service.buyIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
	if service.buyOutboxRows == nil {
		service.buyOutboxRows = make(map[string]economy.OutboxRow)
	}
}

type marketCancelMutationSnapshot struct {
	listings              map[foundation.ListingID]Listing
	cancelResults         map[foundation.ListingID]CancelListingResult
	cancelIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	cancelOutboxRows      map[string]economy.OutboxRow
	inventory             economy.InventoryMutationSnapshot
}

func (service *MarketService) snapshotMarketCancelMutationLocked() marketCancelMutationSnapshot {
	return marketCancelMutationSnapshot{
		listings:              cloneListingMap(service.listings),
		cancelResults:         cloneCancelListingResultMap(service.cancelResults),
		cancelIdempotencyRows: cloneIdempotencyKeyRowMap(service.cancelIdempotencyRows),
		cancelOutboxRows:      cloneOutboxRowMap(service.cancelOutboxRows),
		inventory:             service.inventory.SnapshotMutationState(),
	}
}

func (service *MarketService) restoreMarketCancelMutationLocked(snapshot marketCancelMutationSnapshot) {
	service.listings = cloneListingMap(snapshot.listings)
	service.cancelResults = cloneCancelListingResultMap(snapshot.cancelResults)
	service.cancelIdempotencyRows = cloneIdempotencyKeyRowMap(snapshot.cancelIdempotencyRows)
	service.cancelOutboxRows = cloneOutboxRowMap(snapshot.cancelOutboxRows)
	service.inventory.RestoreMutationState(snapshot.inventory)
}

func (service *MarketService) ensureMarketCancelDurabilityMapsLocked() {
	if service.cancelIdempotencyRows == nil {
		service.cancelIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
	}
	if service.cancelOutboxRows == nil {
		service.cancelOutboxRows = make(map[string]economy.OutboxRow)
	}
}

type marketCancelIdempotencyResult struct {
	ListingID         foundation.ListingID      `json:"listing_id"`
	SellerPlayerID    foundation.PlayerID       `json:"seller_player_id"`
	ReturnedQuantity  int64                     `json:"returned_quantity"`
	RemainingQuantity int64                     `json:"remaining_quantity"`
	ListingStatus     ListingStatus             `json:"listing_status"`
	ReferenceKey      foundation.IdempotencyKey `json:"reference_id"`
}

type marketBuyIdempotencyResult struct {
	ListingID         foundation.ListingID      `json:"listing_id"`
	SellerPlayerID    foundation.PlayerID       `json:"seller_player_id"`
	BuyerPlayerID     foundation.PlayerID       `json:"buyer_player_id"`
	Quantity          int64                     `json:"quantity"`
	TotalAmount       int64                     `json:"total_amount"`
	FeeAmount         int64                     `json:"fee_amount"`
	SellerProceeds    int64                     `json:"seller_proceeds"`
	RemainingQuantity int64                     `json:"remaining_quantity"`
	ListingStatus     ListingStatus             `json:"listing_status"`
	Currency          economy.CurrencyBucket    `json:"currency_type"`
	ReferenceKey      foundation.IdempotencyKey `json:"reference_id"`
	SaleReference     foundation.IdempotencyKey `json:"sale_reference_id"`
	FeeReference      foundation.IdempotencyKey `json:"fee_reference_id,omitempty"`
}

type marketBuyOutboxPayload struct {
	ListingID      foundation.ListingID      `json:"listing_id"`
	SellerPlayerID foundation.PlayerID       `json:"seller_player_id"`
	BuyerPlayerID  foundation.PlayerID       `json:"buyer_player_id"`
	Quantity       int64                     `json:"quantity"`
	TotalAmount    int64                     `json:"total_amount"`
	FeeAmount      int64                     `json:"fee_amount"`
	Currency       economy.CurrencyBucket    `json:"currency_type"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
}

type marketCancelOutboxPayload struct {
	ListingID        foundation.ListingID      `json:"listing_id"`
	SellerPlayerID   foundation.PlayerID       `json:"seller_player_id"`
	ReturnedQuantity int64                     `json:"returned_quantity"`
	ReferenceKey     foundation.IdempotencyKey `json:"reference_id"`
}

func (service *MarketService) claimMarketBuyIdempotency(
	input BuyListingInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, BuyListingResult, bool, error) {
	now := service.clock.Now()
	candidate := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   marketBuyOperation,
		PlayerID:    input.BuyerPlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if service.idempotencyStore == nil || service.marketListingTransactionRepository() != nil {
		service.ensureMarketBuyDurabilityMapsLocked()
		existing, ok := service.buyIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, BuyListingResult{}, false, err
			}
			service.buyIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), BuyListingResult{}, false, nil
		}
		return resolveMarketBuyIdempotencyClaim(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, BuyListingResult{}, false, err
	}
	return marketBuyResultFromClaim(claim)
}

func resolveMarketBuyIdempotencyClaim(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, BuyListingResult, bool, error) {
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, BuyListingResult{}, false, err
	}
	return marketBuyResultFromClaim(claim)
}

func marketBuyResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, BuyListingResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, BuyListingResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := marketBuyResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, BuyListingResult{}, false, err
		}
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, BuyListingResult{}, false, nil
	default:
		return claim.Row, BuyListingResult{}, false, ErrMarketBuyInProgress
	}
}

func (service *MarketService) claimMarketCancelIdempotency(
	input CancelListingInput,
	referenceKey foundation.IdempotencyKey,
	requestHash string,
) (economy.IdempotencyKeyRow, CancelListingResult, bool, error) {
	now := service.clock.Now()
	candidate := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   marketCancelOperation,
		PlayerID:    input.SellerPlayerID,
		RequestHash: requestHash,
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if service.idempotencyStore == nil || service.marketListingTransactionRepository() != nil {
		service.ensureMarketCancelDurabilityMapsLocked()
		existing, ok := service.cancelIdempotencyRows[referenceKey]
		if !ok {
			if err := candidate.Validate(); err != nil {
				return economy.IdempotencyKeyRow{}, CancelListingResult{}, false, err
			}
			service.cancelIdempotencyRows[referenceKey] = candidate.Clone()
			return candidate.Clone(), CancelListingResult{}, false, nil
		}
		return resolveMarketCancelIdempotencyClaim(existing, candidate)
	}
	claim, err := service.idempotencyStore.ClaimIdempotencyKey(context.Background(), candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, CancelListingResult{}, false, err
	}
	return marketCancelResultFromClaim(claim)
}

func resolveMarketCancelIdempotencyClaim(
	existing economy.IdempotencyKeyRow,
	candidate economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, CancelListingResult, bool, error) {
	claim, err := economy.ResolveIdempotencyClaim(&existing, candidate)
	if err != nil {
		return economy.IdempotencyKeyRow{}, CancelListingResult{}, false, err
	}
	return marketCancelResultFromClaim(claim)
}

func marketCancelResultFromClaim(claim economy.IdempotencyClaimResult) (economy.IdempotencyKeyRow, CancelListingResult, bool, error) {
	if !claim.Duplicate {
		return claim.Row, CancelListingResult{}, false, nil
	}
	switch claim.Row.Status {
	case economy.IdempotencyStatusCompleted:
		result, err := marketCancelResultFromIdempotencyRow(claim.Row)
		if err != nil {
			return claim.Row, CancelListingResult{}, false, err
		}
		return claim.Row, result, true, nil
	case economy.IdempotencyStatusFailed:
		return claim.Row, CancelListingResult{}, false, nil
	default:
		return claim.Row, CancelListingResult{}, false, ErrMarketCancelInProgress
	}
}

func (service *MarketService) completeMarketBuyIdempotency(row economy.IdempotencyKeyRow, result BuyListingResult, input BuyListingInput) error {
	if row.Key.IsZero() {
		return nil
	}
	completed, err := service.completedMarketBuyIdempotencyRow(row, result, input)
	if err != nil {
		return err
	}
	if service.idempotencyStore != nil && service.marketListingTransactionRepository() == nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), completed)
		if err != nil {
			return err
		}
	}
	return service.recordCompletedMarketBuyRows(completed, economy.OutboxRow{})
}

func (service *MarketService) failMarketBuyIdempotency(row economy.IdempotencyKeyRow, cause error) error {
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
	if service.idempotencyStore != nil && service.marketListingTransactionRepository() == nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row)
		if err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensureMarketBuyDurabilityMapsLocked()
	service.buyIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func (service *MarketService) completeMarketCancelIdempotency(row economy.IdempotencyKeyRow, result CancelListingResult, input CancelListingInput) error {
	if row.Key.IsZero() {
		return nil
	}
	completed, err := service.completedMarketCancelIdempotencyRow(row, result, input)
	if err != nil {
		return err
	}
	if service.idempotencyStore != nil && service.marketListingTransactionRepository() == nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), completed)
		if err != nil {
			return err
		}
	}
	return service.recordCompletedMarketCancelRows(completed, economy.OutboxRow{})
}

func (service *MarketService) failMarketCancelIdempotency(row economy.IdempotencyKeyRow, cause error) error {
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
	if service.idempotencyStore != nil && service.marketListingTransactionRepository() == nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row)
		if err != nil {
			return errors.Join(cause, err)
		}
	}
	service.ensureMarketCancelDurabilityMapsLocked()
	service.cancelIdempotencyRows[row.Key] = row.Clone()
	return cause
}

func (service *MarketService) insertMarketBuyOutbox(result BuyListingResult, input BuyListingInput) error {
	row, err := service.marketBuyOutboxRow(result, input)
	if err != nil {
		return err
	}
	if service.outboxStore != nil {
		if err := service.outboxStore.InsertOutboxRow(context.Background(), row); err != nil {
			return err
		}
	}
	service.ensureMarketBuyDurabilityMapsLocked()
	service.buyOutboxRows[row.OutboxID] = row.Clone()
	return nil
}

func (service *MarketService) insertMarketCancelOutbox(result CancelListingResult, input CancelListingInput) error {
	row, err := service.marketCancelOutboxRow(result, input)
	if err != nil {
		return err
	}
	if service.outboxStore != nil {
		if err := service.outboxStore.InsertOutboxRow(context.Background(), row); err != nil {
			return err
		}
	}
	service.ensureMarketCancelDurabilityMapsLocked()
	service.cancelOutboxRows[row.OutboxID] = row.Clone()
	return nil
}

func (service *MarketService) completedMarketBuyIdempotencyRow(row economy.IdempotencyKeyRow, result BuyListingResult, input BuyListingInput) (economy.IdempotencyKeyRow, error) {
	payload, err := marketBuyIdempotencyResultJSON(result, input)
	if err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	return row, nil
}

func (service *MarketService) completedMarketCancelIdempotencyRow(row economy.IdempotencyKeyRow, result CancelListingResult, input CancelListingInput) (economy.IdempotencyKeyRow, error) {
	payload, err := marketCancelIdempotencyResultJSON(result, input)
	if err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	return row, nil
}

func (service *MarketService) marketBuyOutboxRow(result BuyListingResult, input BuyListingInput) (economy.OutboxRow, error) {
	payload, err := json.Marshal(marketBuyOutboxPayload{
		ListingID:      result.Listing.ListingID,
		SellerPlayerID: result.Listing.SellerPlayerID,
		BuyerPlayerID:  input.BuyerPlayerID,
		Quantity:       result.Quantity,
		TotalAmount:    result.TotalAmount,
		FeeAmount:      result.FeeAmount,
		Currency:       result.Listing.Currency,
		ReferenceKey:   result.ReferenceKey,
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	now := service.clock.Now()
	row, err := economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         "market_buy:" + result.ReferenceKey.String(),
		Topic:            marketBuyOutboxTopic,
		EventType:        marketBuyCompletedEventType,
		AggregateType:    marketBuyAggregateType,
		AggregateID:      result.Listing.ListingID.String(),
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   result.ReferenceKey,
		PayloadJSON:      payload,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	return row, nil
}

func (service *MarketService) marketCancelOutboxRow(result CancelListingResult, input CancelListingInput) (economy.OutboxRow, error) {
	payload, err := json.Marshal(marketCancelOutboxPayload{
		ListingID:        result.Listing.ListingID,
		SellerPlayerID:   input.SellerPlayerID,
		ReturnedQuantity: result.ReturnedQuantity,
		ReferenceKey:     result.ReferenceKey,
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	now := service.clock.Now()
	row, err := economy.NewOutboxRow(economy.OutboxRow{
		OutboxID:         "market_cancel:" + result.ReferenceKey.String(),
		Topic:            marketBuyOutboxTopic,
		EventType:        marketCancelEventType,
		AggregateType:    marketCancelAggregateType,
		AggregateID:      result.Listing.ListingID.String(),
		IdempotencyScope: economy.IdempotencyScopeEconomy,
		IdempotencyKey:   result.ReferenceKey,
		PayloadJSON:      payload,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return economy.OutboxRow{}, err
	}
	return row, nil
}

func (service *MarketService) recordCompletedMarketBuyRows(row economy.IdempotencyKeyRow, outboxRow economy.OutboxRow) error {
	service.ensureMarketBuyDurabilityMapsLocked()
	if existing, ok := service.buyIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.buyIdempotencyRows[row.Key] = row.Clone()
	if outboxRow.OutboxID != "" {
		service.buyOutboxRows[outboxRow.OutboxID] = outboxRow.Clone()
	}
	return nil
}

func (service *MarketService) recordCompletedMarketCancelRows(row economy.IdempotencyKeyRow, outboxRow economy.OutboxRow) error {
	service.ensureMarketCancelDurabilityMapsLocked()
	if existing, ok := service.cancelIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.cancelIdempotencyRows[row.Key] = row.Clone()
	if outboxRow.OutboxID != "" {
		service.cancelOutboxRows[outboxRow.OutboxID] = outboxRow.Clone()
	}
	return nil
}

func marketBuyRequestHash(input BuyListingInput) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"market_buy|listing=%s|buyer=%s|quantity=%d|request=%s",
		input.ListingID,
		input.BuyerPlayerID,
		input.Quantity,
		input.RequestID,
	)))
	return fmt.Sprintf("sha256:%x", hash[:])
}

func marketCancelRequestHash(input CancelListingInput) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf(
		"market_cancel|listing=%s|seller=%s",
		input.ListingID,
		input.SellerPlayerID,
	)))
	return fmt.Sprintf("sha256:%x", hash[:])
}

func marketBuyIdempotencyResultJSON(result BuyListingResult, input BuyListingInput) (json.RawMessage, error) {
	payload, err := json.Marshal(marketBuyIdempotencyResult{
		ListingID:         result.Listing.ListingID,
		SellerPlayerID:    result.Listing.SellerPlayerID,
		BuyerPlayerID:     input.BuyerPlayerID,
		Quantity:          result.Quantity,
		TotalAmount:       result.TotalAmount,
		FeeAmount:         result.FeeAmount,
		SellerProceeds:    result.SellerProceeds,
		RemainingQuantity: result.Listing.RemainingQuantity,
		ListingStatus:     result.Listing.Status,
		Currency:          result.Listing.Currency,
		ReferenceKey:      result.ReferenceKey,
		SaleReference:     result.SaleReference,
		FeeReference:      result.FeeReference,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func marketCancelIdempotencyResultJSON(result CancelListingResult, input CancelListingInput) (json.RawMessage, error) {
	payload, err := json.Marshal(marketCancelIdempotencyResult{
		ListingID:         result.Listing.ListingID,
		SellerPlayerID:    input.SellerPlayerID,
		ReturnedQuantity:  result.ReturnedQuantity,
		RemainingQuantity: result.Listing.RemainingQuantity,
		ListingStatus:     result.Listing.Status,
		ReferenceKey:      result.ReferenceKey,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func marketBuyResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (BuyListingResult, error) {
	var payload marketBuyIdempotencyResult
	if err := json.Unmarshal(row.ResultJSON, &payload); err != nil {
		return BuyListingResult{}, err
	}
	if payload.ReferenceKey.IsZero() {
		return BuyListingResult{}, ErrMarketBuyIdempotencyResult
	}
	return BuyListingResult{
		Listing: Listing{
			ListingID:         payload.ListingID,
			SellerPlayerID:    payload.SellerPlayerID,
			RemainingQuantity: payload.RemainingQuantity,
			Status:            payload.ListingStatus,
			Currency:          payload.Currency,
		},
		Quantity:       payload.Quantity,
		TotalAmount:    payload.TotalAmount,
		FeeAmount:      payload.FeeAmount,
		SellerProceeds: payload.SellerProceeds,
		ReferenceKey:   payload.ReferenceKey,
		SaleReference:  payload.SaleReference,
		FeeReference:   payload.FeeReference,
		Duplicate:      true,
	}, nil
}

func marketCancelResultFromIdempotencyRow(row economy.IdempotencyKeyRow) (CancelListingResult, error) {
	var payload marketCancelIdempotencyResult
	if err := json.Unmarshal(row.ResultJSON, &payload); err != nil {
		return CancelListingResult{}, err
	}
	if payload.ReferenceKey.IsZero() {
		return CancelListingResult{}, ErrMarketCancelIdempotencyResult
	}
	return CancelListingResult{
		Listing: Listing{
			ListingID:         payload.ListingID,
			SellerPlayerID:    payload.SellerPlayerID,
			RemainingQuantity: payload.RemainingQuantity,
			Status:            payload.ListingStatus,
		},
		ReturnedQuantity: payload.ReturnedQuantity,
		ReferenceKey:     payload.ReferenceKey,
		Duplicate:        true,
	}, nil
}

func createListingInputMatchesResult(input CreateListingInput, result CreateListingResult) error {
	listing := result.Listing
	matches := listing.ListingID == input.ListingID &&
		listing.SellerPlayerID == input.SellerPlayerID &&
		listing.ItemID == input.ItemRef.Definition.ItemID &&
		listing.ItemInstanceID == input.ItemRef.ItemInstanceID &&
		listing.OriginalQuantity == input.Quantity &&
		listing.UnitPrice == input.UnitPrice &&
		listing.Currency == input.Currency &&
		listing.SourceReturnLocation == input.SourceLocation &&
		sameOptionalTime(listing.ExpiresAt, input.ExpiresAt)
	if !matches {
		return fmt.Errorf("listing %q: %w", input.ListingID, ErrCreateListingReferenceMismatch)
	}
	return nil
}

func (service *MarketService) recordSuspiciousTradeLocked(listing Listing, input BuyListingInput, total int64, referenceKey foundation.IdempotencyKey) {
	threshold := service.suspiciousPolicy.HighValueSaleThreshold
	if threshold <= 0 || total < threshold {
		return
	}
	service.nextSuspiciousLogID++
	service.suspiciousTradeLogs = append(service.suspiciousTradeLogs, SuspiciousTradeLog{
		LogID:          fmt.Sprintf("market-suspicious-trade-%d", service.nextSuspiciousLogID),
		ListingID:      listing.ListingID,
		SellerPlayerID: listing.SellerPlayerID,
		BuyerPlayerID:  input.BuyerPlayerID,
		Currency:       listing.Currency,
		Quantity:       input.Quantity,
		UnitPrice:      listing.UnitPrice,
		TotalAmount:    total,
		Reason:         "high_value_market_sale",
		ReferenceKey:   referenceKey,
		CreatedAt:      service.clock.Now(),
	})
}

// ValidateListingSourceLocation applies the market source-location policy used by CreateListing.
func ValidateListingSourceLocation(sellerID foundation.PlayerID, location economy.ItemLocation) error {
	if err := economy.ValidatePlayerTradeOrEquipLocation(location, location.Kind == economy.LocationKindShipEquipped); err != nil {
		return err
	}
	switch location.Kind {
	case economy.LocationKindAccountInventory:
		if location.ID.String() != sellerID.String() {
			return fmt.Errorf("source location %q owner %q: %w", location, sellerID, ErrListingSourceLocation)
		}
		return nil
	case economy.LocationKindShipCargo,
		economy.LocationKindPlanetStorage,
		economy.LocationKindStationStorage:
		return nil
	default:
		return fmt.Errorf("source location %q: %w", location.Kind, ErrListingSourceLocation)
	}
}

func marketEscrowLocation(listingID foundation.ListingID) (economy.ItemLocation, error) {
	return economy.NewItemLocation(economy.LocationKindMarketEscrow, listingID.String())
}

func accountInventoryLocation(playerID foundation.PlayerID) (economy.ItemLocation, error) {
	return economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
}

func (listing Listing) isExpired(now time.Time) bool {
	return listing.ExpiresAt != nil && !listing.ExpiresAt.After(now)
}

func (listing *Listing) transitionTo(next ListingStatus) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if !listing.Status.CanTransitionTo(next) {
		return fmt.Errorf("%s to %s: %w", listing.Status, next, ErrInvalidListingTransition)
	}
	listing.Status = next
	return nil
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func sameOptionalTime(left *time.Time, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func cloneCreateListingResult(result CreateListingResult) CreateListingResult {
	result.Listing = cloneListing(result.Listing)
	result.EscrowMove = cloneMoveItemResult(result.EscrowMove)
	return result
}

func cloneBuyListingResult(result BuyListingResult) BuyListingResult {
	result.Listing = cloneListing(result.Listing)
	if result.FeeCredit != nil {
		feeCredit := *result.FeeCredit
		result.FeeCredit = &feeCredit
	}
	result.ItemMove = cloneMoveItemResult(result.ItemMove)
	return result
}

func cloneCancelListingResult(result CancelListingResult) CancelListingResult {
	result.Listing = cloneListing(result.Listing)
	result.ReturnMove = cloneMoveItemResult(result.ReturnMove)
	return result
}

func cloneExpireListingResult(result ExpireListingResult) ExpireListingResult {
	result.Listing = cloneListing(result.Listing)
	result.ReturnMove = cloneMoveItemResult(result.ReturnMove)
	return result
}

func cloneMarkListingStaleResult(result MarkListingStaleResult) MarkListingStaleResult {
	result.Listing = cloneListing(result.Listing)
	return result
}

func cloneListingMap(listings map[foundation.ListingID]Listing) map[foundation.ListingID]Listing {
	if listings == nil {
		return nil
	}
	cloned := make(map[foundation.ListingID]Listing, len(listings))
	for listingID, listing := range listings {
		cloned[listingID] = cloneListing(listing)
	}
	return cloned
}

func cloneBuyListingResultMap(results map[foundation.IdempotencyKey]BuyListingResult) map[foundation.IdempotencyKey]BuyListingResult {
	if results == nil {
		return nil
	}
	cloned := make(map[foundation.IdempotencyKey]BuyListingResult, len(results))
	for referenceKey, result := range results {
		cloned[referenceKey] = cloneBuyListingResult(result)
	}
	return cloned
}

func cloneCancelListingResultMap(results map[foundation.ListingID]CancelListingResult) map[foundation.ListingID]CancelListingResult {
	if results == nil {
		return nil
	}
	cloned := make(map[foundation.ListingID]CancelListingResult, len(results))
	for listingID, result := range results {
		cloned[listingID] = cloneCancelListingResult(result)
	}
	return cloned
}

func cloneIdempotencyKeyRowMap(rows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow) map[foundation.IdempotencyKey]economy.IdempotencyKeyRow {
	if rows == nil {
		return nil
	}
	cloned := make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow, len(rows))
	for referenceKey, row := range rows {
		cloned[referenceKey] = row.Clone()
	}
	return cloned
}

func cloneOutboxRowMap(rows map[string]economy.OutboxRow) map[string]economy.OutboxRow {
	if rows == nil {
		return nil
	}
	cloned := make(map[string]economy.OutboxRow, len(rows))
	for outboxID, row := range rows {
		cloned[outboxID] = row.Clone()
	}
	return cloned
}

func cloneMoveItemResult(result economy.MoveItemResult) economy.MoveItemResult {
	result.StackableItems = append([]economy.StackableItem(nil), result.StackableItems...)
	result.DeletedStackableItems = append([]economy.StackableItem(nil), result.DeletedStackableItems...)
	result.InstanceItems = append([]economy.InstanceItem(nil), result.InstanceItems...)
	result.LedgerEntries = append([]economy.ItemLedgerEntry(nil), result.LedgerEntries...)
	return result
}
