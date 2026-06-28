# Server Authoritative Loop Review

Date: 2026-06-28

## Verdict

The architecture is directionally correct for a server-authoritative MMO.

The current playable loop does not feel DarkOrbit-like because it is too
command/response-shaped. Server authority is not the problem. The missing piece
is a continuous server-owned action loop.

DarkOrbit's core feel is:

```text
target lock -> continuous weapon cadence -> NPC retaliation -> movement under
pressure -> visible shield/hull/resource changes -> loot/death feedback
```

The current code has:

```text
target select -> manual Fire command -> one server-resolved basic laser shot
```

## What Is Right

### Auth And Session Boundary

The WebSocket/session architecture is correctly server-owned:

- session is resolved server-side
- player identity is not trusted from command payload
- bootstrap emits real session/player/ship/stats/wallet/cargo/world events

Evidence:

- `internal/game/server/transport.go`
- `internal/game/realtime/gateway.go`
- `internal/game/server/runtime_sessions.go`

### Movement Authority

`move_to` validates and submits intent to the worker, then returns real
server-owned map/entity state.

Evidence:

- `internal/game/server/handlers.go`
- `internal/game/world/worker/worker.go`
- `internal/game/world/movement.go`

Client interpolation uses server timing:

- `client/src/state/movement.ts`
- `client/src/render/world-renderer.ts`

### AOI And Hidden Data

AOI is filtered server-side and produces client-safe public entities.

Evidence:

- `internal/game/server/runtime_world_snapshot.go`
- `internal/game/world/aoi/snapshot.go`
- `internal/game/world/visibility`

### Per-Shot Combat Validation

`combat.use_skill` correctly resolves:

- authenticated player
- ship can act
- attacker/target combat actor sync
- visibility
- range/policy through combat service
- cooldown
- energy
- kill
- loot
- XP
- quest progress
- snapshots/events

Evidence:

- `internal/game/server/combat_loot_repair.go`
- `internal/game/combat/service.go`

## What Is Missing

### 1. Active Combat State

There is no server-owned "this player is attacking that target until stopped or
invalid" state.

Needed:

- active target per player
- attack stance per player
- selected weapon/ammo
- next-fire time
- stop reasons
- target-lost semantics
- compact combat reconciliation snapshot

### 2. Server Combat Tick

The combat service has `ExecuteBasicAttack`, but reviewed code does not expose a
server tick that repeatedly fires while a target remains valid.

This means cadence comes from repeated client commands, not from the world.

Needed:

```text
for each active combat stance:
  resolve current attacker/target positions
  validate visibility/range/policy/ship/capacitor/cooldown
  fire if ready
  emit shot/damage/miss/cooldown/snapshot events
  stop stance on invalid terminal conditions
```

### 3. NPC Offensive AI

Enemy aggro can acquire and chase targets. The missing player-facing layer is
NPC damage/attack cadence.

Needed:

- NPC attack target state
- NPC weapon cooldown
- NPC range checks
- NPC shot/damage events
- safe-zone/leash resets
- no hidden aggro internals in payloads

Use existing combat service concepts rather than inventing separate damage
truth.

### 4. Better Combat Event Contracts

Current events are enough to render a shot result, but not enough to drive
latency-friendly combat presentation.

Suggested event vocabulary:

```text
combat.target_locked
combat.target_lost
combat.attack_started
combat.attack_stopped
combat.shot_started
combat.shot_resolved
combat.damage
combat.miss
combat.cooldown_started
combat.state_snapshot
npc.attack_started
npc.attack_stopped
```

Names can change. Semantics matter.

### 5. Long-Route Movement Contract

The client currently chains long movement in bounded steps. That preserves
authority, but it makes long travel depend on client timers.

Consider a server-owned long movement intent:

```text
movement.set_destination
movement.stop
movement.destination_reached
movement.route_snapshot
```

The server can still clamp, reject, path, or split internally.

## Contract Principles

Keep these:

- client sends intent only
- server validates every tick/shot
- hidden data never leaves server
- snapshots repair missed events
- no fake optimistic resource mutation

Add these:

- accepted pending presentation is allowed
- server-owned stance/cadence drives combat
- periodic compact reconciliation snapshots exist
- stop reasons are public and stable

## Recommended Slice

### Slice A: Player Attack Stance

Add:

- `combat.start_attack { target_id, weapon_slot? }`
- `combat.stop_attack {}`
- active attack map in runtime or owning combat actor
- server tick fires basic laser when ready
- event stream for start/shot/damage/stop

### Slice B: Client Stance UI

Add:

- attack toggle in actionbar
- reticle state
- range/visibility/cooldown readout
- shot cadence animation
- stop reason toast/log line

### Slice C: NPC Return Fire

Add:

- hostile NPC attacks player if aggro target valid
- shield/hull changes visible from server event
- death/disabled path still authoritative

### Slice D: Reconciliation

Add:

- compact combat snapshot on bootstrap/reconnect
- periodic or post-event target/player combat actor snapshot

## Bottom Line

Do not weaken server authority to make the game feel faster.

Instead, move from server-authoritative commands to server-authoritative combat
state. That is the difference between "a backend accepted my click" and "I am in
a fight."

