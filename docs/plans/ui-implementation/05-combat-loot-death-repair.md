# Phase 05: Combat, Loot, Death, And Repair UI

## Status

- State: Completed for authenticated runtime MVP; durable repair/death
  hardening remains tracked separately
- Owner: Real-time combat loop UI
- Depends on: Phase 04 plus minimal wallet/cargo/active-loadout snapshots from
  Phase 06 or an equivalent prerequisite slice implemented in this phase
- Unlocks: first real playable action loop

## Goal

Expose the backend combat, loot, death, and repair systems through real UI
controls and server events: target visible NPCs, fire skills, receive combat
results, pick visible loot, handle ship disabled/death state, and repair through
server-authoritative services.

This phase cannot mark loot pickup, repair, or action slot behavior done until
wallet, cargo, and active loadout truth are available through server snapshots.

## Source Specs

Read before implementation:
- `docs/plans/modules/05-combat-damage-targeting.md`
- `docs/plans/modules/06-loot-drop-ownership.md`
- `docs/plans/modules/07-death-repair-respawn.md`
- `docs/plans/modules/02-inventory-cargo-wallet-ledger.md`
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `internal/game/combat`
- `internal/game/loot`
- `internal/game/death`
- `internal/game/runtime/combat_gateway.go`

## Server Features To Expose

- target selection from AOI-visible entities
- `combat.use_skill` for basic laser and later skill slots
- cooldown/energy enforcement
- combat damage events
- NPC death events
- loot creation events
- loot pickup command
- cargo snapshot after pickup
- XP snapshot after eligible kill/pickup
- ship disabled/death snapshot
- repair quote and repair command

## Commands

```text
combat.use_skill
loot.pickup
death.repair_quote
death.repair_ship
```

Client may send target/drop ids as intent only. Server must re-check session,
visibility, range, ownership, cooldown, energy, ship state, cargo capacity, and
wallet balance.

## Operation Contracts

| Operation | Client Payload | Server Authority And Validation | Mutation/Event Contract |
| --- | --- | --- | --- |
| `combat.use_skill` | `target_id`, `skill_id`, `request_id` | player/ship/loadout from session; target visible; range; cooldown; energy; ship not disabled | spend energy/start cooldown only on accepted use; emit combat result and snapshots after commit/tick |
| `loot.pickup` | `drop_id`, optional amount, `request_id` | player from session; drop visible/owned/public; range; cargo capacity; duplicate pickup reference | lock drop and cargo, validate, move items/XP through inventory/progression services, write ledger/event with unique `loot_pickup:<drop_id>`, commit, then broadcast |
| `death.repair_quote` | ship id or empty active-ship intent | active/owned disabled ship; current repair policy; wallet visibility | no mutation; returns server-calculated quote id, cost, expiry, and quote version |
| `death.repair_ship` | quote id or active-ship intent, `request_id` | re-resolve owned disabled ship; quote freshness or recalculated price; wallet balance | debit wallet through ledger, repair ship, emit `wallet.snapshot`, `ship.snapshot`, and `death.repaired` after commit; duplicate request returns cached/safe result |

Repair commands must not trust client price totals. If a quote is stale, the
server either rejects with a safe stale-quote error or recalculates and returns a
fresh quote path.

## Events

```text
target.updated
combat.damage
combat.miss
combat.cooldown_started
combat.npc_killed
loot.created
loot.updated
loot.removed
loot.picked_up
player.snapshot
ship.snapshot
cargo.snapshot
wallet.snapshot
progression.snapshot
death.ship_disabled
death.repaired
```

Event payloads must be client-safe and recipient-filtered:
- combat events include actor/target public ids, skill id, result type, visible
  damage numbers, and resulting public health/shield when visible
- loot events include drop id, public item summary, owner/public pickup state,
  and visible position only
- death/repair events include disabled/repaired state and public ship stats only
- every event is emitted after commit with `seq`; missed or stale events are
  repaired by `player`, `ship`, `cargo`, `wallet`, and `progression` snapshots

## UI Surfaces

Mockup areas covered:
- center hostile markers
- selected target/object panel
- bottom action bar: Laser, Rocket, Shield
- cargo topbar
- combat/event log
- ship hull/shield/cap panel
- repair disabled state modal/panel

Only server-backed skills from loadout/skill snapshots may be enabled. If Rocket
or Shield is not implemented yet, those slots render locked, empty, or disabled
from server loadout data instead of firing fake effects.

