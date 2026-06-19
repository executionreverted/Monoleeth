# Phase 02: Game Server Transport And Runtime Composition

## Status

- State: Completed
- Owner: Concrete game server boundary
- Depends on: Phase 01
- Unlocks: real browser-server communication

## Implemented Notes

- `cmd/game-server` starts a single-process Go HTTP server with Phase 01 auth
  routes, `/healthz`, and authenticated `/ws`.
- `/ws` resolves the HttpOnly session cookie and checks the configured browser
  origin before upgrade, then resolves command identity server-side for every
  request through `realtime.Gateway`.
- The runtime composes the auth service, realtime gateway, world worker,
  baseline player/ship/stat/wallet/cargo state, AOI filtering, and command
  observability hooks. Feature-specific services such as combat, market,
  crafting, quests, and production still attach in their later UI phases.
- Bootstrap emits `session.ready`, `player.snapshot`, `ship.snapshot`,
  `stats.updated`, `wallet.snapshot`, `cargo.snapshot`, and `world.snapshot`
  from server-owned state with per-session monotonic `seq`.
- World snapshots use AOI/fog filtering and do not serialize internal world
  ids, zone ids, account ids, player ids, session ids, hidden entities,
  procedural seeds, or future spawn candidates.
- Vite proxies `/api` and `/ws` to the Go server for local browser work. The
  existing client smoke check still uses its test fixture until the browser auth
  and transport phases replace it.
- Events are emitted only after the in-memory command/tick mutation completes.
  Durable outbox/replay and a true bounded event queue remain future scaling
  work; the MVP uses per-write deadlines so a slow socket cannot block the
  world tick or other sessions.

## Goal

Create the concrete Go game server that serves auth endpoints, upgrades
authenticated WebSocket connections, composes the existing gameplay services,
and routes client requests through `realtime.Gateway` into real runtime
handlers.

## Source Specs

Read before implementation:
- `docs/plans/modules/14-world-aoi-fog-security.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `internal/game/realtime`
- `internal/game/runtime`
- `internal/game/world/worker`

Fetch current docs through Context7 for any WebSocket/router/server library
selected for the transport.

## Server Runtime Shape

MVP is a single-process game server:

```text
cmd/game-server
  -> auth HTTP handlers
  -> WebSocket endpoint
  -> session resolver
  -> runtime composition
  -> world worker tick loop
  -> gameplay services
  -> client event broadcaster
