# Hangar Ship List And Active Ship Management Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Hangar becomes a ship management surface with an owned ship list,
selected ship detail, active ship state, and safe active-ship switching.

**Architecture:** Hangar state and active ship are server-owned. Client can
select rows locally, but activation is an authenticated command validated by
the hangar/ship service.

**Tech Stack:** Go ships/hangar service, runtime handler, realtime protocol,
TypeScript HUD/reducer, browser smoke.

---

## Required Reading

```text
docs/plans/ui-patch-3-goal.md
docs/plans/ui-patch-3/00-index.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
internal/game/ships/service.go
output/mockups/final-mockup.png
```

## Current Behavior

- The left navigation label says Hangar, but `systemsPanel` only shows one
  active ship metric and loadout count.
- `hangarSnapshotLocked` returns a single active runtime ship.
- `internal/game/ships/service.go` has a `HangarService.SetActiveShip` domain
  path, but the browser runtime does not expose `hangar.activate_ship`.

## Target UX

- Hangar window has:
  - owned ship list/carousel
  - selected ship preview/silhouette
  - active badge
  - hull/shield/capacity/speed/radar/cargo stats
  - state: available, active, disabled, repairing, locked
  - actions: Activate, Repair if disabled, Manage Loadout
- Ship list clearly distinguishes owned/locked/unavailable states.
- Active ship selection is disabled if:
  - ship not owned
  - ship disabled/repairing
  - not in safe hangar area
  - cargo exceeds target capacity
  - rank requirement missing
- Manage Loadout opens the inventory/loadout slot board for the selected or
  active ship, depending on supported server contract.

## Server Contract Tasks

1. Add `hangar.activate_ship` operation if not already present.
   - Payload:
     ```json
     {"ship_id":"starter"}
     ```
   - Resolve player and active context server-side.

2. Implement runtime handler.
   - Use `HangarService.SetActiveShip` if runtime composition supports it.
   - If current runtime only has one starter ship, first add a real owned ship
     read model rather than faking multiple ships.
   - Validate safe area/combat/cargo/rank using server-owned providers.
   - Emit `hangar.snapshot`, `ship.snapshot`, `stats.updated`,
     `cargo.snapshot`, `loadout.snapshot`, and AOI update as needed.

3. Improve `hangar.snapshot`.
   - Include all owned ships from the hangar service/store.
   - Include enough public ship definition stats for comparison.
   - Do not include hidden catalog/premium availability as owned.

## Client Tasks

1. Split Hangar from the generic `systemsPanel`.
   - Dedicated `hangarPanel`.
   - Keep Inventory/Loadout in phase 04.

2. Add selected ship UI state.
   - Local selected ship id is okay.
   - Active ship remains from server snapshot.

3. Add action handlers.
   - `onHangarActivateShip(shipID)`
   - `onOpenLoadoutForShip(shipID)` if needed.

4. Tests.
   - Go handler tests for safe area, cargo too large, disabled ship, unknown
     ship, already active no-op.
   - Client reducer/protocol tests.
   - Browser smoke: open Hangar, select ship, verify active state/locked
     reason, activate if seeded with multiple owned ships.

## Files Likely Touched

```text
internal/game/realtime/envelope.go
internal/game/server/handlers.go
internal/game/server/progression_inventory_handlers.go
internal/game/server/runtime.go
internal/game/server/server_test.go
internal/game/ships/service.go
internal/game/ships/service_test.go
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
```

## Acceptance Checklist

- [x] Hangar window has owned ship list.
- [x] Active ship is visible and cannot be confused with selected ship.
- [x] Selected ship detail shows real server/catalog stats.
- [x] Activate uses real `hangar.activate_ship`.
- [x] Activation validates safe area, cargo, rank, and ship state server-side.
- [x] Active change reconciles ship, stats, hangar, loadout, and AOI state.
- [x] No fake unowned ship list is shown in real mode.
- [x] Browser smoke covers the hangar surface.

## Implementation Notes

- Runtime now uses the real `ships.HangarService` and `ships.MVPShipCatalog`
  for starter ownership and active ship state. The runtime ship id is aligned
  with the ship domain catalog (`starter`) so hangar, loadout, scanner, repair,
  and inventory-equipped locations refer to the same ship id.
- `hangar.activate_ship` accepts only a `ship_id` intent. The server resolves
  player/session, current cargo, safe hangar area, combat/disabled state,
  ownership, rank, and target ship usability before mutating active ship state.
- Hangar snapshot rows include owned ship list data, active flag, effective
  active ship stats, catalog role/tier/slot/capacity stats, and locked reason.
  The browser does not render fake unowned ships in real mode.
- Browser smoke opens the Hangar window in a real authenticated session and
  verifies the owned active starter row, detail panel, disabled active activate
  control, and responsive screenshot under `output/screenshots/ui-patch-3/`.
- Hangar row selection is local UI state. Selecting a row changes the detail
  pane; activation remains a separate `hangar.activate_ship` command guarded by
  server validation.

## Verification

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/ships ./internal/game/server
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
cd ..
git diff --check
```
