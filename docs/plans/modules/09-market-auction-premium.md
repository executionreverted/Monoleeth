# Market, Auction, And Premium Entitlements

Date: 2026-06-17

## Purpose

Bu modül player economy, system economy ve premium convenience dengesini yönetir.

Kapsam:

- Fixed-price market listings
- Market escrow
- Buy/sell flow
- Tradeable item/currency checks
- Intergalactic Auction Hall
- Weekly stock
- Premium paid/free split usage
- Premium entitlement grants

## Owns

```text
MarketService
AuctionService
PremiumEntitlementService
```

## Does Not Own

- Wallet primitive
- Item primitive
- Ship unlock internals
- Crafting recipe
- Fraud provider integration details

## Market Philosophy

Neredeyse her şey trade edilebilir:

- raw materials
- processed materials
- modules
- consumables
- coordinate scrolls
- eligible premium currency
- X Core fragments, maybe X Core controlled

Trade edilemeyen:

- ships
- quest soulbound items
- feature unlock soulbound items
- free-earned premium currency
- locked/fraud/pending items

## Market Listing Data

```text
listing_id
seller_player_id
item_instance_id
item_id
quantity
unit_price
currency_type
status
expires_at
metadata_json
created_at
updated_at
```

Status:

```text
active
sold
cancelled
expired
stale
locked
```

## Create Listing Flow

```go
func CreateListing(ctx context.Context, seller PlayerID, itemRef ItemRef, qty int64, price Money) error {
	return db.Tx(ctx, func(tx Tx) error {
		if qty <= 0 || price.Amount <= 0 {
			return ErrInvalidListing
		}
		item := inventory.LockItem(tx, seller, itemRef)
		if !market.CanTrade(item) {
			return ErrItemNotTradeable
		}
		if item.Quantity < qty {
			return ErrNotEnoughQuantity
		}
		if err := inventory.MoveToEscrow(tx, seller, itemRef, qty, "market_listing"); err != nil {
			return err
		}
		listing := Listing{Seller: seller, ItemID: item.ItemID, Quantity: qty, UnitPrice: price.Amount, Currency: price.Currency}
		tx.Market().Insert(listing)
		return nil
	})
}
```

## Buy Listing Flow

```go
func BuyListing(ctx context.Context, buyer PlayerID, listingID string, qty int64) error {
	return db.Tx(ctx, func(tx Tx) error {
		listing := tx.Market().Lock(listingID)
		if listing.Status != "active" {
			return ErrListingNotActive
		}
		if qty <= 0 || qty > listing.Quantity {
			return ErrInvalidQuantity
		}
		total := listing.UnitPrice * qty
		fee := market.Fee(total)

		wallet.Debit(tx, buyer, listing.Currency, total, "market_buy", listingID)
		wallet.Credit(tx, listing.Seller, listing.Currency, total-fee, "market_sale", listingID)
		wallet.CreditSystem(tx, listing.Currency, fee, "market_fee", listingID)

		inventory.MoveFromEscrowToPlayer(tx, listing.EscrowRef, buyer, qty, "market_buy", listingID)
		listing.Quantity -= qty
		if listing.Quantity == 0 {
			listing.Status = "sold"
		}
		tx.Market().Save(listing)
		return nil
	})
}
```

## Market Fees

Fee is economy sink.

MVP:

```text
sale_fee = 5%
listing_fee = optional small credits
```

Trade role can reduce fee later.

## Premium Currency Trading

Wallet buckets:

```text
premium_paid
premium_earned
premium_market_acquired
```

Trade rule suggestion:

```text
premium_paid can be listed/sold
premium_earned cannot be listed
premium_market_acquired cannot be relisted immediately or cannot be relisted at all
```

This prevents:

- free reward inflation
- laundering
- fraud loops

## Auction Hall Philosophy

World-generated system auction:

- controlled rare supply
- credit/premium sink
- weekly excitement
- stock-bypass at higher price

Ships are granted as unlocks, not tradeable item instances.

## Auction Lot Data

```text
auction_id
world_id
lot_type
payload_json
currency_type
start_price
current_bid
current_bidder
buy_now_price
starts_at
ends_at
status
```

Lot types:

