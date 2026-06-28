# Game Feel Gap Analysis

Date: 2026-06-28

## Short Verdict

The game does not feel like DarkOrbit because the problem is not mainly assets.

DarkOrbit feel comes from continuous target lock, rhythmic automatic fire,
movement-under-pressure, NPC retaliation, clear map risk/reward, dense loot and
resource opportunities, and social/rank/faction motivation.

This codebase has a real server-authoritative foundation, but the game-feel loop
is still too MVP, too panel-driven, and too one-shot.

## What Is Correct

- Server truth is the right choice. The client does not author position, damage,
  loot, wallet, cargo, cooldowns, or hidden facts. Evidence:
  `client/src/protocol/envelope.ts`,
  `internal/game/server/server_world_movement_test.go`.
- Movement is server-owned. `move_to` is an intent; the server owns speed,
  timing, and position. Evidence:
  `internal/game/world/worker/worker.go`,
  `internal/game/world/types.go`,
  `internal/game/world/movement.go`.
- Client interpolation exists, so this is not simple snapshot dragging.
  Evidence: `client/src/state/movement.ts`,
  `client/src/render/world-renderer.ts`.
- Combat, loot, death, and repair are driven from real server events/snapshots.
  Evidence: `internal/game/server/combat_loot_repair.go`,
  `client/src/state/reducer-events.ts`.

These are strong foundations. Keep them.

## Why It Feels Wrong

### 1. Movement Feels Like Straight-Line Accounting

Server movement is a clean linear route:

```text
origin -> target -> speed -> arrive_at_ms
```

That is correct for authority and easy to test, but it lacks game feel:

- no acceleration/deceleration impression
- no turning momentum
- no drift or engine push
- no orbit/kite affordance
- no camera anticipation
- no movement/combat blending beyond "you may move and press Fire separately"

Long movement is also client-segmented by `LONG_RANGE_MOVE_STEP_UNITS`, which is
understandable for control, but emotionally reads like route bookkeeping rather
than space travel.

Evidence:

- `client/src/state/movement.ts`
- `client/src/app/client-app-commands.ts`
- `internal/game/world/movement.go`

Recommendation:

- Keep server position truth.
- Add render-only movement feel: engine trails, destination preview, softer
  camera lead/deadzone, turn interpolation, range rings, and visual arrival
  affordances.
- Consider a server-owned long-route intent so the client is not responsible for
  chaining travel steps.

### 2. Camera Is Too Literal

The renderer centers directly on the local player position:

```text
center = authoritativeDisplayPosition(local)
```

This is safe, but static. It makes the player feel like a marker on a tactical
map instead of a ship in an action space.

Missing render-only camera behavior:

- deadzone
- lead in travel direction
- slight combat framing toward locked target
- impact shake
- danger-direction emphasis
- subtle zoom/readability changes

Evidence:

- `client/src/render/world-renderer.ts`
- `client/src/render/world-renderer-base.ts`

Recommendation:

- Add camera behavior that never changes server truth. It should only affect
  presentation.

### 3. Targeting Works, But Lock Does Not Feel Central

The current client can click entities and cycle targets. That is good.

Evidence:

- `client/src/render/world-renderer-entities.ts`
- `client/src/app/target-cycle.ts`
- `client/src/ui/hud-render-panels.ts`

But DarkOrbit target lock is the center of play. Here it is mostly selection +
panel:

- no persistent server-owned active target state
- no attack stance bound to the target
- weak target-lost / out-of-range / aggro feedback
- target highlight is present but not emotionally dominant
- selected target and minimap priority are not strong enough

Recommendation:

- Add sticky target lock semantics.
- Show weapon range, lock line, target status, aggro state, and target lost
  transitions.
- Make selected target feel like a combat state, not just selected UI data.

### 4. Combat Rhythm Is One-Shot Basic Laser

The offensive loop is currently one `combat.use_skill` command per shot.

Evidence:

- `client/src/app/client-app-commands.ts`
- `internal/game/combat/service.go`
- `internal/game/server/combat_loot_repair.go`
- `client/src/ui/hud-render-panels.ts`

The server does correct validation:

