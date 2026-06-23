# Phase 06: Progression, Inventory, Loadout, And Crafting UI

## Status

- State: Authenticated read-model MVP plus server-owned `crafting.start`,
  `crafting.complete`, and `crafting.cancel` contracts completed; remaining
  progression/inventory UI mutations are tracked in `docs/todo.md`
- Owner: Player growth and item management UI
- Depends on: Phase 05
- Unlocks: persistent character loop and equipment/craft loop

Current slice completed:
- Authenticated read-only snapshots are exposed for progression, inventory,
  cargo, hangar, loadout, stats, and crafting recipes.
- The browser client requests these snapshots after the authenticated world
  bootstrap and renders them in the real HUD without demo values.
- Loot pickup now reconciles a real inventory snapshot alongside cargo.
- `crafting.start` now maps gateway request ids to a server-owned
  `craft_start:*` domain reference, reserves materials, debits wallet, and
  returns crafting/inventory/wallet snapshots from authenticated player state.
- `crafting.complete` now validates job owner/state and server time, commits
  reserved materials, grants output/XP once, and returns
  crafting/inventory/progression snapshots.
- Browser crafting tab now renders real recipes and active jobs, sends only
  `crafting.start` recipe intent, `crafting.complete` job intent, or
  `crafting.cancel` job intent, and uses pending/server snapshots to reconcile
  buttons, timers, and cancelled terminal state.
- `crafting.cancel` releases the active reservation once through the economy
  reservation service, marks the job cancelled, returns crafting/inventory/
  wallet snapshots, emits one internal `craft.job_cancelled` domain event, and
  keeps the Phase 06 MVP no-fee-refund policy.
- Phase 10 records the exact missing browser/server contracts for skill unlock,
  inventory move, hangar activation, and loadout equip/unequip controls. These
  controls must stay absent, locked, or read-only until those contracts are
  implemented and verified.

## Goal

Expose progression, rank, role XP, pilot skills, inventory, cargo, ship hangar,
module loadout, stat aggregation, and crafting through real server-backed UI.

Dependency note: minimal wallet, cargo, active ship, and active loadout runtime
snapshots may be implemented before or inside Phase 05 so combat/loot/repair can
avoid fake state. This phase still owns the full UI for inventory, hangar,
loadout, progression, and crafting.

## Source Specs

Read before implementation:
- `docs/plans/modules/01-player-progression-rank-role-skills.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/03-ship-hangar-loadout.md`
- `docs/plans/modules/04-module-stat-aggregation.md`
- `docs/plans/modules/08-crafting-recipes-materials.md`
- `internal/game/progression`
- `internal/game/economy`
- `internal/game/ships`
- `internal/game/modules`
- `internal/game/stats`
- `internal/game/crafting`

## Server Features To Expose

- player progression snapshot
- rank/role XP views
- pilot skill tree view and unlock command
- wallet/cargo/inventory query
- inventory move command
- hangar/active ship query
- activate ship command
- loadout query
- module equip/unequip commands
- effective stat snapshot
- recipe list/query
- craft start/complete/cancel state
- crafting output and XP events

## Commands And Queries

```text
progression.snapshot
progression.unlock_skill
inventory.snapshot
inventory.move
hangar.snapshot
hangar.activate_ship
loadout.snapshot
loadout.equip_module
loadout.unequip_module
stats.snapshot
crafting.recipes
crafting.start
crafting.complete
crafting.cancel
```

## Mutation Contract Notes

| Operation | Idempotency / Ledger Requirement | Required Validation |
| --- | --- | --- |
| `progression.unlock_skill` | unique skill unlock reference per player/skill | owned player, available points, rank/role requirements |
| `inventory.move` | request id plus domain movement reference where needed | ownership, positive amount, source/destination capacity, item rules |
| `hangar.activate_ship` | active ship transition is idempotent for same ship | owned ship, usable state, location/rank requirements, not already invalidated |
| `loadout.equip_module` | equip reference per slot/item transition | owned item, ship slot, module compatibility, item not already escrowed/equipped elsewhere |
| `loadout.unequip_module` | unequip reference per slot transition | owned active ship, destination capacity, module exists in slot |
| `crafting.start` | `craft_start:<job_or_request_id>` and wallet/item ledger refs | recipe, materials, wallet, location, rank, queue limits |
| `crafting.complete` | `craft_complete:<job_id>` unique reference | server time, job owner/state, output capacity |
| `crafting.cancel` | `craft_cancel:<job_id>` unique reference | job owner/state, running-only state, material release, no fee refund |

