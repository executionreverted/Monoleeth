# DarkOrbit Ammo And Weapon Combat Plan

Date: 2026-06-28

## Finding

The ammunition data is not missing from Kalaazu. It is missing from our combat
runtime semantics.

Kalaazu already gives us the important split:

- laser cannons are equipment, e.g. `equipment_weapon_laser_lf-1`,
  `equipment_weapon_laser_lf-2`, `equipment_weapon_laser_lf-3`, and
  `equipment_weapon_laser_lf-4`
- laser ammunition is stackable ammo, e.g. `ammunition_laser_lcb-10`,
  `ammunition_laser_mcb-25`, `ammunition_laser_mcb-50`,
  `ammunition_laser_ucb-100`, `ammunition_laser_sab-50`, and
  `ammunition_laser_rsb-75`
- rockets and rocket-launcher ammunition are separate stackable ammo families,
  e.g. `ammunition_rocket_plt-2021` and
  `ammunition_rocketlauncher_hstrm-01`

Our content importer currently turns these rows into item definitions, and maps
laser equipment into offensive modules. The missing piece is a typed combat-ammo
catalog plus server-owned active ammo selection, inventory consumption, damage
modifiers, and UI quickbar behavior.

## External Reference Shape

DarkOrbit-style combat separates base weapon damage from ammo multiplier:

- LF-style laser cannons contribute base laser damage.
- Battery/ammo selection multiplies or transforms that damage.
- A laser shot consumes battery rounds.
- Rocket shots consume rocket ammo and run on their own cooldown/accuracy/damage
  lane.
- Rocket launcher ammo is separate from normal rockets and depends on an
  equipped launcher capacity.

Useful reference pages:

- DarkOrbitWiki lasers and ammunition:
  https://darkorbitwiki.com/equipment/lasers-and-ammunition/
- DarkOrbitWiki rockets:
  https://darkorbitwiki.com/equipment/rockets/
- DarkOrbitWiki rocket launchers and ammunition:
  https://darkorbitwiki.com/equipment/rocket-launchers-and-ammunition/
- DarkOrbit board laser ammo reference:
  https://board-en.darkorbit.com/threads/ammo-laser-ammo.785/
- Kalaazu item seed:
  https://github.com/manulaiko/Kalaazu/blob/develop/Persistence/database/items/dump.sql

Reference values to project into our data:

| Family | Item | Combat Meaning |
| --- | --- | --- |
| Laser cannon | LF-1 | base laser damage source |
| Laser cannon | LF-2 | stronger base laser damage source |
| Laser cannon | LF-3 | stronger base laser damage source |
| Laser cannon | LF-4 | stronger base laser damage source |
| Laser ammo | LCB-10 | x1 laser damage |
| Laser ammo | MCB-25 | x2 laser damage |
| Laser ammo | MCB-50 | x3 laser damage |
| Laser ammo | UCB-100 | x4 laser damage |
| Laser ammo | SAB-50 | shield-leech lane, x2 shield effect |
| Laser ammo | RSB-75 | burst laser damage lane, slower fire/cooldown |
| Laser ammo | CBO-100 | MCB-50-style damage plus partial SAB-style shield leech |
| Rocket | R-310 | low-tier direct rocket damage |
| Rocket | PLT-2026 | mid direct rocket damage |
| Rocket | PLT-2021 | high direct rocket damage |
| Rocket | PLT-3030 | higher direct rocket damage with accuracy tradeoff |
| Rocket launcher | HST-1 | launcher capacity 3 |
| Rocket launcher | HST-2 | launcher capacity 5 |
| Rocket launcher ammo | ECO-10 / HSTRM-01 / SAR / UBR / CBR | launcher-only salvo ammo |

## Current Code Gap

Current state:

- `internal/game/contentseed/kalaazu/testdata/items.sql` contains laser ammo,
  rocket ammo, rocket-launcher ammo, and LF laser equipment rows.
- `internal/game/contentseed/kalaazu/items.go` maps equipment type `16` into
  `modules.StatWeaponDamage`.
- `internal/game/combat/service.go` calculates laser damage as current weapon
  damage against laser resistance. There is no ammo multiplier/effect.
- `internal/game/combat/types.go` has no ammo fields on `BasicAttackInput` or
  `BasicAttackResult`.
- `internal/game/server/combat_engagement_handlers.go` accepts only
  `target_id` for `combat.start_attack`. There is no selected ammo, fallback
  ammo, or not-enough-ammo stop reason.

Important normalization issue:

Kalaazu LF-3 and LF-4 rows describe 150 and 200 damage in text, while the `bonus`
column is `127` for both in the checked seed copy. Because our module importer
currently uses `bonus` directly for weapon damage, LF-3/LF-4 can become wrong
unless we add a source-specific weapon damage normalization rule.

## Target Server Contract

Client sends intents only:

```text
combat.select_ammo { family, item_id }
quickbar.assign { slot_id, item_id }
combat.start_attack { target_id }
combat.fire_rocket { target_id }
```

The client must never send:

- damage
- multiplier
- hit result
- cooldown
- item quantity
- fallback decision

The server owns:

- active laser ammo per player
- active rocket ammo per player
- active rocket-launcher ammo per player
- quickbar assignment validation
- ammo fallback
- inventory consumption
- attack rejection or engagement stop when ammo is unavailable
- damage formula and special effects
- post-commit events/snapshots

