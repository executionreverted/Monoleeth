package admin

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/production"
)

const (
	ReasonAdminCompensation  economy.LedgerReason = "admin_compensation"
	ReasonAdminAuctionRefund economy.LedgerReason = "admin_auction_refund"

	auctionBidLedgerReason    economy.LedgerReason = "auction_bid"
	auctionRefundLedgerReason economy.LedgerReason = "auction_refund"
)

var (
	ErrMissingInventoryService       = errors.New("missing admin inventory service")
	ErrMissingWalletService          = errors.New("missing admin wallet service")
	ErrMissingMarketService          = errors.New("missing admin market service")
	ErrMissingAuctionService         = errors.New("missing admin auction service")
	ErrMissingCraftingService        = errors.New("missing admin crafting service")
	ErrMissingProductionStore        = errors.New("missing admin production store")
	ErrMissingRepairReference        = errors.New("missing admin repair reference")
	ErrUnsupportedLedgerAction       = errors.New("unsupported admin ledger action")
	ErrItemDefinitionMismatch        = errors.New("admin item definition mismatch")
	ErrAuctionBidLedgerMismatch      = errors.New("auction bid ledger mismatch")
	ErrAuctionBidAlreadyRefunded     = errors.New("auction bid already refunded")
	ErrUnsafeActiveAuctionBidRefund  = errors.New("unsafe active auction bid refund")
	ErrUnsafeWinningAuctionBidRefund = errors.New("unsafe winning auction bid refund")
	ErrMarketListingTerminal         = errors.New("market listing terminal")
	ErrCraftJobNotReady              = errors.New("craft job not ready")
)

type InventoryService interface {
	StackableItems() []economy.StackableItem
	InstanceItems() []economy.InstanceItem
	ItemLedgerEntries() []economy.ItemLedgerEntry
	AddItem(input economy.AddItemInput) (economy.AddItemResult, error)
	SystemRemoveItem(input economy.RemoveItemInput) (economy.RemoveItemResult, error)
}

type WalletService interface {
	WalletBalances() []economy.WalletBalance
	CurrencyLedgerEntries() []economy.CurrencyLedgerEntry
	CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error)
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
}

type MarketService interface {
	Listing(listingID foundation.ListingID) (market.Listing, bool)
	CancelListing(input market.CancelListingInput) (market.CancelListingResult, error)
	MarkListingStale(input market.MarkListingStaleInput) (market.MarkListingStaleResult, error)
}

type AuctionService interface {
	Lot(auctionID foundation.AuctionID) (auction.Lot, bool)
}

type CraftingService interface {
	Job(jobID crafting.CraftJobID) (crafting.CraftJob, bool)
	CompleteCraft(input crafting.CompleteCraftInput) (crafting.CompleteCraftResult, error)
}

type ServiceConfig struct {
	Inventory       InventoryService
	Wallet          WalletService
	Market          MarketService
	Auction         AuctionService
	Crafting        CraftingService
	Production      *production.InMemoryStore
	Clock           foundation.Clock
	RouteLossRoller production.RouteLossRoller
}

type Service struct {
	inventory       InventoryService
	wallet          WalletService
	market          MarketService
	auction         AuctionService
	crafting        CraftingService
	production      *production.InMemoryStore
	clock           foundation.Clock
	routeLossRoller production.RouteLossRoller
}

type PlayerInventoryReport struct {
	PlayerID       foundation.PlayerID     `json:"player_id"`
	StackableItems []economy.StackableItem `json:"stackable_items,omitempty"`
	InstanceItems  []economy.InstanceItem  `json:"instance_items,omitempty"`
	GeneratedAt    time.Time               `json:"generated_at"`
}

type PlayerWalletLedgerReport struct {
	PlayerID      foundation.PlayerID           `json:"player_id"`
	Balances      []economy.WalletBalance       `json:"balances,omitempty"`
	LedgerEntries []economy.CurrencyLedgerEntry `json:"ledger_entries,omitempty"`
	GeneratedAt   time.Time                     `json:"generated_at"`
}

type PlayerItemLedgerReport struct {
	PlayerID      foundation.PlayerID       `json:"player_id"`
	LedgerEntries []economy.ItemLedgerEntry `json:"ledger_entries,omitempty"`
	GeneratedAt   time.Time                 `json:"generated_at"`
}

type CompensateCurrencyInput struct {
	LedgerEntry     economy.CurrencyLedgerEntry `json:"ledger_entry"`
	RepairReference string                      `json:"repair_reference"`
}

