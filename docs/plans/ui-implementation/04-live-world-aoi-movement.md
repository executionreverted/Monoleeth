# Phase 04: Live World, AOI, Movement, And Map State

## Status

- State: Planned
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
- minimap projection from client-safe AOI/fog data

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

AOI events are filtered per connected session after server visibility/fog checks.
A global entity event must not be broadcast directly to every socket.

The minimap separates:
- live AOI contacts: precise, currently visible, short-lived entities
- remembered fog/intel: coarse or stale discovered locations safe for this
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

- [ ] Add non-placeholder entity type contract for visible client entities.
- [ ] Add server world snapshot handler.
- [ ] Add per-session AOI update event broadcasting after visibility checks.
- [ ] Wire `move_to` and `stop` through concrete WebSocket.
- [ ] Add server-side idempotent player spawn/attach bootstrap.
- [ ] Add sector status summary payload.
- [ ] Add minimap-safe live AOI and remembered fog/intel projection payloads.
- [ ] Update client reducer for `aoi.entity_updated`.
- [ ] Update renderer to use real entity types, not placeholder names.
- [ ] Add interpolation using server snapshots/corrections.
- [ ] Add empty/loading/offline states for world canvas.
- [ ] Add responsive minimap panel matching mockup direction.

## Abuse And Safety Checklist

- [ ] Client cannot set final position.
- [ ] Client cannot set speed.
- [ ] Client cannot force AOI inclusion.
- [ ] Hidden entities are not serialized.
- [ ] Minimap does not reveal hidden or future entities.
- [ ] World/zone/internal ids are server-only or dev-only.
- [ ] AOI events are recipient-filtered per session.
- [ ] Movement commands are rate-limited separately from movement cooldown rules.
- [ ] Reconnect snapshot reconciles stale local positions.
- [ ] Disabled/dead ships cannot move.

## Tests

- [ ] WebSocket `move_to` changes position only by server tick/speed.
- [ ] `move_to` rejects non-finite coordinates and excessive paths safely.
- [ ] `move_to` rejects disabled/dead ship state.
- [ ] `stop` clears movement target server-side.
- [ ] Hidden entity in worker is absent from snapshot and minimap.
- [ ] Two players with different fog/AOI receive different filtered radar data.
- [ ] Entity entering AOI appears in client reducer.
- [ ] Entity leaving AOI disappears.
- [ ] Reconnect/multi-tab attach does not duplicate player entities.
- [ ] Position correction wins over local interpolation.
- [ ] Browser smoke clicks world and observes server correction.
- [ ] Mobile and desktop world/map panels do not overlap.

## Done Criteria

- Browser world view is live server state.
- Movement works through real socket intent.
- AOI entities are server-filtered.
- Minimap uses client-safe data.
- Placeholder entity type names are removed from normal client UI.
- Tests and browser smoke pass.
