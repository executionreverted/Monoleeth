# Phase 04: Live World, AOI, Movement, And Map State

## Status

- State: Completed
- Owner: World canvas and movement loop
- Depends on: Phase 02, Phase 03
- Unlocks: playable world navigation and map truth

## Goal

Make the center canvas a real live world view: authenticated player spawn,
server-owned position, AOI-filtered entities, movement intents, corrections,
sector status, and minimap state all come from the Go server.

## Source Specs

Read before implementation:
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/2026-06-17-world-system-design.md`
- `internal/game/world`
- `internal/game/world/worker`
- `internal/game/world/aoi`
- `internal/game/world/visibility`
- `client/src/render`
- `client/src/state`

## Server Features To Expose

- player spawn/attach
- authoritative movement target
- stop command
- client-safe sector/region display and status
- AOI enter/update/left
- visible entity snapshots
- hidden entity filtering
- server position correction
- radar range from stats
- sector danger/contested status summary
- minimap projection from client-safe AOI/known-intel data

## Commands

```text
world.snapshot
move_to
stop
```

`move_to` payload:

```json
{
  "target": { "x": 100, "y": -50 }
}
```

Server resolves player and active ship from session. Client target is an intent,
not final position.

`move_to` must validate finite coordinates, active ship state, disabled/dead
state, max path/sector handoff limits, server speed/stats, visibility/routing
rules, request id/idempotency behavior, and movement rate limits. The response
must include accepted/rejected state and the snapshot or correction path that
the client should use.

`stop` must resolve the current movement state server-side and return either a
confirmed stop or a safe no-op/stale request result. The server may emit
`movement.stopped` and/or `position.corrected` after the authoritative tick.

## Spawn And Reconnect Semantics

Player attach is idempotent:
- if the player already has a live entity, reconnect/multi-tab attaches to that
  server-owned entity
- if no live entity exists, the server creates the spawn from deterministic
  server rules and records the ownership
- the browser never supplies player id, spawn location, world id, or zone id
- reconnect snapshots reconcile all local interpolation and stale entity state

## Events

```text
world.snapshot
sector.snapshot
aoi.entity_entered
aoi.entity_updated
aoi.entity_left
position.corrected
movement.stopped
stats.updated
```

Entity payloads must be client-safe:

```json
{
  "entity_id": "npc-1",
  "entity_type": "npc",
  "position": { "x": 10, "y": 20 },
  "display": {
    "label": "Drone Swarm",
    "disposition": "hostile"
  }
}
```

Avoid leaking world id, zone id, hidden flags, seeds, future spawn candidates, or
internal movement state.

## AOI And Minimap Contract

AOI events are filtered per connected session after server visibility,
radar/stealth, and known-intel checks.
A global entity event must not be broadcast directly to every socket.

The minimap separates:
- live AOI contacts: precise, currently visible, short-lived entities
- known-intel memory: coarse or stale discovered locations safe for this
  player

Neither channel may include hidden entities, undiscovered planets, future spawn
candidates, procedural seeds, hidden flags, or internal world/zone ids. Sector
display labels and danger/contested summaries are client-safe projections only.

## UI Surfaces

Mockup areas covered:
- center world canvas
- "YOU" marker
- visible friendly/hostile/outpost/portal/unknown markers
- sector topbar
- danger/contested indicator
- minimap
- movement target/correction marker

## TODO

- [x] Add non-placeholder entity type contract for visible client entities.
- [x] Add server world snapshot handler.
- [x] Add per-session AOI update event broadcasting after visibility checks.
- [x] Wire `move_to` and `stop` through concrete WebSocket.
- [x] Add server-side idempotent player spawn/attach bootstrap.
- [x] Add sector status summary payload.
- [x] Add minimap-safe live AOI and known-intel memory projection payloads.
- [x] Update client reducer for `aoi.entity_updated`.
- [x] Update renderer to use real entity types, not placeholder names.
- [x] Add interpolation using server snapshots/corrections.
- [x] Add empty/loading/offline states for world canvas.
- [x] Add responsive minimap panel matching mockup direction.

## Implemented Contracts

- `world.snapshot` now returns `sector`, `entities`, `minimap`, and
  `snapshot_cursor`. `sector` is a client-safe projection with name, region,
  danger, and contested state. `minimap.live_contacts` is built only from the
  same AOI-filtered visible entity set; `minimap.remembered` is currently an
  empty safe known-intel memory channel until discovery/intel data is exposed.
- Visible entity payloads use public entity types: `player`, `npc`, `loot`, and
  `planet_signal`. Normal client state no longer receives placeholder type
  names. Server-side deprecated aliases remain only to avoid unrelated domain
  churn.
- Visible entities include server-authored `status_flags` and optional
  `display` metadata. The viewer ship is marked with `self`; the browser uses
  that flag for the `YOU` marker, camera center, smoke click mapping, and minimap
  center.
- Worker ticks compute AOI diffs per authenticated session and emit
  `aoi.entity_entered`, `aoi.entity_updated`, and `aoi.entity_left` only after
  visibility filtering. `move_to` and `stop` also reconcile with
  `position.corrected`; `stop` emits `movement.stopped`.
- Movement commands resolve the player and ship from the authenticated session.
  The client may submit only a finite target intent. The runtime rejects
  disabled ships, excessive movement targets, forbidden server-owned fields, and
  movement bursts before mutating worker state.
- 2026-06-19 follow-up: long-range browser navigation keeps the server
  `move_to` range limit intact. The client stores only the requested navigation
  destination locally, sends bounded intermediate `move_to` intents, and
  continues after server-authored movement/correction timing until the final
  coordinate is reached or the route is stopped/replaced. Rejected movement
  responses clear speculative target markers back to authoritative self
  movement instead of leaving stale planet-range markers on screen.
- 2026-06-19 follow-up: `position.corrected` remains reducer/debug state, but
  the renderer no longer draws a cyan correction ring on top of the player ship.
  Player clicks now show only intentional movement/selection markers, not a
  planet-like correction circle at screen center.

## Abuse And Safety Checklist

- [x] Client cannot set final position.
- [x] Client cannot set speed.
- [x] Client cannot force AOI inclusion.
- [x] Hidden entities are not serialized.
- [x] Minimap does not reveal hidden or future entities.
- [x] World/zone/internal ids are server-only or dev-only.
- [x] AOI events are recipient-filtered per session.
- [x] Movement commands are rate-limited separately from movement cooldown rules.
- [x] Reconnect snapshot reconciles stale local positions.
- [x] Disabled/dead ships cannot move.

## Tests

- [x] WebSocket `move_to` changes position only by server tick/speed.
- [x] `move_to` rejects non-finite coordinates and excessive paths safely.
- [x] `move_to` rejects disabled/dead ship state.
- [x] `stop` clears movement target server-side.
- [x] Hidden entity in worker is absent from snapshot and minimap.
- [x] Two players with different radar/AOI permissions receive different
      filtered radar data.
- [x] Entity entering AOI appears in client reducer.
- [x] Entity leaving AOI disappears.
- [x] Reconnect/multi-tab attach does not duplicate player entities.
- [x] Position correction wins over local interpolation.
- [x] Browser smoke clicks world and observes server correction.
- [x] Mobile and desktop world/map panels do not overlap.

## Verification

- `go test ./internal/game/server`
- `go test ./...`
- `npm --cache /tmp/gameproject-npm-cache run typecheck`
- `npm --cache /tmp/gameproject-npm-cache run test`
- `npm --cache /tmp/gameproject-npm-cache run smoke`
- `npm --cache /tmp/gameproject-npm-cache run check`
- Real browser screenshots saved under
  `output/screenshots/ui-implementation/04/`.

## Done Criteria

- Browser world view is live server state.
- Movement works through real socket intent.
- AOI entities are server-filtered.
- Minimap uses client-safe data.
- Placeholder entity type names are removed from normal client UI.
- Tests and browser smoke pass.
