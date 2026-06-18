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
- world/zone/sector identity
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
- [ ] Add AOI update event broadcasting to connected client.
- [ ] Wire `move_to` and `stop` through concrete WebSocket.
- [ ] Add server-side player spawn/bootstrap.
- [ ] Add sector status summary payload.
- [ ] Add minimap-safe AOI/fog projection payload.
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
- [ ] Movement commands are rate-limited separately from movement cooldown rules.
- [ ] Reconnect snapshot reconciles stale local positions.

## Tests

- [ ] WebSocket `move_to` changes position only by server tick/speed.
- [ ] `stop` clears movement target server-side.
- [ ] Hidden entity in worker is absent from snapshot and minimap.
- [ ] Entity entering AOI appears in client reducer.
- [ ] Entity leaving AOI disappears.
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
