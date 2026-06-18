package simulations

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/auction"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
)

const (
	simulationSeedReason      economy.LedgerReason = "simulation_seed"
	simulationMarketUnitPrice int64                = 100
	simulationAuctionBid      int64                = 120
	simulationAuctionBuyNow   int64                = 300
	simulationMarketFeeSink   foundation.PlayerID  = "simulation_market_fee_sink"
)

// MarketBuyCancelRaceConfig tunes fixed-price market buy/cancel race simulations.
type MarketBuyCancelRaceConfig struct {
	Races     int
	StartTime time.Time
}

// MarketBuyCancelRaceSummary reports conservation checks for market buy/cancel races.
type MarketBuyCancelRaceSummary struct {
	Races                    int
	BuySuccesses             int
	CancelSuccesses          int
	ListingNotActiveFailures int
	SellerSourceQuantity     int64
	BuyerQuantity            int64
	EscrowQuantity           int64
	BuyerBalance             int64
	SellerBalance            int64
	SystemFeeBalance         int64
}

// AuctionBidBuyNowRaceConfig tunes auction bid/buy-now race simulations.
type AuctionBidBuyNowRaceConfig struct {
	Races     int
	StartTime time.Time
}

// AuctionBidBuyNowRaceSummary reports terminal-state checks for auction races.
type AuctionBidBuyNowRaceSummary struct {
	Races                int
	BidSuccesses         int
	BuyNowSuccesses      int
	LotNotActiveFailures int
	ClosedLots           int
	BuyerGrants          int
	BidderBalance        int64
	BuyerBalance         int64
}

type normalizedRaceConfig struct {
	races     int
	startTime time.Time
}

// RunMarketBuyCancelRaceSimulation runs deterministic market buy/cancel races
// against the fixed-price market service and its economy primitives.
func RunMarketBuyCancelRaceSimulation(config MarketBuyCancelRaceConfig) (MarketBuyCancelRaceSummary, error) {
	normalized, err := normalizeRaceConfig(config.Races, config.StartTime)
	if err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}

	clock := &simulationClock{now: normalized.startTime}
	inventory := economy.NewInventoryService(clock)
	wallet := economy.NewWalletService(clock)
	service, err := market.NewMarketService(market.MarketServiceConfig{
		Clock:             clock,
		Inventory:         inventory,
		Wallet:            wallet,
		FeePolicy:         market.DefaultFeePolicy(),
		SystemFeePlayerID: simulationMarketFeeSink,
	})
	if err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}

	sellerID := foundation.PlayerID("simulation_seller")
	buyerID := foundation.PlayerID("simulation_buyer")
	sourceLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, sellerID.String())
	if err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}
	buyerLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, buyerID.String())
	if err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}
	itemDefinition, err := simulationMarketItemDefinition()
	if err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}
	if err := seedSimulationItems(inventory, sellerID, itemDefinition, int64(normalized.races), sourceLocation, "market_seed_items"); err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}
	initialBuyerCredits := int64(normalized.races) * 1_000
	if err := seedSimulationCredits(wallet, buyerID, initialBuyerCredits, "market_seed_buyer"); err != nil {
		return MarketBuyCancelRaceSummary{}, err
	}

	summary := MarketBuyCancelRaceSummary{Races: normalized.races}
	escrowLocations := make([]economy.ItemLocation, 0, normalized.races)
	for index := 0; index < normalized.races; index++ {
		listingID := foundation.ListingID(fmt.Sprintf("simulation_market_listing_%d", index+1))
		create, err := service.CreateListing(market.CreateListingInput{
			ListingID:      listingID,
			SellerPlayerID: sellerID,
			ItemRef:        economy.MoveItemRef{Definition: itemDefinition},
			SourceLocation: sourceLocation,
			Quantity:       1,
			UnitPrice:      simulationMarketUnitPrice,
			Currency:       economy.CurrencyBucketCredits,
		})
		if err != nil {
			return MarketBuyCancelRaceSummary{}, err
		}
		escrowLocations = append(escrowLocations, create.Listing.EscrowLocation)

		outcomes := runMarketBuyCancelRace(service, buyerID, sellerID, listingID, foundation.RequestID(fmt.Sprintf("simulation_market_buy_%d", index+1)))
		raceSuccesses := 0
		raceListingFailures := 0
		for _, outcome := range outcomes {
			switch {
			case outcome.err == nil && outcome.operation == "buy":
				raceSuccesses++
				summary.BuySuccesses++
			case outcome.err == nil && outcome.operation == "cancel":
				raceSuccesses++
				summary.CancelSuccesses++
			case errors.Is(outcome.err, market.ErrListingNotActive):
				raceListingFailures++
				summary.ListingNotActiveFailures++
			default:
				return MarketBuyCancelRaceSummary{}, outcome.err
			}
		}
		if raceSuccesses != 1 || raceListingFailures != 1 {
			return MarketBuyCancelRaceSummary{}, fmt.Errorf("market race %d successes/failures = %d/%d: %w", index, raceSuccesses, raceListingFailures, ErrInvalidSimulationConfig)
		}
	}

	summary.SellerSourceQuantity = inventory.TotalItemQuantity(sellerID, itemDefinition.ItemID, sourceLocation)
	summary.BuyerQuantity = inventory.TotalItemQuantity(buyerID, itemDefinition.ItemID, buyerLocation)
	for _, escrowLocation := range escrowLocations {
		summary.EscrowQuantity += inventory.TotalItemQuantity(sellerID, itemDefinition.ItemID, escrowLocation)
	}
	summary.BuyerBalance = wallet.Balance(buyerID, economy.CurrencyBucketCredits)
	summary.SellerBalance = wallet.Balance(sellerID, economy.CurrencyBucketCredits)
	summary.SystemFeeBalance = wallet.Balance(simulationMarketFeeSink, economy.CurrencyBucketCredits)
	expectedBuyerBalance := initialBuyerCredits - int64(summary.BuySuccesses)*simulationMarketUnitPrice
	expectedFeeBalance := int64(summary.BuySuccesses) * ((simulationMarketUnitPrice * market.DefaultFeePolicy().SaleFeeBasisPoints) / 10_000)
	expectedSellerBalance := int64(summary.BuySuccesses)*simulationMarketUnitPrice - expectedFeeBalance
	if summary.BuySuccesses+summary.CancelSuccesses != normalized.races ||
		summary.ListingNotActiveFailures != normalized.races ||
		summary.SellerSourceQuantity+summary.BuyerQuantity != int64(normalized.races) ||
		summary.EscrowQuantity != 0 ||
		summary.BuyerBalance != expectedBuyerBalance ||
		summary.SellerBalance != expectedSellerBalance ||
		summary.SystemFeeBalance != expectedFeeBalance {
		return MarketBuyCancelRaceSummary{}, fmt.Errorf("market race summary %+v: %w", summary, ErrInvalidSimulationConfig)
	}
	return summary, nil
}

