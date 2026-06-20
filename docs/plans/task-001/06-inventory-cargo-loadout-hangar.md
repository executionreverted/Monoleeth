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

### Public Contract Matrix

| Flow | Request | Response / events | Idempotency | Validation | Reconcile |
| --- | --- | --- | --- | --- | --- |
| `loadout.equip_module` | slot id and owned item instance id only | loadout, inventory, stats, active ship snapshots | WebSocket duplicate request cache plus item-move ledger refs `module_equip:<player>:<ship>:<item_instance>:<request_id>` when movement occurs | auth, ownership, rank, compatibility, cargo capacity, slot availability | snapshot merge only unless public equip event is added |
| `loadout.unequip_module` | slot id only | loadout, inventory/cargo, stats snapshots | WebSocket duplicate request cache plus item-move ledger refs `module_unequip:<player>:<ship>:<item_instance>:<request_id>` when movement occurs | auth, active ship, occupied slot, destination capacity | snapshot merge only unless public unequip event is added |
| `hangar.activate_ship` | owned ship id only | hangar, active ship, cargo, loadout, stats snapshots | WebSocket duplicate request cache; domain `SetActiveShip` is state-setting and same-active no-op, but durable `ship_activate:<player>:<ship>:<request>` idempotency is not implemented | auth, ownership, disabled state, safe context, rank, cargo transfer capacity | snapshot merge; clear incompatible selected module state |
| `inventory.move` | source/destination/stack intent only after server contract exists | inventory/cargo/storage snapshots and ledger refs | request id plus item/stack/source/dest domain key | auth, ownership, capacity, tradeability, locks, escrow, amount | hidden until implemented; no fake move UI |

Cargo payloads must include or reference display metadata: display name,
category, art key, quantity, unit mass/capacity impact, cargo location, move
eligibility, and safe locked reason. The UI must not render raw `item_id` as
normal player copy.

## Subagent Review Additions - 2026-06-20

- Add a backend/UI contract gap for cargo display metadata. Cargo snapshots need
  display name, category, unit weight/capacity impact, cargo location, and move
  eligibility; the UI must not render raw `item_id` rows as player copy.
- The current combined `Inventory / Cargo` window must be replaced, not merely
  restyled. Smoke must verify top tabs for Equipment, Inventory, Cargo, and
  Crafting or a documented hidden Crafting blocker.
- Slot/layout smoke should compare the Equipment surface against
  `darkorbit-envanter-ornek-layout.png`: ship preview, grouped slots, item
  inventory, selected detail, and stable dimensions.
- Add duplicate/idempotency tests for `loadout.equip_module`,
  `loadout.unequip_module`, and `hangar.activate_ship`, including duplicate
  request id, wrong slot, unknown module, cargo overflow, and active-ship
  reconciliation.

## Second Subagent Review Additions - 2026-06-20

- Add explicit tab state and DOM selectors for `Equipment`, `Inventory`,
  `Cargo`, and `Crafting`. Smoke must verify top tabs, empty/loading states,
  selected tab persistence, and that the current combined `Inventory / Cargo`
  surface is gone.
- Keep `Cargo` and `Crafting` read-only or hidden where mutations are still
  guarded. `inventory.move` and `crafting.start`/`complete`/`cancel` must not
  appear as enabled UI until their server contracts exist.
- Add a client pending key for equip/unequip paths by operation, slot id, and
  item instance id. Drag/drop and click fallback must emit exactly one command
  per user action.
- Runtime hangar activation currently risks using base cargo capacity instead
  of module-aware effective capacity. Either wire the effective provider or
  keep affected activation paths blocked with game copy and a named blocker.
- Add gateway duplicate replay tests for equip, unequip, and activate; browser
  smoke must count exact commands instead of only checking that some command
  happened.

## Third Subagent Review Additions - 2026-06-20

- Cargo payloads are still too thin for game UI. Add `cargo.items[]` display
  fields: `item_id`, `display_name`, `category`, `art_key`, `quantity`,
  `unit_weight`, `used_units`, `location`, `move_eligible`, and
  `locked_reason`.
- `inventory.move` remains absent from the public protocol. Either implement
  the operation, payload, response snapshots, ledger/reference behavior,
  duplicate replay rules, and public errors, or keep all move UI hidden with a
  named blocker.
