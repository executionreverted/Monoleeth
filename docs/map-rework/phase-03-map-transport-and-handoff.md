# Phase 03: Map Transport And Session Handoff

## Goal

Replace the single infinite coordinate-plane runtime with bounded, server-owned
map sessions. Each live player is attached to exactly one active 10000x10000 map
at a time, and transport must deliver only that map's information stream.

MVP may keep one authenticated WebSocket connection per browser session, but the
server must treat it as a map-scoped subscription. No command, event,
snapshot, minimap contact, worker cursor, or queued event from another map may
reach the client.

## Current state to replace/reuse, with exact file refs

Reuse:
- `docs/plans/ui-implementation/02-game-server-transport-runtime.md:96`
  defines authenticated `/ws`, session resolution, gateway routing, bootstrap,
  and post-mutation async event delivery.
- `docs/plans/ui-implementation/02-game-server-transport-runtime.md:133`
  requires event envelopes with `event_id`, `type`, `seq`, `server_time`, `v`,
  per-session filtering, reconnect repair, and slow-client backpressure.
- `internal/game/server/handlers.go:112` centralizes operation registration in
  `Runtime.commandHandlers`.
- `internal/game/server/handlers.go:170` and
  `internal/game/server/handlers.go:195` resolve `session.snapshot` and
  `world.snapshot` from the authenticated server session, not client identity.
- `internal/game/server/handlers.go:372` rejects trusted/server-owned payload
  fields before command handlers trust a request.
- `internal/game/server/runtime.go:767` builds the authenticated bootstrap event
  bundle and already includes `world.snapshot`.
- `internal/game/server/runtime.go:896` creates monotonic per-session event
  envelopes.
- `client/src/protocol/envelope.ts:163` defines the current request envelope.
- `client/src/protocol/envelope.ts:193` defines the current async event envelope.
- `client/src/protocol/envelope.ts:207` rejects internal ids and hidden/procedural
  data in server payloads client-side.

Replace or generalize:
- `internal/game/server/runtime.go:71` configures a single `WorldID` and
  `ZoneID`; the map rework needs a map catalog and active map ownership instead
  of one runtime-wide coordinate plane.
- `internal/game/server/runtime.go:88` stores one `Worker` on `Runtime`; this
  must become a per-map worker registry or a map-runtime router.
- `internal/game/server/runtime.go:92` stores one `worldID` and `zoneID`; live
  commands must resolve the player's current map server-side.
- `internal/game/server/runtime.go:100` through
  `internal/game/server/runtime.go:104` stores `eventSeq`, `sessions`,
  `lastAOI`, `lastMove`, and queued events without map membership. These need
  map-aware ownership and reset semantics during handoff.
- `internal/game/server/runtime.go:305` creates one worker from the runtime
  config. The new runtime must create, start, tick, and stop workers per map.
- `internal/game/server/runtime.go:943` builds AOI for one worker and one
  zone; this must select only the viewer's active map worker.
- `internal/game/server/runtime.go:1029` ticks one worker and then diffs every
  session. The new tick path must fan out per map and only to sessions
  subscribed to that map.
- `client/src/state/reducer.ts:73` initializes one `sector`, one `minimap`, and
  one `visibleEntities` set. Map transfer must clear or replace these atomically
  on the client.

## Target model

The server owns a bounded map catalog. A map is a gameplay container with:
- internal `MapID`
- public `map_key`/display metadata safe for the client
- bounds fixed at `0 <= x <= 10000` and `0 <= y <= 10000`
- portal definitions
- safe zones and PvP policy references
- per-map enemy pools and spawn rules
- per-map worker, AOI index, event queue, and snapshot cursor
- optional rare planet scan/claim/production configuration

The player session model becomes:

```text
auth session -> player id -> active map membership -> map worker entity
```

One browser WebSocket may stay open across a portal transfer. Internally, the
socket subscription must be rebound from the origin map stream to the destination
map stream only after the authoritative handoff commits. A future scaled runtime
may use separate socket shards or worker-owned socket servers, but the protocol
contract is the same: the client receives only its current map stream.

Transfer lifecycle:

```text
idle
  -> transfer_pending
  -> detached_from_origin
  -> attached_to_destination
  -> active
```

During `transfer_pending`, normal gameplay commands are rejected or queued only
if the command is explicitly marked safe. MVP should reject movement, combat,
loot, scan, market-in-space, and portal entry while transfer is active.

## Data structures/contracts to add or change

Add internal server types:

```text
MapDefinition
  map_id internal
  map_key public
  display_name
  width = 10000
  height = 10000
  default_spawn_key
  portal_ids
  safe_zone_keys
  pvp_policy_key
  enemy_pool_key
  planet_generation_profile_key

MapRuntime
  internal_map_id
  worker
  aoi_index
  sessions: session_id -> player_id
  last_aoi: session_id -> snapshot
  subscription_epoch: session_id -> map_subscription_epoch
  event_seq or cursor source
  queued_events
  tick_state

PlayerMapState
  player_id
  active_map_id
  entity_id
  position
  transfer_state
  active_transfer_id optional
  spawn_protection_until
  portal_protection_until
  last_map_revision

TransferState
  transfer_id
  player_id
  from_internal_map_id
  to_internal_map_id
  portal_id
  status
  started_at
  expires_at
  destination_spawn_key
  request_id
  idempotency_key
```

Do not serialize internal `map_id`, map worker ids, transfer tokens, origin
worker cursors, destination worker cursors before attach, or any hidden map
content. If the client needs a map identifier for display/routing, expose a
public `map_key` only.

Change bootstrap/world snapshot:

```json
{
  "map": {
    "map_key": "mmo-1",
    "display_name": "Orbit 1-1",
    "bounds": { "width": 10000, "height": 10000 },
    "pvp_policy": "safe",
    "danger": "low"
  },
  "sector": { "name": "Orbit 1-1", "region": "Mars Gate" },
  "entities": [],
  "minimap": { "radar_range": 900, "live_contacts": [], "remembered": [] },
  "snapshot_cursor": 42
}
```

Add transport events:

```text
map.transfer_started
map.transfer_completed
map.transfer_failed
map.snapshot
```

`world.snapshot` may continue to carry the full current-map baseline for
compatibility, but the payload must become map-bounded and map-scoped. On
`map.transfer_completed`, the client must clear visible entities, selected
target, movement target, known live loot, and live minimap contacts before
applying the destination snapshot.

Add operation:

```text
portal.enter
```

Payload:

```json
{
  "portal_id": "portal_1_1_to_1_2",
  "request_id": "client-generated retry id"
}
```

The payload is only an intent. The server resolves current map, player entity,
portal definition, distance, cooldown, transfer state, and safe destination.

Client parser updates must add forbidden keys for:

```text
map_id
worker_id
map_worker_id
transfer_id
transfer_token
destination_worker
origin_worker
```

Internal metrics/logs should record public `map_key` or redacted internal ids
only where safe for operators. Do not log session tokens or transfer secrets.

### Map Transfer Transaction

Portal transfer must be one player-owned transaction, even in the first
in-memory MVP. Production persistence can arrive later, but the contract should
already match a durable state machine.

Transaction rules:

1. Acquire a player-level transfer lock keyed by `player_id`.
2. Build an idempotency key from
   `portal_enter:<player_id>:<source_map_id>:<portal_id>:<request_id>`.
3. Resolve active source map from server state.
4. Validate portal, proximity, transfer state, ship state, destination map,
   destination spawn, safe spawn/protection, map entry gates, and cooldown.
5. Create or reuse a `TransferState` record with status `transfer_pending`.
6. Settle source movement and detach the player from the source worker.
7. Attach the player and all active sessions to the destination worker.
8. Update `PlayerMapState.active_map_id`, position, protection state, and
   `map_subscription_epoch`.
9. Commit portal cooldown only after destination attach succeeds.
10. Emit `map.transfer_completed` and destination snapshot after commit.

Rollback/recovery rules:

- Validation failure leaves the player in the source map and does not consume
  portal cooldown.
- Failure before source detach leaves the player in the source map.
- Failure after source detach but before destination attach must reattach to
  the source map or complete the destination attach from the transfer record;
  never leave the player in neither map.
- Reconnect during `transfer_pending` returns a loading/locked state until the
  transfer completes or is safely rolled back.
- Reconnect after destination attach returns the destination snapshot.
- Duplicate retry with the same idempotency key returns the original result.
- A conflicting transfer while a transfer lock exists returns
  `ERR_TRANSFER_ACTIVE`.
- Multi-tab sessions for one player transfer together and receive the same
  destination epoch.

### Map Subscription Epoch

Every map-scoped snapshot and event must carry `map_subscription_epoch`, issued
by the server whenever a session attaches to a map. The server drops queued
events whose epoch no longer matches the session's active map subscription. The
client must also ignore AOI, combat, loot, scan, portal, and map events with an
old epoch.

Epoch is not gameplay authority. It is a stale-event guard. Commands still
resolve active map from server-side session/player state.

## Implementation tasks in order

1. Define map terminology in docs before code: bounded map, internal `MapID`,
   public `map_key`, map runtime, map worker, active map subscription, and map
   transfer state.
2. Add a map catalog with at least the starter map and one destination map.
   Every map must specify width, height, spawn points, portal ids, enemy pool,
   PvP policy reference, and safe zone reference.
