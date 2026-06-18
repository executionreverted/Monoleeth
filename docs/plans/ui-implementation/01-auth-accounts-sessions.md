# Phase 01: Auth, Accounts, Sessions, And Admin Seed

## Status

- State: Planned
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

## TODO

- [ ] Define account, player profile, session, and role models.
- [ ] Add email normalization and validation.
- [ ] Add password hashing and verification.
- [ ] Add session creation, expiry, renewal posture, and logout invalidation.
- [ ] Add auth store/repository interfaces.
- [ ] Add in-memory or durable MVP store implementation.
- [ ] Add admin seed command/startup option.
- [ ] Add HTTP handlers for register/login/logout/session.
- [ ] Add safe public response models.
- [ ] Add WebSocket session resolver adapter.
- [ ] Add client-safe auth error codes.
- [ ] Add auth rate-limit posture for login/register.
- [ ] Document local admin seed flow.

## Abuse And Safety Checklist

- [ ] Login never logs password.
- [ ] Password hashes are not serialized.
- [ ] Session token/cookie is not returned in JSON unless explicitly designed.
- [ ] Wrong email and wrong password use the same public error shape.
- [ ] Login failures can be rate-limited.
- [ ] Logout invalidates the server session.
- [ ] Expired session cannot open WebSocket.
- [ ] Admin seed cannot create a weak default silently.
- [ ] Client cannot choose admin role.

## Tests

- [ ] Password hash verifies correct password.
- [ ] Password hash rejects wrong password.
- [ ] Register creates account and player profile once.
- [ ] Duplicate email is rejected safely.
- [ ] Login creates a valid session.
- [ ] Logout invalidates session.
- [ ] `GET /api/session` returns authenticated public shape.
- [ ] Expired session is rejected.
- [ ] Admin seed creates/updates admin role without logging secrets.
- [ ] WebSocket session resolver maps session id to server-owned player id.

## Done Criteria

- Browser can register or login with email/password.
- Admin account can be seeded without committed secrets.
- Session endpoint works.
- Logout works.
- Authenticated session can be resolved by the future WebSocket gateway.
- Tests cover positive, negative, duplicate, expiry, and admin seed paths.
- `go test ./...` and `git diff --check` pass.
