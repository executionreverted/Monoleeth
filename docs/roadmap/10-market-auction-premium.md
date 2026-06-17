# Phase 10: Market, Auction, And Premium

## Status

- State: Not started
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

- [ ] Define market listing model.
- [ ] Define listing status state machine.
- [ ] Implement `CreateListing`.
- [ ] Validate quantity and price are positive.
- [ ] Lock item.
- [ ] Validate item trade flags.
- [ ] Validate item is not reserved/equipped/escrowed.
- [ ] Move item to market escrow.
- [ ] Insert listing.
- [ ] Implement `CancelListing`.
- [ ] Return item from escrow to seller.
- [ ] Implement `BuyListing`.
- [ ] Lock listing.
- [ ] Validate active status.
- [ ] Validate requested quantity.
- [ ] Calculate total server-side.
- [ ] Calculate market fee server-side.
- [ ] Debit buyer wallet.
- [ ] Credit seller wallet minus fee.
- [ ] Credit system sink fee.
- [ ] Move escrow item to buyer.
- [ ] Mark listing sold or reduce quantity.
- [ ] Implement listing expiration.
- [ ] Implement stale listing marker for coordinate scrolls.

## TODO: Auction

- [ ] Define auction lot model.
- [ ] Define lot payload types.
- [ ] Implement lot creation from server catalog.
- [ ] Implement `PlaceBid`.
- [ ] Lock auction.
- [ ] Validate active and not ended.
- [ ] Validate bid amount.
- [ ] Debit bidder.
- [ ] Refund previous bidder in same transaction.
- [ ] Update current bid.
- [ ] Implement `BuyNow`.
- [ ] Refund current bidder if any.
- [ ] Grant payload.
- [ ] Close auction.
- [ ] Implement auction close worker or command.
- [ ] Ensure close operation is idempotent.

## TODO: Premium

- [ ] Define entitlement model.
- [ ] Define provider reference uniqueness.
- [ ] Implement entitlement create.
- [ ] Implement entitlement claim.
- [ ] Grant premium currency pack.
- [ ] Grant loadout slot skeleton.
- [ ] Grant weekly X Core purchase right.
- [ ] Enforce one X Core per player per period.
- [ ] Enforce weekly world stock cannot go negative.
- [ ] Prevent free-earned premium from being listed or traded.
- [ ] Add suspicious trade log event.

## Tests

- [ ] Create listing moves item to escrow.
- [ ] Cancel listing returns item.
- [ ] Listed item cannot be equipped.
- [ ] Reserved item cannot be listed.
- [ ] Buy listing transfers item and currency exactly once.
- [ ] Concurrent buy only one succeeds.
- [ ] Seller cancel racing buyer cannot duplicate item.
- [ ] Partial buy leaves correct escrow quantity.
- [ ] Free premium cannot be listed.
- [ ] Auction bid debits bidder.
- [ ] Auction bid refunds previous bidder.
- [ ] Buy-now closes auction.
- [ ] Bid racing buy-now cannot both win.
- [ ] Auction close grants payload once.
- [ ] Weekly stock concurrent purchase cannot go negative.
- [ ] Entitlement webhook replay is idempotent.
- [ ] Weekly X Core limit enforced.
- [ ] Planet claimed marks listed intel stale.

## Abuse And Safety Checks

- [ ] Market duplication blocked by escrow location.
- [ ] Auction refund duplication blocked by transaction and ledger reference.
- [ ] Premium webhook replay blocked by provider reference uniqueness.
- [ ] Free premium laundering blocked by bucket split.
- [ ] Price manipulation produces suspicious transaction logs.
- [ ] Chargeback/fraud lock hook exists for future provider integration.

## Done Criteria

- [ ] Players can list and buy eligible items safely.
- [ ] Auctions can bid, refund, buy now, and close safely.
- [ ] Premium buckets are enforced.
- [ ] Weekly X Core purchase limit works.
- [ ] Intel listing stale hook exists.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, begin by running buy/cancel race and auction bid/buy-now race tests. This phase is mostly about preventing duplicated value.