## TODO

- [x] Register real `loot.pickup` operation in Go realtime registry.
- [x] Add runtime loot pickup command handler.
- [x] Add per-command payload/error contracts for combat, loot, quote, and
      repair operations.
- [x] Add combat result event mapper to client-safe payloads.
- [x] Add loot event mapper and AOI-visible drop updates.
- [ ] Add death/disabled ship event mapper.
- [x] Add repair quote and repair command handlers.
- [x] Add wallet/cargo/progression snapshot broadcasts after committed
      loot/repair mutations.
- [x] Update client command builders and reducer for combat/loot/death events.
- [x] Add action bar controls with cooldown/energy disabled states.
- [x] Add selected target panel with real target health/status when visible.
- [x] Add loot pickup UI flow from selected drop.
- [x] Add repair UI when ship disabled.
- [x] Add combat log lines from server events only.

## Implemented Contracts

- `combat.use_skill` is registered on the authenticated realtime gateway and
  resolves attacker, ship, stats, range, cooldown, energy, and target
  visibility from server-owned runtime state. The browser sends only
  `target_id` and `skill_id` intent.
- Combat results emit `ship.snapshot`, `player.snapshot`,
  `combat.cooldown_started`, `target.updated`, `combat.damage` or
  `combat.miss`, `combat.npc_killed`, `progression.snapshot`, and AOI
  reconciliation events. Payloads use client-safe public ids and visible
  amounts; trusted fields such as `player_id`, `damage`, `xp`, hidden flags,
  seeds, and loot tables are rejected or omitted.
- NPC death creates server-owned loot through the loot service, inserts the drop
  into the world worker, emits `loot.created`, and reconciles the map through
  AOI events. Pickup uses `loot.pickup`, validates visibility/range/ownership
  and cargo capacity, then emits `loot.picked_up`, `loot.removed`,
  `cargo.snapshot`, `progression.snapshot`, and AOI removal after mutation.
- Runtime repair exposes `death.repair_quote` and `death.repair_ship` for the
  active server-owned starter ship. The browser cannot provide a trusted ship,
  price, wallet, or player identity. This runtime bridge currently supports the
  free starter repair path; durable non-zero wallet-ledger repair remains a
  hardening item tracked in `docs/todo.md`.
- The client reducer stores ship, progression, repair quote, target combat
  status, cooldowns, cargo, wallet, and combat log entries only from snapshots,
  responses, and events. Default unauthenticated state remains empty.
- The HUD now shows real target health/status, ship hull/shield/capacitor,
  disabled repair controls, and a bottom action bar. Laser and Loot send server
  commands only when the current selected target and ship state allow them;
  Rocket and Shield render disabled until server-backed skill/loadout data is
  exposed.

## Abuse And Safety Checklist

- [x] Hidden target attack returns safe not-visible/not-found error.
- [x] Out-of-range attack does not spend energy.
- [x] Cooldown is server-time only.
- [x] Client cannot submit damage, hit, crit, XP, loot table, or cooldown.
- [x] Hidden/far loot cannot be picked up.
- [x] Duplicate pickup does not duplicate cargo or XP.
- [x] Disabled ship cannot attack.
- [x] Repair checks wallet and ship ownership server-side.
- [ ] Repair debit uses wallet ledger and server-calculated price.
- [x] Non-server-backed action slots cannot execute fake effects.

## Tests

- [x] WebSocket `combat.use_skill` rejects client-authored attacker id.
- [x] Hidden target attack returns safe not-visible/not-found error.
- [x] Out-of-range attack rejects without spending energy.
- [x] Browser can select visible hostile and fire.
- [x] Energy/cooldown UI updates from server event.
- [x] NPC death creates visible loot event.
- [x] Browser can pick visible owned/public loot.
- [x] Hidden/far loot pickup is rejected without cargo mutation.
- [x] Cargo snapshot updates after pickup.
- [x] Duplicate pickup request returns cached/safe result.
- [ ] Death state disables combat buttons.
- [ ] Repair quote rejects stale/tampered prices.
- [ ] Repair rejects insufficient wallet without changing ship state.
- [ ] Repair command debits wallet and re-enables ship.
- [x] Browser smoke covers fight -> loot -> cargo update.

## Done Criteria

- Real fight/loot loop works from browser against Go server.
- No combat or loot result is client-decided.
- Death/repair state is visible and actionable.
- Tests and browser smoke pass.