3. Refactor runtime composition from one `Worker` to a map runtime registry.
   Keep auth, gateway, economy, inventory, progression, quest, market, and
   observability services shared unless a service explicitly owns live map state.
4. Store `PlayerMapState` server-side and make session resolution return only
   identity; command handlers must derive active map membership from server
   state after authentication. MVP may store this in memory, but production
   persistence must preserve transfer idempotency and recovery semantics.
5. Make player attach idempotent per active map. Reconnect must attach to the
   existing entity if present, or spawn from the current map's server-owned spawn
   rule if absent.
6. Change `world.snapshot` to select the active map worker, apply map-local AOI,
   and return map-bounded public metadata plus visible entities.
7. Change tick fanout to iterate map runtimes, compute AOI diffs for sessions
   subscribed to that map, and enqueue only per-session filtered events.
8. Add map subscription rebind primitives:
   unsubscribe from origin map, clear origin `lastAOI`, attach to destination
   map, issue a new `map_subscription_epoch`, seed destination `lastAOI`, then
   publish the destination snapshot.
9. Add transfer-state validation around all live commands. Movement, combat,
   loot pickup, scan pulse, stealth toggle, and portal entry must fail while
   the player is not `active`.
10. Add `portal.enter` handler only as an intent entrypoint. Phase 04 owns the
    detailed portal and PvP validation, but this phase owns the transport-safe
    handoff mechanics.
11. Add client state handling for map transfer events. The reducer must clear
    current-map live state before applying destination map data.
12. Extend protocol payload leak rejection with map/worker/transfer internals.
13. Add `map_subscription_epoch` to every map-scoped snapshot/event and stale
    event rejection tests on server and client.
14. Add observability counters for transfer started/completed/failed, transfer
    rejection reason, map subscription count, and cross-map leak test failures.

## Tests to add/update

- Authenticated bootstrap includes only the player's active map snapshot.
- Two sessions in different maps never receive each other's AOI events.
- `world.snapshot` for a player in map A does not include entities from map B.
- A single WebSocket can be rebound from map A to map B without reconnecting.
- Reconnect during `transfer_pending` resolves to one safe state, not duplicate
  entities in both maps.
- Reconnect after `attached_to_destination` returns the destination map snapshot.
- Map transfer clears previous `lastAOI`; destination entities arrive as fresh
  entered/snapshot entities.
- All map-scoped events carry `map_subscription_epoch`, and old-epoch events
  are dropped server-side and ignored client-side.
- Origin map subscribers receive `aoi.entity_left` or equivalent despawn for the
  transferring player only after the origin detach commits.
- Destination map subscribers receive the transferred player only after the
  destination attach commits and visibility allows it.
- Movement, combat, loot pickup, scan, stealth, and repeated portal entry fail
  safely while transfer is active.
- Client reducer clears visible entities, selected target, movement target,
  known live loot, and live minimap contacts on transfer completion.
- Protocol parser rejects server payloads containing internal map, worker, or
  transfer fields.
- Race test: simultaneous disconnect, transfer, and tick cannot leave a player
  visible in two maps.

## Migration/doc updates

- Update `docs/plans/ui-implementation/02-game-server-transport-runtime.md` to
  state that one WebSocket is only a transport channel; the active subscription
  is map-scoped.
- Update `docs/plans/ui-implementation/04-live-world-aoi-movement.md` to replace
  sector handoff and infinite movement assumptions with bounded map movement.
- Update `docs/plans/modules/14-world-aoi-fog-security.md` to replace
  world/zone membership language with current-map membership.
- Update API/event docs to document `portal.enter`, `map.transfer_started`,
  `map.transfer_completed`, `map.transfer_failed`, and map-scoped snapshots.
- Update client state docs to require live state clearing on map transfer.
- Add release notes explaining that fog-wave world discovery is removed; live
  visibility is current map plus radar range only.

## Progress Notes

2026-06-21 local TASK-0284 Phase 03 portal handoff after split:

- Added `portal.enter` as a registered realtime operation and
  `map.transfer_started`, `map.transfer_completed`, and
  `map.transfer_failed` as client event contracts.
- Implemented the transport-safe `portal.enter` MVP against the split runtime
  files. The server resolves the authenticated player/session active map,
  rejects client-authored internal map/worker/transfer fields, validates the
  portal from the current map catalog, proximity, destination spawn, cooldown,
  and active transfer state, then synchronously moves the player entity and all
  active player sessions from the source map instance to the destination map
  instance.
- Added map subscription epoch issuance on session attach/rebind, included
  `map_subscription_epoch` in map-scoped snapshots/events, cleared source
  `LastAOI` during rebind, seeded destination `LastAOI`, and filtered queued
  old-epoch origin events before delivery.
- Added transfer-state guards for movement, stop, combat skill, loot pickup,
  scan pulse, stealth toggle, and repeated portal entry while a transfer is
  active.
