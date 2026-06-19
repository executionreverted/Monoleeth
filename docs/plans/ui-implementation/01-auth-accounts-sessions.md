# Phase 01: Auth, Accounts, Sessions, And Admin Seed

## Status

- State: Completed and wired through `cmd/game-server`
- Owner: Auth/session boundary
- Depends on: existing foundation ids/errors/contracts
- Unlocks: authenticated WebSocket, real player ownership, admin tools

## Goal

Add a real mail/password account system and server-owned session lifecycle so
the browser can log in, reconnect, log out, and open a WebSocket without ever
supplying trusted player identity in gameplay payloads.

## Source Specs

Read before implementation:
- `docs/plans/modules/15-api-events-errors.md`
- `docs/plans/modules/16-testing-observability-balancing.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`
- `internal/game/foundation`
- current `client/src/net` and `client/src/protocol`

If adding or upgrading password hashing, cookie, database, router, or migration
libraries, fetch current docs through Context7 before implementation.

## Server Ownership

The server owns:
- `account_id`
- `player_id`
- email normalization
- password hash
- session id
- session expiry
- admin role
- account/player bootstrap state

The browser may send:
- email
- password
- display/callsign during registration, if registration is enabled

The browser must never send:
- account id as authority
- player id as authority
- role/admin flag
- password hash
- session expiry

## MVP Auth Shape

Recommended first slice:
- account registration endpoint for local/dev playtesting
- login endpoint
- logout endpoint
- current session endpoint
- admin seed command or startup seed using environment inputs
- session cookie with `HttpOnly`, `SameSite=Lax`, secure flag when HTTPS
- WebSocket handshake reads the same session cookie

Production-hardening can come later, but the MVP must still hash passwords and
avoid logging secrets.

## Cookie, CSRF, And Origin Posture

Cookie-authenticated routes must have an explicit browser security posture:
- restrict CORS to configured first-party origins; do not use wildcard CORS
  with credentials
- require same-origin or configured allowed-origin checks on every state-changing
  auth POST, including `logout`
- use a CSRF token or same-origin-only deployment posture documented in server
  config; local dev exceptions must be explicit
- validate `Origin` on WebSocket upgrades to prevent cross-site WebSocket
  hijacking
- reject WebSocket upgrades before sending any gameplay state when the cookie is
  missing, expired, revoked, or from a disallowed origin

## Suggested Packages

```text
internal/game/auth/
internal/game/auth/password.go
internal/game/auth/session.go
internal/game/auth/store.go
internal/game/auth/http.go
internal/game/auth/seed.go
cmd/game-server/
```

Use in-memory stores only if the phase explicitly documents the loss of data
after restart. Prefer a small repository interface so durable storage can be
added without rewriting handlers.

Implemented MVP note:
- `internal/game/auth` now provides the volatile in-memory auth repository,
  PBKDF2-SHA256 password hashing, hashed-at-rest opaque session tokens,
  auth HTTP handlers, same-origin/allowed-origin checks, admin env seeding, and
  resolver helpers for the future WebSocket upgrade.
- The in-memory repository is intentionally process-local and loses accounts and
  sessions on restart. Phase 02 can wire it into `cmd/game-server`; later
  storage can replace the `auth.Store` interface without changing handlers.

