# Phase 06: Death, Repair, And Crafting

## Status

- State: Complete, verified 2026-06-18
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
- [x] Emit player death, ship disabled, and cargo dropped events. Verified 2026-06-18 by `DeathService.ProcessDeath` event payload and duplicate retry tests.
- [x] Block cargo transfer while in lethal/dead transaction. Verified 2026-06-18 by `DeathService` cargo transfer guard integration tests, in-flight transfer serialization tests, and economy guard duplicate-retry tests.
- [x] Add module durability loss hook.
- [x] Invalidate stats when module breaks. Verified 2026-06-18 by `DeathService` carrying module-break stat invalidation signals from its durability hook through cached duplicate results, plus `LoadoutService.BreakEquippedModule` and `StatService` broken-module exclusion tests.

## TODO: Respawn And Repair

- [x] Define respawn location priority. Verified 2026-06-18 by `DefaultRespawnPriority` and `SelectRespawnLocation` tests covering checkpoint, owned-planet, safe-station, and origin fallback order.
- [x] Implement MVP respawn to last checkpoint or origin station. Verified 2026-06-18 by `RespawnService.SelectLocation` tests selecting the configured origin fallback when no checkpoint exists and checkpoint when it is present.
- [x] Keep player alive but ship disabled after death. Verified 2026-06-18 by `DeathService.ProcessDeath` preserving player death/respawn identity while disabling the active ship, and by `runtime.CombatCommandHandler` rejecting combat for disabled active ships without spending energy or starting cooldowns.
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
- [x] Add craft location authorization hook before material reservation, wallet debit, and job creation. Verified 2026-06-17 by `CraftingService.StartCraft` authorizer rejection test.
- [x] Reject already-owned non-repeatable ship unlock crafts before material reservation, wallet debit, and job creation. Verified 2026-06-17 by `CraftingService.StartCraft` owned-output test.
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
- [x] Serialize concurrent completion retries for the same job and return later callers the cached canonical duplicate result. Verified 2026-06-17 by concurrent item-output and ship-unlock completion tests.

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
- [x] Dead/disabled ship cannot fight. Verified 2026-06-18 by `TestCombatUseSkillRejectsDisabledActiveShipBeforeMutation` and `TestWithActiveShipCombatLeaseSerializesDeathDisable`.
- [x] Starter fallback works when all ships disabled.
- [x] Repair charges correct wallet amount. Verified 2026-06-17 by successful repair debit ledger test.
- [x] Repair restores ship.
- [x] Repair restore failure compensates wallet charge. Verified 2026-06-17 by restore-failure compensation test.
- [x] Missing material fails craft start.
- [x] Missing credits fails craft start.
- [x] Rank too low fails craft start.
- [x] Wrong location fails craft start.
- [x] Location authorizer rejection fails craft start before reservation, wallet debit, or job creation. Verified 2026-06-17 by `CraftingService.StartCraft` authorizer rejection test.
- [x] Already-owned non-repeatable ship unlock craft fails before reservation, wallet debit, or job creation. Verified 2026-06-17 by `CraftingService.StartCraft` owned-output test.
- [x] Start craft reserves or consumes materials.
- [x] Missing craft start reference is rejected. Verified 2026-06-17 by `CraftingService.StartCraft` idempotency hardening tests.
- [x] Duplicate craft start with the same player/reference/recipe/location returns the original job without another reservation or wallet debit. Verified 2026-06-17 by `CraftingService.StartCraft` idempotency hardening tests.
- [x] Duplicate craft start with the same player/reference but different recipe or location rejects before economy mutation. Verified 2026-06-17 by `CraftingService.StartCraft` idempotency hardening tests.
- [x] Craft start references are scoped by player. Verified 2026-06-17 by `CraftingService.StartCraft` idempotency hardening tests.
- [x] Complete before time fails.
- [x] Complete after time creates output once.
- [x] Duplicate complete does not duplicate output.
- [x] Concurrent complete retries create one item output, one ship unlock, and one craft XP grant while returning duplicate callers the canonical result. Verified 2026-06-17 by `CraftingService.CompleteCraft` concurrent retry tests.
- [x] Ship unlock recipe is idempotent.
- [x] Craft XP granted once.

## Abuse And Safety Checks

- [x] Death duplication blocked.
- [x] Cargo hiding during death blocked. Verified 2026-06-18 by player-facing ship cargo move/add guard tests while death processing owns cargo state.
- [x] Repair cost is server-calculated. Verified 2026-06-17 by `RepairService` catalog quote tests.
- [x] Client cannot avoid module durability loss after death. Verified 2026-06-18 by `DeathService` reading equipped module item ids from a server-owned provider and caching them per lethal attempt before calling the durability hook.
- [x] Material duplication blocked by reservation state.
- [x] Craft start retry duplication blocked by player-scoped idempotency reference. Verified 2026-06-17 by `CraftingService.StartCraft` duplicate-reference tests.
- [x] Early craft completion blocked by server time.
- [x] Unknown recipe and wrong MVP station location type blocked by server catalog validation.
- [x] Craft completion retry races are serialized per job in the in-memory Phase 06 service. Verified 2026-06-17 by concurrent completion tests.
- [x] Planet/building craft location ownership validation blocks fake locations. Verified 2026-06-18 by `CraftingService.StartCraft` fail-closed nil-authorizer tests and `production.CraftLocationAuthorizer` tests for unknown, unowned, other-owned, missing-production, missing-building, inactive-building, and active-building cases.
- [x] Low-tier craft XP spam has at least a tracking hook for later balancing. Verified 2026-06-18 by `TestCompleteCraftTracksLowTierCraftXPOnceForBalancing`.

## Done Criteria

- [x] Death and repair loop works in tests. Verified 2026-06-18 by `go test ./internal/game/death -count=1`.
- [x] Crafting produces first module and ship unlock.
- [x] Crafting consumes materials and credits safely. Verified 2026-06-18 by `go test ./internal/game/crafting -count=1`, including material reservation, wallet debit, duplicate start, duplicate completion, and XP tracking tests.
- [x] Disabled ship and starter fallback rules are enforced.
- [x] `go test ./...` passes. Verified 2026-06-18 after the craft XP tracking slice.
- [x] `git diff --check` passes. Verified 2026-06-18 after the craft XP tracking slice.

## Resume Notes

If resuming here, inspect whether craft jobs remember recipe version and whether duplicate completion is tested. Those two are common future economy bugs.

Verified slices:

- `DeathService.ProcessDeath` emits `player.died`, `ship.disabled`, and `death.cargo_dropped` after successful death processing. Duplicate lethal-event retries return the cached result without re-emitting death events.
- `DeathService` now exposes a process-local cargo transfer guard for Phase 06: player-facing ship cargo adds/moves acquire short transfer leases, death processing waits for already-active leases before touching cargo, new player-facing cargo transfers fail while death processing is in flight, trusted system inventory moves continue for death-owned flows, and duplicate economy retries return cached results before consulting the guard.
- Module durability handling now gets equipped module item ids from server-owned loadout state through `EquippedModuleProvider`, not from death command input. The selected ids are cached in the lethal-event attempt so retries do not re-read a later loadout.
- Planet and planet-building craft recipes now fail closed without a craft
  location authorizer. The production-backed authorizer validates discovery
  planet ownership, production storage initialization, and active building state
  before any crafting economy mutation.
- Craft completion now emits optional `CraftXPObservation` telemetry after a
  successful non-duplicate XP grant. The current low-tier bucket is rank 1 or
  below with craft duration at or below 30 minutes; actual XP reduction remains
  tracked as a later balancing follow-up.