- visibility
- range
- cooldown
- energy
- PvP policy
- death/loot/XP side effects

But the rhythm is not DarkOrbit-like. DarkOrbit combat is not "click skill,
wait, click skill." It is "lock target, stay in combat, move/kite/change ammo,
server cadence continues."

Recommendation:

Add server-owned attack mode:

```text
combat.set_target
combat.start_attack
combat.stop_attack
combat.attack_started
combat.attack_stopped
combat.shot_started
combat.shot_resolved
combat.state_snapshot
```

The client should not spam shots. The server should tick attack cadence while
range, visibility, cooldown, energy, ship state, and policy remain valid.

### 5. NPC Threat Is Too Thin

NPC aggro/chase exists, especially in later map work. The missing center is
player-facing return fire.

Evidence:

- `internal/game/world/worker/enemy_aggro.go`
- `docs/map-rework/phase-08-enemy-pools-spawners-ecs.md`
- `internal/game/server/combat_loot_repair.go`

If an NPC mostly moves toward a player but does not reliably fire/damage through
the same authoritative combat rules, it reads as a hostile marker, not danger.

Recommendation:

- NPCs need an offensive combat tick.
- Use the same combat service/policy primitives where possible.
- Server must own target choice, cooldown, range, hit/miss, damage, leash reset,
  and safe-zone behavior.
- Client receives safe public events only.

### 6. Feedback Shows Events, But Does Not Punch

The renderer has laser, damage, miss, destroyed, loot spawn, and pickup effects.
That is good.

Evidence:

- `client/src/render/world-renderer-effects.ts`
- `client/src/state/reducer-events.ts`

But the effects mostly "show that an event happened." They do not yet make the
hit feel physical.

Missing:

- pre-fire charge or muzzle flash
- beam channel / projectile cadence
- shield impact ring
- hull impact distinction
- target flash
- HP/shield delta animation
- kill burst with debris
- loot reveal sparkle
- pickup beam/magnet
- reticle pulse / micro camera shake

Recommendation:

- Drive strong visuals from server event timing.
- For latency hiding, show pending charge after an accepted attack state, but do
  not mutate HP/resources before server truth.

### 7. Spatial Readability Is Too Technical

The world has labels, rings, bars, and markers. It is readable as a debug map,
but not yet as a space MMO encounter.

Evidence:

- `client/src/render/world-renderer-types.ts`
- `client/src/render/world-renderer-entities.ts`
- `client/src/render/world-renderer-sprites.ts`

Problems:

- primary sprites are faint and small
- hostile/loot/portal hierarchy is soft
- weapon/pickup/radar ranges are not always naturally readable
- safe zone, PvP danger, gate importance, and loot desirability do not dominate
  the world view

Recommendation:

- Increase sprite presence.
- Show range rings contextually.
- Make hostile targets and loot more visually valuable.
- Use minimap and world reticle together for selected/hostile state.

### 8. Risk/Reward Exists As Rule, Not As Desire

The project has safe zones, PvP maps, portals, loot, repair, scanner, planets,
production, and routes. Those are real systems.

The missing part is visible player motivation:

- why should I enter this dangerous sector?
- what better drop can I get there?
- what honor/rank/bounty value is at stake?
- what upgrade am I chasing?
- what will happen if I die?

Recommendation:

- Make danger maps pay better.
- Show reward reasons in the HUD/world.
- Add early honor/bounty/rank pressure.
- Make death/repair/cargo-loss costs visible enough to matter.

## Highest-Impact Feel Fixes

1. Server-owned attack mode.
2. NPC return-fire loop.
3. Render-only movement/camera feel.
4. Strong target lock and range feedback.
5. Stronger combat/loot event visuals.
6. Ammo + rocket before broader companion systems.
7. Visible risk/reward surfaces.
8. Denser real sectors.
9. One early wave/gate PvE loop.

## Conclusion

The game is not on a bad path. The server-authoritative base is a real strength.
But the DarkOrbit feeling is currently very thin because the heart of the game
is missing: rhythmic target-lock combat, reciprocal threat, map risk desire, and
reward pressure.

Asset polish will not fix that by itself.

