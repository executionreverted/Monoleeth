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
- [ ] Server-owned shop catalog/system-product contract exists or the expanded
      market payload explicitly provides equivalent metadata.
- [ ] Categories map to real server catalog/listing data.
- [ ] No player-facing `server recalculates` copy remains.
- [ ] `server_recalculates` is absent from normal player UI and client smoke
      expectations.
- [ ] Raw temporary names are removed or replaced by server display names.
- [ ] Quantity and purchase/list/bid actions reconcile with server responses.
- [ ] Market totals/fees/escrow are not trusted from the client.
- [ ] Auction and premium event paths reconcile or refresh passive clients.
- [ ] Auction/premium grants are real for their payload type or hidden/locked
      with a named blocker.
- [ ] Purchase/bid buttons debounce pending actions and smoke asserts one click
      emits exactly one mutation command.
- [ ] New purchase/grant paths have ledger/reference tests or a named durable
      transaction blocker.
- [ ] Browser smoke covers shop category/detail/buy behavior.

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
