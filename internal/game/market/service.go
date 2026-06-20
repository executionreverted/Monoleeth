package market

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const (
	marketListingReason economy.LedgerReason = "market_listing"
	marketBuyReason     economy.LedgerReason = "market_buy"
	marketSaleReason    economy.LedgerReason = "market_sale"
	marketFeeReason     economy.LedgerReason = "market_fee"
	marketCancelReason  economy.LedgerReason = "market_cancel"
	marketExpireReason  economy.LedgerReason = "market_expire"

	defaultSystemFeePlayerID             foundation.PlayerID = "market-fee-sink"
	defaultHighValueSaleThresholdCredits int64               = 100
)

// InventoryService is the economy inventory boundary used by market escrow.
type InventoryService interface {
	SystemMoveItem(input economy.MoveItemInput) (economy.MoveItemResult, error)
	TotalItemQuantity(playerID foundation.PlayerID, itemID foundation.ItemID, location economy.ItemLocation) int64
}

// WalletService is the economy wallet boundary used by market settlement.
type WalletService interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	Balance(playerID foundation.PlayerID, currency economy.CurrencyBucket) int64
}

// MarketServiceConfig wires MarketService to economy primitives.
type MarketServiceConfig struct {
	Clock             foundation.Clock
	Inventory         InventoryService
	Wallet            WalletService
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
	feePolicy         FeePolicy
	suspiciousPolicy  SuspiciousTradePolicy
	systemFeePlayerID foundation.PlayerID

	listings      map[foundation.ListingID]Listing
	createResults map[foundation.IdempotencyKey]CreateListingResult
	buyResults    map[foundation.IdempotencyKey]BuyListingResult
	cancelResults map[foundation.ListingID]CancelListingResult
	expireResults map[foundation.ListingID]ExpireListingResult
	staleResults  map[foundation.ListingID]MarkListingStaleResult

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
		clock:             clock,
		inventory:         config.Inventory,
		wallet:            config.Wallet,
		feePolicy:         feePolicy,
		suspiciousPolicy:  suspiciousPolicy,
		systemFeePlayerID: systemFeePlayerID,
		listings:          make(map[foundation.ListingID]Listing),
		createResults:     make(map[foundation.IdempotencyKey]CreateListingResult),
		buyResults:        make(map[foundation.IdempotencyKey]BuyListingResult),
		cancelResults:     make(map[foundation.ListingID]CancelListingResult),
		expireResults:     make(map[foundation.ListingID]ExpireListingResult),
		staleResults:      make(map[foundation.ListingID]MarkListingStaleResult),
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
func (service *MarketService) BuyListing(input BuyListingInput) (BuyListingResult, error) {
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
		return BuyListingResult{}, err
	}
	sellerCredit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     listing.SellerPlayerID,
		Currency:     listing.Currency,
		Amount:       sellerProceeds,
		Reason:       marketSaleReason,
		ReferenceKey: saleReference,
	})
	if err != nil {
		return BuyListingResult{}, err
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
			return BuyListingResult{}, err
		}
		feeCredit = &credit
	}

	buyerLocation, err := accountInventoryLocation(input.BuyerPlayerID)
	if err != nil {
		return BuyListingResult{}, err
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
		return BuyListingResult{}, err
	}

	listing.RemainingQuantity -= input.Quantity
	if listing.RemainingQuantity == 0 {
		if err := listing.transitionTo(ListingStatusSold); err != nil {
			return BuyListingResult{}, err
		}
	}
	listing.UpdatedAt = service.clock.Now()
	service.listings[input.ListingID] = cloneListing(listing)
	service.recordSuspiciousTradeLocked(listing, input, total, referenceKey)

	result := BuyListingResult{
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
	service.buyResults[referenceKey] = cloneBuyListingResult(result)
	return result, nil
}

// CancelListing returns remaining active escrow to the seller's recorded source location.
func (service *MarketService) CancelListing(input CancelListingInput) (CancelListingResult, error) {
	if err := input.validate(); err != nil {
		return CancelListingResult{}, err
	}
	referenceKey, err := foundation.MarketCancelIdempotencyKey(input.ListingID)
	if err != nil {
		return CancelListingResult{}, err
	}

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
	if listing.Status != ListingStatusActive && listing.Status != ListingStatusStale {
		return CancelListingResult{}, fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrListingNotActive)
	}
	if listing.RemainingQuantity <= 0 {
		return CancelListingResult{}, fmt.Errorf("listing remaining %d: %w", listing.RemainingQuantity, ErrListingNotActive)
	}
	if err := service.validateEscrowQuantity(listing, listing.RemainingQuantity); err != nil {
		return CancelListingResult{}, err
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
		return CancelListingResult{}, err
	}

	returnedQuantity := listing.RemainingQuantity
	if err := listing.transitionTo(ListingStatusCancelled); err != nil {
		return CancelListingResult{}, err
	}
	listing.UpdatedAt = service.clock.Now()
	service.listings[input.ListingID] = cloneListing(listing)

	result := CancelListingResult{
		Listing:          cloneListing(listing),
		ReturnedQuantity: returnedQuantity,
		ReturnMove:       returnMove,
		ReferenceKey:     referenceKey,
	}
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

func cloneMoveItemResult(result economy.MoveItemResult) economy.MoveItemResult {
	result.StackableItems = append([]economy.StackableItem(nil), result.StackableItems...)
	result.InstanceItems = append([]economy.InstanceItem(nil), result.InstanceItems...)
	result.LedgerEntries = append([]economy.ItemLedgerEntry(nil), result.LedgerEntries...)
	return result
}