type CurrencyCompensationResult struct {
	OriginalEntry economy.CurrencyLedgerEntry `json:"original_entry"`
	ReferenceKey  foundation.IdempotencyKey   `json:"reference_id"`
	Credit        *economy.CreditWalletResult `json:"credit,omitempty"`
	Debit         *economy.DebitWalletResult  `json:"debit,omitempty"`
	Duplicate     bool                        `json:"duplicate"`
}

type CompensateItemInput struct {
	LedgerEntry     economy.ItemLedgerEntry `json:"ledger_entry"`
	ItemDefinition  economy.ItemDefinition  `json:"item_definition"`
	RepairReference string                  `json:"repair_reference"`
}

type ItemCompensationResult struct {
	OriginalEntry economy.ItemLedgerEntry   `json:"original_entry"`
	ReferenceKey  foundation.IdempotencyKey `json:"reference_id"`
	Add           *economy.AddItemResult    `json:"add,omitempty"`
	Remove        *economy.RemoveItemResult `json:"remove,omitempty"`
	Duplicate     bool                      `json:"duplicate"`
}

type MarkIntelListingStaleInput struct {
	ListingID foundation.ListingID `json:"listing_id"`
	Reason    string               `json:"reason"`
}

type DisableMarketListingInput struct {
	ListingID foundation.ListingID `json:"listing_id"`
	Reason    string               `json:"reason"`
}

type DisableMarketListingResult struct {
	Stale  *market.MarkListingStaleResult `json:"stale,omitempty"`
	Cancel market.CancelListingResult     `json:"cancel"`
}

type RefundAuctionBidInput struct {
	AuctionID       foundation.AuctionID        `json:"auction_id"`
	BidLedgerEntry  economy.CurrencyLedgerEntry `json:"bid_ledger_entry"`
	RepairReference string                      `json:"repair_reference"`
}

type RefundAuctionBidResult struct {
	Lot          *auction.Lot               `json:"lot,omitempty"`
	ReferenceKey foundation.IdempotencyKey  `json:"reference_id"`
	Credit       economy.CreditWalletResult `json:"credit"`
}

type RepairCraftJobInput struct {
	JobID crafting.CraftJobID `json:"job_id"`
	Now   time.Time           `json:"now,omitempty"`
}

type RepairCraftJobResult struct {
	Job             crafting.CraftJob             `json:"job"`
	Completion      *crafting.CompleteCraftResult `json:"completion,omitempty"`
	AlreadyComplete bool                          `json:"already_complete"`
}

type DryRunPlanetSettlementInput struct {
	PlanetID  foundation.PlanetID `json:"planet_id"`
	SettledAt time.Time           `json:"settled_at"`
}

type DryRunRouteSettlementInput struct {
	RouteID    foundation.RouteID `json:"route_id"`
	SettledAt  time.Time          `json:"settled_at"`
	LossRoller production.RouteLossRoller
}

func NewService(config ServiceConfig) *Service {
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &Service{
		inventory:       config.Inventory,
		wallet:          config.Wallet,
		market:          config.Market,
		auction:         config.Auction,
		crafting:        config.Crafting,
		production:      config.Production,
		clock:           clock,
		routeLossRoller: config.RouteLossRoller,
	}
}

func (service *Service) InspectPlayerInventory(playerID foundation.PlayerID) (PlayerInventoryReport, error) {
	if service == nil || service.inventory == nil {
		return PlayerInventoryReport{}, ErrMissingInventoryService
	}
	if err := playerID.Validate(); err != nil {
		return PlayerInventoryReport{}, err
	}

	report := PlayerInventoryReport{PlayerID: playerID, GeneratedAt: service.clock.Now()}
	for _, item := range service.inventory.StackableItems() {
		if item.OwnerPlayerID == playerID {
			report.StackableItems = append(report.StackableItems, item)
		}
	}
	for _, item := range service.inventory.InstanceItems() {
		if item.OwnerPlayerID == playerID {
			report.InstanceItems = append(report.InstanceItems, item)
		}
	}
	sortStackableItems(report.StackableItems)
	sortInstanceItems(report.InstanceItems)
	return report, nil
}