- Added client protocol support for `OPERATIONS.portalEnter` and
  `CommandBuilder.portalEnter(portalID)`, plus client/server forbidden-field
  rejection for internal map, worker, destination, and transfer keys.
- Added client reducer handling for transfer lifecycle events. On
  `map.transfer_completed`, the reducer clears origin visible entities,
  selected target, movement target, live loot, live minimap contacts, world
  effects, combat cooldowns, and transient scanner/live signal state before
  applying the destination snapshot. Old-epoch map-scoped events are ignored.
- Added focused server and client tests for payload spoof rejection,
  out-of-range/cooldown non-mutation, successful all-session transfer,
  idempotent duplicate request handling, active-transfer command guards,
  old-epoch event dropping, map-session isolation after tick, snapshot epoch
  presence, reducer destination replacement, and protocol leak rejection.

2026-06-21 local TASK-0286 review-blocker fix:

- Updated successful `portal.enter` response handling so the client reads the
  destination snapshot nested under `snapshot`, clears origin map-local live
  state immediately, and applies the destination map epoch/snapshot without
  waiting for `map.transfer_completed`.
- Made old-epoch `map.transfer_started` events delivered after the successful
  response an explicit late lifecycle no-op. The event can no longer regress a
  completed destination snapshot or restore origin transfer state; the following
  `map.transfer_completed` event remains idempotent and destination-authoritative.
- Added reducer regression tests for response-only handoff and the real
  response-then-`map.transfer_started`-then-`map.transfer_completed` ordering.

2026-06-21 local TASK-0290 scan transfer race hardening:

- Added a per-player scan map guard that captures the active internal map,
  world/zone, session id, and `map_subscription_epoch` for the whole
  `scan.pulse` mutation path. A concurrent `portal.enter` for the same player
  now fails while that guard is active instead of rebinding the session before
  scan events are queued.
- Re-check the captured map/epoch before scanner mutation, before resolve, and
  while queueing scan events so stale old-map scan results cannot be stamped
  with a destination epoch.
- Added focused server regression coverage for a portal attempt interleaved
  before scan event queueing and for a forced map/epoch change before scanner
  mutation; the latter proves no scanner cooldown, capacitor spend, or queued
  scan event occurs after the guard detects stale map state.

2026-06-21 local TASK-0292 portal rollback and strict snapshot parsing fixes:

- Added rollback cleanup for destination worker/session state when
  `portal.enter` fails after the destination player has been spawned or
  destination session attachment has begun. The rollback now removes the
  destination player entity, worker session attachments, runtime active-session
  entries, `LastAOI`, hidden-player state, and destination session location
  before restoring source map ownership.
- Added deterministic same-package regression coverage that forces failure
  after destination session attach, then asserts the player exists only in the
  source map, both sessions are restored to the source worker/runtime maps, no
  destination entity/session state remains, no cooldown is consumed, and no
  failed transfer is cached as an idempotent success.
- Tightened client transfer parsing so successful `portal.enter` responses and
  `map.transfer_completed` events require a nested destination `snapshot` object
  with snapshot entities before clearing origin live state or applying
  destination map truth. Missing or non-object nested snapshots no longer fall
  back to treating the whole portal/transfer payload as a snapshot.
- Added reducer regression coverage for missing and invalid nested snapshots
  while preserving the existing late old-epoch `map.transfer_started` no-op
  behavior after a response-applied destination snapshot.

Deferred to Phase 04+:

- Full safe-zone/PvP/combat escape policy, rank/faction gates, portal locking,
  spawn protection semantics, and durable transfer persistence/reconnect
  recovery remain future work.
- Transfer observability counters and a real browser portal smoke test remain
  future hardening beyond this split-file MVP.
- Scanner, production, market, and route gameplay semantics beyond current-map
  transport safety are not changed by this slice.

## Risks and acceptance criteria

Risks:
- A player can briefly exist in two workers if detach/attach is not atomic.
- Origin events can leak after subscription rebind if queued events are not
  tagged and filtered by active map.
- Reconnect during transfer can duplicate entities or strand the player.
- Public map keys can accidentally become internal map ids if naming is not
  strict.
- One-WS MVP can hide future sharding issues unless map subscription semantics
  are explicit.

Acceptance criteria:
- Every live entity event is generated from the player's active map only.
- Portal handoff is idempotent and leaves one authoritative player entity.
- Client state is empty/loading or destination-authoritative during transfer,
  never a mix of origin and destination map truth.
- No internal map id, worker id, transfer token, hidden entity, procedural seed,
  or cross-map entity appears in any response, event, or snapshot.
- Full verification for the implementation phase includes server tests, client
  reducer/protocol tests, and at least one real browser transfer smoke test.
