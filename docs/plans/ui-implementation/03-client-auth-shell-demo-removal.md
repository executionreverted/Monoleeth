# Phase 03: Client Auth Shell And Demo Removal

## Status

- State: Planned
- Owner: Browser client auth and truth boundary
- Depends on: Phase 01, Phase 02
- Unlocks: real client state, no more default mock gameplay

## Goal

Replace the current offline demo-first client with a real authenticated shell.
The browser must login/register, open the server WebSocket, render only
server-provided gameplay state, and stop showing fake HP/cargo/wallet/entities
by default.

## Source Files

Read before implementation:
- `client/src/app/client-app.ts`
- `client/src/app/demo-state.ts`
- `client/src/state/reducer.ts`
- `client/src/protocol/*`
- `client/src/net/realtime-client.ts`
- `client/src/ui/hud.ts`
- `client/src/styles.css`
- `output/mockups/final-mockup.png`

Fetch current docs through Context7 for frontend libraries if adding React,
state libraries, routing, form helpers, or test libraries.

## Client Truth Rules

Default client state after page load:
- unauthenticated shell
- no fake player snapshot
- no fake wallet
- no fake cargo
- no fake visible entities
- no fake quest/inventory/planet counts

After login:
- `GET /api/session` confirms account/player
- WebSocket connects
- server sends initial snapshots
- reducer stores snapshots
- renderer draws only server AOI entities

Demo mode may remain only if explicit:

```text
?demo=1
```

Smoke fixtures may remain only for targeted protocol tests. Real browser smoke
must use `cmd/game-server` once Phase 02 exists.

## UI Shell

Add a compact auth surface before the game:
- email
- password
- login
- create account, if registration is enabled
- safe error display
- loading state
- session restore state

After auth, use the mockup direction:
- full-bleed game surface
- top bar status
- left ship/nav rail
- right contextual rail
- bottom action/log bar

Do not build a marketing landing page.

## Client API Layer

Suggested modules:

```text
client/src/net/http-client.ts
client/src/auth/auth-client.ts
client/src/auth/auth-state.ts
client/src/ui/auth-panel.ts
```

Expected calls:
- `register(email, password, callsign)`
- `login(email, password)`
- `logout()`
- `loadSession()`

HTTP auth calls must use same-origin or the configured Vite proxy and send
cookies with `credentials: "include"` when cross-origin dev proxying requires it.
The client must not store session tokens, account ids, player ids, or admin
flags in `localStorage` as trusted authority. Local storage may only keep
presentation preferences.

## WebSocket Lifecycle

The client must model connection state explicitly:
- `restoring`: loading `GET /api/session`
- `authenticated_pending_socket`: session exists, socket not ready
- `connected`: bootstrap snapshots received
- `reconnecting`: retrying after transient close, with stale gameplay marked
  pending
- `auth_expired`: server rejected/closed because session expired or revoked
- `logged_out`: user logout or no session; gameplay state cleared

On logout or auth failure, close the socket, stop reconnect attempts, clear
gameplay reducers, and return to the auth shell. Failed WebSocket connection
must never fall back to demo data.

## Bootstrap Binding

Default real mode bootstraps from Phase 02 server messages:
- `session.ready`
- `player.snapshot`
- `ship.snapshot`
- `stats.updated`
- `wallet.snapshot`
- `cargo.snapshot`
- `world.snapshot` and AOI events

`debug_snapshot` is allowed only in explicit dev/demo tooling and cannot be the
normal UI bootstrap path.

## TODO

- [ ] Remove automatic `seedDemoState()` from default startup.
- [ ] Add explicit demo mode guard if demo remains.
- [ ] Replace initial reducer gameplay values with empty/loading state.
- [ ] Add auth HTTP client.
- [ ] Add auth state reducer/store.
- [ ] Add login/register/logout UI.
- [ ] Add session restore on page load.
- [ ] Add cookie-aware auth HTTP handling with no trusted session token in
      `localStorage`.
- [ ] Connect WebSocket only after authenticated session exists.
- [ ] Bootstrap game state from Phase 02 real snapshot/events.
- [ ] Add WebSocket lifecycle states for restore, connected, reconnecting,
      auth-expired, and logged-out.
- [ ] Show disconnected/reconnecting states without fake values.
- [ ] Update HUD panels to support empty/loading/locked states.
- [ ] Add real-server smoke path.
- [ ] Keep old JS WebSocket fixture only as protocol unit/smoke fallback.

## Abuse And Safety Checklist

- [ ] Password input is never logged to command log.
- [ ] Auth errors do not reveal whether email exists.
- [ ] Client cannot manually enter player id.
- [ ] Logout clears client gameplay state.
- [ ] Auth-expired WebSocket close clears gameplay state and stops reconnect
      until login.
- [ ] Failed WebSocket connect does not restore demo state.
- [ ] Demo mode is visibly and technically separated from real mode.
- [ ] Client does not trust player id, account id, role, or session token from
      browser storage.

## Tests

- [ ] Reducer initial state has no fake gameplay values.
- [ ] Login success opens WebSocket.
- [ ] Logout closes WebSocket and clears game state.
- [ ] Session restore loads authenticated shell.
- [ ] Expired session restore returns auth shell with no stale gameplay.
- [ ] WebSocket auth rejection/close clears stale gameplay state.
- [ ] Reconnect after transient close does not use demo fallback.
- [ ] `?demo=1` state is isolated from default real mode.
- [ ] Unauthenticated page shows auth panel, not game HUD data.
- [ ] Failed login shows safe error.
- [ ] Browser smoke confirms no demo text/state in default mode.

## Done Criteria

- Default client no longer appears as a fake live game.
- Browser can login to the real Go server.
- Browser can connect to `/ws` after login.
- Logout returns to auth shell and clears gameplay state.
- Tests and browser smoke pass.
