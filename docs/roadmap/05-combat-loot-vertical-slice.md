# Phase 05: Combat And Loot Vertical Slice

## Status

- State: Not started
- Owner: Realtime gameplay loop
- Depends on: Phase 02, Phase 03, Phase 04
- Unlocks: death, crafting, quest kill progress, first playable loop

## Goal

Build the first playable loop: move, see an NPC, attack with a basic laser, spend energy, obey cooldown, apply shield/HP damage, kill NPC, create owner-locked loot, pick loot into cargo, and grant XP once.

## Source Specs

Read before implementation:

- `docs/plans/modules/05-combat-damage-targeting.md`
- `docs/plans/modules/06-loot-drop-ownership.md`
- `docs/plans/modules/01-player-progression-rank-role-skills.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`

## Module Ownership

Owns:

- `CombatService`
- `TargetingService`
- `DamageService`
- `EnergyService`
- `CooldownService`
- `AggroService`
- `LootService`
- `DropOwnershipService`

Does not own:

- inventory primitives
- XP table definitions
- ship/module equip
- AOI/fog truth
- death cargo drop

## MVP Scope

Combat:

- targeted basic laser
- one manual rocket skill skeleton
- energy cost
- cooldown
- shield and HP
- NPC death
- highest damage contributor gets loot owner lock

Loot:

- raw material drop table
- 60 second owner lock
- 120 second public window
- 180 second total lifetime
- all-or-nothing pickup
- cargo capacity check

## TODO: Combat Core

- [ ] Define combat actor state.
- [ ] Define target state.
- [ ] Define cooldown state.
- [ ] Define energy state.
- [ ] Define NPC combat state.
- [ ] Implement `ValidateTarget`.
- [ ] Require attacker alive.
- [ ] Require target alive.
- [ ] Require same world/zone.
- [ ] Require visibility via `VisibilityService`.
- [ ] Require range from server position and effective stats.
- [ ] Require cooldown ready using server time.
- [ ] Require enough energy.
- [ ] Implement energy spend.
- [ ] Implement energy regen on server tick.
- [ ] Implement cooldown start.
- [ ] Implement hit roll with server RNG.
- [ ] Implement shield/HP damage formula v0.
- [ ] Implement death-once state transition for NPCs.
- [ ] Track damage contribution.
- [ ] Emit combat events.

## TODO: Loot

- [ ] Define world loot drop model.
- [ ] Define loot table v0.
- [ ] Implement server-only loot roll.
- [ ] Create drops on `combat.npc_killed`.
- [ ] Set owner lock and expiration timestamps.
- [ ] Schedule owner lock expiration.
- [ ] Schedule drop despawn.
- [ ] Serialize visible drops only.
- [ ] Implement pickup command.
- [ ] Validate pickup range.
- [ ] Validate pickup visibility.
- [ ] Validate owner lock.
- [ ] Add item to cargo through `CargoService`.
- [ ] Delete or mark drop claimed after successful pickup.
- [ ] Emit loot picked event.
- [ ] Grant loot XP only for server-generated eligible drops.

## TODO: Vertical Slice Harness

- [ ] Add a deterministic test scenario with one player and one NPC.
- [ ] Spawn starter ship with laser stats.
- [ ] Move player into range.
- [ ] Attack until NPC dies.
- [ ] Create loot.
- [ ] Pick loot into cargo.
- [ ] Grant combat XP once.
- [ ] Produce a player snapshot after mutation.

## Tests

- [ ] Attack hidden target fails.
- [ ] Attack out of range fails.
- [ ] Cooldown prevents double attack.
- [ ] Energy shortage prevents attack.
- [ ] Energy exactly equal to cost is allowed.
- [ ] Client timestamp is ignored.
- [ ] Shield overflow applies HP damage.
- [ ] Simultaneous lethal damage processes NPC death once.
- [ ] Highest valid contributor receives loot lock.
- [ ] Duplicate NPC death does not duplicate drops.
- [ ] Owner can pick up during lock.
- [ ] Non-owner cannot pick up during lock.
- [ ] Anyone can pick up after lock.
- [ ] Expired drop cannot be picked up.
- [ ] Far pickup fails.
- [ ] Hidden pickup fails.
- [ ] Concurrent pickup only one succeeds.
- [ ] Cargo full blocks pickup and drop remains.
- [ ] Player-death source gives no loot XP.

## Abuse And Safety Checks

- [ ] Range spoofing blocked by server position.
- [ ] Hidden target attack blocked by visibility check at attack time.
- [ ] Cooldown skipping blocked by server cooldown map.
- [ ] Energy desync resolves through authoritative snapshot.
- [ ] Loot table spoof blocked because client never sends loot contents.
- [ ] Vacuum loot blocked by range and visibility.
- [ ] Duplicate pickup blocked by drop lock/state.

## Done Criteria

- [ ] First server-only combat loop works in tests.
- [ ] Loot pickup uses cargo and ledger primitives.
- [ ] XP from combat and loot is idempotent.
- [ ] Hidden or far interactions fail safely.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, run the deterministic vertical slice test first. If it fails, fix that before adding new combat features.
