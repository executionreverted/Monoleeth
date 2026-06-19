# Shop Catalog Categories And Product Detail Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Shop becomes a game economy surface with categories, product/listing
lists, selected product detail, quantity controls, buy/list/sell/auction/premium
actions, and server-calculated totals.

**Architecture:** Market, auction, premium, wallet, inventory, escrow, fees, and
totals are server-owned. Client may filter and select locally but cannot invent
prices, totals, stock, or ownership.

**Tech Stack:** Go market/auction/premium handlers, TypeScript HUD/reducer,
CSS catalog layout, browser smoke.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/ui-implementation/08-market-auction-premium-economy.md
output/mockups/final-mockup.png
```

## Current Behavior

- `economyPanel` shows one market listing row, one own listing/sell row, one
  auction row, and one premium row.
- There are no categories, no product grid/list, no selected product detail,
  no quantity controls beyond hard-coded quantity 1 for buys.
- Existing commands include `market.search`, `market.create_listing`,
  `market.buy`, `market.cancel`, `auction.search`, `auction.bid`,
  `auction.buy_now`, `auction.claim_grant`, `premium.entitlements`,
  `premium.claim`, and `premium.purchase_weekly_xcore`.

## Target UX

- Shop window has:
  - category rail: Market, Sell, Auction, Premium, maybe Materials/Modules if
    backed by current data
  - product/listing grid or list
  - selected product detail pane
  - quantity stepper/controls where supported
  - wallet summary
  - server-calculated price/escrow/fee labels
  - buy/list/cancel/bid/buy-now/claim actions
- Rows look like game shop entries, not debug log rows.
- No unsupported category displays fake products.

## Implementation Tasks

1. Create shop view model in client.
   - Derive categories from actual server snapshots:
     - active market listings
     - owned listings
     - sellable inventory rows
     - auction lots
     - premium entitlements/stock
   - Preserve selected category/item locally.

2. Build catalog/detail UI.
   - Category rail or tabs.
   - List/grid of items.
   - Detail pane with name, type, rarity, quantity, price, stock, expiration,
     owner state, and allowed action.
   - Quantity controls for market buy and listing create.

3. Preserve server authority.
   - Never compute final totals as truth.
   - Client can display "estimated" using server-provided estimate fields.
   - Server recalculates all totals on mutation.
   - Disable actions when wallet/snapshot clearly cannot satisfy them, but
     still rely on server validation.

4. Consider search/filter.
   - `market.search(itemID)` can be used for item filter.
   - Avoid spamming; debounce and respect rate limit posture.

5. Tests.
   - Browser smoke for:
     - category switch
     - select listing
     - buy if affordable
     - list a sellable item if available
     - cancel own listing
     - auction bid/buy-now/premium claim locked/enabled states
   - Client tests for derived categories and selected detail.

## Files Likely Touched

```text
client/src/ui/hud.ts
client/src/styles.css
client/src/state/types.ts
client/src/state/reducer.ts
client/src/protocol/commands.ts
client/tests/browser-smoke.mjs
internal/game/server/economy_handlers.go
internal/game/server/server_test.go
```

## Acceptance Checklist

- [x] Shop has categories/tabs.
- [x] Shop has product/listing list or grid.
- [x] Selecting a product/listing updates detail pane.
- [x] Quantity controls exist where supported.
- [x] Buy/List/Cancel/Bid/BuyNow/Claim actions map to real server contracts.
- [x] Server-calculated totals/escrow/fees remain authoritative.
- [x] No fake products, fake stock, fake category counts, or fake balances.
- [x] Browser smoke covers category/detail/action behavior.

## Implementation Notes

- `client/src/ui/hud.ts` now derives Shop categories from authenticated server
  snapshots: market listings, sellable inventory/owned listings, auction lots,
  premium stock/entitlements, and auction grants.
- The Shop window renders category rail, selected list row, detail pane,
  quantity controls for market buy/listing create, and real mutation controls
  for buy, list, cancel, bid, buy now, premium claim, and weekly purchase.
- Market subtotal text is labeled as an estimate while mutation handlers still
  rely on server recalculation, wallet debit, escrow, and ledger results.
- `client/tests/browser-smoke.mjs` now drives the real Shop window through
  Market, Sell, Auction, and Premium categories, then performs the existing
  server-backed economy mutation flow. Screenshots live at
  `output/screenshots/ui-patch-3/shop-{viewport}.png`.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/market ./internal/game/auction ./internal/game/premium ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
