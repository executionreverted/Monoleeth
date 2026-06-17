# Phase 05: Combat And Loot Vertical Slice

## Status

- State: In progress
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

- [x] Define combat actor state.
- [x] Define target state.
- [x] Define cooldown state.
- [x] Define energy state.
- [x] Define NPC combat state.
- [x] Implement `ValidateTarget`.
- [x] Require attacker alive.
- [x] Require target alive.
- [x] Require same world/zone.
- [x] Require visibility via `VisibilityService`.
- [x] Require range from server position and effective stats.
- [x] Require cooldown ready using server time.
- [x] Require enough energy.
- [x] Implement energy spend.
- [x] Implement energy regen on server tick.
- [x] Implement cooldown start.
- [x] Implement hit roll with server RNG.
- [x] Implement shield/HP damage formula v0.
- [x] Implement death-once state transition for NPCs.
- [x] Track damage contribution.
- [x] Emit combat events.

## TODO: Loot

- [x] Define world loot drop model.
- [x] Define loot table v0.
- [x] Implement server-only loot roll.
- [x] Create drops on `combat.npc_killed`.
- [x] Set owner lock and expiration timestamps.
- [x] Schedule owner lock expiration.
- [x] Schedule drop despawn.
- [x] Serialize visible drops only.
- [x] Implement pickup command.
- [x] Validate pickup range.
- [x] Validate pickup visibility.
- [x] Validate owner lock.
- [x] Add item to cargo through `CargoService`.
- [x] Delete or mark drop claimed after successful pickup.
- [x] Emit loot picked event.
- [x] Grant loot XP only for server-generated eligible drops.

## TODO: Vertical Slice Harness

- [x] Add a deterministic test scenario with one player and one NPC.
- [x] Spawn starter ship with laser stats.
- [x] Move player into range.
- [x] Attack until NPC dies.
- [x] Create loot.
- [x] Pick loot into cargo.
- [x] Grant combat XP once.
- [x] Produce a player snapshot after mutation.

## Tests

- [x] Attack hidden target fails.
- [x] Attack out of range fails.
- [x] Cooldown prevents double attack.
- [x] Energy shortage prevents attack.
- [x] Energy exactly equal to cost is allowed.
- [ ] Client timestamp is ignored.
- [x] Shield overflow applies HP damage.
- [x] Simultaneous lethal damage processes NPC death once.
- [x] Highest valid contributor receives loot lock.
- [x] Duplicate NPC death does not duplicate drops.
- [x] Owner can pick up during lock.
- [x] Non-owner cannot pick up during lock.
- [x] Anyone can pick up after lock.
- [x] Expired drop cannot be picked up.
- [x] Far pickup fails.
- [x] Hidden pickup fails.
- [x] Concurrent pickup only one succeeds.
- [x] Cargo full blocks pickup and drop remains.
- [x] Player-death source gives no loot XP.

## Abuse And Safety Checks

- [x] Range spoofing blocked by server position.
- [x] Hidden target attack blocked by visibility check at attack time.
- [x] Cooldown skipping blocked by server cooldown map.
- [x] Energy desync resolves through authoritative snapshot.
- [x] Loot table spoof blocked because client never sends loot contents.
- [x] Vacuum loot blocked by range and visibility.
- [x] Duplicate pickup blocked by drop lock/state.

## Done Criteria

- [x] First server-only combat loop works in tests.
- [x] Loot pickup uses cargo and ledger primitives.
- [x] XP from combat and loot is idempotent.
- [x] Hidden or far interactions fail safely.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

## Resume Notes

If resuming here, run the deterministic vertical slice test first. If it fails, fix that before adding new combat features.

Verified slices:

- Basic laser combat is implemented in `internal/game/combat` with server-owned actor state, cooldowns, energy, visibility/range validation, hit roll, shield-first damage, NPC death-once state, damage contribution tracking, and combat events.
- `stats.CombatStats` now includes `WeaponEnergyCost`, so energy cost comes from server-calculated stat snapshots instead of client payloads.
- Loot drops are implemented in `internal/game/loot` with server-only roll tables, owner lock/public/expired windows, visible-only payload filtering, cargo-backed pickup, claim-once behavior, loot events, and loot XP grants for eligible server-generated drops.
- Loot owner-lock expiry and despawn now produce explicit scheduled drop tasks that the world worker delayed scheduler can drain and map back into `LootService`.
- Player-death drops can be created from server-calculated item stacks and are explicitly not eligible for loot XP.
- A deterministic backend vertical slice test ensures a starter ship, composes Laser Alpha stats through `StatService`, moves the player into range through the world worker, kills one NPC, grants combat XP idempotently, creates loot, picks it into ship cargo through `CargoService`, grants loot XP, and reads the final player progression snapshot.
- Final verification for this wave passed with `go test ./...`, `go test -race ./internal/game/combat ./internal/game/loot`, and `git diff --check`.

Remaining follow-up:

- Add a client-timestamp regression around combat intents once a concrete gateway command exists.
- Replace the vertical-slice test-local stat input adapter with real Phase 03 runtime provider wiring before exposing gateway combat/loot commands.
- Add realtime gateway commands after authenticated session/player resolution is wired.
