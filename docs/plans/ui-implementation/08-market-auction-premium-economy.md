# Phase 08: Market, Auction, Premium, And Economy UI

## Status

- State: Completed
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

- [x] Add authenticated market query and command handlers.
- [x] Add escrow-backed listing create/buy/cancel UI paths.
- [x] Add auction search/bid/buy-now handlers.
- [x] Add auction grant claim/display handlers.
- [x] Add premium entitlement query/claim handlers.
- [x] Add weekly X Core purchase handler with per-player limit, stock lock,
      wallet/premium ledger debit, and idempotency.
- [x] Add wallet snapshot updates after every economy mutation.
- [x] Add inventory snapshot updates after escrow/item mutations.
- [x] Add market/auction/premium client panels.
- [x] Add price/fee preview that is explicitly server quoted or pending.
- [x] Add admin-only economy dashboard query if admin role exists.
- [x] Add safe empty/loading/error states.

## UI Patch 3 Notes

- The Shop surface now uses a real category/list/detail layout instead of a
  single mixed row stack. Categories are derived from server snapshots for
  Market, Sell, Auction, and Premium.
- Quantity controls are local UI intent only; market/listing mutations still
  send quantity and rely on server recalculation, escrow movement, wallet
  ledger writes, and authoritative response snapshots.
- Browser smoke opens the real Shop window, switches categories, selects detail
  rows, and exercises buy/list/cancel/bid/premium actions through the same
  server contracts.

## Abuse And Safety Checklist

- [x] Client cannot author price totals as truth.
- [x] Quantity, unit price, bid amount, currency id, and multiplication totals
      are positive, bounded, and overflow-safe.
- [x] Client cannot list unowned/untradeable/escrowed items.
- [x] Market buy/cancel race cannot duplicate items or credits.
- [x] Auction bid/buy-now race cannot duplicate grants.
- [x] Premium webhook/provider event replay is idempotent.
- [x] Premium paid-only policy is enforced server-side.
- [x] Weekly X Core limit and stock depletion are enforced under concurrency.
- [x] Admin economy dashboards require admin session.

## Tests

- [x] Market search respects query/rate posture.
- [x] Create listing moves item to escrow once.
- [x] Buy listing transfers item/credits once.
- [x] Cancel listing returns escrow once.
- [x] Auction bid refunds/replaces safely.
- [x] Auction buy-now closes once.
- [x] Premium claim is idempotent.
- [x] Weekly X Core purchase enforces limit, stock, ledger debit, and duplicate
      request safety.
- [x] Stale/tampered price quote or listing version is rejected, or mutation
      recalculates totals server-side.
- [x] Negative, zero, overflow, or excessive quantity/price/bid inputs reject
      before mutation.
- [x] Browser market buy updates wallet and inventory from server snapshots.
- [x] Browser cannot submit forged totals.
- [x] Admin economy dashboard rejects non-admin.

## Done Criteria

- Player can trade through real market/auction UI.
- Wallet and inventory panels reconcile from server after trades.
- Premium claims are visible and idempotent.
- No client-side economy truth is trusted.
- Tests and browser smoke pass.