- Runtime hangar activation still uses base capacity/speed style providers
  instead of module-aware effective stat recompute. Wire module-aware providers
  or block non-starter activation paths with a named blocker.
- Add per-action pending keys for `equip:<slot>:<item>`,
  `unequip:<slot>`, and `activate:<ship>` so click and drag/drop cannot send
  duplicate commands before reconciliation.
- Browser smoke must assert exact command count `1` for click and drag/drop
  equip/unequip, not only that some command appears.

## Fourth Subagent Review Additions - 2026-06-20

- Decide the Crafting tab policy now that `crafting.recipes` exists. Either
  hide the tab until crafting mutations are wired, or render a read-only recipe
  grid backed by `crafting.recipes` with display metadata and no enabled
  mutation buttons.
- Unknown catalog cargo items must fail closed. Snapshot enrichment should not
  fall back to `display_name = item_id`, `category = unknown`, or
  `unit_weight = 0` in normal player UI without a named backend blocker and
  safe placeholder copy.
- Active ship rows in Hangar should show an `Active` status badge, not a
  disabled primary `Activate` button. The `Activate` command should appear only
  for inactive owned ships where the server says activation is meaningful.
- Broaden Phase 06 raw-id smoke to all visible text plus `title` and
  `aria-label` in Inventory, Equipment, Cargo, Crafting, Hangar, module detail,
  and slot cards.
- Remove smoke expectations that bless active-ship disabled action clutter.

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

## Implementation Evidence - 2026-06-20

- Runtime cargo snapshots now enrich each cargo stack from the server-owned
  item catalog with `display_name`, `category`, `art_key`, `rarity`,
  `unit_weight`, `used_units`, `location`, `move_eligible`, and
  `locked_reason`, while preserving `item_id` and `quantity` for stable ids.
- Cargo stacks are sorted by `item_id` before serialization so UI/tests do not
  depend on Go map iteration order.
- Because public `inventory.move` is still absent, cargo transfer affordances
  remain locked in payloads with `move_eligible: false` and a safe
  `cargo_transfer_unavailable` reason.
- Client state now parses cargo display metadata without dropping the stable
  item id. The HUD cargo strip renders `display_name` when present instead of
  raw `item_id`, and smoke fixtures carry the same metadata shape.
- Client command dispatch now keeps semantic pending keys for
  `equip:<slot>:<item>`, `unequip:<slot>`, and `activate:<ship>` so click,
  drag/drop, or repeated UI events cannot send the same loadout/hangar intent
  again before server reconciliation. Duplicate blocked actions do not build a
  new request envelope, so `client_seq` does not advance for rejected local
  duplicates.
- Browser smoke now double-clicks and double-drops loadout equip/unequip
  actions in the same browser task and asserts the `Sent loadout.equip_module.`
  and `Sent loadout.unequip_module.` command counts increase by exactly one for
  click and drag/drop paths.
- Read-only subagent review confirmed `hangar.activate_ship` is currently
  protected by the client semantic key plus transport request cache/same-active
  no-op, but the hangar domain service does not yet accept a request id or
  produce a durable `ship_activate:<player>:<ship>:<request>` idempotency key.
  The browser smoke also lacks an inactive owned ship fixture, so exact
  double-click activate coverage remains open.
- Server WebSocket replay coverage now extends the existing loadout and hangar
  mutation tests. `loadout.equip_module`, `loadout.unequip_module`, and
  `hangar.activate_ship` each send the exact same request twice on the same
  session and assert the second raw response is byte-identical to the first
  cached response after first-command events are drained.
- This replay test proves gateway/session request-cache behavior only. It does
  not prove durable command-result idempotency, cross-session replay,
  restart/cache-eviction behavior, or hangar domain idempotency.
- The Inventory window now renders a server-snapshot-backed tab shell with
  `Equipment`, `Inventory`, `Cargo`, and `Crafting` tabs instead of one stacked
  inventory/cargo/loadout body. The Equipment tab owns the active ship preview,
  slot board, module grid, selected module detail, and real equip/unequip
  actions. The Cargo tab owns capacity and cargo stack display. The Inventory
  tab owns stored modules and account stack display. The Crafting tab is a
  quiet locked state because public crafting mutations are still absent.
- Inventory tab smoke now asserts the tab labels, active panel switching,
  Equipment/Cargo panel isolation, cargo display metadata, no raw cargo
  `item_id`, no enabled Crafting mutation button, no `Inventory / Cargo` window
  title, scoped drag/drop selectors, and exact one-command equip/unequip
  behavior after returning to Equipment.
