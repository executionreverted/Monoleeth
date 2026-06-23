# UI Patch 3 Goal

Date: 2026-06-19

## Objective

Patch the authenticated browser game so Inventory, Hangar, Planets, Quests, and
Shop become real game systems instead of generic summary panels, while also
fixing planet modal/navigation behavior, disconnect movement settlement,
radar/known-intel map rendering, and modal click safety during movement.

This goal is complete only when:

- planet list/detail opens through a centered draggable modal/window, not
  inline list expansion
- planet detail has quick navigation that sends a real `move_to` intent to a
  server-returned coordinate
- WebSocket disconnect settles active movement to the current server-computed
  position and reconnect reconciles there
- radar/known-intel map rendering stays server-owned without visual fog-of-war
  reveal, hidden entities, or procedural truth leaks
- windows/modals remain clickable while the ship is moving
- Inventory shows the active ship, loadout slots, module inventory, and
  drag/drop equip/unequip backed by real server contracts
- Hangar shows an owned ship list, selected ship detail, active ship state, and
  real activation or locked reasons
- Planets is a catalog/detail/action surface for discovered planets, production,
  storage, routes, and locked unsupported actions
- the left menu says Quests, not Galaxy, and opens a selectable quest board
- Shop has categories, product/listing rows, selected detail, quantity controls,
  and real market/auction/premium actions
- every gameplay value comes from authenticated server snapshots, responses, or
  events
- screenshots under `output/screenshots/ui-patch-3/` show clear progress toward
  `output/mockups/final-mockup.png`

## Required Reading Before Every Slice

Read these before starting any UI Patch 3 implementation turn:

```text
AGENTS.md
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
the matching docs/plans/ui-patch-3 phase file
docs/todo.md
```

For visual/UI work, inspect:

```text
output/mockups/final-mockup.png
output/assets/mockup-hud/background/starfield_2048x1152.png
output/assets/hud-svg/
client/
```

For relevant server work, read:

```text
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
docs/2026-06-17-progression-economy-systems-design.md
```

## External UI References

Use DarkOrbit only as a reference for game UI shape and interaction patterns:

```text
https://darkorbit.fandom.com/wiki/Hangar
https://darkorbit.fandom.com/wiki/Weapons_UI
https://darkorbit.fandom.com/wiki/User_Interface
```

Observed patterns to adapt:

- Hangar is ship list + active selection + manage/equipment surface.
- Equipment uses inventory items dragged into ship slots.
- Shop/product UI uses list/category browsing, selected item detail, quantity
  controls, and a clear buy action.
- Feature windows open over the game map.

Do not copy DarkOrbit data, names, art, economy values, or unimplemented
features. Adapt the interaction pattern to this game's server-owned state.

## Working Style

- Main session is the project manager.
- Use Symphony/subagents for independent research or implementation slices when
  useful.
- Worker prompts must tell workers to follow `docs/symphony-worker-rules.md`.
- Worker prompts must not ask workers to read `AGENTS.md` or
  `docs/symphony-operating-model.md`.
- Workers must not spawn subagents, manage Symphony, or commit.
- Review every diff before applying.
- Implement one vertical slice at a time.
- Update phase checklists only after verification.
- Every UI slice must inspect `output/mockups/final-mockup.png` before and
  after the change.

## Non-Negotiable Rules

- No fake gameplay values.
- No client-authored player id, position, speed, damage, cooldown, XP, loot,
  wallet, cargo, inventory, loadout, quest progress, market total, planet
  ownership, or hidden world truth.
- UI-only state may track window positions, selected tabs, selected rows,
  drag/drop hover, focused panel, and pending presentation state.
- Server state owns world position, active route, known-intel memory, visible
  entities, combat, loot, inventory, wallet, active ship, loadout, quests,
  planets, production, routes, market, auction, and premium.
- Unsupported gameplay actions must remain disabled/locked unless real server
  contracts are implemented in the same slice.
- Every mutation must validate auth, ownership, current-map visibility,
  radar/stealth detection, known-intel permission, range/capacity, rank,
  cooldown/rate limit, idempotency, and ledger/transaction boundaries as
  applicable.