func (service *Service) InspectPlayerWalletLedger(playerID foundation.PlayerID) (PlayerWalletLedgerReport, error) {
	if service == nil || service.wallet == nil {
		return PlayerWalletLedgerReport{}, ErrMissingWalletService
	}
	if err := playerID.Validate(); err != nil {
		return PlayerWalletLedgerReport{}, err
	}

	report := PlayerWalletLedgerReport{PlayerID: playerID, GeneratedAt: service.clock.Now()}
	for _, balance := range service.wallet.WalletBalances() {
		if balance.PlayerID == playerID {
			report.Balances = append(report.Balances, balance)
		}
	}
	for _, entry := range service.wallet.CurrencyLedgerEntries() {
		if entry.PlayerID == playerID {
			report.LedgerEntries = append(report.LedgerEntries, entry)
		}
	}
	sortWalletBalances(report.Balances)
	sortCurrencyLedgerEntries(report.LedgerEntries)
	return report, nil
}

func (service *Service) InspectPlayerItemLedger(playerID foundation.PlayerID) (PlayerItemLedgerReport, error) {
	if service == nil || service.inventory == nil {
		return PlayerItemLedgerReport{}, ErrMissingInventoryService
	}
	if err := playerID.Validate(); err != nil {
		return PlayerItemLedgerReport{}, err
	}

	report := PlayerItemLedgerReport{PlayerID: playerID, GeneratedAt: service.clock.Now()}
	for _, entry := range service.inventory.ItemLedgerEntries() {
		if entry.PlayerID == playerID {
			report.LedgerEntries = append(report.LedgerEntries, entry)
		}
	}
	sortItemLedgerEntries(report.LedgerEntries)
	return report, nil
}

func (service *Service) CompensateCurrencyLedgerEntry(input CompensateCurrencyInput) (CurrencyCompensationResult, error) {
	if service == nil || service.wallet == nil {
		return CurrencyCompensationResult{}, ErrMissingWalletService
	}
	if err := input.LedgerEntry.Validate(); err != nil {
		return CurrencyCompensationResult{}, err
	}
	referenceKey, err := adminCompensationReference(input.LedgerEntry.LedgerID.String(), input.RepairReference)
	if err != nil {
		return CurrencyCompensationResult{}, err
	}

	result := CurrencyCompensationResult{
		OriginalEntry: input.LedgerEntry,
		ReferenceKey:  referenceKey,
	}
	switch input.LedgerEntry.Action {
	case economy.LedgerActionDecrease:
		credit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     input.LedgerEntry.PlayerID,
			Currency:     input.LedgerEntry.Currency,
			Amount:       input.LedgerEntry.Amount.Int64(),
			Reason:       ReasonAdminCompensation,
			ReferenceKey: referenceKey,
		})
		if err != nil {
			return CurrencyCompensationResult{}, err
		}
		result.Credit = &credit
		result.Duplicate = credit.Duplicate
	case economy.LedgerActionIncrease:
		debit, err := service.wallet.DebitWallet(economy.DebitWalletInput{
			PlayerID:     input.LedgerEntry.PlayerID,
			Currency:     input.LedgerEntry.Currency,
			Amount:       input.LedgerEntry.Amount.Int64(),
			Reason:       ReasonAdminCompensation,
			ReferenceKey: referenceKey,
		})
		if err != nil {
			return CurrencyCompensationResult{}, err
		}
		result.Debit = &debit
		result.Duplicate = debit.Duplicate
	default:
		return CurrencyCompensationResult{}, fmt.Errorf("ledger action %q: %w", input.LedgerEntry.Action, ErrUnsupportedLedgerAction)
	}
	return result, nil
}

