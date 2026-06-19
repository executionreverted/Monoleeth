# Phase 06 - Inventory, Cargo, Loadout, And Hangar Game Layouts

## Goal

Rebuild Inventory, Cargo, Loadout, and Hangar as real game system windows. The
layout should follow the local inventory mockup: ship preview, slot board,
available item grid, top tabs, and clear active ship state.

## Problems Covered

- Inventory and cargo are stacked together instead of being tabbed systems.
- Active ship and slots are not visually central enough.
- Drag/drop or explicit equip/unequip flow needs to feel like a game mechanic.
- Hangar lacks a strong owned ship list and active ship selection surface.
- Current panels do not match the DarkOrbit inventory layout reference.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/plans/task-001/05-seeded-game-content-catalog.md
docs/todo.md
output/mockups/darkorbit-envanter-ornek-layout.png
output/mockups/final-mockup.png
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/04-module-stat-aggregation.md
docs/plans/modules/08-crafting-recipes-materials.md
client/src/ui/hud.ts
client/src/styles.css
internal/game/server/progression_inventory_handlers.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
```

## Design Contract

- Tabs: `Equipment`, `Inventory`, `Cargo`, `Crafting` if crafting is included.
- Equipment tab: active ship preview left, slot groups center, item inventory
  right.
- Cargo tab: cargo capacity, resource stacks, loot, transfer/move actions when
  server contracts exist.
- Inventory tab: filters by module/item type, selected item detail, action row.
- Hangar: owned ship list, active ship marker, selected ship stats, activate
  action if server-safe.
- All item counts, stats, slot contents, cargo, and active ship truth are from
  server snapshots/responses.
- Phase 05 catalog/display metadata is required before final UI polish; do not
  build the final layout around raw ids or thin cargo payloads.
- Public contract matrix must define request, response, public event/snapshot,
  idempotency key, validation, reconciliation, and error states for
  `inventory.move`, `loadout.equip_module`, `loadout.unequip_module`, and
  `hangar.activate_ship`.
- Browser reconciliation is snapshot-only unless this phase explicitly adds
  public events for `ship.active_changed`, `module.equipped`,
  `module.unequipped`, or `player.stats_invalidated`.

## Implementation Plan

1. Split UI surfaces.
   - Inventory/Cargo/Loadout must not render as one long vertical form.
   - Add top tabs with stable active tab state.
   - Keep drag/drop hover local; mutations stay server-owned.

2. Build slot board.
   - Group slots by weapons/lasers, launchers, generators, extras/modules,
     scanner/support/cargo where catalog supports it.
   - Empty slots render as stable slots, not text rows.
   - Equipped modules show type, tier, durability, and important stat effects.
   - Use display names/categories/art keys from server payloads, never raw ids.
   - Cargo rows need display name, category, quantity, capacity impact,
     location, transfer eligibility, and action affordances from server-safe
     metadata.

3. Wire real actions.
   - Keep existing `loadout.equip_module` and `loadout.unequip_module`.
   - Add `inventory.move` only if server contract is implemented in this slice.
   - Crafting controls stay hidden or locked until `crafting.start/complete`
     exist.
   - Duplicate equip/unequip request ids must not duplicate ledger/item
     movement.
   - Click and drag/drop paths must emit exactly one command per user action.

4. Rework Hangar.
   - Show owned ships and selected ship detail.
   - Use real `hangar.activate_ship`.
   - Display locked reasons only as game copy: safe zone, cargo not empty, rank,
     damaged, unavailable.
   - Validate cargo overflow using module-aware effective capacity, or document
     the runtime `BaseShipCargoCapacityProvider` as a blocker before enabling
     non-starter activation.
   - Cover non-starter owned ships, safe-zone/in-combat/rank/disabled/cargo
     overflow failures.

5. Add tests.
   - Tab switching.
   - Drag/drop or click equip emits one command.
   - Invalid slot rejection reconciles state.
   - Active ship switch reconciles loadout/stats.
   - Tab switching for Equipment/Inventory/Cargo/Crafting.
   - Server duplicate request-id idempotency and ledger assertions.

## Likely Files

```text
client/src/ui/hud.ts
client/src/styles.css
client/src/state/types.ts
client/src/state/reducer.ts
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/tests/browser-smoke.mjs
internal/game/server/progression_inventory_handlers.go
internal/game/runtime/providers.go
internal/game/server/server_test.go
internal/game/modules/catalog.go
internal/game/ships/catalog.go
docs/plans/task-001/06-inventory-cargo-loadout-hangar.md
```

## Acceptance Criteria

- [ ] Inventory/Cargo/Loadout use tabs, not a single stacked panel.
- [ ] Public contract matrix exists for inventory/loadout/hangar operations.
- [ ] Active ship is visible in the Equipment/Loadout surface.
- [ ] Slot groups render as slots with stable dimensions.
- [ ] Module inventory is filterable/selectable.
- [ ] Equip and unequip call real server contracts.
- [ ] Cargo has its own tab/surface and does not masquerade as inventory.
- [ ] Cargo/inventory payloads include display metadata and action affordances
      needed by the UI.
- [ ] Hangar shows owned ships, selected ship detail, and active ship state.
- [ ] Activation uses real `hangar.activate_ship` or remains clearly unavailable
      with game copy and a named contract blocker.
- [ ] Hangar activation validates module-aware cargo capacity or records a
      named blocker.
- [ ] Crafting controls are implemented with real contracts or hidden/blocked
      with a named owner phase.
- [ ] No Phase 06 surface shows `server-owned`, raw item ids, or internal
      validation copy to normal players.
- [ ] Browser smoke verifies real data, not client fixtures.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Inventory|Cargo|Loadout|Hangar|Ship)' -count=1
go test ./internal/game/modules ./internal/game/ships -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```

Capture screenshots under:

```text
output/screenshots/task-001/06/
```
