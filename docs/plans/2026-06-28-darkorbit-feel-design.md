# DarkOrbit Feel Design

Date: 2026-06-28

## Goal

Turn the `docs/polish/` findings into a code direction that makes the first real
authenticated session feel closer to old DarkOrbit without faking gameplay
state.

## Product Target

The next milestone is:

```text
Make the first 20 minutes feel like a dangerous, rewarding space MMO.
```

This is not a full DarkOrbit clone. It is the smallest server-authoritative
slice that changes the feel from:

```text
click command -> backend responds
```

to:

```text
lock target -> enter attack stance -> fight while moving -> take/avoid damage
-> kill -> loot -> see upgrade progress -> choose next risk/reward step
```

## Recommended Approach

Use a narrow vertical slice, not a broad feature buffet.

### Approach A: Visual Polish First

Pros:

- quickest screenshots
- low backend risk

Cons:

- makes the same thin loop prettier
- does not solve one-shot combat
- does not create danger or progression hunger

Rejected for now.

### Approach B: Content Expansion First

Pros:

- sectors become less empty
- progression desire improves

Cons:

- combat still feels like manual endpoint pressing
- more enemies without return fire still feel like targets, not threats

Good as the second half of the milestone, but not first.

### Approach C: Combat Loop First, Then Dense Sector Slice

Pros:

- attacks the core feel problem
- preserves server authority
- makes existing entities more meaningful
- gives UI/renderer a real event cadence to animate

Cons:

- touches server protocol, runtime tick, client state, HUD, renderer, and e2e
- needs careful stop/reconcile semantics

Chosen.

## Architecture

Add a server-owned combat engagement layer on top of the existing combat
service.

The current `combat.use_skill` path stays as a compatibility/manual-shot
command. The new DarkOrbit-feel path adds:

```text
combat.start_attack
combat.stop_attack
combat.attack_started
combat.attack_stopped
combat.shot_started
combat.shot_resolved
combat.state_snapshot
```

Names may be adjusted during implementation, but the behavior must exist:

- client sends intent to start/stop attack
- runtime stores server-owned active combat state per player
- runtime tick fires basic laser when cooldown/energy/range/visibility/policy
  allow it
- movement remains independent and server-owned
- target loss, range loss, death, disabled ship, map transfer, safe-zone policy,
  or low energy stops or pauses combat
- snapshots repair missed events

NPC return fire is implemented through the same policy direction:

- enemy aggro selects/chases as it does today
- hostile NPCs with weapon stats can fire at valid targets
- NPC damage updates player ship/combat actor server-side
- client receives safe public damage/shot/stop/snapshot events

## Data Flow

### Player Attack

```text
HUD attack toggle
  -> CommandBuilder.combatStartAttack(target_id)
  -> realtime.OperationCombatStartAttack
  -> Runtime.handleCombatStartAttack
  -> validate ship/session/map/target visibility
  -> store active attack state
  -> queue combat.attack_started + combat.state_snapshot
  -> runtime tick
  -> execute basic attack if ready
  -> queue combat.shot_started, combat.damage/miss, cooldown, target/player snapshots
```

### NPC Attack

```text
Worker enemy aggro picks target
  -> runtime combat tick sees hostile target state
  -> validates NPC actor and target player
  -> executes basic attack or NPC attack equivalent
  -> updates player ship snapshot
  -> queues damage/ship/player events to target player
  -> queues public target update where visible
```

### Client Presentation

```text
events
  -> reducer updates combat engagement state
  -> HUD action slot shows attack stance/cooldown/stop reason
  -> target panel shows lock/range/attack state
  -> renderer shows shot start, beam/projectile, impact, shield/hull feedback
```

## Boundaries

This implementation pass owns:

- server-owned player attack stance
- NPC return fire
- combat feedback
- dense starter-to-risk content slice
- browser feel e2e proof

This milestone does not implement:

- full ammo economy
- rockets
- drones
- P.E.T.
- full Galaxy Gate
- factions/companies
- durable multi-process combat state

It must leave clear seams for those later.

## Content Slice

After attack stance and NPC return fire work, add one denser content path:

```text
1-1 onboarding: passive + light hostile training density
1-2 PvE farming: multiple enemy bands and better drops
1-3 risk/PvP: better rewards, more pressure, clearer reason to enter
```

No client fake contacts.

## Verification Philosophy

Keep existing server-authoritative verification, then add feel gates:

- Go domain tests for combat state and NPC attack
- server handler/gateway tests for trusted payload rejection and state snapshots
- client reducer/UI tests for stance events
- renderer/unit tests for feedback debug state
- browser e2e for real server:
  - fresh auth
  - lock/start attack
  - shots continue while moving
  - NPC damages player
  - kill/loot still works
  - no fake/hidden data leaks
  - screenshot artifacts

## Done Criteria

The milestone is done only when:

- a player can start attack once and see server cadence continue
- the player can move while attack continues
- at least one hostile NPC can damage the player with server-owned rules
- the HUD shows attack stance and target state clearly
- combat feedback is visible without reading the log
- first sector path has multiple real contacts and one upgrade/risk hook
- e2e proof uses real authenticated server state
- `docs/polish/02-darkorbit-feel-acceptance-criteria.md` is rechecked and
  updated with evidence