func (service *Service) CompensateItemLedgerEntry(input CompensateItemInput) (ItemCompensationResult, error) {
	if service == nil || service.inventory == nil {
		return ItemCompensationResult{}, ErrMissingInventoryService
	}
	if err := input.LedgerEntry.Validate(); err != nil {
		return ItemCompensationResult{}, err
	}
	if err := input.ItemDefinition.Validate(); err != nil {
		return ItemCompensationResult{}, err
	}
	if input.ItemDefinition.ItemID != input.LedgerEntry.ItemID {
		return ItemCompensationResult{}, fmt.Errorf("definition %q ledger %q: %w", input.ItemDefinition.ItemID, input.LedgerEntry.ItemID, ErrItemDefinitionMismatch)
	}
	referenceKey, err := adminCompensationReference(input.LedgerEntry.LedgerID.String(), input.RepairReference)
	if err != nil {
		return ItemCompensationResult{}, err
	}

	result := ItemCompensationResult{
		OriginalEntry: input.LedgerEntry,
		ReferenceKey:  referenceKey,
	}
	switch input.LedgerEntry.Action {
	case economy.LedgerActionDecrease:
		add, err := service.inventory.AddItem(economy.AddItemInput{
			PlayerID:       input.LedgerEntry.PlayerID,
			ItemDefinition: input.ItemDefinition,
			Quantity:       input.LedgerEntry.Quantity.Int64(),
			Location:       input.LedgerEntry.Location,
			Reason:         ReasonAdminCompensation,
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return ItemCompensationResult{}, err
		}
		result.Add = &add
		result.Duplicate = add.Duplicate
	case economy.LedgerActionIncrease:
		remove, err := service.inventory.SystemRemoveItem(economy.RemoveItemInput{
			PlayerID: input.LedgerEntry.PlayerID,
			ItemRef: economy.RemoveItemRef{
				Definition:     input.ItemDefinition,
				ItemInstanceID: input.LedgerEntry.ItemInstanceID,
			},
			SourceLocation: input.LedgerEntry.Location,
			Quantity:       input.LedgerEntry.Quantity.Int64(),
			Reason:         ReasonAdminCompensation,
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return ItemCompensationResult{}, err
		}
		result.Remove = &remove
		result.Duplicate = remove.Duplicate
	default:
		return ItemCompensationResult{}, fmt.Errorf("ledger action %q: %w", input.LedgerEntry.Action, ErrUnsupportedLedgerAction)
	}
	return result, nil
}

func (service *Service) MarkIntelListingStale(input MarkIntelListingStaleInput) (market.MarkListingStaleResult, error) {
	if service == nil || service.market == nil {
		return market.MarkListingStaleResult{}, ErrMissingMarketService
	}
	if err := validateRepairReason(input.Reason); err != nil {
		return market.MarkListingStaleResult{}, err
	}
	return service.market.MarkListingStale(market.MarkListingStaleInput{
		ListingID: input.ListingID,
		Reason:    strings.TrimSpace(input.Reason),
	})
}

func (service *Service) DisableSuspiciousMarketListing(input DisableMarketListingInput) (DisableMarketListingResult, error) {
	if service == nil || service.market == nil {
		return DisableMarketListingResult{}, ErrMissingMarketService
	}
	if err := input.ListingID.Validate(); err != nil {
		return DisableMarketListingResult{}, err
	}
	if err := validateRepairReason(input.Reason); err != nil {
		return DisableMarketListingResult{}, err
	}

	listing, ok := service.market.Listing(input.ListingID)
	if !ok {
		return DisableMarketListingResult{}, fmt.Errorf("listing %q: %w", input.ListingID, market.ErrListingNotFound)
	}
	if listing.Status.IsTerminal() {
		return DisableMarketListingResult{}, fmt.Errorf("listing %q status %q: %w", input.ListingID, listing.Status, ErrMarketListingTerminal)
	}

	var stale *market.MarkListingStaleResult
	if listing.Status == market.ListingStatusActive {
		staleResult, err := service.market.MarkListingStale(market.MarkListingStaleInput{
			ListingID: input.ListingID,
			Reason:    strings.TrimSpace(input.Reason),
		})
		if err != nil {
			return DisableMarketListingResult{}, err
		}
		stale = &staleResult
		listing = staleResult.Listing
	}

	cancel, err := service.market.CancelListing(market.CancelListingInput{
		SellerPlayerID: listing.SellerPlayerID,
		ListingID:      input.ListingID,
		RequestID:      foundation.RequestID("admin-disable-" + input.ListingID.String()),
	})
	if err != nil {
		return DisableMarketListingResult{}, err
	}
	return DisableMarketListingResult{Stale: stale, Cancel: cancel}, nil
}

func (service *Service) RefundAuctionBid(input RefundAuctionBidInput) (RefundAuctionBidResult, error) {
	if service == nil || service.wallet == nil {
		return RefundAuctionBidResult{}, ErrMissingWalletService
	}
	if service.auction == nil {
		return RefundAuctionBidResult{}, ErrMissingAuctionService
	}
	if err := input.AuctionID.Validate(); err != nil {
		return RefundAuctionBidResult{}, err
	}
	if err := input.BidLedgerEntry.Validate(); err != nil {
		return RefundAuctionBidResult{}, err
	}
	if err := validateAuctionBidLedger(input.AuctionID, input.BidLedgerEntry); err != nil {
		return RefundAuctionBidResult{}, err
	}
	if err := validateRepairReason(input.RepairReference); err != nil {
		return RefundAuctionBidResult{}, err
	}
	referenceKey, err := auctionAdminRefundReference(input.BidLedgerEntry.LedgerID)
	if err != nil {
		return RefundAuctionBidResult{}, err
	}

	loaded, ok := service.auction.Lot(input.AuctionID)
	if !ok {
		return RefundAuctionBidResult{}, fmt.Errorf("auction %q: %w", input.AuctionID, auction.ErrLotNotFound)
	}
	lot := &loaded
	if !loaded.Status.IsTerminal() &&
		loaded.CurrentBidderID == input.BidLedgerEntry.PlayerID &&
		loaded.CurrentBid == input.BidLedgerEntry.Amount.Int64() {
		return RefundAuctionBidResult{}, ErrUnsafeActiveAuctionBidRefund
	}
	if loaded.Status == auction.LotStatusClosed && loaded.WinningPlayerID == input.BidLedgerEntry.PlayerID {
		return RefundAuctionBidResult{}, ErrUnsafeWinningAuctionBidRefund
	}
	if auctionBidAlreadyRefunded(service.wallet.CurrencyLedgerEntries(), input.AuctionID, input.BidLedgerEntry, referenceKey) {
		return RefundAuctionBidResult{}, ErrAuctionBidAlreadyRefunded
	}

	credit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     input.BidLedgerEntry.PlayerID,
		Currency:     input.BidLedgerEntry.Currency,
		Amount:       input.BidLedgerEntry.Amount.Int64(),
		Reason:       ReasonAdminAuctionRefund,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return RefundAuctionBidResult{}, err
	}
	return RefundAuctionBidResult{Lot: lot, ReferenceKey: referenceKey, Credit: credit}, nil
}

func (service *Service) RepairStuckCraftJob(input RepairCraftJobInput) (RepairCraftJobResult, error) {
	if service == nil || service.crafting == nil {
		return RepairCraftJobResult{}, ErrMissingCraftingService
	}
	if err := input.JobID.Validate(); err != nil {
		return RepairCraftJobResult{}, err
	}
	job, ok := service.crafting.Job(input.JobID)
	if !ok {
		return RepairCraftJobResult{}, fmt.Errorf("craft job %q: %w", input.JobID, crafting.ErrCraftJobNotFound)
	}
	if job.State == crafting.CraftJobStateCompleted {
		return RepairCraftJobResult{Job: job, AlreadyComplete: true}, nil
	}
	now := input.Now
	if now.IsZero() {
		now = service.clock.Now()
	}
	if now.Before(job.CompletesAt) {
		return RepairCraftJobResult{}, fmt.Errorf("craft job %q completes_at %s: %w", input.JobID, job.CompletesAt, ErrCraftJobNotReady)
	}
	completion, err := service.crafting.CompleteCraft(crafting.CompleteCraftInput{
		PlayerID: job.PlayerID,
		JobID:    input.JobID,
	})
	if err != nil {
		return RepairCraftJobResult{}, err
	}
	return RepairCraftJobResult{Job: completion.Job, Completion: &completion}, nil
}

func (service *Service) DryRunPlanetSettlement(input DryRunPlanetSettlementInput) (production.PlanetProductionSettlementResult, error) {
	if service == nil || service.production == nil {
		return production.PlanetProductionSettlementResult{}, ErrMissingProductionStore
	}
	if err := input.PlanetID.Validate(); err != nil {
		return production.PlanetProductionSettlementResult{}, err
	}
	if input.SettledAt.IsZero() {
		return production.PlanetProductionSettlementResult{}, fmt.Errorf("settled_at: %w", production.ErrZeroProductionTimestamp)
	}
	return service.production.Clone().SettlePlanetProduction(input.PlanetID, input.SettledAt)
}

func (service *Service) DryRunRouteSettlement(input DryRunRouteSettlementInput) (production.RouteSettlementResult, error) {
	if service == nil || service.production == nil {
		return production.RouteSettlementResult{}, ErrMissingProductionStore
	}
	if err := input.RouteID.Validate(); err != nil {
		return production.RouteSettlementResult{}, err
	}
	if input.SettledAt.IsZero() {
		return production.RouteSettlementResult{}, fmt.Errorf("settled_at: %w", production.ErrZeroProductionTimestamp)
	}
	roller := input.LossRoller
	if roller == nil {
		roller = service.routeLossRoller
	}
	return service.production.Clone().SettleRoute(input.RouteID, input.SettledAt, roller)
}

func adminCompensationReference(subjectID string, repairReference string) (foundation.IdempotencyKey, error) {
	if err := validateRepairReason(repairReference); err != nil {
		return "", err
	}
	return foundation.AdminCompensationIdempotencyKey(subjectID, strings.TrimSpace(repairReference))
}

func auctionAdminRefundReference(ledgerID economy.LedgerID) (foundation.IdempotencyKey, error) {
	if err := ledgerID.Validate(); err != nil {
		return "", err
	}
	return foundation.AdminCompensationIdempotencyKey("auction-"+ledgerID.String(), "refund")
}

func validateRepairReason(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrMissingRepairReference
	}
	return nil
}

