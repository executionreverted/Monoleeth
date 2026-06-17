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
- [x] Implement death state lock.
- [x] Ensure death processes once.
- [x] Calculate zone-based cargo drop percent.
- [x] Select cargo drops from eligible cargo.
- [x] Preserve non-droppable and soulbound items.
- [x] Remove dropped cargo through inventory service.
- [x] Create world drops through loot service.
- [x] Mark active ship disabled.
- [x] Record respawn location.
- [ ] Emit player death, ship disabled, and cargo dropped events.
- [ ] Block cargo transfer while in lethal/dead transaction.
- [x] Add module durability loss hook.
- [ ] Invalidate stats when module breaks.

## TODO: Respawn And Repair

- [ ] Define respawn location priority.
- [ ] Implement MVP respawn to last checkpoint or origin station.
- [ ] Keep player alive but ship disabled after death.
- [x] Implement repair cost formula. Verified 2026-06-17 by `RepairService` tests using catalog credit value and repair multiplier.
- [x] Implement repair command. Verified 2026-06-17 by `RepairService.RepairShip` orchestration tests.
- [x] Debit wallet through wallet service. Verified 2026-06-17 by credit repair wallet ledger tests.
- [x] Restore disabled ship to available.
- [x] Prevent repair of non-disabled ship.
- [x] Ensure starter ship fallback when no usable ship exists.
- [x] Add login safety check for starter ship.

## TODO: Crafting

- [x] Define recipe catalog.
- [x] Define craft job model.
- [x] Define recipe input requirements.
- [x] Define recipe location requirements.
- [x] Implement `StartCraft`.
- [x] Validate rank requirement.
- [x] Validate role level requirement.
- [x] Validate location requirement.
- [x] Reserve or consume materials using inventory service.
- [x] Debit craft fee using wallet service.
- [x] Create running craft job with server `completes_at`.
- [x] Implement `CompleteCraft`.
- [x] Reject early completion.
- [x] Commit reservation or consume inputs.
- [x] Create item output through inventory service.
- [x] Grant ship unlock output through ship service.
- [x] Mark job completed once.
- [x] Grant craft XP once.
- [x] Store recipe version on job.

## Tests

- [x] Death processed once for duplicate lethal event.
- [x] Death zone/policy mismatch is rejected before mutation. Verified 2026-06-17 by `DeathService.ProcessDeath` hardening tests.
- [x] Ship disable failure leaves cargo and loot untouched. Verified 2026-06-17 by `DeathService.ProcessDeath` hardening tests.
- [x] New lethal event for an already-disabled active ship does not create another cargo drop/death cycle. Verified 2026-06-17 by `DeathService.ProcessDeath` hardening tests.
- [x] Cargo drop percent is inside zone range.
- [x] Non-droppable item stays in cargo.
- [x] Dropped items become world drops.
- [x] Active ship becomes disabled.
- [x] Death cargo removal ledger uses `death_cargo_drop:*` references instead of `loot_pickup:*`. Verified 2026-06-17 by `DeathService.ProcessDeath` hardening tests.
- [x] Death cargo rows from another owner, another ship cargo location, or non-ship-cargo location are rejected before inventory mutation. Verified 2026-06-17 by `DeathService.ProcessDeath` hardening tests.
- [ ] Dead/disabled ship cannot fight.
- [x] Starter fallback works when all ships disabled.
- [x] Repair charges correct wallet amount. Verified 2026-06-17 by successful repair debit ledger test.
- [x] Repair restores ship.
- [x] Repair restore failure compensates wallet charge. Verified 2026-06-17 by restore-failure compensation test.
- [x] Missing material fails craft start.
- [x] Missing credits fails craft start.
- [x] Rank too low fails craft start.
- [x] Wrong location fails craft start.
- [x] Start craft reserves or consumes materials.
- [x] Complete before time fails.
- [x] Complete after time creates output once.
- [x] Duplicate complete does not duplicate output.
- [x] Ship unlock recipe is idempotent.
- [x] Craft XP granted once.

## Abuse And Safety Checks

- [x] Death duplication blocked.
- [ ] Cargo hiding during death blocked.
- [x] Repair cost is server-calculated. Verified 2026-06-17 by `RepairService` catalog quote tests.
- [ ] Client cannot avoid module durability loss after death.
- [x] Material duplication blocked by reservation state.
- [x] Early craft completion blocked by server time.
- [x] Unknown recipe and wrong MVP station location type blocked by server catalog validation.
- [ ] Planet/building craft location ownership validation blocks fake locations.
- [ ] Low-tier craft XP spam has at least a tracking hook for later balancing.

## Done Criteria

- [ ] Death and repair loop works in tests.
- [x] Crafting produces first module and ship unlock.
- [ ] Crafting consumes materials and credits safely.
- [x] Disabled ship and starter fallback rules are enforced.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, inspect whether craft jobs remember recipe version and whether duplicate completion is tested. Those two are common future economy bugs.
