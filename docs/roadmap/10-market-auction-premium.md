# Phase 10: Market, Auction, And Premium

## Status

- State: MVP complete - fixed-price market, auction, premium entitlement/
  weekly-stock, policy, fraud-review, and stale-listing domain hooks implemented
  2026-06-18; durable adapters, wallet-currency listing runtime wiring, and the
  concrete planet-claim-to-market listing adapter are tracked as follow-ups
- Owner: Player economy and monetization safety
- Depends on: Phase 02, Phase 03, Phase 06, Phase 08
- Unlocks: player trading, controlled rare supply, premium convenience

## Goal

Build fixed-price market listings, escrow-backed item sales, basic system auctions, premium wallet bucket rules, weekly stock limits, and idempotent entitlement claiming.

## Why This Comes Late

Market and premium systems amplify every earlier exploit. They must wait until inventory, ledger, item locations, trade flags, idempotency, and server-side ownership checks are solid.

## Source Specs

Read before implementation:

- `docs/plans/modules/09-market-auction-premium.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/13-intel-coordinate-trading.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `MarketService`
- `AuctionService`
- `PremiumEntitlementService`

Does not own:

- wallet primitive
- item primitive
- ship unlock internals
- craft recipe
- fraud provider integration details

## MVP Scope

Market:

- fixed-price sell listings
- no buy orders
- market escrow
- partial buy if stackable
- sale fee
- listing expiration
- stale intel listing hook

Auction:

- server-generated lots
- bid
- refund previous bidder
- buy-now
- close and grant payload

Premium:

- `premium_paid`
- `premium_earned`
- `premium_market_acquired`
- weekly X Core purchase limit
- entitlement state machine
- webhook replay protection skeleton

## TODO: Market

- [x] Define market listing model.
- [x] Define listing status state machine.
- [x] Implement `CreateListing`.
- [x] Validate quantity and price are positive.
- [x] Lock item.
- [x] Validate item trade flags.
- [x] Validate item is not reserved/equipped/escrowed.
- [x] Move item to market escrow.
- [x] Insert listing.
- [x] Implement `CancelListing`.
- [x] Return item from escrow to seller.
- [x] Implement `BuyListing`.
- [x] Lock listing.
- [x] Validate active status.
- [x] Validate requested quantity.
- [x] Calculate total server-side.
- [x] Calculate market fee server-side.
- [x] Debit buyer wallet.
- [x] Credit seller wallet minus fee.
- [x] Credit system sink fee.
- [x] Move escrow item to buyer.
- [x] Mark listing sold or reduce quantity.
- [x] Implement listing expiration.
- [x] Implement stale listing marker for coordinate scrolls.

Progress note, 2026-06-18: `BuyListing` rejects expired active listings without
mutating escrow or buyer inventory. `ExpireListing` now marks expired active or
stale listings and returns remaining escrow idempotently with a distinct
`market_expire:<listing_id>` ledger reference. `MarkListingStale` marks active
intel/coordinate listings stale so they cannot be bought while seller cancel or
expiration can still release escrow.

## TODO: Auction

- [x] Define auction lot model.
- [x] Define lot payload types.
- [x] Implement lot creation from server catalog.
- [x] Implement `PlaceBid`.
- [x] Lock auction.
- [x] Validate active and not ended.
- [x] Validate bid amount.
- [x] Debit bidder.
- [x] Refund previous bidder in same transaction.
- [x] Update current bid.
- [x] Implement `BuyNow`.
- [x] Refund current bidder if any.
- [x] Grant payload.
- [x] Close auction.
- [x] Implement auction close worker or command.
- [x] Ensure close operation is idempotent.

Progress note, 2026-06-18: `AuctionService` is an in-memory MVP with
server-created catalog-backed lots, mutex-serialized bids/buy-now/close, domain
idempotency keys for bid/refund/buy-now/close ledger refs, and skeleton payload
grants. Durable DB transactions and grant adapters remain later hardening work.

## TODO: Premium

- [x] Define entitlement model.
- [x] Define provider reference uniqueness.
- [x] Implement entitlement create.
- [x] Implement entitlement claim.
- [x] Grant premium currency pack.
- [x] Grant loadout slot skeleton.
- [x] Grant weekly X Core purchase right.
- [x] Enforce one X Core per player per period.
- [x] Enforce weekly world stock cannot go negative.
- [x] Prevent free-earned premium from being listed or traded.
- [x] Add suspicious trade log event.

Progress note, 2026-06-18: `PremiumEntitlementService` enforces paid-only
premium use through `ValidatePaidPremiumUse` and future wallet-currency listing
flows can call `ValidatePremiumCurrencyListing`; actual wallet-currency market
listing runtime wiring remains a follow-up because the fixed-price market MVP
lists items only.

## Tests

- [x] Create listing moves item to escrow.
- [x] Cancel listing returns item.
- [x] Listed item cannot be equipped.
- [x] Reserved item cannot be listed.
- [x] Buy listing transfers item and currency exactly once.
- [x] Concurrent buy only one succeeds.
- [x] Seller cancel racing buyer cannot duplicate item.
- [x] Partial buy leaves correct escrow quantity.
- [x] Free premium cannot be listed.
- [x] Auction bid debits bidder.
- [x] Auction bid refunds previous bidder.
- [x] Buy-now closes auction.
- [x] Bid racing buy-now cannot both win.
- [x] Auction close grants payload once.
- [x] Weekly stock concurrent purchase cannot go negative.
- [x] Entitlement webhook replay is idempotent.
- [x] Weekly X Core limit enforced.
- [x] Planet claim invokes configured listed-intel stale marker.

## Abuse And Safety Checks

- [x] Market duplication blocked by escrow location.
- [x] Auction refund duplication blocked by transaction and ledger reference.
- [x] Premium webhook replay blocked by provider reference uniqueness.
- [x] Free premium laundering blocked by bucket split.
- [x] Price manipulation produces suspicious transaction logs.
- [x] Chargeback/fraud lock hook exists for future provider integration.

## Done Criteria

- [x] Players can list and buy eligible items safely.
- [x] Auctions can bid, refund, buy now, and close safely.
- [x] Premium buckets are enforced.
- [x] Weekly X Core purchase limit works.
- [x] Intel listing stale hook exists.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Resume Notes

If resuming here, begin by running buy/cancel race and auction bid/buy-now race tests. This phase is mostly about preventing duplicated value.
