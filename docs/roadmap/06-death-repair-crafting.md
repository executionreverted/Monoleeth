# Phase 06: Death, Repair, And Crafting

## Status

- State: In progress
- Owner: Risk loop and item production
- Depends on: Phase 02, Phase 03, Phase 05
- Unlocks: module economy, ship unlock recipes, quest craft objectives, market supply

## Goal

Add meaningful risk and recovery through death, cargo drop, disabled ships, repair, starter fallback, and basic crafting from raw materials into modules and ship unlocks.

## Source Specs

Read before implementation:

- `docs/plans/modules/07-death-repair-respawn.md`
- `docs/plans/modules/08-crafting-recipes-materials.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/03-ship-hangar-loadout.md`
- `docs/plans/modules/04-module-stat-aggregation.md`
- `docs/2026-06-17-progression-economy-systems-design.md`

## Module Ownership

Owns:

- `DeathService`
- `RepairService`
- `RespawnService`
- `CraftingService`
- `RecipeService`
- `MaterialService`

Does not own:

- combat lethal damage calculation
- loot pickup
- cargo transfer primitive
- ship unlock internals
- wallet primitive
- inventory primitive

## MVP Scope

Death:

- active ship disabled
- cargo material drop
- simple checkpoint/origin respawn
- credit repair
- starter fallback
- module durability hooks prepared

Crafting:

- station/account inventory craft
- planet storage craft later-compatible
- no cancellation
- recipes for processed materials, one module, one ship unlock
- craft XP once

## TODO: Death

- [x] Define death record model.
- [x] Define lethal event idempotency key.
- [ ] Implement death state lock.
- [ ] Ensure death processes once.
- [x] Calculate zone-based cargo drop percent.
- [x] Select cargo drops from eligible cargo.
- [x] Preserve non-droppable and soulbound items.
- [ ] Remove dropped cargo through cargo service.
- [ ] Create world drops through loot service.
- [x] Mark active ship disabled.
- [ ] Record respawn location.
- [ ] Emit player death, ship disabled, and cargo dropped events.
- [ ] Block cargo transfer while in lethal/dead transaction.
- [ ] Add module durability loss hook.
- [ ] Invalidate stats when module breaks.

## TODO: Respawn And Repair

- [ ] Define respawn location priority.
- [ ] Implement MVP respawn to last checkpoint or origin station.
- [ ] Keep player alive but ship disabled after death.
- [ ] Implement repair cost formula.
- [ ] Implement repair command.
- [ ] Debit wallet through wallet service.
- [x] Restore disabled ship to available.
- [x] Prevent repair of non-disabled ship.
- [x] Ensure starter ship fallback when no usable ship exists.
- [x] Add login safety check for starter ship.

## TODO: Crafting

- [ ] Define recipe catalog.
- [ ] Define craft job model.
- [ ] Define recipe input requirements.
- [ ] Define recipe location requirements.
- [ ] Implement `StartCraft`.
- [ ] Validate rank requirement.
- [ ] Validate role level requirement.
- [ ] Validate location requirement.
- [ ] Reserve or consume materials using inventory service.
- [ ] Debit craft fee using wallet service.
- [ ] Create running craft job with server `completes_at`.
- [ ] Implement `CompleteCraft`.
- [ ] Reject early completion.
- [ ] Commit reservation or consume inputs.
- [ ] Create item output through inventory service.
- [ ] Grant ship unlock output through ship service.
- [ ] Mark job completed once.
- [ ] Grant craft XP once.
- [ ] Store recipe version on job.

## Tests

- [ ] Death processed once for duplicate lethal event.
- [x] Cargo drop percent is inside zone range.
- [x] Non-droppable item stays in cargo.
- [ ] Dropped items become world drops.
- [x] Active ship becomes disabled.
- [x] Dead/disabled ship cannot fight.
- [x] Starter fallback works when all ships disabled.
- [ ] Repair charges correct wallet amount.
- [x] Repair restores ship.
- [ ] Repair rollback prevents partial charge/state change.
- [ ] Missing material fails craft start.
- [ ] Missing credits fails craft start.
- [ ] Rank too low fails craft start.
- [ ] Wrong location fails craft start.
- [ ] Start craft reserves or consumes materials.
- [ ] Complete before time fails.
- [ ] Complete after time creates output once.
- [ ] Duplicate complete does not duplicate output.
- [ ] Ship unlock recipe is idempotent.
- [ ] Craft XP granted once.

## Abuse And Safety Checks

- [ ] Death duplication blocked.
- [ ] Cargo hiding during death blocked.
- [ ] Repair cost is server-calculated.
- [ ] Client cannot avoid module durability loss after death.
- [ ] Material duplication blocked by reservation state.
- [ ] Early craft completion blocked by server time.
- [ ] Hidden recipe or fake location blocked by server catalog validation.
- [ ] Low-tier craft XP spam has at least a tracking hook for later balancing.

## Done Criteria

- [ ] Death and repair loop works in tests.
- [ ] Crafting produces first module and ship unlock.
- [ ] Crafting consumes materials and credits safely.
- [x] Disabled ship and starter fallback rules are enforced.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, inspect whether craft jobs remember recipe version and whether duplicate completion is tested. Those two are common future economy bugs.