func validateAuctionBidLedger(auctionID foundation.AuctionID, entry economy.CurrencyLedgerEntry) error {
	if entry.Action != economy.LedgerActionDecrease || entry.Reason != auctionBidLedgerReason {
		return fmt.Errorf("ledger %q action %q reason %q: %w", entry.LedgerID, entry.Action, entry.Reason, ErrAuctionBidLedgerMismatch)
	}
	expectedPrefix := "auction_bid:" + auctionID.String() + ":" + entry.PlayerID.String() + ":"
	if !strings.HasPrefix(entry.ReferenceKey.String(), expectedPrefix) {
		return fmt.Errorf("ledger %q reference %q: %w", entry.LedgerID, entry.ReferenceKey, ErrAuctionBidLedgerMismatch)
	}
	return nil
}

func auctionBidAlreadyRefunded(
	entries []economy.CurrencyLedgerEntry,
	auctionID foundation.AuctionID,
	bidEntry economy.CurrencyLedgerEntry,
	adminReferenceKey foundation.IdempotencyKey,
) bool {
	for _, entry := range entries {
		if entry.PlayerID != bidEntry.PlayerID ||
			entry.Currency != bidEntry.Currency ||
			entry.Action != economy.LedgerActionIncrease ||
			entry.Amount.Int64() != bidEntry.Amount.Int64() {
			continue
		}
		if entry.Reason == auctionRefundLedgerReason && hasAuctionRefundReference(entry.ReferenceKey, auctionID, bidEntry.PlayerID) {
			return true
		}
		if entry.Reason == ReasonAdminAuctionRefund &&
			entry.ReferenceKey != adminReferenceKey &&
			hasAdminAuctionRefundReference(entry.ReferenceKey, bidEntry.LedgerID) {
			return true
		}
	}
	return false
}

