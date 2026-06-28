# DarkOrbit Feel Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the first server-authoritative DarkOrbit-feel slice: continuous attack stance, NPC return fire, stronger combat feedback, a denser starter-to-risk sector path, and a real browser feel gate.

**Architecture:** Add a server-owned combat engagement layer beside the existing one-shot `combat.use_skill` path. Runtime owns active target/attack state and fires on tick while visibility/range/cooldown/energy/policy remain valid; the client renders stance and feedback from safe server events only.

**Tech Stack:** Go game server/runtime, realtime JSON protocol, TypeScript client state/HUD, PixiJS renderer, Vitest, Go tests, Playwright browser e2e.

---

## Progress 2026-06-28

Completed and committed before this update:

- Realtime combat engagement contract, server-owned attack stance, client HUD
  stance feedback, and focused Go/client tests.
- Kalaazu DB default seed slices required for the browser feel gate, including
  map/portal/NPC/item/ship/module/shop rows and runtime DB-first seeding.

Current browser canary slice:

- Added `client/tests/e2e/phase11-darkorbit-feel-flow.mjs` and
  `npm run e2e:darkorbit-feel`.
- The canary boots a real Postgres content DB, publishes the Kalaazu default
  snapshot, registers a real browser account, proves minimized
  `combat.start_attack`, observes server shot cadence while moving, kills a
  default-data Origin NPC, receives server-created loot, picks it up into cargo,
  sends `combat.state` keepalives during the long kill, travels
  `1-1 -> 1-2 -> 1-3`, requires a fresh current-map AOI NPC in `1-3`,
  starts attack there, observes NPC return-fire damage against the real player
  ship, captures desktop and mobile screenshots, writes a structured run-notes
  artifact, and scans smoke state, WebSocket frames, and process logs for
  hidden/fake data tokens. `DARKORBIT_FEEL_LONG_RUN_MS=600000` enables the
  scripted 10-minute observation loop without slowing the default gate.
- The default-data kill/loot browser proof and mobile screenshot proof are now
  covered. A browser proof review was recorded in
  `docs/polish/11-darkorbit-feel-browser-proof-review.md`.
- Follow-up HUD polish from that review: when manual target selection is empty
  but the server-owned combat engagement is still active, the target panel and
  laser hotbar now fall back to the visible active engagement target. This keeps
  the combat lock readable and exposes Stop instead of showing an empty target
  panel during a real fight.
- Follow-up HUD polish from user review: the topbar no longer duplicates Stop,
  Sync, Mail, Chat, Social, and Logout controls in prime combat space. It is now
  a compact one-row server-state strip while menu panels keep the management
  surface.
- Browser-proof hardening: the realtime client now distinguishes the server's
  `session invalid` WebSocket policy close from other policy closes such as
  `client too slow`, so dense AOI pressure can reconnect instead of being
  misreported as an auth expiry.

Remaining before this plan is complete:

- Run or schedule the opt-in 10-minute browser observation loop
  (`DARKORBIT_FEEL_LONG_RUN_MS=600000`) and record its human playtest notes.

## Phase 0: Guardrails

### Task 0.1: Record The Implementation Boundary

**Files:**

- Modify: `docs/polish/09-polish-backlog.md`
- Modify: `docs/plans/2026-06-28-darkorbit-feel-design.md`

**Step 1: Add the phase boundary**

Document that this implementation pass owns only:

- server-owned player attack stance
- NPC return fire
- combat feedback
- dense starter-to-risk content slice
- feel e2e proof

Explicitly out of scope:

- ammo economy
- rockets
- drones
- P.E.T.
- full signal gate
- factions/companies

**Step 2: Run doc whitespace check**

Run:

```bash
git diff --check -- docs/polish docs/plans/2026-06-28-darkorbit-feel-design.md
```

Expected: no output.

**Step 3: Commit**

Only if the user asks for commits:

```bash
git add docs/polish/09-polish-backlog.md docs/plans/2026-06-28-darkorbit-feel-design.md
git commit -m "docs: define DarkOrbit feel implementation boundary"
```

## Phase 1: Realtime Combat Contract

### Task 1.1: Add Operation And Event Names

**Files:**

- Modify: `internal/game/realtime/envelope.go`
- Modify: `internal/game/realtime/envelope_test.go`
- Modify: `client/src/protocol/envelope.ts`
- Modify: `client/src/protocol/envelope.test.ts`

