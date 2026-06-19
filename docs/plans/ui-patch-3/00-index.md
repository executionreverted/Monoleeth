# UI Patch 3 Plan Index

Date: 2026-06-19

## Purpose

UI Patch 3 turns the browser HUD from a collection of thin summary panels into
game systems. The current client can authenticate, move, scan, show real
server state, and open draggable windows, but several menu surfaces still feel
like generic data renderers instead of an MORPG cockpit.

This patch fixes the next playtest issues:

- planet list detail opens inline instead of as a modal/catalog detail
- planet quick navigation is missing
- disconnect does not explicitly settle an active route to the current
  authoritative position
- fog of war is not visible even though server visibility concepts exist
- inventory does not show a ship/loadout slot board or drag/drop equip flow
- hangar does not expose a usable ship list and active ship selection
- planet windows do not expose planet catalog/detail/action workflows
- the "Galaxy" menu is semantically wrong for the current game surface and
  should be Quests
- modal/window clicks are unreliable while moving
- shop lacks category browsing, product list, selected product detail, and
  purchase/listing flows that resemble a game shop

## Primary References

```text
docs/plans/ui-patch-3-goal.md
output/mockups/final-mockup.png
output/assets/mockup-hud/background/starfield_2048x1152.png
output/assets/hud-svg/
docs/todo.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
```

## DarkOrbit Reference Notes

Use these only as interaction/layout references. Do not copy data, names, or
unimplemented gameplay truth.

- DarkOrbit Hangar has a ship list, active ship selection, and a manage screen
  where modules/items are dragged from inventory into ship slots.
- DarkOrbit shop-like weapon UI uses a browsable list, selected item showcase,
  description/detail area, quantity controls, and buy action.
- DarkOrbit in-game UI opens feature windows over the space map instead of
  replacing the game surface.

Useful research links:

```text
https://darkorbit.fandom.com/wiki/Hangar
https://darkorbit.fandom.com/wiki/Weapons_UI
https://darkorbit.fandom.com/wiki/User_Interface
```

## Current Code Audit Findings

- `client/src/ui/hud.ts` still renders the planet detail inline in
  `planetsPanel` through `planetDetailBlock`.
- `client/src/ui/hud.ts` has reusable windows/modals, but feature surfaces are
  still simple rows: `cargoPanel`, `economyPanel`, `systemsPanel`, and
  `questsPanel` do not model the actual game workflows.
- `client/src/ui/hud.ts` labels the quest window as `Galaxy`.
- `client/src/ui/hud.ts` `systemsPanel` merges hangar, loadout, inventory, and
  crafting into a few metric rows.
- `client/src/ui/hud.ts` `economyPanel` shows one market row, one auction row,
  one premium row, and no category/product/detail shop structure.
- `client/src/ui/hud.ts` `questBlock` focuses one quest/offer instead of a
  selectable quest board.
- `client/src/styles.css` and HUD event guards improved input isolation in
  patch 2, but moving state and overlay pointer handling still need a dedicated
  modal interaction smoke test because the playtest reports modal clicks fail
  while moving.
- `client/src/render/world-renderer.ts` has starfield, memory markers, scan
  waves, and projectiles, but no fog/darkness overlay.
- `client/src/state/types.ts` has `minimap.remembered`, but server runtime
  currently returns it empty.
- `internal/game/server/runtime.go` `minimapFromAOI` returns
  `Remembered: []minimapMemoryPayload{}`.
- `internal/game/world/visibility/fog.go` has fog memory primitives, but the
  runtime does not project safe remembered cells/planets into minimap/world
  rendering.
- `internal/game/server/transport.go` unregisters the socket and detaches the
  session, but there is no explicit disconnect settlement that stops an active
  route at the current server-computed position.
- `internal/game/world/worker/worker.go` can advance movement on tick and stop
  a player, but it lacks a command/helper that settles movement to "now" on
  session detach.
- `internal/game/server/progression_inventory_handlers.go`
  `hangarSnapshotLocked` returns only the active runtime ship; it is not yet a
  real catalog/list of owned ships.
- `internal/game/server/progression_inventory_handlers.go`
  `loadoutSnapshotLocked` returns fixed starter slots and does not expose
  authenticated equip/unequip mutations.
- Protocol tests mention `loadout.equip_module` and
  `loadout.unequip_module`, and the implementation plan mentions
  `hangar.activate_ship`, but runtime handler registration does not include
  those commands yet.

## Phase List

1. [Planet Detail Modal, Quick Navigation, And Disconnect Settlement](./01-planet-modal-navigation-disconnect-settlement.md)
2. [Modal Interaction And Movement Input Isolation](./02-modal-input-during-movement.md)
3. [Fog Of War And Remembered Map Rendering](./03-fog-of-war-visibility-rendering.md)
4. [Inventory Loadout Slot Board And Drag Drop Equip](./04-inventory-loadout-drag-drop.md)
5. [Hangar Ship List And Active Ship Management](./05-hangar-ship-management.md)
6. [Planets Catalog Detail And Planet Actions](./06-planets-catalog-actions.md)
7. [Quest Board Replacing Galaxy Menu](./07-quests-board-rework.md)
8. [Shop Catalog Categories And Product Detail](./08-shop-catalog-product-detail.md)
9. [Mockup And DarkOrbit Parity Verification Gate](./09-mockup-darkorbit-parity-verification.md)

## Suggested Symphony Split

- World/session worker: phase 01 disconnect settlement and planet navigation.
- Input/window worker: phase 02 modal click safety while moving.
- Fog/render worker: phase 03 server remembered map payload plus renderer fog.
- Systems worker: phases 04 and 05 inventory/loadout/hangar contracts and UI.
- Planet/quest/economy worker: phases 06, 07, and 08 domain windows.
- QA worker: phase 09 screenshots, smoke tests, and no-fake-state audit.

Workers must follow `docs/symphony-worker-rules.md`, must not spawn subagents,
must not manage Symphony, and must not commit.

## Global Implementation Rules

- No fake gameplay values.
- No hidden planet, NPC, loot, or procedural seed leaks.
- Client requests are intents only.
- Window position, selected tab, selected list item, drag hover, and pending UI
  animation may be client-local.
- Position, active route, cargo, wallet, inventory, loadout, active ship, quest
  progress, planet ownership, production, route state, market totals, and
  premium state must come from server snapshots/events/responses.
- Any newly enabled mutation must add server validation, idempotency posture,
  reducer reconciliation, browser smoke, and server tests in the same slice.
- Unsupported actions must be visibly locked or absent; do not make clickable
  placeholders that imply a real gameplay operation.

## Verification Commands

Use narrow commands during implementation, then run the full gate:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```

UI screenshots for this patch should live under:

```text
output/screenshots/ui-patch-3/
```

## Done Criteria

- Every phase file has its checklist updated only for work actually completed.
- All user-reported playtest issues are either fixed or explicitly blocked by a
  named missing server contract in the relevant phase file.
- Browser smoke proves windows/modals remain clickable while the ship is moving.
- Planet quick navigation uses server movement intent and never client-authors
  position.
- Socket disconnect/reconnect leaves the player at the server-settled current
  position, not the old origin or full destination.
- Fog of war visibly darkens unexplored space and uses only server-safe
  visibility/fog memory payloads.
- Inventory, hangar, planets, quests, and shop are real game surfaces with
  category/list/detail/action structure.
- `final-mockup.png` remains the visual HUD target, and screenshots show the
  patch moving toward it.
- Full verification passes.