```

The single-process runtime is allowed for MVP, but it must keep boundaries clear
so later PostgreSQL/Redis/NATS/WebSocket scaling can replace in-memory stores.

## Runtime Composition Must Include

- account/session resolver from Phase 01
- player bootstrap service
- world worker and AOI snapshot service
- combat service
- loot service
- progression service
- inventory/cargo/wallet services
- ship/hangar/loadout services
- module/stat services
- crafting service
- quest service
- discovery/scanner/claim/intel services
- production/routes services
- market/auction/premium services
- observability command logger/metrics hooks

Not every service needs a full UI in this phase, but the runtime must make it
possible to attach feature handlers in later phases without rebuilding the
server from scratch.

## WebSocket Transport

Endpoint:

```text
GET /ws
```

Behavior:
- reject unauthenticated sessions
- resolve `SessionID -> CommandContext` server-side
- decode text JSON request envelopes
- run `realtime.Gateway.HandleRequest`
- send response envelope
- send async client-safe events after mutations
- send session/world bootstrap snapshot on connect
- keep checking session expiry/revocation during the socket lifetime
- close or throttle abusive clients
- never trust player id from payload

## Bootstrap Bundle

The first authenticated socket messages must be enough for the default HUD to
render without fake data:
- `session.ready`: account public shape, player public shape, roles, server time,
  protocol version, and reconnect/snapshot cursors
- `player.snapshot`: callsign, rank/role/progression public fields
- `ship.snapshot`: active ship id/display, hull, shield, capacitor, disabled
  state, and repair state
- `stats.updated`: effective movement, radar, combat, cargo, and capacitor stats
- `wallet.snapshot`: visible balances only
- `cargo.snapshot`: used capacity, max capacity, and visible stacks
- `world.snapshot`: client-safe sector status and AOI baseline

No bootstrap payload may include password hashes, raw session tokens, hidden
entities, procedural seeds, internal world/zone ids, or future spawn candidates.

## Broadcast And Reconciliation Contract

Async events must be published only after the domain mutation commits. The MVP
can implement this with an in-process committed-event queue, but the contract
must be outbox-compatible:
- every event has `event_id`, `type`, `seq`, `server_time`, `v`, and
  client-safe payload
- `seq` is monotonic per connected session or per replay stream
- per-session queues apply visibility filtering before enqueue
- slow clients are backpressured, dropped, or asked to resync without blocking
  world ticks or other sessions
- reconnect sends snapshots with enough version/cursor data to repair missed or
  stale events
- duplicate or stale events do not re-apply client-side mutations

## Base Commands

Add or wire these first:

```text
session.snapshot
world.snapshot
move_to
stop
debug_snapshot (dev-only)
```

If the existing `debug_snapshot` remains, it must be clearly dev-only and not be
the normal UI bootstrap path.

## Base Operation Contracts

| Operation | Client Payload | Server Authority | Response/Event |
| --- | --- | --- | --- |
| `session.snapshot` | empty | session cookie/resolver | safe account, player, roles, expiry, server time |
| `world.snapshot` | optional client viewport hint | server player/ship position, AOI, fog | client-safe sector, AOI baseline, snapshot cursor |
| `move_to` | finite target `{x,y}` | server player, active ship, stats, movement rules | response plus `position.corrected`/AOI events |
| `stop` | optional request reason | server player active movement state | response plus `movement.stopped` or `position.corrected` |
| `debug_snapshot` | dev-only, empty or explicit fixture id | dev-mode server config and admin/dev guard | never used by default UI bootstrap |

## Base Events

```text
session.ready
player.snapshot
ship.snapshot
stats.updated
wallet.snapshot
cargo.snapshot
aoi.entity_entered
aoi.entity_updated
aoi.entity_left
position.corrected
server.notice
```

## TODO

- [x] Add `cmd/game-server` entrypoint.
- [x] Add HTTP server setup with auth routes.
- [x] Add WebSocket upgrade endpoint.
- [x] Add server config for address, allowed origins, cookie/session settings,
      admin seed, and dev mode.
- [x] Compose single-process base runtime services.
- [x] Create authenticated session resolver for `realtime.Gateway`.
- [x] Add request read/write loop with response envelopes.
- [x] Add server event broadcaster per connected session.
- [x] Add post-command event publishing with per-session `seq` and write
      deadline backpressure.
- [x] Add initial session/player/ship/stats/wallet/cargo/world bootstrap on
      connect.
- [x] Add socket handling for session expiry/revocation after connect.
- [x] Add world worker tick lifecycle.
- [x] Add graceful shutdown.
- [x] Add Vite dev proxy notes/config for `/api` and `/ws`.
- [x] Keep debug commands dev-only.

## Abuse And Safety Checklist

- [x] Unauthenticated WebSocket fails before any gameplay state is sent.
- [x] Cross-session request id cache is session-scoped.
- [x] Command context comes from session resolver only.
- [x] Origin/cookie policy is explicit.
- [x] Bad JSON returns safe error and does not crash socket loop.
- [x] Slow or spammy socket cannot block world tick.
- [x] Hidden/internal worker state is filtered before broadcast.
- [x] Events are published only after completed in-memory mutations.
- [x] Logout or session expiry closes the socket or rejects later commands.

## Tests

- [x] HTTP auth route integration test with session cookie.
- [x] WebSocket unauthenticated connection rejected.
- [x] Authenticated WebSocket receives `session.ready`.
- [x] Authenticated bootstrap includes player, ship, stats, wallet, cargo, and
      world snapshots.
- [x] `move_to` through socket reaches world worker and returns response.
- [x] Duplicate request id returns cached response.
- [x] Bad payload returns `ERR_INVALID_PAYLOAD`.
- [x] Hidden entity in worker does not serialize to socket client.
- [x] Event `seq` is monotonic and reconnect snapshot carries the latest
      cursor.
- [x] Existing socket cannot keep mutating state after logout/session expiry.
- [x] Graceful shutdown closes sockets.

## Done Criteria

- `cmd/game-server` can be run locally.
- Browser can login and open `/ws`.
- The WebSocket path uses the real Go gateway, not the JavaScript smoke fixture.
- Initial snapshot comes from runtime composition.
- Base movement command works through the socket.
- Tests and full verification pass.