**Step 1: Write failing Go registry tests**

Add tests proving these operations are registered:

```text
combat.start_attack
combat.stop_attack
combat.state
```

Add tests proving these client event names are known:

```text
combat.attack_started
combat.attack_stopped
combat.shot_started
combat.shot_resolved
combat.state_snapshot
```

Expected failure before implementation: unknown operation/event constants.

Run:

```bash
go test ./internal/game/realtime -run 'Combat.*Attack|RealtimeOperationRegistry' -count=1
```

**Step 2: Add constants**

In `internal/game/realtime/envelope.go`, add:

```go
OperationCombatStartAttack Operation = "combat.start_attack"
OperationCombatStopAttack  Operation = "combat.stop_attack"
OperationCombatState       Operation = "combat.state"
```

Add events:

```go
EventCombatAttackStarted ClientEventType = "combat.attack_started"
EventCombatAttackStopped ClientEventType = "combat.attack_stopped"
EventCombatShotStarted   ClientEventType = "combat.shot_started"
EventCombatShotResolved  ClientEventType = "combat.shot_resolved"
EventCombatStateSnapshot ClientEventType = "combat.state_snapshot"
```

Register operations with `RateLimitPostureIntentBurst`.

**Step 3: Add TypeScript protocol constants**

In `client/src/protocol/envelope.ts`, add matching `OPERATIONS` and
`CLIENT_EVENTS` entries.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/realtime -run 'Combat.*Attack|RealtimeOperationRegistry' -count=1
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/protocol/envelope.test.ts
```

Expected: pass.

### Task 1.2: Add Command Builder Methods

**Files:**

- Modify: `client/src/protocol/commands.ts`
- Modify: `client/src/protocol/envelope.test.ts` or create focused command test if existing pattern supports it

**Step 1: Write failing TypeScript test**

Test:

- `combatStartAttack(targetID)` sends only `{ target_id }`
- `combatStopAttack()` sends `{}`
- no trusted/server-owned fields are included

Expected failure: methods do not exist.

**Step 2: Implement command builder**

Add:

```ts
combatStartAttack(targetID: string): RequestEnvelope<{ target_id: string }> {
  return this.build(OPERATIONS.combatStartAttack, { target_id: targetID });
}

combatStopAttack(): RequestEnvelope<Record<string, never>> {
  return this.build(OPERATIONS.combatStopAttack, {});
}

combatState(): RequestEnvelope<Record<string, never>> {
  return this.build(OPERATIONS.combatState, {});
}
```

**Step 3: Run test**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/protocol/envelope.test.ts
```

Expected: pass.

## Phase 2: Server Player Attack Stance

### Task 2.1: Add Runtime Combat Engagement State

**Files:**

- Modify: `internal/game/server/runtime.go`
- Create: `internal/game/server/combat_engagement.go`
- Create: `internal/game/server/combat_engagement_test.go`

**Step 1: Write failing unit tests**

In `combat_engagement_test.go`, test pure state behavior:

- start stores player target and weapon key
- duplicate start for same target is idempotent enough for response
- stop removes state and records stop reason
- disabled/death/map transfer cleanup removes state

Use small helper structs; do not require full runtime yet.

Expected failure: types/functions missing.

**Step 2: Add state types**

Create `internal/game/server/combat_engagement.go`:

```go
type combatEngagementState struct {
	PlayerID      foundation.PlayerID
	TargetID      world.EntityID
	SkillID       string
	StartedAt     time.Time
	NextFireAt    time.Time
	LastStopReason string
}

type combatStopReason string
```

Initial stop reasons:

```text
manual
target_lost
target_not_visible
out_of_range
cooldown
not_enough_energy
ship_disabled
target_destroyed
map_changed
policy_blocked
```

Add `activeCombatEngagements map[foundation.PlayerID]combatEngagementState` to
`Runtime`.

Initialize in `NewRuntime`.

**Step 3: Add helpers**

Implement helpers:

