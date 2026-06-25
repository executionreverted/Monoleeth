# §10 — Security, Anti-Cheat & Abuse Prevention

## 10.1 Auth — strong baseline, with specific gaps

**Finding 10.1 — Login timing oracle → email enumeration (C7)** — *HIGH*
`internal/game/auth/service.go:172-185`: when `AccountByEmail` returns `ErrAccountNotFound`,
`Login` returns immediately via `recordAuthAttemptFailure`. When the account exists it runs a
210k-iteration PBKDF2 (`VerifyPassword`). The ~100 ms gap distinguishes valid from invalid
emails — a classic user-enumeration oracle. **Fix:** on the not-found path, run a dummy
PBKDF2 verify against a pre-computed hash (constant-time) so both paths take ~the same time.

**Finding 10.2 — Password hashing: PBKDF2-SHA256 @210k, constant-time compare** — *GOOD, with caveats*
`auth/password.go:13-19,40-60,88`: salt+key from `crypto/rand`; `subtle.ConstantTimeCompare`;
rejects NUL/CR/LF. Currently within OWASP guidance.
- **Caveat A — no max password length (PBKDF2 CPU DoS)** — *MED* (`password.go:104-112`):
  only `len >= 8` enforced. PBKDF2 iterates HMAC over the whole password each round, so a
  multi-MB password is an amplification DoS vector. **Fix:** cap at e.g. 1024 bytes.
- **Caveat B — iteration count read from stored hash at verify; no floor** — *MED*
  (`password.go:72-84`): if an attacker can write/replace a `PasswordHash`, they set
  `iterations=1` to neutralize hashing. **Fix:** enforce a minimum-iteration floor at verify;
  prefer argon2id/bcrypt for the next rotation.

**Finding 10.3 — Session token storage is correct (hash-only)** — *GOOD*
`auth/session.go:91,99-107`, `store.go:124-128`: only `TokenHash` (SHA-256) persisted; raw
token never stored. `token.go`: 32-byte session tokens (256-bit) from `crypto/rand`, base64url.
Entropy is sufficient. `createSession` mints a fresh token per login (no fixation), retries on
duplicate.
- **Caveat — no concurrent-session cap; old sessions stay valid 24h** — *LOW*: a stolen old
  cookie remains valid for the full TTL even after a new login. **Fix:** optionally invalidate
  prior sessions on login, or cap concurrent sessions per account.

**Finding 10.4 — Cookie flags: HttpOnly+SameSite=Lax always; Secure is config-gated** — *MED*
`auth/http.go:169-184`: `HttpOnly: true`, `SameSite=Lax` always on. `Secure` is config-driven
(`HTTPConfig.CookieSecure`), enforced only in production via a startup check
(`server/config.go:219-220`). A misconfigured staging env that forgets the flag issues
non-Secure cookies. **Fix:** also consider `__Host-` prefix + HSTS/nosniff/no-store headers
on auth responses (currently absent).

## 10.2 CSRF / Origin

**Finding 10.5 — No CSRF token; defense is Origin + SameSite=Lax only** — *MED*
`auth/http.go:88,105,122`: Register/Login/Logout pass `requireOrigin=true`; `GET /api/session`
passes `false`. Reasonable, **but only as strong as the Origin check**:
- **`AllowMissingOrigin` disables the only Origin backstop (C14)** — *HIGH*:
  `auth/origin.go:25-29`: when true (env `GAME_ALLOW_MISSING_ORIGIN`), requests with **no**
  `Origin` bypass the check entirely. Given CSRF relies entirely on this check, enabling it in
  production reopens CSRF for Register/Login/Logout. The WS accept uses `InsecureSkipVerify`
  (`transport.go:58-62`) *because* origin was checked earlier — but with `AllowMissingOrigin`,
  a no-Origin WS connects. **Fix:** never set `AllowMissingOrigin=true` in production (add a
  startup guard like the `CookieSecure` one); consider an actual CSRF token for state changes.
