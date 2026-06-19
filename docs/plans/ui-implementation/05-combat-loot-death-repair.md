# Phase 05: Combat, Loot, Death, And Repair UI

## Status

- State: Planned
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

- [ ] Register real `loot.pickup` operation in Go realtime registry.
- [ ] Add runtime loot pickup command handler.
- [ ] Add per-command payload/error contracts for combat, loot, quote, and
      repair operations.
- [ ] Add combat result event mapper to client-safe payloads.
- [ ] Add loot event mapper and AOI-visible drop updates.
- [ ] Add death/disabled ship event mapper.
- [ ] Add repair quote and repair command handlers.
- [ ] Add wallet/cargo/progression snapshot broadcasts after committed
      loot/repair mutations.
- [ ] Update client command builders and reducer for combat/loot/death events.
- [ ] Add action bar controls with cooldown/energy disabled states.
- [ ] Add selected target panel with real target health/status when visible.
- [ ] Add loot pickup UI flow from selected drop.
- [ ] Add repair UI when ship disabled.
- [ ] Add combat log lines from server events only.

## Abuse And Safety Checklist

- [ ] Hidden target attack returns safe not-visible/not-found error.
- [ ] Out-of-range attack does not spend energy.
- [ ] Cooldown is server-time only.
- [ ] Client cannot submit damage, hit, crit, XP, loot table, or cooldown.
- [ ] Hidden/far loot cannot be picked up.
- [ ] Duplicate pickup does not duplicate cargo or XP.
- [ ] Disabled ship cannot attack.
- [ ] Repair checks wallet and ship ownership server-side.
- [ ] Repair debit uses wallet ledger and server-calculated price.
- [ ] Non-server-backed action slots cannot execute fake effects.

## Tests

- [ ] WebSocket `combat.use_skill` rejects client-authored attacker id.
- [ ] Hidden target attack returns safe not-visible/not-found error.
- [ ] Out-of-range attack rejects without spending energy.
- [ ] Browser can select visible hostile and fire.
- [ ] Energy/cooldown UI updates from server event.
- [ ] NPC death creates visible loot event.
- [ ] Browser can pick visible owned/public loot.
- [ ] Hidden/far loot pickup is rejected without cargo mutation.
- [ ] Cargo snapshot updates after pickup.
- [ ] Duplicate pickup request returns cached/safe result.
- [ ] Death state disables combat buttons.
- [ ] Repair quote rejects stale/tampered prices.
- [ ] Repair rejects insufficient wallet without changing ship state.
- [ ] Repair command debits wallet and re-enables ship.
- [ ] Browser smoke covers fight -> loot -> cargo update.

## Done Criteria

- Real fight/loot loop works from browser against Go server.
- No combat or loot result is client-decided.
- Death/repair state is visible and actionable.
- Tests and browser smoke pass.
