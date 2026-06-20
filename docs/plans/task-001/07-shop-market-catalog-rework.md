# Phase 07 - Shop, Market, Auction, And Catalog Rework

## Goal

Make Shop look and behave like a real game shop. Replace raw temporary market
presentation with a server-owned game catalog, category browsing, selected
product detail, item art/slot treatment, and a purchase/listing panel similar to
the local DarkOrbit shop mockup.

## Problems Covered

- Shop currently exposes `Raw Ore` and internal copy such as `Server:
  recalculates`.
- Categories are too generic and do not match the game equipment model.
- Product detail/purchase rail is weak.
- Everything is fixed size and scrolls poorly.
- Market/auction/premium state should still be server authoritative.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/plans/task-001/05-seeded-game-content-catalog.md
docs/todo.md
output/mockups/darkorbit-magaza-ornek-mockup.png
output/mockups/final-mockup.png
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/09-market-auction-premium.md
docs/2026-06-17-progression-economy-systems-design.md
internal/game/server/economy_seed.go
internal/game/server/combat_loot_catalog.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
client/src/ui/hud.ts
client/src/styles.css
```

## Design Contract

- Layout: category rail, product/listing grid, selected product detail/showcase,
  purchase/listing panel.
- Categories should be game categories: Ships, Weapons, Ammo, Launchers,
  Shield Generators, Speed Generators, Extras/Modules, Scanner/Radar,
  Stealth, Cargo/Utility, Boosters, Resources.
- The client can filter and select, but prices, stock, fees, escrow, wallet
  debit, grants, auction state, and premium entitlements are server-owned.
- Player UI must not mention that the server recalculates totals. It can say
  `Price`, `Fee`, `Total`, `Stock`, `Unavailable`, or `Insufficient credits`.
- Shop needs an explicit server-owned catalog contract. Preferred shape:
  `shop.catalog` or a system-product query that returns category, art key,
  product type, catalog version, availability, locked reason, price/quote
  policy, and backing item/module/ship refs.
- Auction and premium grant surfaces must be hidden/locked unless concrete
  grant adapters exist for their payload types.

## Shop Catalog Protocol Contract

Preferred contract:

- Query/op: `shop.catalog`.
- Optional mutation: `shop.buy_product` for system products.
- Player market mutations remain `market.buy`, `market.create_listing`, and
  `market.cancel`; they do not replace the system product catalog.
- Request payload: catalog version/cursor and optional category/filter only.
  Client never sends authoritative price, fee, stock, grant, category, or
  availability truth.
- Response payload: catalog version, categories, products, availability,
  player-facing locked reason, display metadata, art key, grant target summary,
  price policy, stock policy, owned/eligible state, and safe refresh hints.
- Reducer state: `shop.catalog`, selected category/product, pending product
  intent, and last refresh/error. Selection is UI-only; product truth is
  server-owned.
- Events/refresh: product stock or availability changes either carry safe
  catalog deltas or trigger `shop.catalog` refresh. Private purchase/grant
  data remains owner-only.
- Rate limits: catalog refresh is query-rate-limited; buy product uses command
  rate-limit plus wallet/inventory/ship/ledger validation.
- Hidden fields: no provider refs, hidden grant internals, server cost formulas,
  private stock reservations, escrow refs, or raw ids as display names.

## UI Category And Detail Requirements

The primary shop layout must use game categories, not only
`market | sell | auction | premium` tabs.

Required category rail:

- Ships
- Weapons
- Ammo
- Launchers
- Shield Generators
- Speed Generators
- Extras/Modules
- Scanner/Radar
- Stealth
- Cargo/Utility
- Boosters
- Resources

Selected product detail needs art, display name, description, rarity/tier,
slot/category, stat chips, requirements, availability, owned state, price,
stock, and clear buy/list/bid actions backed by the owning server contract.

## Subagent Review Additions - 2026-06-20

- Add a real `shop.catalog` or equivalent server-owned product query. Touch
  points must include realtime operation constants, handler registration,
  `handleShopCatalog`, safe product payloads, TypeScript operation/command
  builders, reducer state, and browser smoke.
- Split system shop products from player market listings. Decide whether buys
  use `shop.buy_product` or a system-backed `market.buy`; either way category,
  stock, price, availability, display name, art key, and backing refs come from
  server-owned catalog/listings.
- Replace `market | sell | auction | premium` as the primary shop layout with
  game categories: Ships, Weapons, Ammo, Launchers, Shield Generators, Speed
  Generators, Extras/Modules, Scanner/Radar, Stealth, Cargo/Utility, Boosters,
  and Resources.
- Remove visible `server_recalculates`/`Server: recalculates` copy and update
  tests that currently assert it. Internal quote metadata may exist only if it
  never renders in normal player UI.
- Passive market, auction, and premium events must update reducer state or
  trigger explicit refresh queries. Add multi-client tests around buy, bid,
  listing/cancel, premium purchase, and grant/claim paths.
- Auction/premium grant UI must be hidden/locked with a named blocker unless
  concrete grant adapters mutate wallet/inventory/unlocks through real services
  and ledger/reference tests.

## Second Subagent Review Additions - 2026-06-20

- Pin the system shop/player market decision now: use `shop.catalog` and
  `shop.buy_product` for system products, keep `market.buy` for player listings,
  unless implementation deliberately documents a system-listing bridge with the
  same metadata and ledger guarantees.
- Remove old smoke truth that expects `Market`, `Sell`, `Auction`, `Premium` as
  the primary shop categories, `raw_ore` as the shop buy item, or
  `server_recalculates` as a rendered/state expectation.
- Add a passive economy privacy matrix for buyer, seller, previous bidder,
  winner, passive shop viewer, premium entitlement owner, and stock viewer.
  Each row must define event, refresh query, private fields blocked, and
  viewer-relative fields such as auction `leading`.
- Hide or lock auction grants, weekly X Core, loadout/cosmetic/badge grants, or
  premium grants unless the payload type has a concrete adapter into its owning
  service plus ledger/reference tests.

## Third Subagent Review Additions - 2026-06-20

- `shop.catalog` and `shop.buy_product` do not exist yet in the realtime ops,
  server handler map, TypeScript operation constants/builders, reducer state,
  or smoke. Phase 07 cannot pass as a real game shop without this contract or a
  deliberately equivalent system-product bridge.
- System shop products must not be represented as player market listings.
  Remove the real-mode `listing-raw-ore-1` path or move it behind an explicit
  dev/demo fixture.
- Existing shop UI/smoke still proves the old `Market`, `Sell`, `Auction`,
  `Premium` economy tabs and `raw_ore` purchase truth. Replace those as primary
  shop assertions with game categories and selected product detail.
- Passive economy event handling is still not enough for buyer, seller,
  previous bidder, winner, premium owner, stock viewer, and passive shop viewer.
  Implement reducer deltas or explicit refresh-needed state for market,
  auction, premium, and shop catalog.
- Auction/premium grants remain skeleton unless concrete adapters mutate the
  owning wallet/inventory/unlock services with ledger/reference evidence. Hide
  those actions or keep them locked with a named blocker.
- Market settlement still has durable transaction/outbox risk. Either add
  rollback-safe transaction coverage or name that backend blocker before
  expanding shop purchase paths.

## Fourth Subagent Review Additions - 2026-06-20

- Add a sell/listing eligibility contract before rendering a real Sell surface.
  Inventory stack snapshots need `list_eligible` plus player-facing
  `locked_reason`, or the client must query `market.sell_options` from the
  server.
- Make `market.create_listing` retry-safe. Repeating the same domain
  idempotency key should return the existing successful result rather than a
  duplicate-listing error or second escrow mutation.
- Auction controls must use `lot.currency_type` for bid/buy-now wallet checks,
  balance labels, and insufficient-funds copy. Do not assume credits-only lots
  if the payload supports other currencies.
- Replace empty/singleton premium stock purchase intent with product-specific
  identity: `stock_id`, `product_id`, `world_id`, `period_key`, price, and
  currency are server-owned and validated.
- Add duplicate-send pending guards and smoke for `market.buy`,
  `market.create_listing`, `market.cancel`, `auction.buy_now`, `premium.claim`,
  and premium stock purchase, not only `auction.bid`.

## Implementation Notes - 2026-06-21

- Market listing create, buy, and cancel now fan out owner-aware realtime events
  to online seller, buyer, and passive viewer sessions.
- Market owner sessions receive their private listing event plus wallet and/or
  inventory refreshes where relevant; passive sessions receive public listing
  create/update/cancel payloads only.
- Multi-client server coverage now proves seller, buyer, and passive viewer
  market fanout, including event sequence continuity and private payload leak
  checks. Auction and premium passive fanout remain open Phase 07 work.

## Implementation Plan

1. Replace shop layout.
   - Use the local shop mockup for structure.
   - Category rail on the left.
   - Product/listing grid in the main bay.
   - Selected product showcase/detail center/right.
   - Purchase panel with quantity and final server-returned quote/result.
   - Include item-art/tile/showcase/right-rail tasks from the mockup; the old
     four-tab `market | sell | auction | premium` structure is not enough.

2. Normalize catalog data.
   - Introduce a server-owned `shop.catalog`/system-product payload or expand
     market payloads with explicit product metadata.
   - Do not create client-only products in real mode.
   - Rename temporary raw ids into presentable game names where they remain.
   - Keep catalog version and reference integrity tied to Phase 05.

3. Keep market/auction real.
   - Existing market buy/list/cancel remains ledger/escrow-backed.
   - Auction bid/buy-now/grant remains server-owned.
   - Premium weekly stock and grants stay server-owned.
   - Name durable transaction/rollback/outbox risk for any new purchase path
     that expands beyond current in-memory MVP boundaries.
   - Every new purchase/grant path needs ledger/reference assertions.
   - Passive market/auction/premium events must either update reducer state or
     trigger explicit refresh queries, with multi-client tests.
   - Pending-action UI must disable/debounce double-clicks; idempotency only
     protects repeated requests with the same request id.

4. Remove internal copy.
   - Delete `Server: recalculates` from UI.
   - Replace raw ids and snake_case names with display names from server
     definitions.
   - Remove or rename player-visible/client-smoke `server_recalculates` usage.
     Internal protocol/state can keep server calculation metadata only if it is
     not rendered in normal player UI.

5. Add tests.
   - Category selection.
   - Product detail and buy panel.
   - Quantity controls.
   - Server rejection and insufficient funds.
   - One click equals one mutation.
   - Passive event reconciliation or refresh.
   - Negative/double-click cases for `market.buy` and `auction.bid`.
   - Catalog/reference integrity for products/listings.

## Likely Files

```text
internal/game/server/economy_seed.go
internal/game/server/economy_handlers.go
internal/game/server/combat_loot_catalog.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
internal/game/server/server_test.go
client/src/ui/hud.ts
client/src/styles.css
client/src/state/types.ts
client/src/state/reducer.ts
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/tests/browser-smoke.mjs
docs/plans/task-001/07-shop-market-catalog-rework.md
```

## Acceptance Criteria

- [ ] Shop follows category/list/detail/buy-panel layout.
- [x] Server-owned shop catalog/system-product contract exists or the expanded
      market payload explicitly provides equivalent metadata.
- [x] `shop.catalog` query and `shop.buy_product` command exist in server
      realtime ops, handler map, client builders, reducer state, and smoke, or
      an equivalent system-product bridge is documented and tested.
- [x] System shop products are separated from player market listings.
- [x] Real-mode system shop no longer depends on the `listing-raw-ore-1` seed.
- [x] Categories map to real server catalog/listing data.
- [x] No player-facing `server recalculates` copy remains.
- [x] `server_recalculates` is absent from normal player UI and client smoke
      expectations.
- [x] Raw temporary names are removed or replaced by server display names.
- [ ] Quantity and purchase/list/bid actions reconcile with server responses.
- [x] Market totals/fees/escrow are not trusted from the client.
- [ ] Auction and premium event paths reconcile or refresh passive clients.
- [ ] Multi-client economy tests cover buyer/seller/passive viewer, previous
      bidder/winner, and premium owner/stock viewer or record exact backend
      fanout blockers.
- [ ] Auction/premium grants are real for their payload type or hidden/locked
      with a named blocker.
- [ ] Purchase/bid buttons debounce pending actions and smoke asserts one click
      emits exactly one mutation command.
- [x] Sell/listing eligibility comes from server inventory metadata or a
      `market.sell_options` query; locked stacks do not appear as enabled sell
      actions.
- [x] `market.create_listing` is retry/idempotency safe for duplicate requests.
- [x] Auction bid/buy-now UI uses `lot.currency_type` wallet data.
- [x] Premium stock purchase identity is product-specific and server-owned.
- [ ] All economy mutations have duplicate-send pending guards and smoke, not
      only `auction.bid`.
- [x] New purchase/grant paths have ledger/reference tests or a named durable
      transaction blocker.
- [x] Browser smoke covers shop category/detail/buy behavior.
- [x] Browser smoke fails on primary `Market/Sell/Auction/Premium` category
      truth, `raw_ore` shop purchase truth, and normal-player
      `server_recalculates` expectations.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Market|Auction|Premium|Economy|Catalog)' -count=1
go test ./internal/game/economy/... -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```

Capture screenshots under:

```text
output/screenshots/task-001/07/
```