- **`requestOrigin` trusts `r.Host`** — *LOW* (`origin.go:81-87`): `Host` is client-supplied.
  In deployments where the upstream proxy doesn't pin Host, same-origin spoof is possible.
  Depends on edge config.

## 10.3 Brute force / rate limiting

**Finding 10.6 — Auth attempt tracker is in-memory, email-keyed only, no IP dimension (C13)** — *HIGH*
`auth/attempts.go:11-15,44-49,158-162`: `MaxFailures=3`, `Window=5min`, `Lockout=1min`,
keyed by SHA-256(email) only. Issues:
1. **Account-lockout DoS** — an attacker spamming wrong passwords locks out the *legitimate*
   user for 1 min, repeatedly (lockout resets the failure counter to 0, `attempts.go:112`,
   no escalation).
2. **Credential stuffing unthrottled** — each target email has its own bucket, so trying many
   accounts is not slowed.
3. **Registration bypassed** — `AuthAttemptRegister` is email-keyed; unique emails per request
   bypass the register limit.
4. **Process-local** — multi-replica gives `N × 3` attempts/window/account. `NewService`
   installs `InMemoryAuthAttemptTracker` by default (`service.go:82-85`).
5. **No HTTP-level IP limiter wired** — the mux in `server.New` (`server.go:87-98`) is bare;
   the `strict_auth_attempts` posture is documented (`http.go:19-42`) but not enforced.

**Fix:** add an IP/ASN dimension (shared store — Redis — for multi-replica), escalate lockout
duration on repeat, rate-limit by IP at the HTTP edge (middleware), and key registration by IP+email.

**Finding 10.7 — Realtime rate limiter: per-op token buckets, but process-local** — *MED*
`realtime/rate_limiter.go:57-62,99-130`: good per-op posture (combat 8/250ms, loot 6/500ms,
scan 2/5s). Same multi-instance caveat as 10.6 (`:70-71`). **Also:** nil-limiter short-circuits
to allow-all (`:100-102`); verify `Runtime` always wires one in prod (`config.go:75`
`disableRealtimeLimiter`). **Move-rate-limit** (`runtime.lastMove[playerID]`, `handlers.go:410`)
is also process-local.

## 10.4 Anti-cheat posture — genuinely strong

This is the codebase's strongest area. The server does not trust the client for any gameplay
truth:

**Finding 10.8 — Identity never comes from payload** — *GOOD*
`realtime/gateway.go:107-119`: `ResolveSession` is the *only* source of `PlayerID`/`WorldID`/
`ZoneID`; resolver "must not read identity from request payloads" (`gateway.go:20-21`).
Transport-session vs resolver-session mismatch is rejected (`gateway.go:112-118`).

**Finding 10.9 — `rejectTrustedPayload` blocklist is the central anti-cheat gate** — *GOOD*
`server/handlers.go:21-101,526-602`: every gameplay handler starts with
`rejectTrustedPayload(request.Payload)`. The blocklist includes `player_id`, `session_id`,
`account_id`, `admin`, `roles`, `damage`, `speed`, `xp`, `rank`, `wallet_amount`, `balance`,
`loot`, `seller_player_id`, `buyer_player_id`, `current_bid`, `winning_player_id`, `map_id`,
`transfer_token`, etc. The recursive walker (`findTrustedPayloadKey`, `:580`) catches nested
spoofing. A client cannot declare damage dealt, position speed, or another player's identity.

**Finding 10.10 — Position/movement is server-recomputed** — *GOOD (with the C6 gap)*
`handlers.go:388-414`: ship-can-move, map bounds (`ValidateActivePosition`), max-distance
from server-known position, per-player move rate. Position is never trusted from the client —
the worker recomputes it. **The gap is the speed hack (C6, §5.1).**

**Finding 10.11 — Combat is fully server-validated** — *GOOD (with the C2 RNG gap)*
`combat/service.go:214-267`: both actors exist & alive, same world/zone, `visibility.CanInteract`
(hidden-target rejection), weapon range, attack policy, cooldown, energy cost. Damage computed
entirely server-side; client sends only intent. **The gap is nil RNG → always hit (C2).**