- `startCombatEngagementLocked(playerID, targetID, skillID, now)`
- `stopCombatEngagementLocked(playerID, reason, now)`
- `combatEngagementSnapshotLocked(playerID, now)`
- `clearCombatEngagementsForEntityLocked(entityID, reason, now)`

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/server -run 'TestCombatEngagement' -count=1
```

Expected: pass.

### Task 2.2: Add Start/Stop/State Handlers

**Files:**

- Modify: `internal/game/server/handlers.go`
- Create or modify: `internal/game/server/combat_engagement_handlers.go`
- Modify: `internal/game/server/server_combat_loot_death_test.go`

**Step 1: Write failing server handler tests**

Add tests proving:

- `combat.start_attack` rejects trusted fields such as `player_id`, `damage`,
  `cooldown`, `position`
- missing `target_id` fails
- hidden target fails before state mutation
- out-of-range target fails before state mutation
- valid target queues `combat.attack_started` and `combat.state_snapshot`
- `combat.stop_attack` queues `combat.attack_stopped`
- duplicate request id replays safely through existing gateway cache

Run:

```bash
go test ./internal/game/server -run 'TestCombatStartAttack|TestCombatStopAttack|TestCombatState' -count=1
```

Expected failure: handlers missing.

**Step 2: Register handlers**

In `commandHandlers()` add:

```go
realtime.OperationCombatStartAttack: runtime.handleCombatStartAttack,
realtime.OperationCombatStopAttack:  runtime.handleCombatStopAttack,
realtime.OperationCombatState:       runtime.handleCombatState,
```

**Step 3: Implement handlers**

`handleCombatStartAttack`:

- `rejectTrustedPayload`
- decode `{ target_id }`
- require supported `basic_laser` for now, or implicit basic laser
- lock runtime
- `validateShipCanActLocked`
- sync attacker and target combat actor
- verify entity visible
- verify current range/policy using existing combat service policy checks where
  possible; if a dry-run helper is needed, add it to `combat.Service`
- store engagement
- queue:
  - `EventCombatAttackStarted`
  - `EventCombatStateSnapshot`
- return `{ "accepted": true, "target_id": "...", "skill_id": "basic_laser" }`

`handleCombatStopAttack`:

- reject trusted payload
- stop engagement with reason `manual`
- queue stopped + state snapshot
- return `{ "accepted": true }`

`handleCombatState`:

- return current public engagement snapshot

**Step 4: Run focused server tests**

Run:

```bash
go test ./internal/game/server -run 'TestCombatStartAttack|TestCombatStopAttack|TestCombatState' -count=1
```

Expected: pass.

## Phase 3: Server Combat Tick

### Task 3.1: Fire Player Engagements On Runtime Tick

**Files:**

- Modify: `internal/game/server/runtime_world_snapshot.go`
- Modify: `internal/game/server/combat_engagement.go`
- Modify: `internal/game/server/combat_loot_repair.go`
- Create or modify: `internal/game/server/server_combat_engagement_tick_test.go`

**Step 1: Extract reusable basic attack execution**

The existing `handleCombatUseSkill` owns a large one-shot flow. Extract the
shared "execute validated basic laser and queue events" portion into a helper:

```go
func (runtime *Runtime) executeBasicLaserLocked(input runtimeBasicLaserInput) (runtimeBasicLaserResult, error)
```

It should preserve existing behavior for `combat.use_skill`.

**Step 2: Run existing combat tests before tick work**

Run:

```bash
go test ./internal/game/server -run 'TestCombat|TestLoot|TestShieldRepair|TestSeededPVP' -count=1
```

Expected: pass before changing semantics.

**Step 3: Write failing tick tests**

Add tests:

- start attack, advance fake clock beyond cooldown, runtime tick fires without
  another client command
- target HP/shield changes
- cooldown updates
- player can move while engagement remains active
- out-of-range stops or pauses with public stop reason
- target killed stops engagement and still creates loot
- map transfer clears engagement

Run:

```bash
go test ./internal/game/server -run 'TestCombatEngagementTick' -count=1
```

Expected failure: tick does not process engagements.

**Step 4: Add tick hook**

In `tickAndCollectAOIEvents`, after map workers tick and before AOI event drain,
call a locked helper:

```go
runtime.tickCombatEngagementsLocked(now)
```

This helper iterates stable-sorted player ids to keep deterministic tests.

**Step 5: Queue events**

For each fired shot, queue:

- `combat.shot_started`
- `combat.cooldown_started`
- `combat.damage` or `combat.miss`
- `combat.shot_resolved`
- `target.updated`
- `player.snapshot` / `ship.snapshot` where resources changed
- `combat.state_snapshot`

Preserve existing `combat.damage`, `combat.miss`, and `combat.cooldown_started`
events so current client feedback continues working.

**Step 6: Run tests**

Run:

```bash
go test ./internal/game/server -run 'TestCombatEngagementTick|TestCombatUseSkill|TestCombatLoot|TestSeededPVP' -count=1
```

Expected: pass.

### Task 3.2: Add Reconnect/Bootstrap Combat Snapshot

**Files:**

- Modify: `internal/game/server/runtime_sessions.go`
- Modify: `internal/game/server/server_auth_transport_test.go`
- Modify: `client/src/state/reducer-auth.test.ts` later in client phase

**Step 1: Write failing bootstrap test**

Test that `bootstrapEvents` includes `combat.state_snapshot` after session ready
when player has an active engagement.

Run:

```bash
go test ./internal/game/server -run 'TestBootstrap.*CombatState|TestReconnect.*CombatState' -count=1
```

Expected failure: no snapshot.

**Step 2: Emit snapshot**

In `bootstrapEvents`, append `EventCombatStateSnapshot` after ship/stats or
after world snapshot. Prefer after world snapshot if target visibility is
resolved there.

**Step 3: Run tests**

Run:

```bash
go test ./internal/game/server -run 'TestBootstrap.*CombatState|TestReconnect.*CombatState' -count=1
```

Expected: pass.

## Phase 4: NPC Return Fire

Status: implemented and focused-tested on 2026-06-28. Runtime now reads
server-only worker aggro targets, rechecks hidden/protection/safe-zone policy,
executes NPC basic laser through the combat service, and queues only
client-safe combat/ship/player events to the target player's sessions.

### Task 4.1: Define NPC Attack Tick Policy

**Files:**

- Modify: `internal/game/world/worker/enemy_aggro.go` if target exposure is needed
- Modify: `internal/game/server/npc_actor_projection.go`
- Create: `internal/game/server/npc_combat_tick.go`
- Create: `internal/game/server/server_npc_return_fire_test.go`

**Step 1: Write failing tests**

Tests:

- aggressive NPC in `1-3` attacks visible eligible player within range
- passive `1-1` / `1-2` NPCs do not attack
- safe-zone/protection blocks NPC attack before mutation
- stealth/hidden ineligible player is not attacked
- player receives `combat.damage`, `ship.snapshot`, `player.snapshot`
- no aggro profile ids, leash origin, pool ids, or hidden target metadata leak

Run:

```bash
go test ./internal/game/server -run 'TestNPCReturnFire|TestEnemyAggro' -count=1
```

Expected failure: no return fire.

**Step 2: Add public-safe NPC combat tick**

Implement runtime-side helper:

```go
func (runtime *Runtime) tickNPCCombatLocked(now time.Time)
```

It should:

- inspect active map instances
- find spawner-backed aggressive NPCs with current aggro target
- resolve target player from worker entity mapping
- sync NPC and player combat actors
- validate range/visibility/policy/safe-zone/protection
- execute attack with NPC stats/cooldown
- apply target player ship snapshot
- queue safe events

If worker does not expose enough public-safe aggro target data, add a narrow
server-only accessor that returns entity ids only, never client payloads.

**Step 3: Call from runtime tick**

Call NPC combat tick after worker aggro movement and before AOI diff building.

**Step 4: Run tests**

Run:

```bash
go test ./internal/game/server -run 'TestNPCReturnFire|TestEnemyAggro|TestSeededPVP' -count=1
go test ./internal/game/world/worker -run 'Test.*Aggro' -count=1
```

Expected: pass.

## Phase 5: Client Combat State And HUD

Status: reducer state and HUD attack toggle implemented/focused-tested on
2026-06-28. Client now reconciles `combat.attack_*`, `combat.state_snapshot`,
and shot lifecycle events into server-derived attack stance/effects; slot 1
starts `combat.start_attack` when idle and sends `combat.stop_attack` while
attacking the selected target.

### Task 5.1: Add Client Combat Engagement State

**Files:**

- Modify: `client/src/state/types.ts`
- Modify: `client/src/state/reducer-events.ts`
- Modify: `client/src/state/reducer-combat-loot.test.ts`
- Modify: `client/src/state/reducer.ts`

**Step 1: Write failing reducer tests**

Tests:

- `combat.attack_started` sets active attack target/skill/state
- `combat.attack_stopped` clears active state and records reason
- `combat.state_snapshot` reconciles active/idle state
- `combat.shot_started` appends a laser/world effect source-target pair
- `combat.shot_resolved` keeps damage/miss effects server-owned
- logout/auth reset clears combat engagement state

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/state/reducer-combat-loot.test.ts
```