// RunAuctionBidBuyNowRaceSimulation runs deterministic bid/buy-now races
// against the auction service and wallet primitive.
func RunAuctionBidBuyNowRaceSimulation(config AuctionBidBuyNowRaceConfig) (AuctionBidBuyNowRaceSummary, error) {
	normalized, err := normalizeRaceConfig(config.Races, config.StartTime)
	if err != nil {
		return AuctionBidBuyNowRaceSummary{}, err
	}

	clock := &simulationClock{now: normalized.startTime}
	wallet := economy.NewWalletService(clock)
	service, err := auction.NewService(auction.ServiceConfig{
		Clock:  clock,
		Wallet: wallet,
	})
	if err != nil {
		return AuctionBidBuyNowRaceSummary{}, err
	}

	bidderID := foundation.PlayerID("simulation_bidder")
	buyerID := foundation.PlayerID("simulation_auction_buyer")
	initialBidderCredits := int64(normalized.races) * 500
	initialBuyerCredits := int64(normalized.races) * 1_000
	if err := seedSimulationCredits(wallet, bidderID, initialBidderCredits, "auction_seed_bidder"); err != nil {
		return AuctionBidBuyNowRaceSummary{}, err
	}
	if err := seedSimulationCredits(wallet, buyerID, initialBuyerCredits, "auction_seed_buyer"); err != nil {
		return AuctionBidBuyNowRaceSummary{}, err
	}

	summary := AuctionBidBuyNowRaceSummary{Races: normalized.races}
	expectedGrants := make(map[foundation.AuctionID]auction.LotPayload, normalized.races)
	for index := 0; index < normalized.races; index++ {
		auctionID := foundation.AuctionID(fmt.Sprintf("simulation_auction_%d", index+1))
		buyNowPrice := simulationAuctionBuyNow
		payload, err := simulationAuctionPayload()
		if err != nil {
			return AuctionBidBuyNowRaceSummary{}, err
		}
		expectedGrants[auctionID] = payload
		if _, err := service.CreateLot(auction.CreateLotInput{
			AuctionID:   auctionID,
			WorldID:     "world_1",
			Payload:     payload,
			Currency:    economy.CurrencyBucketCredits,
			StartPrice:  100,
			BuyNowPrice: &buyNowPrice,
			StartsAt:    clock.Now().Add(-time.Minute),
			EndsAt:      clock.Now().Add(time.Hour),
		}); err != nil {
			return AuctionBidBuyNowRaceSummary{}, err
		}

		outcomes := runAuctionBidBuyNowRace(
			service,
			auctionID,
			bidderID,
			buyerID,
			foundation.RequestID(fmt.Sprintf("simulation_bid_%d", index+1)),
			foundation.RequestID(fmt.Sprintf("simulation_buy_now_%d", index+1)),
		)
		raceBidSuccesses := 0
		raceBuyNowSuccesses := 0
		raceLotFailures := 0
		for _, outcome := range outcomes {
			switch {
			case outcome.err == nil && outcome.operation == "bid":
				raceBidSuccesses++
				summary.BidSuccesses++
			case outcome.err == nil && outcome.operation == "buy_now":
				raceBuyNowSuccesses++
				summary.BuyNowSuccesses++
			case errors.Is(outcome.err, auction.ErrLotNotActive):
				raceLotFailures++
				summary.LotNotActiveFailures++
			default:
				return AuctionBidBuyNowRaceSummary{}, outcome.err
			}
		}
		if raceBuyNowSuccesses != 1 || raceBidSuccesses+raceLotFailures != 1 {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction race %d bid/buy/failures = %d/%d/%d: %w", index, raceBidSuccesses, raceBuyNowSuccesses, raceLotFailures, ErrInvalidSimulationConfig)
		}

		lot, ok := service.Lot(auctionID)
		if !ok {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction %q: %w", auctionID, ErrInvalidSimulationConfig)
		}
		if lot.Status == auction.LotStatusClosed && lot.WinningPlayerID == buyerID {
			summary.ClosedLots++
		} else {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction %q final status/winner = %q/%q: %w", auctionID, lot.Status, lot.WinningPlayerID, ErrInvalidSimulationConfig)
		}
	}

	seenGrants := make(map[foundation.AuctionID]struct{}, normalized.races)
	for _, grant := range service.Grants() {
		expectedPayload, ok := expectedGrants[grant.AuctionID]
		if !ok {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction unexpected grant %q: %w", grant.AuctionID, ErrInvalidSimulationConfig)
		}
		if _, exists := seenGrants[grant.AuctionID]; exists {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction duplicate grant %q: %w", grant.AuctionID, ErrInvalidSimulationConfig)
		}
		if grant.PlayerID != buyerID ||
			grant.Reason != auction.CloseReasonBuyNow ||
			!auctionPayloadsEqual(grant.Payload, expectedPayload) {
			return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction grant %q = %+v: %w", grant.AuctionID, grant, ErrInvalidSimulationConfig)
		}
		seenGrants[grant.AuctionID] = struct{}{}
		summary.BuyerGrants++
	}
	if len(seenGrants) != len(expectedGrants) {
		return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction grants seen %d want %d: %w", len(seenGrants), len(expectedGrants), ErrInvalidSimulationConfig)
	}
	summary.BidderBalance = wallet.Balance(bidderID, economy.CurrencyBucketCredits)
	summary.BuyerBalance = wallet.Balance(buyerID, economy.CurrencyBucketCredits)
	if summary.BuyNowSuccesses != normalized.races ||
		summary.BidSuccesses+summary.LotNotActiveFailures != normalized.races ||
		summary.ClosedLots != normalized.races ||
		summary.BuyerGrants != normalized.races ||
		summary.BidderBalance != initialBidderCredits ||
		summary.BuyerBalance != initialBuyerCredits-int64(normalized.races)*simulationAuctionBuyNow {
		return AuctionBidBuyNowRaceSummary{}, fmt.Errorf("auction race summary %+v: %w", summary, ErrInvalidSimulationConfig)
	}
	return summary, nil
}