Every item or currency mutation must use the existing inventory/wallet services
and ledger/event references, never direct balance or stack edits.

## Events

```text
progression.snapshot
skill.unlocked
inventory.snapshot
cargo.snapshot
wallet.snapshot
hangar.snapshot
loadout.snapshot
stats.updated
crafting.job_started
crafting.job_completed
crafting.job_failed
crafting.job_cancelled
```

## UI Surfaces

Mockup areas covered:
- left navigation: Inv, Hangar
- ship status panel
- topbar energy/cargo/capacity values
- topbar wallet/credits value
- inventory drawer/panel
- loadout panel
- skill/progression panel
- crafting panel
- stat comparison states

## TODO

- [x] Add query handler for progression snapshots.
- [ ] Add `progression.unlock_skill` authenticated command.
- [x] Add inventory/cargo snapshot handlers.
- [ ] Add `inventory.move` authenticated command.
- [x] Add hangar/loadout query handlers.
- [x] Add `hangar.activate_ship` handler with ownership/state validation.
- [x] Add equip/unequip runtime handlers using ledger-backed module movement.
- [ ] Add stat snapshot events after loadout, progression, and active ship
      changes.
- [x] Add stat snapshot query handler.
- [x] Add crafting recipe query handler.
- [x] Add `crafting.start` handler backed by `CraftingService.StartCraft`.
- [x] Add `crafting.complete` handler backed by `CraftingService.CompleteCraft`.
- [x] Add crafting cancel handler with reservation release, no-fee-refund
      policy, internal cancellation event, and server snapshot reconciliation.
- [x] Map `crafting.start` request ids to crafting domain idempotency
      references.
- [x] Add read-only client systems panel for inventory, hangar, loadout, and
      crafting recipe snapshots.
- [ ] Add skill tree/progression panel and skill unlock action.
- [ ] Add drag/click item movement with pending server state.
- [x] Add craft job timers from server timestamps.
- [x] Update topbar and ship panel from real snapshots.

## Abuse And Safety Checklist

- [x] Client cannot grant XP through exposed snapshot operations.
- [x] Client cannot set rank/skill points through exposed snapshot operations.
- [x] Client cannot create inventory items through exposed snapshot operations.
- [ ] Client cannot bypass cargo/storage capacity.
- [x] Client cannot equip unowned or invalid modules.
- [x] Client cannot activate unowned or disabled ships.
- [x] Client cannot fake stat totals through exposed snapshot operations.
- [x] Craft start checks recipe, wallet, materials, location, rank, and idempotency.
- [x] Craft completion is server-time/idempotency controlled.
- [x] Craft cancel releases only eligible reserved materials once and does not
      refund the craft fee in the MVP policy.
- [x] Wallet/credits display is snapshot-driven, not locally calculated.

## Tests

- [ ] Skill unlock consumes point once.
- [ ] Duplicate skill unlock does not double-spend points.
- [ ] Inventory move rejects unowned/negative/excess amounts.
- [ ] Duplicate inventory move cannot duplicate stacks.
- [x] Hangar activate rejects unowned/unusable ship.
- [x] Hangar activate emits `stats.updated`.
- [ ] Loadout equip rejects unowned item.
- [ ] Duplicate equip/unequip does not duplicate modules.
- [ ] Loadout equip updates stat snapshot.
- [x] Craft start reserves materials and debits wallet once.
- [x] Craft complete grants output once.
- [x] Craft cancel releases reservation once, emits cancellation event once,
      does not refund the craft fee, rejects wrong-owner/completed jobs, and
      reconciles browser state.
- [x] Server snapshot queries use authenticated session state and reject
      client-authored progression truth.
- [x] Client reducer reconciles inventory, hangar, loadout, crafting, stats,
      wallet, cargo, and progression snapshots.
- [x] Browser inventory panel uses server snapshot.
- [x] Browser topbar credits uses server wallet snapshot.
- [x] Browser equip action updates loadout/stats from server event.
- [x] Browser crafting timer survives reconnect snapshot.

## Done Criteria

- Player can inspect progression, inventory, hangar, loadout, stats, and craft
  through real UI.
- Exposed item/currency mutations go through services and ledgers; unexposed
  mutation contracts remain locked/read-only and are tracked in `docs/todo.md`.
- No fake counts remain in these panels.
- Tests and browser smoke pass.