- Visible/hover inventory location copy now maps internal locations such as
  `account_inventory`, `ship_equipped`, and `ship_cargo` to player-facing
  labels such as `Stored`, `Equipped`, and `Cargo hold`.
- Equipment now groups server-owned loadout slots by slot type with stable
  group containers and keeps the existing slot ids/drop targets intact for
  equip/unequip.
- Module bays now expose local presentation filters for all, weapons, defense,
  and utility modules. Filtering uses server-owned `module_slot_type` metadata,
  preserves selected-module reconciliation, and does not emit gameplay
  commands.
- Browser smoke now asserts slot group/slot count parity with the loadout
  snapshot, stable slot rects without panel horizontal overflow, module filter
  controls, filtered card type parity, and the existing exact one-command
  equip/unequip click and drag/drop flows after returning to `All`.
- Hangar detail now renders an `Active` status badge for the active ship instead
  of a disabled primary `Activate` button. The `hangar.activate_ship` button is
  absent for the active ship and remains reserved for inactive activatable
  ships.
- Browser smoke now asserts the active hangar detail shows the `Active` badge,
  has no visible `hangar-activate` button for the active ship, and sends no
  activation command while inspecting the starter ship.
- Targeted verification for this slice:
  `go test ./internal/game/server -run 'TestCombatKillCreatesLootAndPickupUpdatesCargo|TestPhase06SnapshotQueriesUseServerResolvedState' -count=1`,
  `GOCACHE=/tmp/gameproject-go-cache go test ./internal/game/server -run 'TestLoadoutEquipAndUnequipMutateServerOwnedInventory|TestHangarActivateShipUsesServerOwnedHangarState' -count=1`,
  `npm --cache /tmp/gameproject-npm-cache run typecheck`,
  `npm --cache /tmp/gameproject-npm-cache run smoke`,
  `npm --cache /tmp/gameproject-npm-cache run test -- --run src/state/reducer.test.ts`,
  `GOCACHE=/tmp/gameproject-go-cache go test ./...`,
  `npm --cache /tmp/gameproject-npm-cache run check`, and `git diff --check`.

## Acceptance Criteria

- [x] Inventory/Cargo/Loadout use tabs, not a single stacked panel.
- [ ] Public contract matrix exists for inventory/loadout/hangar operations.
- [x] Active ship is visible in the Equipment/Loadout surface.
- [x] Slot groups render as slots with stable dimensions.
- [x] Module inventory is filterable/selectable.
- [ ] Equip and unequip call real server contracts.
- [x] Cargo has its own tab/surface and does not masquerade as inventory.
- [ ] Cargo/inventory payloads include display metadata and action affordances
      needed by the UI.
- [x] Cargo payload includes display name/category/art key/unit weight/location
      and move eligibility fields or a named server-contract blocker.
- [x] Cargo tab smoke proves no raw `item_id`/snake_case label appears when
      cargo has items.
- [ ] Hangar shows owned ships, selected ship detail, and active ship state.
- [ ] Activation uses real `hangar.activate_ship` or remains clearly unavailable
      with game copy and a named contract blocker.
- [x] Active Hangar ship shows status copy/badge instead of a disabled primary
      `Activate` control.
- [ ] Hangar activation validates module-aware cargo capacity or records a
      named blocker.
- [x] Crafting controls are implemented with real contracts or hidden/blocked
      with a named owner phase.
- [ ] Crafting tab policy is explicit: hidden or read-only recipes from
      `crafting.recipes`, with display metadata and no fake mutation buttons.
- [ ] Unknown catalog cargo/inventory records fail closed without rendering raw
      ids or zero-weight capacity lies.
- [ ] No Phase 06 surface shows `server-owned`, raw item ids, or internal
      validation copy to normal players.
- [ ] Browser smoke verifies real data, not client fixtures.
- [x] Browser smoke asserts equip/unequip click and drag/drop paths emit exactly
      one command per action.
- [x] Gateway/WebSocket tests cover duplicate request-id replay for equip,
      unequip, and hangar activation.
- [ ] Durable domain idempotency for `hangar.activate_ship` is implemented or
      remains explicitly blocked.
- [x] Client pending keys prevent duplicate equip/unequip/activate sends before
      server reconciliation.

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
