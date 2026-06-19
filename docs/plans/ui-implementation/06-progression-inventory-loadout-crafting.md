# Phase 06: Progression, Inventory, Loadout, And Crafting UI

## Status

- State: In progress
- Owner: Player growth and item management UI
- Depends on: Phase 05
- Unlocks: persistent character loop and equipment/craft loop

Current slice completed:
- Authenticated read-only snapshots are exposed for progression, inventory,
  cargo, hangar, loadout, stats, and crafting recipes.
- The browser client requests these snapshots after the authenticated world
  bootstrap and renders them in the real HUD without demo values.
- Loot pickup now reconciles a real inventory snapshot alongside cargo.
- Mutation commands remain future work and must still use ledger/service-backed
  flows before any UI action is enabled.

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
| `crafting.cancel` | `craft_cancel:<job_id>` unique reference | job owner/state, cancellation window, refund/release policy |

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
- [ ] Add `hangar.activate_ship` handler with ownership/state validation.
- [ ] Add equip/unequip runtime handlers using ledger-backed module movement.
- [ ] Add stat snapshot events after loadout, progression, and active ship
      changes.
- [x] Add stat snapshot query handler.
- [x] Add crafting recipe query handler.
- [ ] Add crafting start/complete handlers.
- [ ] Add crafting cancel handler, refund/release behavior, and
      `crafting.job_cancelled` event.
- [ ] Map request ids to crafting domain idempotency references.
- [x] Add read-only client systems panel for inventory, hangar, loadout, and
      crafting recipe snapshots.
- [ ] Add skill tree/progression panel and skill unlock action.
- [ ] Add drag/click item movement with pending server state.
- [ ] Add craft job timers from server timestamps.
- [x] Update topbar and ship panel from real snapshots.

## Abuse And Safety Checklist

- [x] Client cannot grant XP through exposed snapshot operations.
- [x] Client cannot set rank/skill points through exposed snapshot operations.
- [x] Client cannot create inventory items through exposed snapshot operations.
- [ ] Client cannot bypass cargo/storage capacity.
- [ ] Client cannot equip unowned or invalid modules.
- [ ] Client cannot activate unowned or disabled ships.
- [x] Client cannot fake stat totals through exposed snapshot operations.
- [ ] Craft start checks recipe, wallet, materials, location, rank, and idempotency.
- [ ] Craft completion is server-time/idempotency controlled.
- [ ] Craft cancel releases only eligible reserved materials/wallet amounts once.
- [x] Wallet/credits display is snapshot-driven, not locally calculated.

## Tests

- [ ] Skill unlock consumes point once.
- [ ] Duplicate skill unlock does not double-spend points.
- [ ] Inventory move rejects unowned/negative/excess amounts.
- [ ] Duplicate inventory move cannot duplicate stacks.
- [ ] Hangar activate rejects unowned/unusable ship.
- [ ] Hangar activate emits `stats.updated`.
- [ ] Loadout equip rejects unowned item.
- [ ] Duplicate equip/unequip does not duplicate modules.
- [ ] Loadout equip updates stat snapshot.
- [ ] Craft start reserves materials and debits wallet once.
- [ ] Craft complete grants output once.
- [ ] Craft cancel releases reservation/refund once and emits cancellation event.
- [x] Server snapshot queries use authenticated session state and reject
      client-authored progression truth.
- [x] Client reducer reconciles inventory, hangar, loadout, crafting, stats,
      wallet, cargo, and progression snapshots.
- [x] Browser inventory panel uses server snapshot.
- [x] Browser topbar credits uses server wallet snapshot.
- [ ] Browser equip action updates loadout/stats from server event.
- [ ] Browser crafting timer survives reconnect snapshot.

## Done Criteria

- Player can inspect progression, inventory, hangar, loadout, stats, and craft
  through real UI.
- Item/currency mutations go through services and ledgers.
- No fake counts remain in these panels.
- Tests and browser smoke pass.