**Finding 10.12 — Amounts are int64-only, positive-bounded** — *GOOD*
`foundation/amounts.go:115-123`: `validatePositiveAmount` rejects `<=0` and `> MaxAmount`
(1e12). `Money`/`Quantity` are int64-only (no float). All economy entry points go through
constructors.

**Finding 10.13 — Visibility gates every interaction; generic error prevents info leak** — *GOOD*
`visibility.go:243-248`: `CanInteract` returns a single generic `ErrNotVisible` for
hidden/out-of-range/cross-zone. `abuse_coverage.go` catalogs the required tests.

## 10.5 Client-side anti-cheat surface (cross-ref)

**Finding 10.14 — Client gates are advisory only; server must independently enforce** — *NOTE*
Client combat gates (`laserActionState`, cooldown, capacitor) at
`client/src/ui/hud-render-panels.ts:709-752` are UX-only; a modified client bypasses all of
them. The server does enforce (10.11). **Specific reminders:**
- **Shield-repair tick is client-gated by a hardcoded module ID** — *MED*
  (`client/src/app/client-app-handlers.ts:809-833`): the client decides whether to *send*
  `repair.shield_tick` based on a hardcoded `shield_generator_t1` check. A modified client
  can spam repair ticks without the module. The server must validate module possession,
  state, and durability on every `repair.shield_tick`. Verify server-side enforcement.
- **Cooldown defaults to "ready now" when server omits `cooldown_ready_at_ms`** — *LOW*
  (`client/src/state/reducer-events.ts:144-157`): client re-enables the button immediately;
  server still enforces the real cooldown, but the client will fire requests the server
  rejects (log spam, perceived lag).

**Finding 10.15 — Smoke-state publisher leaks full game state (incl. auth/admin) to `window`** — *HIGH (if shipped)*
`client/src/app/client-app-handlers.ts:724-789`: gated by `?smoke` URL param, a 50 ms interval
publishes `__SPACE_MORPG_SMOKE_STATE__` with `wallet`, `inventory`, `adminInspection`, and
`auth` (account/session/admin flags). **If this param ships to production**, any third-party
script or extension reads the entire state including admin inspection data and auth info.
**Fix:** strip this code path from production builds (build-time flag), or gate it behind a
non-production origin/feature flag.

**Finding 10.16 — Reconnect has no backoff/jitter/max-retries (reconnect storm)** — *HIGH (client)*
`client/src/app/client-app-handlers.ts:165-175`: `scheduleReconnect` retries every fixed
750 ms indefinitely (`shouldReconnect` only checks auth/session/intent). On a sustained
outage or flapping socket, every connected client hammers reconnect every 750 ms.
**Fix:** exponential backoff with jitter + a max-retries ceiling.

**Finding 10.17 — No token in JS; cookie-based auth with `credentials: include`** — *GOOD*
`client/src/auth/auth-client.ts:47-55`: no `localStorage`/`sessionStorage`/`document.cookie`
usage anywhere in `client/src/`. Tokens never reach JS. (Confirmed by grep.) Client-side
`forbiddenPayloadKeys` blocklist (`protocol/envelope.ts:243-311`) defense-in-depth filters
server payloads too.

## 10.6 Seed / admin accounts

**Finding 10.18 — Seed re-rotates admin password + re-grants admin role on every startup** — *MED*
`auth/seed.go:72-79`: if the account exists, seed overwrites `PasswordHash`, merges `RoleAdmin`,
and updates — every startup. Anyone who controls the env (CI/container spec/k8s secret) can
silently rotate the admin password and regain admin on an existing account. No audit log; no
"seed only once" guard. `EnvAdminPassword` is held in process env (`config.go:67,125-130`),
visible via `ps`/container inspect. **Fix:** seed-once guard (track a `seeded` marker), audit
log, and use a secret manager rather than env for the admin password.
