# Phase 02: Game Server Transport And Runtime Composition

## Status

- State: Planned
- Owner: Concrete game server boundary
- Depends on: Phase 01
- Unlocks: real browser-server communication

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
- close or throttle abusive clients
- never trust player id from payload

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

- [ ] Add `cmd/game-server` entrypoint.
- [ ] Add HTTP server setup with auth routes.
- [ ] Add WebSocket upgrade endpoint.
- [ ] Add server config for address, allowed origins, cookie/session settings,
      admin seed, and dev mode.
- [ ] Compose single-process runtime services.
- [ ] Create authenticated session resolver for `realtime.Gateway`.
- [ ] Add request read/write loop with response envelopes.
- [ ] Add server event broadcaster per connected session.
- [ ] Add initial session/world snapshot on connect.
- [ ] Add world worker tick lifecycle.
- [ ] Add graceful shutdown.
- [ ] Add Vite dev proxy notes/config for `/api` and `/ws`.
- [ ] Keep debug commands dev-only.

## Abuse And Safety Checklist

- [ ] Unauthenticated WebSocket fails before any gameplay state is sent.
- [ ] Cross-session request id cache is session-scoped.
- [ ] Command context comes from session resolver only.
- [ ] Origin/cookie policy is explicit.
- [ ] Bad JSON returns safe error and does not crash socket loop.
- [ ] Slow or spammy socket cannot block world tick.
- [ ] Hidden/internal worker state is filtered before broadcast.

## Tests

- [ ] HTTP auth route integration test with session cookie.
- [ ] WebSocket unauthenticated connection rejected.
- [ ] Authenticated WebSocket receives `session.ready`.
- [ ] `move_to` through socket reaches world worker and returns response.
- [ ] Duplicate request id returns cached response.
- [ ] Bad payload returns `ERR_INVALID_PAYLOAD`.
- [ ] Hidden entity in worker does not serialize to socket client.
- [ ] Graceful shutdown closes sockets.

## Done Criteria

- `cmd/game-server` can be run locally.
- Browser can login and open `/ws`.
- The WebSocket path uses the real Go gateway, not the JavaScript smoke fixture.
- Initial snapshot comes from runtime composition.
- Base movement command works through the socket.
- Tests and full verification pass.