## HTTP Endpoints

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/logout
GET  /api/session
```

Registration payload:

```json
{
  "email": "pilot@example.com",
  "password": "not-logged",
  "callsign": "Frontier-01"
}
```

Login payload:

```json
{
  "email": "pilot@example.com",
  "password": "not-logged"
}
```

Session response:

```json
{
  "authenticated": true,
  "account": {
    "email": "pilot@example.com",
    "admin": false
  },
  "player": {
    "callsign": "Frontier-01"
  },
  "server_time": 182736123
}
```

Do not expose password hash, account internal metadata, session token, or hidden
player state.

## Session Lifecycle Contract

Session tokens must be:
- cryptographically random opaque values
- stored as hashes server-side, not raw reusable tokens
- looked up on every authenticated HTTP request and WebSocket upgrade
- revocable by server-side session id
- rotated on login and on any explicit renewal flow
- bounded by `expires_at` and optional idle timeout policy

Logout invalidates the server session before clearing the cookie. Existing
WebSocket connections using that session must either close or reject further
commands once revocation or expiry is observed by the gateway.

## WebSocket Resolver Contract

The WebSocket session resolver returns only server-owned context:

```json
{
  "account_id": "acc_123",
  "player_id": "player_123",
  "session_id": "sess_123",
  "expires_at": 182736123,
  "roles": ["admin"]
}
```

`roles` may be empty. Admin status comes from roles, never from the browser
payload. Denials must use safe stable errors such as `ERR_AUTH_REQUIRED`,
`ERR_SESSION_EXPIRED`, `ERR_SESSION_REVOKED`, or `ERR_ORIGIN_DENIED` without
leaking whether an email/account exists.

## Admin Seed

Add a reproducible admin seed path:
- reads `GAME_ADMIN_EMAIL`
- reads `GAME_ADMIN_PASSWORD`
- reads optional `GAME_ADMIN_CALLSIGN`
- creates account if missing
- updates role safely if account exists
- never logs the password
- fails loudly if seed is requested without required inputs

Document local usage in `README` or a phase note. Do not commit real admin
credentials.

Local env seed flow for the Phase 02 server entrypoint:

```bash
GAME_ADMIN_EMAIL=admin@example.com \
GAME_ADMIN_PASSWORD='replace-with-local-secret' \
GAME_ADMIN_CALLSIGN=Admin \
go run ./cmd/game-server
```

`GAME_ADMIN_EMAIL` and `GAME_ADMIN_PASSWORD` must be supplied together when the
seed is requested. The password is hashed and never logged or serialized.
`GAME_ADMIN_CALLSIGN` is optional and defaults to `Admin`. `cmd/game-server`
wiring is owned by Phase 02; Phase 01 provides `auth.Service.SeedAdminFromEnv`
and `.env.example` placeholders so the server can call the seed hook without
hard-coded credentials.

## TODO

- [x] Define account, player profile, session, and role models.
- [x] Add email normalization and validation.
- [x] Add password hashing and verification.
- [x] Add session creation, expiry, renewal posture, and logout invalidation.
- [x] Store hashed session tokens and support server-side revocation lookup.
- [x] Rotate session tokens on login and any explicit renewal flow.
- [x] Add auth store/repository interfaces.
- [x] Add in-memory or durable MVP store implementation.
- [x] Add admin seed command/startup option.
- [x] Add HTTP handlers for register/login/logout/session.
- [x] Add CORS/CSRF/same-origin posture for cookie-authenticated POSTs.
- [x] Add safe public response models.
- [x] Add WebSocket session resolver adapter.
- [x] Add WebSocket allowed-origin validation.
- [x] Add client-safe auth error codes.
- [x] Add auth rate-limit posture for login/register.
- [x] Document local admin seed flow.

## Abuse And Safety Checklist

- [x] Login never logs password.
- [x] Password hashes are not serialized.
- [x] Session token/cookie is not returned in JSON unless explicitly designed.
- [x] Wrong email and wrong password use the same public error shape.
- [x] Login failures can be rate-limited.
- [x] Logout invalidates the server session.
- [x] Expired session cannot open WebSocket.
- [x] Revoked session cannot continue sending WebSocket commands.
- [x] Cross-site WebSocket upgrades are denied by origin policy.
- [x] Cookie-authenticated POSTs have explicit CSRF/same-origin protection.
- [x] Admin seed cannot create a weak default silently.
- [x] Client cannot choose admin role.

## Tests

- [x] Password hash verifies correct password.
- [x] Password hash rejects wrong password.
- [x] Register creates account and player profile once.
- [x] Duplicate email is rejected safely.
- [x] Login creates a valid session.
- [x] Logout invalidates session.
- [x] Session tokens are stored hashed at rest.
- [x] `GET /api/session` returns authenticated public shape.
- [x] Expired session is rejected.
- [x] Revoked session is rejected by HTTP and WebSocket resolvers.
- [x] Missing session is rejected by WebSocket resolver.
- [x] Valid resolver output includes account id, player id, session id,
      expiry, and roles from server state.
- [x] Disallowed WebSocket origin is rejected.
- [x] Cookie-authenticated logout cannot be triggered cross-site.
- [x] Admin seed creates/updates admin role without logging secrets.
- [x] WebSocket session resolver maps session id to server-owned player id.

## Done Criteria

- Browser can register or login with email/password.
- Admin account can be seeded without committed secrets.
- Session endpoint works.
- Logout works.
- Authenticated session can be resolved by the future WebSocket gateway.
- Tests cover positive, negative, duplicate, expiry, and admin seed paths.
- `go test ./...` and `git diff --check` pass.