Expected failure: state/events missing.

**Step 2: Add state shape**

In `ClientState`, add:

```ts
combatEngagement: {
  active: boolean;
  targetID: string | null;
  skillID: string | null;
  startedAt: number | null;
  nextFireAt: number | null;
  lastStopReason: string | null;
}
```

Keep this as UI state derived from server events, not gameplay truth.

**Step 3: Add event parsing**

Handle new events in `applyEvent`.

**Step 4: Run tests**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/state/reducer-combat-loot.test.ts
```

Expected: pass.

### Task 5.2: Wire HUD Attack Toggle

**Files:**

- Modify: `client/src/app/client-app.ts`
- Modify: `client/src/app/client-app-commands.ts`
- Modify: `client/src/ui/hud-types.ts`
- Modify: `client/src/ui/hud-render-panels.ts`
- Modify: `client/src/ui/hud.ts`
- Modify: `client/src/app/client-app-commands.test.ts`

**Step 1: Write failing tests**

Test:

- actionbar laser slot says `Attack` or `Stop` based on active engagement
- pressing slot `1` sends `combat.start_attack` when target selected and idle
- pressing slot `1` sends `combat.stop_attack` when attacking same target
- disabled ship/offline/no target still blocks

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/app/client-app-commands.test.ts src/ui/hud-render-inventory.test.ts
```

