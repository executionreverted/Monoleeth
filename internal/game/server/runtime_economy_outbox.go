package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/realtime"
)

const (
	runtimeEconomyOutboxLeaseOwner   = "runtime-economy-outbox"
	runtimeEconomyOutboxDrainLimit   = 50
	runtimeEconomyOutboxLeaseTimeout = 30 * time.Second
	runtimeEconomyOutboxRetryDelay   = time.Second
)

// Economy outbox event types mirrored from the owning services. The payload
// structs stay unexported there, so the publisher decodes the player-facing
// fields it needs from the committed row payload.
const (
	economyOutboxEventMarketBuy      = "market.buy_completed"
	economyOutboxEventMarketCancel   = "market.listing_cancelled"
	economyOutboxEventAuctionBid     = "auction.bid_placed"
	economyOutboxEventAuctionBuyNow  = "auction.buy_now"
	economyOutboxEventPremiumCreated = "premium.entitlement_created"
	economyOutboxEventPremiumClaimed = "premium.entitlement_claimed"
)

var errUnknownEconomyOutboxEvent = errors.New("unknown economy outbox event")

// drainEconomyOutboxToRealtime drains committed economy outbox rows (market,
// auction, premium, loot XP) through the shared economy replay worker. Market,
// auction, and premium rows project a client-safe wallet+inventory snapshot
// refresh to the affected player sessions; loot XP rows replay the idempotent XP
// grant. The worker marks each row published after a successful projection, so
// a missed synchronous broadcast is redelivered exactly once and never
// double-applied, because the value mutation already committed inside the
// originating transaction and snapshot projection does not re-mutate state.
func (runtime *Runtime) drainEconomyOutboxToRealtime(now time.Time) (economy.OutboxReplayResult, error) {
	if runtime == nil || runtime.economyOutbox == nil {
		return economy.OutboxReplayResult{}, nil
	}
	if now.IsZero() {
		return economy.OutboxReplayResult{}, fmt.Errorf("now: %w", economy.ErrInvalidOutboxRow)
	}
	worker := economy.OutboxReplayWorker{
		Store:         runtime.economyOutbox,
		Publisher:     runtime.publishEconomyOutboxRow,
		LeaseOwner:    runtimeEconomyOutboxLeaseOwner,
		BatchSize:     runtimeEconomyOutboxDrainLimit,
		LeaseDuration: runtimeEconomyOutboxLeaseTimeout,
		RetryDelay:    runtimeEconomyOutboxRetryDelay,
		Now:           func() time.Time { return now },
	}
	return worker.RunOnce(context.Background())
}

func (runtime *Runtime) publishEconomyOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch row.EventType {
	case loot.EventLootXPReconciliationRequested:
		return runtime.replayLootXPOutboxRow(ctx, row)
	case economyOutboxEventMarketBuy,
		economyOutboxEventMarketCancel,
		economyOutboxEventAuctionBid,
		economyOutboxEventAuctionBuyNow,
		economyOutboxEventPremiumCreated,
		economyOutboxEventPremiumClaimed:
		return runtime.projectEconomySnapshotOutboxRow(row)
	default:
		return fmt.Errorf("event %q: %w", row.EventType, errUnknownEconomyOutboxEvent)
	}
}

func (runtime *Runtime) replayLootXPOutboxRow(ctx context.Context, row economy.OutboxRow) error {
	if runtime.Progression == nil {
		return nil
	}
	publisher, err := loot.NewLootXPOutboxPublisher(runtime.Progression)
	if err != nil {
		return err
	}
	return publisher(ctx, row)
}

func (runtime *Runtime) projectEconomySnapshotOutboxRow(row economy.OutboxRow) error {
	playerIDs, err := economyOutboxAffectedPlayers(row)
	if err != nil {
		return err
	}
	if len(playerIDs) == 0 {
		return nil
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for _, playerID := range playerIDs {
		if playerID.IsZero() {
			continue
		}
		runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventWalletSnapshot, runtime.walletSnapshotLocked(playerID))
		runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(playerID))
	}
	return nil
}

// economyOutboxAffectedPlayers decodes the affected player ids encoded in a
// committed economy outbox row payload. It only reads the player-facing fields
// needed for snapshot fanout; it never trusts client-supplied identity.
func economyOutboxAffectedPlayers(row economy.OutboxRow) ([]foundation.PlayerID, error) {
	switch row.EventType {
	case economyOutboxEventMarketBuy:
		var payload struct {
			BuyerPlayerID  foundation.PlayerID `json:"buyer_player_id"`
			SellerPlayerID foundation.PlayerID `json:"seller_player_id"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("market buy payload: %w", err)
		}
		return []foundation.PlayerID{payload.BuyerPlayerID, payload.SellerPlayerID}, nil
	case economyOutboxEventMarketCancel:
		var payload struct {
			SellerPlayerID foundation.PlayerID `json:"seller_player_id"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("market cancel payload: %w", err)
		}
		return []foundation.PlayerID{payload.SellerPlayerID}, nil
	case economyOutboxEventAuctionBid:
		var payload struct {
			BidderPlayerID  foundation.PlayerID `json:"bidder_player_id"`
			CurrentBidderID foundation.PlayerID `json:"current_bidder_id"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("auction bid payload: %w", err)
		}
		return []foundation.PlayerID{payload.BidderPlayerID, payload.CurrentBidderID}, nil
	case economyOutboxEventAuctionBuyNow:
		var payload struct {
			BuyerPlayerID   foundation.PlayerID `json:"buyer_player_id"`
			WinningPlayerID foundation.PlayerID `json:"winning_player_id"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("auction buy-now payload: %w", err)
		}
		return []foundation.PlayerID{payload.BuyerPlayerID, payload.WinningPlayerID}, nil
	case economyOutboxEventPremiumCreated:
		var payload struct {
			PlayerID foundation.PlayerID `json:"player_id"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("premium created payload: %w", err)
		}
		return []foundation.PlayerID{payload.PlayerID}, nil
	case economyOutboxEventPremiumClaimed:
		var payload struct {
			Entitlement struct {
				PlayerID foundation.PlayerID `json:"player_id"`
			} `json:"entitlement"`
		}
		if err := json.Unmarshal(row.PayloadJSON, &payload); err != nil {
			return nil, fmt.Errorf("premium claimed payload: %w", err)
		}
		return []foundation.PlayerID{payload.Entitlement.PlayerID}, nil
	default:
		return nil, fmt.Errorf("event %q: %w", row.EventType, errUnknownEconomyOutboxEvent)
	}
}