func hasAuctionRefundReference(key foundation.IdempotencyKey, auctionID foundation.AuctionID, playerID foundation.PlayerID) bool {
	prefixes := []string{
		"auction_refund:" + auctionID.String() + ":" + playerID.String() + ":",
		"auction_buy_now_refund:" + auctionID.String() + ":" + playerID.String() + ":",
	}
	value := key.String()
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func hasAdminAuctionRefundReference(key foundation.IdempotencyKey, ledgerID economy.LedgerID) bool {
	return strings.HasPrefix(key.String(), "admin_compensation:auction-"+ledgerID.String()+":")
}

func sortStackableItems(items []economy.StackableItem) {
	sort.Slice(items, func(i, j int) bool {
		return itemSortKey(items[i].Location, items[i].ItemID, items[i].ItemInstanceID) <
			itemSortKey(items[j].Location, items[j].ItemID, items[j].ItemInstanceID)
	})
}

func sortInstanceItems(items []economy.InstanceItem) {
	sort.Slice(items, func(i, j int) bool {
		return itemSortKey(items[i].Location, items[i].ItemID, items[i].ItemInstanceID) <
			itemSortKey(items[j].Location, items[j].ItemID, items[j].ItemInstanceID)
	})
}

func itemSortKey(location economy.ItemLocation, itemID foundation.ItemID, instanceID foundation.ItemID) string {
	return location.String() + ":" + itemID.String() + ":" + instanceID.String()
}

func sortWalletBalances(balances []economy.WalletBalance) {
	sort.Slice(balances, func(i, j int) bool {
		return balances[i].Currency < balances[j].Currency
	})
}

func sortCurrencyLedgerEntries(entries []economy.CurrencyLedgerEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].LedgerID < entries[j].LedgerID
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
}

func sortItemLedgerEntries(entries []economy.ItemLedgerEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].LedgerID < entries[j].LedgerID
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
}
