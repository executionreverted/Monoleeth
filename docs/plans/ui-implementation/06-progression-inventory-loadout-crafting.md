# Phase 06: Progression, Inventory, Loadout, And Crafting UI

## Status

- State: Planned
- Owner: Player growth and item management UI
- Depends on: Phase 05
- Unlocks: persistent character loop and equipment/craft loop

## Goal

Expose progression, rank, role XP, pilot skills, inventory, cargo, ship hangar,
module loadout, stat aggregation, and crafting through real server-backed UI.

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
```

## UI Surfaces

Mockup areas covered:
- left navigation: Inv, Hangar
- ship status panel
- topbar energy/cargo/capacity values
- inventory drawer/panel
- loadout panel
- skill/progression panel
- crafting panel
- stat comparison states

## TODO

- [ ] Add query/command handlers for progression snapshots and skill unlocks.
- [ ] Add inventory/cargo snapshot handlers.
- [ ] Add `inventory.move` authenticated command.
- [ ] Add hangar/loadout query handlers.
- [ ] Add equip/unequip runtime handlers using ledger-backed module movement.
- [ ] Add stat snapshot events after loadout/progression changes.
- [ ] Add crafting query/start/complete handlers.
- [ ] Map request ids to crafting domain idempotency references.
- [ ] Add client panels for inventory, hangar, loadout, skills, crafting.
- [ ] Add drag/click item movement with pending server state.
- [ ] Add craft job timers from server timestamps.
- [ ] Update topbar and ship panel from real snapshots.

## Abuse And Safety Checklist

- [ ] Client cannot grant XP.
- [ ] Client cannot set rank/skill points.
- [ ] Client cannot create inventory items.
- [ ] Client cannot bypass cargo/storage capacity.
- [ ] Client cannot equip unowned or invalid modules.
- [ ] Client cannot fake stat totals.
- [ ] Craft start checks recipe, wallet, materials, location, rank, and idempotency.
- [ ] Craft completion is server-time/idempotency controlled.

## Tests

- [ ] Skill unlock consumes point once.
- [ ] Inventory move rejects unowned/negative/excess amounts.
- [ ] Loadout equip rejects unowned item.
- [ ] Loadout equip updates stat snapshot.
- [ ] Craft start reserves materials and debits wallet once.
- [ ] Craft complete grants output once.
- [ ] Browser inventory panel uses server snapshot.
- [ ] Browser equip action updates loadout/stats from server event.
- [ ] Browser crafting timer survives reconnect snapshot.

## Done Criteria

- Player can inspect progression, inventory, hangar, loadout, stats, and craft
  through real UI.
- Item/currency mutations go through services and ledgers.
- No fake counts remain in these panels.
- Tests and browser smoke pass.
