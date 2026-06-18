package simulations_test

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/observability/simulations"
)

func TestMarketBuyCancelRaceSimulationConservesItems(t *testing.T) {
	summary, err := simulations.RunMarketBuyCancelRaceSimulation(simulations.MarketBuyCancelRaceConfig{
		Races:     20,
		StartTime: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunMarketBuyCancelRaceSimulation() error = %v", err)
	}

	if summary.Races != 20 {
		t.Fatalf("Races = %d, want 20", summary.Races)
	}
	if summary.BuySuccesses+summary.CancelSuccesses != 20 {
		t.Fatalf("buy+cancel successes = %d+%d, want 20", summary.BuySuccesses, summary.CancelSuccesses)
	}
	if summary.ListingNotActiveFailures != 20 {
		t.Fatalf("ListingNotActiveFailures = %d, want one loser per race", summary.ListingNotActiveFailures)
	}
	if summary.SellerSourceQuantity+summary.BuyerQuantity != 20 {
		t.Fatalf("seller+buyer quantity = %d+%d, want 20", summary.SellerSourceQuantity, summary.BuyerQuantity)
	}
	if summary.EscrowQuantity != 0 {
		t.Fatalf("EscrowQuantity = %d, want 0", summary.EscrowQuantity)
	}
	wantBuyerBalance := int64(20*1_000 - summary.BuySuccesses*100)
	if summary.BuyerBalance != wantBuyerBalance {
		t.Fatalf("BuyerBalance = %d, want %d", summary.BuyerBalance, wantBuyerBalance)
	}
	wantSellerBalance := int64(summary.BuySuccesses * 95)
	if summary.SellerBalance != wantSellerBalance {
		t.Fatalf("SellerBalance = %d, want %d", summary.SellerBalance, wantSellerBalance)
	}
	wantFeeBalance := int64(summary.BuySuccesses * 5)
	if summary.SystemFeeBalance != wantFeeBalance {
		t.Fatalf("SystemFeeBalance = %d, want %d", summary.SystemFeeBalance, wantFeeBalance)
	}
}

func TestAuctionBidBuyNowRaceSimulationClosesOnce(t *testing.T) {
	summary, err := simulations.RunAuctionBidBuyNowRaceSimulation(simulations.AuctionBidBuyNowRaceConfig{
		Races:     20,
		StartTime: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunAuctionBidBuyNowRaceSimulation() error = %v", err)
	}

	if summary.Races != 20 {
		t.Fatalf("Races = %d, want 20", summary.Races)
	}
	if summary.BuyNowSuccesses != 20 {
		t.Fatalf("BuyNowSuccesses = %d, want 20", summary.BuyNowSuccesses)
	}
	if summary.BidSuccesses+summary.LotNotActiveFailures != 20 {
		t.Fatalf("bid successes + lot inactive failures = %d+%d, want 20", summary.BidSuccesses, summary.LotNotActiveFailures)
	}
	if summary.ClosedLots != 20 || summary.BuyerGrants != 20 {
		t.Fatalf("closed lots / grants = %d / %d, want 20 / 20", summary.ClosedLots, summary.BuyerGrants)
	}
	if summary.BidderBalance != 20*500 {
		t.Fatalf("BidderBalance = %d, want refunded/unchanged %d", summary.BidderBalance, 20*500)
	}
	if summary.BuyerBalance != 20*1_000-20*300 {
		t.Fatalf("BuyerBalance = %d, want %d", summary.BuyerBalance, 20*1_000-20*300)
	}
}

func TestMarketAuctionRaceSimulationsRejectInvalidConfig(t *testing.T) {
	_, err := simulations.RunMarketBuyCancelRaceSimulation(simulations.MarketBuyCancelRaceConfig{Races: -1})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunMarketBuyCancelRaceSimulation() error = %v, want ErrInvalidSimulationConfig", err)
	}

	_, err = simulations.RunAuctionBidBuyNowRaceSimulation(simulations.AuctionBidBuyNowRaceConfig{Races: -1})
	if !errors.Is(err, simulations.ErrInvalidSimulationConfig) {
		t.Fatalf("RunAuctionBidBuyNowRaceSimulation() error = %v, want ErrInvalidSimulationConfig", err)
	}
}
