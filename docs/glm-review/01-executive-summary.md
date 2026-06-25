# §1 — Executive Summary

## Overall assessment

The codebase is a **server-authoritative** multiplayer space game with a genuinely
strong anti-cheat posture: client identity is never trusted (`rejectTrustedPayload`
blocklist), damage/positions/amounts are computed server-side, int64-only money with
positive-amount validation, per-operation idempotency, visibility-gated interactions,
and cookie-based auth with no JS-readable tokens. The domain decomposition is clean
(small, focused files: `wallet_service.go`, `loot_pickup_handler.go`, etc.) and the
build + `go vet` pass. There is substantial test coverage (~1,750 test functions).

However, the project is **not yet safe for real concurrent players**. The dominant
risks fall into four buckets:

1. **Simulation correctness under concurrency.** A single global `runtime.mu`
   (`internal/game/server/runtime.go:107`) serializes *all* gameplay commands *and*
   the entire per-tick AOI sweep, while `Worker.Tick()` is invoked from **17+ request
   handler call sites** in addition to the 50 ms background loop. This both (a)
   inflates the NPC-AI/spawn tick rate based on client command throughput and
   (b) sets a hard single-core scalability ceiling. A missing `recover()` in the tick
   goroutine means any panic there kills the whole process.

2. **Economy/money safety under failures.** Wallet/inventory/market mutations perform
   DB I/O *while holding the in-memory service mutex*, and several rollback paths only
   restore in-memory state without reversing already-committed DB writes. The result
   is balance-vs-DB desync and potential permanent fund loss on partial failures
   (market buy, auction buy-now).

3. **Two "always-on" gameplay defaults that silently break balance.** `combat.NewService(clock, nil)`
   (`runtime.go:1000`) and `loot.NewService(loot.Config{…})` (`runtime.go:990`) both
   receive a **nil RNG**. Combat always hits (Accuracy/Evasion are inert) and every NPC
   drops every loot row at maximum quantity. Only `deathService` receives the real RNG.

4. **Movement anti-cheat is per-command only.** A 1,200-unit move is allowed every
   75 ms (`runtime.go:46,50`) with no cumulative speed/distance tracking → an effective
   ~16,000 units/s where intended speed is ~120 u/s (a ~133× speed hack).

These are fixable with focused changes; none require a rewrite. The architectural
skeleton (transaction ledger, outbox, idempotency, AOI/fog, spatial index) is sound.

## Critical issues at a glance

| # | Sev | Issue | Primary location |
|---|-----|-------|------------------|
| C1 | **CRITICAL** | No `recover()` in the background tick goroutine → any panic crashes the server | `internal/game/server/runtime.go:1336` |
| C2 | **CRITICAL** | Combat RNG is `nil` → 100% hit rate; Accuracy/Tracking/Evasion stats inert | `internal/game/server/runtime.go:1000` → `internal/game/combat/service.go:281` |
| C3 | **CRITICAL** | Loot RNG is `nil` → every NPC drops every row at max quantity | `internal/game/server/runtime.go:990` → `internal/game/loot/service.go:690` |
| C4 | **HIGH** | `Worker.Tick()` invoked from 17+ handler call sites → variable, client-amplifiable sim rate | `handlers.go:307,361,478`; `runtime_sessions.go:55,70`; `portal_handlers.go:220,309,345`; `death_respawn.go:98`; `death_events.go:59`; `runtime_maps.go:80,186`; `runtime_players.go:113` |
| C5 | **HIGH** | Single global `runtime.mu` serializes all commands + full per-tick AOI sweep (hard scalability ceiling) | `runtime.go:107`; `runtime_world_snapshot.go:191` |
| C6 | **HIGH** | No cumulative speed/distance anti-cheat; 1,200 u per 75 ms move = ~133× speed hack | `handlers.go:406`; `runtime.go:46,50` |
| C7 | **HIGH** | Login timing oracle → email enumeration (no dummy PBKDF2 on not-found) | `internal/game/auth/service.go:172-185` |
| C8 | **HIGH** | Rate-limiter `buckets` map never pruned → unbounded memory growth (~90 entries/player, permanent) | `internal/game/realtime/rate_limiter.go:42,117` |
| C9 | **HIGH** | Per-tick broadcast fan-out is O(sessions × connections), synchronous in the tick goroutine; no session→conn index | `internal/game/server/transport.go:362-378`; `runtime.go:1354` |
| C10 | **HIGH** | DB I/O performed under wallet/inventory/market mutexes → connection-pool exhaustion & global serialization | `economy/wallet_service.go:615-620`; `economy/inventory_move.go:189`; `market/service.go:339-390` |
| C11 | **HIGH** | No connection-pool limits on `sql.DB` (no `SetMaxOpenConns` in production) | `internal/game/contentdb/db.go:22-30` |
| C12 | **HIGH** | In-memory rollback in market buy / auction buy-now does not reverse committed DB writes → fund loss + balance/DB desync | `market/service.go:331-335`; `auction/service.go:277-299` |
| C13 | **MED** | Auth brute-force/lockout tracker is in-memory, email-keyed only, no IP dimension; no HTTP-level limiter wired | `auth/attempts.go:11-15,44-49`; `server/server.go:87-98` |
| C14 | **MED** | `AllowMissingOrigin=true` disables the only CSRF/WS-origin defense | `auth/origin.go:25-29`; `server/transport.go:58-62` |
| C15 | **MED** | Reconnect has no backoff/jitter/max-retries → reconnect storm (client side) | `client/src/app/client-app-handlers.ts:165-175` |

Detailed analysis and fixes for each are in the themed files. The consolidated
fix plan is in `08-critical-fix-plan.md`.