Expected failure.

**Step 2: Add handlers**

Add HUD handler:

```ts
onAttackToggle: () => this.toggleAttack()
```

Add command methods:

```ts
protected toggleAttack(): void
protected sendCombatStartAttack(targetID: string): void
protected sendCombatStopAttack(): void
```

**Step 3: Update action state**

In `laserActionState`, account for:

- `combatEngagement.active`
- selected target mismatch
- stop action
- pending start/stop ops
- cooldown/energy/range display

**Step 4: Run tests**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/app/client-app-commands.test.ts
```

Expected: pass.

## Phase 6: Renderer Feedback

Status: implemented and client-checked on 2026-06-28. Shot-start and resolved
combat events now carry started/resolved phase, source/target entity ids, and
shield/hull/mixed impact metadata into world effects; the renderer draws a
stronger laser muzzle/beam plus shield rings, hull sparks, and target reticle
pulses from those server events.

### Task 6.1: Add Shot Started / Impact Debug Effects

**Files:**

- Modify: `client/src/state/types.ts`
- Modify: `client/src/state/reducer-world.ts`
- Modify: `client/src/render/world-renderer-effects.ts`
- Modify: `client/src/render/world-renderer-types.ts`
- Modify: `client/src/render/world-renderer.ts`
- Modify: `client/src/render/world-renderer-sprites.test.ts` or create focused effect test

**Step 1: Write failing renderer/state tests**

Test:

- `combat.shot_started` creates a laser/charge effect with source and target
- `combat.damage` creates shield or hull impact metadata when payload contains
  `shield_amount` / `hull_amount`
- debug snapshot includes active projectile/impact effects

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/render/world-renderer-sprites.test.ts src/state/reducer-combat-loot.test.ts
```

Expected failure.

**Step 2: Extend effect model**

Extend `WorldFeedbackEffect` with optional:

```ts
phase?: 'started' | 'resolved';
damageKind?: 'shield' | 'hull' | 'mixed';
sourceEntityID?: string;
targetEntityID?: string;
```

**Step 3: Render stronger feedback**

In `world-renderer-effects.ts`, add:

- muzzle flash at source for `shot_started`
- stronger beam line
- shield ring if shield damage > 0
- hull spark if hull damage > 0
- target reticle pulse on hit
- loot reveal burst on kill/drop

**Step 4: Run tests**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/render/world-renderer-sprites.test.ts src/state/reducer-combat-loot.test.ts
```

Expected: pass.

## Phase 7: Dense Sector Content Slice

### Source Reference: Kalaazu Database Dumps

Use the open source Kalaazu database dumps as the reference seed for this phase:

- Repository: `https://github.com/manulaiko/Kalaazu`
- Database folder: `https://github.com/manulaiko/Kalaazu/tree/develop/Persistence/database`
- License checked on 2026-06-28: MIT
- Local provenance note: `docs/polish/10-kalaazu-reference-content-source.md`
- DB seed implementation plan:
  `docs/plans/2026-06-28-kalaazu-db-default-seed-implementation.md`