## Done Means

### Planet Modal And Movement Settlement

- [x] Planet rows open a modal/window detail, not inline expansion.
- [x] Planet detail shows server-safe overview, coordinates, owner/intel state,
      and action status.
- [x] Navigate sends only a real `move_to` intent to server-returned
      coordinates.
- [x] Disconnect while moving settles the ship to the current server-computed
      position and clears active movement.
- [x] Reconnect world snapshot shows the settled position.

### Input Safety While Moving

- [x] Modals/windows remain clickable while movement ETA is active.
- [x] Dragging windows works while moving.
- [x] HUD/modal clicks cannot leak into world movement/selection.
- [x] Empty world clicks still move normally.

### Radar, Stealth, And Known Intel

- [x] Visual fog-of-war overlay is not required for the bounded-map playtest.
- [x] Current radar/contact rendering follows the interpolated server-owned
      player position and server payloads.
- [x] Remembered discoveries render only from server-safe known-intel memory.
- [x] Hidden gameplay data is never serialized to power map visuals.

### Inventory And Loadout

- [x] Inventory shows active ship and loadout slots.
- [x] Module inventory is browsable/selectable.
- [x] Drag/drop equip calls real `loadout.equip_module`.
- [x] Unequip calls real `loadout.unequip_module`.
- [x] Server validates slot compatibility, ownership, rank, durability,
      location, duplicate use, and idempotency.
- [x] Loadout/inventory/stats snapshots reconcile after mutation.

### Hangar

- [x] Hangar shows owned ships and active ship state.
- [x] Selecting a ship opens real detail.
- [x] Activate uses real `hangar.activate_ship` or remains locked with a
      documented server-contract blocker.
- [x] Activation validates safe area, cargo, rank, and ship state server-side.

### Planets

- [x] Planets window is a catalog/detail/action surface.
- [x] Production, storage, and routes show real server summaries when present.
- [x] Claim/build/upgrade/route/auto are real or visibly locked.
- [x] Stale intel is visually distinct from live visibility.

### Quests

- [x] Nav label is `Quests`, not `Galaxy`.
- [x] Quest Board lists offers, active, claimable, and completed quests.
- [x] Selecting a quest shows objectives and rewards.
- [x] Accept, Claim, and Reroll use real quest contracts.

### Shop

- [x] Shop has categories/tabs.
- [x] Product/listing rows and selected detail are visible.
- [x] Quantity controls exist where supported.
- [x] Buy/List/Cancel/Bid/BuyNow/Claim use real contracts.
- [x] Server-calculated totals/fees/escrow/stock remain authoritative.

### Visual And Verification

- [x] HUD composition remains aligned with `final-mockup.png`.
- [x] Desktop/tablet/mobile screenshots are captured under
      `output/screenshots/ui-patch-3/`.
- [x] Browser smoke covers the main phase behaviors in a real authenticated
      session.
- [x] No real-mode fake gameplay values appear.

## Phase Plan

Start at:

```text
docs/plans/ui-patch-3/00-index.md
```

Recommended order:

1. `01-planet-modal-navigation-disconnect-settlement.md`
2. `02-modal-input-during-movement.md`
3. `03-fog-of-war-visibility-rendering.md`
4. `04-inventory-loadout-drag-drop.md`
5. `05-hangar-ship-management.md`
6. `06-planets-catalog-actions.md`
7. `07-quests-board-rework.md`
8. `08-shop-catalog-product-detail.md`
9. `09-mockup-darkorbit-parity-verification.md`

## Required Final Verification

Before claiming this goal complete:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```

Also verify in browser:

- desktop viewport near `1440x900`
- tablet viewport near `1024x768`
- mobile viewport near `390x844`
- authenticated real-server session
- moving ship while clicking modals
- planet modal and Navigate
- visual fog inactive while radar/known-intel map data remains server-owned
- inventory drag/drop equip or locked server-contract blocker
- hangar ship list/activation or locked server-contract blocker
- planet catalog tabs/actions
- quest board selection/action
- shop category/detail/action
