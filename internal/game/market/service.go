package market

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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
	marketBuyAggregateType      = "market_listing"

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

// WalletService is the economy wallet boundary used by market settlement.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	Balance(playerID foundation.PlayerID, currency economy.CurrencyBucket) int64
	SnapshotMutationState() economy.WalletMutationSnapshot
	RestoreMutationState(snapshot economy.WalletMutationSnapshot)
}

// MarketServiceConfig wires MarketService to economy primitives.
type MarketServiceConfig struct {
	Clock             foundation.Clock
	Inventory         InventoryService
	Wallet            WalletService
	IdempotencyStore  economy.IdempotencyStore
	OutboxStore       economy.OutboxStore
	SettlementLogger  observability.SettlementLogger
	FeePolicy         FeePolicy
	SuspiciousPolicy  SuspiciousTradePolicy
	SystemFeePlayerID foundation.PlayerID
}

// MarketService owns in-memory fixed-price listing state for the MVP.
type MarketService struct {
	mu    sync.Mutex
	clock foundation.Clock

	inventory         InventoryService
	wallet            WalletService
	idempotencyStore  economy.IdempotencyStore
	outboxStore       economy.OutboxStore
	settlementLogger  observability.SettlementLogger
	feePolicy         FeePolicy
	suspiciousPolicy  SuspiciousTradePolicy
	systemFeePlayerID foundation.PlayerID

	listings              map[foundation.ListingID]Listing
	createResults         map[foundation.IdempotencyKey]CreateListingResult
	buyResults            map[foundation.IdempotencyKey]BuyListingResult
	cancelResults         map[foundation.ListingID]CancelListingResult
	expireResults         map[foundation.ListingID]ExpireListingResult
	staleResults          map[foundation.ListingID]MarkListingStaleResult
	buyIdempotencyRows    map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	cancelIdempotencyRows map[foundation.IdempotencyKey]economy.IdempotencyKeyRow
	buyOutboxRows         map[string]economy.OutboxRow

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

	return &MarketService{
		clock:                 clock,
		inventory:             config.Inventory,
		wallet:                config.Wallet,
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
	buyerDebit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.BuyerPlayerID,
		Currency:     listing.Currency,
		Amount:       total,
		Reason:       marketBuyReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return BuyListingResult{}, service.failMarketBuyIdempotency(idempotencyRow, err)
	}
	sellerCredit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
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
		credit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
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
	itemMove, err := service.inventory.SystemMoveItem(economy.MoveItemInput{
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
	if err := service.insertMarketBuyOutbox(result, input); err != nil {
		return rollback(err)
	}
	if err := service.completeMarketBuyIdempotency(idempotencyRow, result, input); err != nil {
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

	returnMove, err := service.inventory.SystemMoveItem(economy.MoveItemInput{
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
	if err := service.completeMarketCancelIdempotency(idempotencyRow, result, input); err != nil {
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
	inventory             economy.InventoryMutationSnapshot
}

func (service *MarketService) snapshotMarketCancelMutationLocked() marketCancelMutationSnapshot {
	return marketCancelMutationSnapshot{
		listings:              cloneListingMap(service.listings),
		cancelResults:         cloneCancelListingResultMap(service.cancelResults),
		cancelIdempotencyRows: cloneIdempotencyKeyRowMap(service.cancelIdempotencyRows),
		inventory:             service.inventory.SnapshotMutationState(),
	}
}

func (service *MarketService) restoreMarketCancelMutationLocked(snapshot marketCancelMutationSnapshot) {
	service.listings = cloneListingMap(snapshot.listings)
	service.cancelResults = cloneCancelListingResultMap(snapshot.cancelResults)
	service.cancelIdempotencyRows = cloneIdempotencyKeyRowMap(snapshot.cancelIdempotencyRows)
	service.inventory.RestoreMutationState(snapshot.inventory)
}

func (service *MarketService) ensureMarketCancelDurabilityMapsLocked() {
	if service.cancelIdempotencyRows == nil {
		service.cancelIdempotencyRows = make(map[foundation.IdempotencyKey]economy.IdempotencyKeyRow)
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
	if service.idempotencyStore == nil {
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
	if service.idempotencyStore == nil {
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
	payload, err := marketBuyIdempotencyResultJSON(result, input)
	if err != nil {
		return err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	if service.idempotencyStore != nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row)
		if err != nil {
			return err
		}
	}
	service.ensureMarketBuyDurabilityMapsLocked()
	if existing, ok := service.buyIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.buyIdempotencyRows[row.Key] = row.Clone()
	return nil
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
	if service.idempotencyStore != nil {
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
	payload, err := marketCancelIdempotencyResultJSON(result, input)
	if err != nil {
		return err
	}
	now := service.clock.Now()
	row.Status = economy.IdempotencyStatusCompleted
	row.ResultJSON = payload
	row.UpdatedAt = now
	row.CompletedAt = now
	if service.idempotencyStore != nil {
		_, err = service.idempotencyStore.CompleteIdempotencyKey(context.Background(), row)
		if err != nil {
			return err
		}
	}
	service.ensureMarketCancelDurabilityMapsLocked()
	if existing, ok := service.cancelIdempotencyRows[row.Key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return err
		}
	}
	service.cancelIdempotencyRows[row.Key] = row.Clone()
	return nil
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
	if service.idempotencyStore != nil {
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
		return err
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
	result.InstanceItems = append([]economy.InstanceItem(nil), result.InstanceItems...)
	result.LedgerEntries = append([]economy.ItemLedgerEntry(nil), result.LedgerEntries...)
	return result
}