Relevant source tables:

- `maps/dump.sql` for starter/PVP flags and coordinate limits
- `maps_npcs/dump.sql` for map-specific NPC density targets
- `npcs/dump.sql` for NPC stat bands and AI categories
- `maps_portals/dump.sql` for early map travel graph and portal positions
- `items/dump.sql` for item families, buyability, cooldowns, and upgrade ladder
  shape
- `ships/dump.sql` for ship HP, speed, cargo, and slot layout defaults

Implementation rule:

- It is acceptable to use the dumps as MIT-licensed seed data for density,
  dimensions, NPC stat bands, portal topology, and item family shape.
- The desired runtime truth is the published content DB snapshot seeded from the
  Kalaazu-derived default data, not static Go catalogs.
- Convert names, descriptions, lore-facing labels, and economy numbers into our
  own game identity unless a later review explicitly approves exact wording.
- Keep all resulting gameplay state server-owned. Do not add client-only fake
  population, fake NPCs, fake items, or fake rewards.

### Task 7.1: Author More Early NPC Density

**Files:**

- Create or Modify: `docs/polish/10-kalaazu-reference-content-source.md`
- Modify: `internal/game/world/maps/enemy_catalog.go`
- Modify: `internal/game/content/starter_balance.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `internal/game/server/server_enemy_spawner_test.go`
- Modify: `docs/polish/02-darkorbit-feel-acceptance-criteria.md`

**Step 1: Write failing content tests**

Tests:

- starter map has multiple live/spawnable low-risk contacts
- `1-2` has at least two enemy bands
- `1-3` has at least one aggressive attack-capable enemy band
- risky map reward profile is materially better than safe map
- map dimensions and spawn count ratios are derived from the Kalaazu map/NPC
  reference rows documented in `docs/polish/10-kalaazu-reference-content-source.md`

Run:

```bash
go test ./internal/game/content ./internal/game/world/maps ./internal/game/server -run 'Starter|Enemy|Spawner|DarkOrbit|Balance' -count=1
```

Expected failure until catalog rows are expanded.

**Step 2: Add content rows**

Add or tune rows using the Kalaazu dumps as source grammar:

- low-risk passive/training drone
- low-risk weak attacker
- `1-2` farming enemy
- `1-2` tougher enemy
- `1-3` aggressive raider
- at least one better `1-3` drop profile
- map limits compatible with the reference starter sector scale
- spawn caps/counts that make the map feel populated without sending hidden
  entities outside AOI/fog rules

Do not expose pool/drop internals to client payloads.

**Step 3: Run tests**

Run:

```bash
go test ./internal/game/content ./internal/game/world/maps ./internal/game/server -run 'Starter|Enemy|Spawner|DarkOrbit|Balance' -count=1
```

Expected: pass.

### Task 7.2: Add Early Upgrade Goal Content

**Files:**

- Create or Modify: `docs/polish/10-kalaazu-reference-content-source.md`
- Modify: `internal/game/modules/catalog.go` or content DB seed mapping files if current content pipeline owns modules
- Modify: `internal/game/content/starter_balance.go`
- Modify: `internal/game/contentseed/quests.go`
- Modify: `internal/game/content/bundle_test.go`
- Modify: `docs/polish/07-progression-economy-content-review.md`

**Step 1: Write failing content tests**

Tests:

- fresh player can see one next laser/shield/cargo upgrade in shop/crafting/content catalog
- at least one upgrade requires combat loot or risky-map material
- quest/loot path surfaces upgrade progress within first session
- item family shape is informed by Kalaazu `items/dump.sql`, but player-facing
  names/descriptions are normalized into this project's setting

Run:

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/modules -run 'Upgrade|Starter|Quest|Module' -count=1
```

Expected failure until content rows are added.

**Step 2: Add minimal ladder**

Add content for a first-session ladder, using the Kalaazu item dump as a shape
reference:

- LF-1 starter retained
- LF-2 equivalent or "laser_beta_t1"
- shield upgrade
- cargo/radar or speed utility
- one material path from NPC loot

Keep names temporary if needed, but document temporary naming.

**Step 3: Run tests**

Run:

