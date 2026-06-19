# Phase 08: Market, Auction, Premium, And Economy UI

## Status

- State: Planned
- Owner: Player economy UI
- Depends on: Phase 06, Phase 07
- Unlocks: trade, premium, and economy visibility

## Goal

Expose fixed-price market listings, auction lots, premium entitlements, wallet
balances, escrow-backed item movement, and economy dashboards through real
server-backed UI.

## Source Specs

Read before implementation:
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/09-market-auction-premium.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `internal/game/market`
- `internal/game/auction`
- `internal/game/premium`
- `internal/game/economy`
- `internal/game/observability`

## Server Features To Expose

- wallet snapshot
- market search/list query
- create listing
- buy listing
- cancel listing
- auction lot list/detail
- create bid
- buy now
- auction close/claim grant snapshot
- premium entitlement list
- premium claim
- weekly X Core purchase
- economy flow summaries/admin dashboard where admin-only

## Commands And Queries

```text
wallet.snapshot
market.search
market.create_listing
market.buy
market.cancel
auction.search
auction.bid
auction.buy_now
auction.claim_grant
premium.entitlements
premium.claim
premium.purchase_weekly_xcore
admin.economy_dashboard
```

## Price, Quote, And Premium Contracts

Market/auction price previews must use one of two explicit patterns:
- server quote query returns quote id, listing id/version, fees, total, expiry,
  and tamper-proof server-side validation on mutation
- mutation ignores client totals and recalculates listing price, fees, wallet
  balance, and escrow state under lock

If quote ids are used, tests must cover stale quote, listing version mismatch,
tampered quote/listing ids, and expired quote. If no quote ids are used, UI must
label previews as pending estimates and accept the server response as truth.

`premium.purchase_weekly_xcore` must enforce:
- weekly per-player purchase limit
- global or configured stock depletion under lock
- wallet or premium bucket debit through ledger
- idempotency for duplicate purchase requests
- safe `premium.stock_consumed` or equivalent event/snapshot after commit

## Events

```text
wallet.snapshot
inventory.snapshot
market.listing_created
market.listing_updated
market.sale_completed
market.listing_cancelled
auction.lot_updated
auction.bid_placed
auction.closed
premium.entitlement_created
premium.entitlement_claimed
economy.flow_updated
premium.stock_consumed
```

## UI Surfaces

Mockup areas covered:
- left navigation: Shop
- credits topbar
- inventory item sell actions
- market panel
- auction panel
- premium panel
- admin/economy dashboards in later/admin mode

## TODO

- [ ] Add authenticated market query and command handlers.
- [ ] Add escrow-backed listing create/buy/cancel UI paths.
- [ ] Add auction search/bid/buy-now handlers.
- [ ] Add auction grant claim/display handlers.
- [ ] Add premium entitlement query/claim handlers.
- [ ] Add weekly X Core purchase handler with per-player limit, stock lock,
      wallet/premium ledger debit, and idempotency.
- [ ] Add wallet snapshot updates after every economy mutation.
- [ ] Add inventory snapshot updates after escrow/item mutations.
- [ ] Add market/auction/premium client panels.
- [ ] Add price/fee preview that is explicitly server quoted or pending.
- [ ] Add admin-only economy dashboard query if admin role exists.
- [ ] Add safe empty/loading/error states.

## Abuse And Safety Checklist

- [ ] Client cannot author price totals as truth.
- [ ] Quantity, unit price, bid amount, currency id, and multiplication totals
      are positive, bounded, and overflow-safe.
- [ ] Client cannot list unowned/untradeable/escrowed items.
- [ ] Market buy/cancel race cannot duplicate items or credits.
- [ ] Auction bid/buy-now race cannot duplicate grants.
- [ ] Premium webhook/provider event replay is idempotent.
- [ ] Premium paid-only policy is enforced server-side.
- [ ] Weekly X Core limit and stock depletion are enforced under concurrency.
- [ ] Admin economy dashboards require admin session.

## Tests

- [ ] Market search respects query/rate posture.
- [ ] Create listing moves item to escrow once.
- [ ] Buy listing transfers item/credits once.
- [ ] Cancel listing returns escrow once.
- [ ] Auction bid refunds/replaces safely.
- [ ] Auction buy-now closes once.
- [ ] Premium claim is idempotent.
- [ ] Weekly X Core purchase enforces limit, stock, ledger debit, and duplicate
      request safety.
- [ ] Stale/tampered price quote or listing version is rejected, or mutation
      recalculates totals server-side.
- [ ] Negative, zero, overflow, or excessive quantity/price/bid inputs reject
      before mutation.
- [ ] Browser market buy updates wallet and inventory from server snapshots.
- [ ] Browser cannot submit forged totals.
- [ ] Admin economy dashboard rejects non-admin.

## Done Criteria

- Player can trade through real market/auction UI.
- Wallet and inventory panels reconcile from server after trades.
- Premium claims are visible and idempotent.
- No client-side economy truth is trusted.
- Tests and browser smoke pass.