```text
ship_unlock
module_blueprint
x_core
x_core_fragment_bundle
rare_material_bundle
cosmetic
intel_cache
building_blueprint
```

## Bid Flow

```go
func PlaceBid(ctx context.Context, bidder PlayerID, auctionID string, amount int64) error {
	return db.Tx(ctx, func(tx Tx) error {
		auction := tx.Auctions().Lock(auctionID)
		if auction.Status != "active" || time.Now().After(auction.EndsAt) {
			return ErrAuctionClosed
		}
		if amount <= auction.CurrentBid {
			return ErrBidTooLow
		}
		wallet.Debit(tx, bidder, auction.Currency, amount, "auction_bid", auctionID)
		if auction.CurrentBidder != nil {
			wallet.Credit(tx, *auction.CurrentBidder, auction.Currency, auction.CurrentBid, "auction_refund", auctionID)
		}
		auction.CurrentBid = amount
		auction.CurrentBidder = &bidder
		tx.Auctions().Save(auction)
		return nil
	})
}
```

## Buy Now Flow

```text
lock auction
validate active
debit buy_now_price
refund current bidder if any
grant payload
mark closed
emit auction.buy_now
```

## Weekly Stock

Premium shop stock example:

```text
Slazar:
credit_price = 500k
premium_price = 300
premium_weekly_world_stock = 100
auction_buy_now_price = 350 premium
```

Stock data:

```text
world_id
item_or_unlock_id
period_key
stock_total
stock_remaining
```

## Premium Entitlements

Premium purchases grant entitlements:

```text
entitlement_id
player_id
type
payload_json
source
provider_reference
state
created_at
claimed_at
```

Entitlement types:

- premium currency pack
- loadout slot
- name change token
- badge/title
- weekly X Core purchase right
- cosmetic skin

Grant flow:

```text
payment provider confirms
create entitlement
claim entitlement transaction
credit wallet/item/unlock
ledger writes source provider ref
```

## Weekly X Core Purchase

Rule:

```text
1 X Core per player per week via premium
```

Data:

```text
player_id
period_key
x_core_premium_purchased_count
```

Validation:

```go
if count >= 1 {
	return ErrWeeklyLimitReached
}
```

## Events Emitted

```text
market.listing_created
market.listing_cancelled
market.sale_completed
market.listing_expired
auction.lot_created
auction.bid_placed
auction.bid_refunded
auction.buy_now
auction.lot_closed
premium.entitlement_created
premium.entitlement_claimed
premium.weekly_stock_consumed
```

## Edge Cases

- Buyer buys listing while seller cancels.
- Listing expires during buy request.
- Partial buy leaves correct escrow quantity.
- Bidder outbid refund fails; transaction should rollback.
- Buy-now while bid request concurrent.
- Premium stock reaches zero with concurrent purchases.
- Entitlement provider sends webhook twice.
- Chargeback after premium spent.

## Abuse Vectors

### Market Duplication

Risk:

- Listed item remains in seller inventory and escrow.

Defense:

- move to escrow in transaction
- item location check
- no use/equip from escrow

### Auction Refund Duplication

Risk:

- Previous bidder refunded twice.

Defense:

- bid state transition in transaction
- ledger reference unique

### Premium Webhook Replay

Risk:

- Same provider event grants currency twice.

Defense:

- provider_reference unique
- entitlement state machine
- idempotent claim

### Price Manipulation / RMT

Risk:

- Player sells worthless item for huge premium/credits.

Defense:

- suspicious transaction logs
- trade velocity limits
- new account restrictions
- manual review tools

### Free Premium Laundering

Risk:

- Quest premium listed as paid premium.

Defense:

- separate wallet buckets
- source preserved in ledger

## Testing Checklist

- Create listing moves item to escrow.
- Cancel listing returns item.
- Buy listing transfers item/currency exactly once.
- Concurrent buy only one succeeds.
- Auction bid refunds previous bidder.
- Buy-now closes auction.
- Weekly stock concurrent purchase cannot go negative.
- Webhook replay idempotent.
- Free premium cannot be listed.

## Implementation Notes

MVP:

- fixed-price sell listings
- no buy orders yet
- simple auction with bid and buy-now
- premium paid/earned split
- weekly X Core purchase limit
- basic suspicious trade logs