```bash
go test ./internal/game/content ./internal/game/contentseed ./internal/game/modules -run 'Upgrade|Starter|Quest|Module' -count=1
```

Expected: pass.

## Phase 8: Browser Feel E2E

### Task 8.1: Add Combat Engagement Browser Test

**Files:**

- Create: `client/tests/e2e/phase11-darkorbit-feel-flow.mjs`
- Modify: `client/package.json`
- Modify: `docs/playtest-vertical-slice-status.md`
- Modify: `docs/polish/02-darkorbit-feel-acceptance-criteria.md`

**Step 1: Write e2e flow**

Script:

```text
build client
start Go server with real authenticated playtest seed
register fresh user
wait for world snapshot
select visible hostile target
click/press attack
verify outbound combat.start_attack
wait for at least two server-driven shot/damage/cooldown events without second attack command
move while attack remains active
wait for NPC return-fire damage or shield change
kill target
loot drop
verify upgrade/progression signal exists
capture desktop screenshot
scan DOM/state/WebSocket/logs for hidden/fake data leaks
```

**Step 2: Add npm script**

In `client/package.json`:

```json
"e2e:darkorbit-feel": "npm run build && node tests/e2e/phase11-darkorbit-feel-flow.mjs"
```

**Step 3: Run focused e2e**

Run:

```bash
cd client && npm --cache /tmp/gameproject-npm-cache run e2e:darkorbit-feel
```

Expected: pass and write screenshots under:

```text
output/screenshots/ui-implementation/darkorbit-feel/
```

## Phase 9: Documentation Reconciliation

### Task 9.1: Update Polish And Plan Status

**Files:**

- Modify: `docs/polish/00-index.md`
- Modify: `docs/polish/02-darkorbit-feel-acceptance-criteria.md`
- Modify: `docs/polish/09-polish-backlog.md`
- Modify: `docs/playtest-vertical-slice-status.md`
- Modify: `docs/todo.md` only for gaps not fixed

**Step 1: Record implemented evidence**

For each completed slice, add:

- command/test evidence
- screenshot paths
- remaining feel gaps
- production limitations

**Step 2: Do not overclaim**

Use statuses:

- `contract complete`
- `vertical slice complete`
- `feel incomplete`
- `production incomplete`
- `polish complete`

Only mark `polish complete` if the acceptance criteria are actually met.

**Step 3: Run doc check**

Run:

```bash
git diff --check -- docs/polish docs/playtest-vertical-slice-status.md docs/todo.md
```

Expected: no output.

## Phase 10: Full Verification

### Task 10.1: Run Focused Verification

Run:

```bash
go test ./internal/game/realtime ./internal/game/combat ./internal/game/world/worker ./internal/game/server -run 'Combat|NPC|Aggro|Enemy|DarkOrbit|RealtimeOperationRegistry' -count=1
cd client && npm --cache /tmp/gameproject-npm-cache run test -- src/state/reducer-combat-loot.test.ts src/app/client-app-commands.test.ts
cd client && npm --cache /tmp/gameproject-npm-cache run e2e:darkorbit-feel
```

Expected: all pass.

### Task 10.2: Run Full Verification

Run:

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

Expected: all pass.

## Implementation Order Summary

Use this order:

1. Protocol constants and command builder.
2. Runtime combat engagement state.
3. Start/stop/state handlers.
4. Player combat tick.
5. Bootstrap/reconnect snapshot.
6. NPC return fire.
7. Client reducer state.
8. HUD attack toggle.
9. Renderer feedback.
10. Dense content slice.
11. Browser feel e2e.
12. Docs/status reconciliation.
13. Full verification.

## Commit Shape

If commits are requested, keep them small:

```text
game: add combat engagement protocol
game: add server-owned attack stance
game: tick player combat engagements
game: add NPC return fire
client: render combat engagement state
client: add DarkOrbit feel combat feedback
game: densify starter sector content
test: add DarkOrbit feel browser flow
docs: record DarkOrbit feel evidence
```

## Risk Notes

- Do not let the combat tick mutate state without visibility/range/policy checks.
- Do not leak aggro pool ids, target memory, leash origin, loot rolls, or hidden
  entities.
- Do not remove `combat.use_skill` until e2e and compatibility paths are updated.
- Do not fake extra entities client-side to satisfy sector density.
- Keep passive beginner enemies if needed, but add at least one real threat.