func normalizeRaceConfig(races int, startTime time.Time) (normalizedRaceConfig, error) {
	normalized := normalizedRaceConfig{
		races:     races,
		startTime: startTime,
	}
	if normalized.races == 0 {
		normalized.races = 1
	}
	if normalized.startTime.IsZero() {
		normalized.startTime = time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	}
	if normalized.races < 1 {
		return normalizedRaceConfig{}, fmt.Errorf("races %d: %w", normalized.races, ErrInvalidSimulationConfig)
	}
	return normalized, nil
}

type raceOutcome struct {
	operation string
	err       error
}

func runMarketBuyCancelRace(
	service *market.MarketService,
	buyerID foundation.PlayerID,
	sellerID foundation.PlayerID,
	listingID foundation.ListingID,
	requestID foundation.RequestID,
) []raceOutcome {
	return runTwoOperationRace(
		func() raceOutcome {
			_, err := service.BuyListing(market.BuyListingInput{
				BuyerPlayerID: buyerID,
				ListingID:     listingID,
				Quantity:      1,
				RequestID:     requestID,
			})
			return raceOutcome{operation: "buy", err: err}
		},
		func() raceOutcome {
			_, err := service.CancelListing(market.CancelListingInput{
				SellerPlayerID: sellerID,
				ListingID:      listingID,
			})
			return raceOutcome{operation: "cancel", err: err}
		},
	)
}

