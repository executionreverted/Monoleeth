# Inventory Loadout Slot Board And Drag Drop Equip Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Inventory becomes a real equipment/loadout surface: the active ship is
visible, slots are visible, module inventory is browsable, and drag/drop equip
or unequip uses authenticated server contracts.

**Architecture:** Client drag/drop is presentation only. Equip/unequip mutates
through server-owned loadout handlers that validate ownership, active ship,
slot compatibility, rank, durability, duplicate use, capacity, and idempotency.

**Tech Stack:** Go loadout/economy/runtime handlers, realtime protocol,
TypeScript command builder/reducer/HUD drag/drop, CSS slot board.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
docs/todo.md
output/mockups/final-mockup.png
```

## Current Behavior

- `systemsPanel` shows a few metrics for hangar/loadout/inventory.
- `cargoPanel` shows cargo stacks, not a module inventory/loadout board.
- `loadoutSnapshotLocked` returns hard-coded starter slots.
- `loadout.equip_module` and `loadout.unequip_module` appear in protocol
  tests/plans, but runtime handlers are not registered.
- `docs/todo.md` explicitly tracks authenticated browser loadout mutation
  contracts as open.

## Target UX

- Inventory window opens centered and game-like.
- Left/middle: active ship silhouette with slot groups:
  - offensive
  - defensive
  - utility
  - future extras locked/hidden if not implemented
- Right: module/item inventory grid with filters.
- Selecting an item shows detail:
  - name, rarity, type, durability, location, bind state
  - compatible slots
  - locked reason when it cannot be equipped
- Drag module onto compatible slot to equip.
- Drag equipped module back to inventory or press unequip to unequip.
- Pending equip shows a small pending state, then reconciles from
  `loadout.snapshot` and `inventory.snapshot`.
- Unsupported items stay visible as cargo/storage, but not as fake modules.

## Server Contract Tasks

1. Add realtime operations if missing.
   - `loadout.equip_module`
   - `loadout.unequip_module`

2. Add command payloads.
   - Equip:
     ```json
     {"slot_id":"offensive_1","item_instance_id":"..."}
     ```
   - Unequip:
     ```json
     {"slot_id":"offensive_1"}
     ```

3. Implement runtime handlers.
   - Resolve player from session.
   - Resolve active ship server-side.
   - Validate slot exists on active ship.
   - Validate module instance belongs to player.
   - Validate item location is account inventory or already equipped on the
     same active ship.
   - Validate module definition compatibility/rank/durability.
   - Move item location through inventory/ledger primitives.
   - Use domain idempotency:
     - `module_equip:<player_id>:<item_instance_id>:<slot_id>`
     - `module_unequip:<player_id>:<slot_id>:<request_id>` until a better
       durable slot-transition key exists.
   - Emit/reconcile `loadout.snapshot`, `inventory.snapshot`,
     `stats.updated`, and any safe `ship.snapshot` changes.

4. Use existing module domain service where possible.
   - Start with `internal/game/modules/loadout_service.go`.
   - Do not duplicate compatibility/rank/durability rules in the handler.

## Client Tasks

1. Extend protocol constants and command builder.
   - `OPERATIONS.loadoutEquipModule`
   - `OPERATIONS.loadoutUnequipModule`
   - `CommandBuilder.loadoutEquipModule(slotID, itemInstanceID)`
   - `CommandBuilder.loadoutUnequipModule(slotID)`

2. Extend HUD handlers and UI.
   - Split Inventory from Hangar concerns.
   - Render active ship slot board.
   - Render inventory module grid.
   - Add drag/drop events with keyboard/click fallback.
   - Keep all pending state clearly pending and reconcile from server truth.

3. Reducer/state.
   - Parse updated snapshots.
   - Keep selected item/slot as local UI state inside HUD if possible.
   - Clear impossible pending states on server rejection.

4. Tests.
   - Go handler tests for ownership, wrong slot, wrong module type, duplicate
     request, already equipped elsewhere, insufficient rank, broken module.
   - Client protocol/reducer tests.
   - Browser smoke drag/drop equip and unequip in a seeded real session.

## Files Likely Touched

```text
internal/game/realtime/envelope.go
internal/game/server/handlers.go
internal/game/server/progression_inventory_handlers.go
internal/game/server/server_test.go
internal/game/modules/loadout_service.go
internal/game/modules/loadout_store.go
internal/game/economy/inventory_move.go
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/protocol/envelope.test.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
docs/todo.md
```

## Acceptance Checklist

- [x] Inventory window shows active ship and slot board.
- [ ] Module inventory grid is browsable and filterable.
- [x] Selecting a module shows compatible slots and details.
- [x] Drag/drop equip calls real `loadout.equip_module`.
- [x] Unequip calls real `loadout.unequip_module`.
- [x] Server validates ownership, active ship, slot, compatibility, rank,
      durability, location, and duplicate use.
- [x] Equip/unequip reconciles inventory, loadout, stats, and ship snapshots.
- [x] No fake module counts or fake slot contents are displayed.
- [ ] Browser smoke covers at least one successful equip/unequip and one
      rejected invalid equip.

## Implementation Notes

- Runtime now registers authenticated `loadout.equip_module` and
  `loadout.unequip_module` operations. Payloads carry only slot/item intent;
  player, active ship, slot layout, module definitions, rank, durability,
  location, and idempotency references are resolved server-side.
- Starter module items are seeded through the economy inventory and mirrored
  into the loadout store. The scanner starts equipped in the utility slot;
  laser and shield modules remain in account inventory for equip testing.
- Inventory UI now renders a ship/loadout slot board and server-owned module
  cards with drag/drop plus button fallback. Selecting a module opens a real
  detail panel with compatible slot, durability, state, location, and an
  `Equip Selected` control. Successful mutations reconcile from
  `inventory.snapshot`, `loadout.snapshot`, and `stats.updated`.
- Remaining polish for this phase is category filtering. Browser smoke covers
  selected module detail plus successful equip/unequip; invalid equip coverage
  is in Go handler tests.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/modules ./internal/game/server ./internal/game/economy
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