## Laser Attack Rule

On every server-owned laser tick:

1. Resolve attacker, target, visibility, range, safe-zone policy, cooldown, and
   energy as today.
2. Resolve selected laser ammo from server-owned player combat settings.
3. If selected ammo has quantity, use it.
4. If selected ammo is empty, fallback to `LCB-10` only if the player has it.
5. If no selected ammo and no `LCB-10`, reject the attack or stop the engagement
   with `not_enough_ammo`.
6. Consume ammo server-side through inventory service/ledger.
7. Apply damage:

```text
final_laser_damage =
  equipped_laser_base_damage
  * ammo_multiplier_or_effect
  * server_modifiers
  * resistance_result
```

8. Emit safe combat events with public result fields plus client-safe ammo state:
   selected ammo id, remaining quantity, stop reason, cooldown, hit/miss, and
   damage numbers the viewer is allowed to see.

For the first implementation slice, keep special ammo conservative:

- LCB-10, MCB-25, MCB-50, UCB-100: pure multipliers.
- SAB-50, CBO-100, RSB-75: cataloged and selectable, but either disabled with a
  clear server reason or implemented as explicit follow-up slices.

## Rocket Rule

Normal rockets should not be smuggled into basic laser attacks.

First rocket slice:

- Add `combat.fire_rocket`.
- Resolve active rocket ammo.
- Validate target/range/visibility/cooldown.
- Consume one rocket ammo unit.
- Apply rocket damage/accuracy from the typed ammo catalog.
- Emit `combat.rocket_fired` and `combat.damage`.

Rocket-launcher slice:

- Requires equipped launcher module.
- Launcher determines capacity/cooldown.
- Launcher ammo determines salvo damage/effect.
- Consume launcher ammo by salvo count.

## Required Data Model

Add a typed combat ammo definition derived from Kalaazu item rows:

```text
CombatAmmoDefinition
  item_id
  family: laser | rocket | rocket_launcher | mine
  ammo_key
  multiplier
  flat_damage
  shield_leech
  cooldown_ms
  accuracy_modifier
  allowed_target_kind
  fallback_rank
  selectable
  buyable
  slotbar_order
```

Do not infer combat behavior from item display text at runtime. Use importer
normalization once, then publish typed content rows.

## Implementation Slices

1. Content normalization:
   - map Kalaazu laser ammo rows by `loot_id` prefix `ammunition_laser_`
   - map rockets by `ammunition_rocket_`
   - map rocket-launcher ammo by `ammunition_rocketlauncher_`
   - normalize LF weapon damage from trusted item mapping, not raw `bonus` where
     it conflicts with source values
   - add tests for LCB/MCB/UCB/SAB/RSB/CBO and LF-1..LF-4

2. Server combat settings:
   - persist or runtime-store active ammo selection per player
   - add `combat.select_ammo`
   - validate ownership, quantity, family, item definition, and session owner
   - expose selected ammo in combat/inventory snapshots

3. Laser consumption and multiplier:
   - thread an ammo context into combat execution
   - consume selected ammo or fallback LCB-10 via inventory ledger
   - add `not_enough_ammo` combat stop reason
   - include ammo fields in result/events

4. Client quickbar:
   - inventory item drag to actionbar slot
   - server-backed quickbar assignment
   - click ammo slot to select active ammo
   - show active laser ammo icon/count and fallback state
   - attack button disables only when server says no target/ammo/energy/cooldown

5. Rocket lane:
   - add active rocket ammo selection
   - add `combat.fire_rocket`
   - wire rocket cooldown, ammo consumption, damage event, and HUD button

6. Special ammo follow-up:
   - SAB shield leech
   - CBO mixed damage/leech
   - RSB burst cooldown lane
   - CPU auto-buy / auto-rocket behavior later, not in the first vertical slice

## Tests

Server:

- contentseed test proves ammo definitions are generated from Kalaazu rows
- contentseed test proves LF-1/LF-2/LF-3/LF-4 weapon damage normalization
- `combat.select_ammo` rejects spoofed player, unknown item, non-ammo item,
  wrong family, and unowned ammo
- laser attack consumes selected ammo once per fired tick
- selected MCB-50 produces 3x base laser damage before resist
- selected UCB-100 produces 4x base laser damage before resist
- empty selected ammo falls back to LCB-10 when present
- no selected ammo and no LCB-10 stops engagement with `not_enough_ammo`
- duplicate/replayed command does not double-consume ammo

Client:

- inventory ammo items render as quickbar-draggable
- dropping ammo on quickbar sends assignment intent, not local truth mutation
- selecting a quickbar ammo slot sends `combat.select_ammo`
- HUD updates active ammo/count only from server snapshot/event
- empty selected ammo shows fallback or stopped state from server

E2E:

- starter player has LCB-10 and can attack
- selecting MCB-50 lowers its quantity on each shot
- damage events show materially faster kill with MCB-50 than LCB-10
- removing all laser ammo prevents attack and surfaces `not_enough_ammo`

## Product Outcome

This is a high-value DarkOrbit-feel slice because it adds:

- meaningful ammo choice
- visible economy pressure per shot
- upgrade hunger from LF cannons
- tactical quickbar usage
- a real reason for inventory/shop/cargo to matter during combat

This should be implemented before adding more decorative UI polish.