func runAuctionBidBuyNowRace(
	service *auction.Service,
	auctionID foundation.AuctionID,
	bidderID foundation.PlayerID,
	buyerID foundation.PlayerID,
	bidRequestID foundation.RequestID,
	buyNowRequestID foundation.RequestID,
) []raceOutcome {
	return runTwoOperationRace(
		func() raceOutcome {
			_, err := service.PlaceBid(auction.PlaceBidInput{
				AuctionID:      auctionID,
				BidderPlayerID: bidderID,
				Amount:         simulationAuctionBid,
				RequestID:      bidRequestID,
			})
			return raceOutcome{operation: "bid", err: err}
		},
		func() raceOutcome {
			_, err := service.BuyNow(auction.BuyNowInput{
				AuctionID:     auctionID,
				BuyerPlayerID: buyerID,
				RequestID:     buyNowRequestID,
			})
			return raceOutcome{operation: "buy_now", err: err}
		},
	)
}

func runTwoOperationRace(first func() raceOutcome, second func() raceOutcome) []raceOutcome {
	var wg sync.WaitGroup
	start := make(chan struct{})
	outcomes := make(chan raceOutcome, 2)
	for _, operation := range []func() raceOutcome{first, second} {
		wg.Add(1)
		go func(operation func() raceOutcome) {
			defer wg.Done()
			<-start
			outcomes <- operation()
		}(operation)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	results := make([]raceOutcome, 0, 2)
	for outcome := range outcomes {
		results = append(results, outcome)
	}
	return results
}

func simulationMarketItemDefinition() (economy.ItemDefinition, error) {
	source, err := catalog.NewVersionedDefinitionFromStrings("market_raw_ore", "v1")
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack, err := foundation.NewQuantity(1000000)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		"market_raw_ore",
		"Market Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagMarketTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
}

func seedSimulationItems(
	inventory *economy.InventoryService,
	playerID foundation.PlayerID,
	itemDefinition economy.ItemDefinition,
	quantity int64,
	location economy.ItemLocation,
	reference string,
) error {
	referenceKey, err := foundation.LootPickupIdempotencyKey(reference)
	if err != nil {
		return err
	}
	_, err = inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: itemDefinition,
		Quantity:       quantity,
		Location:       location,
		Reason:         simulationSeedReason,
		ReferenceKey:   referenceKey,
	})
	return err
}

func seedSimulationCredits(wallet *economy.WalletService, playerID foundation.PlayerID, amount int64, reference string) error {
	referenceKey, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(reference))
	if err != nil {
		return err
	}
	_, err = wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       simulationSeedReason,
		ReferenceKey: referenceKey,
	})
	return err
}

func simulationAuctionPayload() (auction.LotPayload, error) {
	source, err := catalog.NewAuctionLotSource("simulation_auction_unlock", "v1")
	if err != nil {
		return auction.LotPayload{}, err
	}
	return auction.LotPayload{
		Type:     auction.LotPayloadTypeShipUnlock,
		Source:   source,
		Quantity: 1,
	}, nil
}

func auctionPayloadsEqual(left auction.LotPayload, right auction.LotPayload) bool {
	return left.Type == right.Type &&
		left.Source == right.Source &&
		left.Quantity == right.Quantity &&
		bytes.Equal(left.Metadata, right.Metadata)
}
