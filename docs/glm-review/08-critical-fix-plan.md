# §16 — Critical Issues & Suggested Fix Plan

A prioritized, phased plan. Each item references the detailed analysis file and the
`file:line` to change. Phases are ordered by risk reduction and effort; each phase is
independently shippable.

---

## Phase 0 — Trivial one-line correctness fixes (do first, ~hours)

These are tiny changes with outsized correctness impact. No architectural work required.

### P0-1 — Wire RNG into combat and loot (C2, C3)
- `internal/game/server/runtime.go:1000` → `combat.NewService(clock, rng)`
- `internal/game/server/runtime.go:990` → `loot.Config{ RNG: rng, … }`
- `rng` already exists at `runtime.go:738-741`; it's currently passed only to `deathService`
  (`:1069`).
- **Test:** add an integration test asserting the runtime's combat & loot services have a
  non-nil RNG; add a statistical miss test for high-evasion targets. (§14.3)

### P0-2 — Add `recover()` to the tick goroutine (C1)
- `internal/game/server/runtime.go:1336-1361`: wrap the loop body:
  ```go
  for {
      select {
      case <-ctx.Done(): return
      case <-ticker.C:
          func() {
              defer func() {
                  if r := recover(); r != nil {
                      runtime.metrics.RecordTickPanic(...) // structured log + stack
                  }
              }()
              runtime.runDurableOutboxRealtimePumpTick()
              events := runtime.tickAndCollectAOIEvents()
              // ... fan-out ...
          }()
      }
  }
  ```
- Record, don't exit.

### P0-3 — Set DB connection-pool limits (C11)
- `internal/game/contentdb/db.go:22-30` (or a new `ConfigurePool(db, cfg)`):
  ```go
  db.SetMaxOpenConns(cfg.MaxOpenConns)   // e.g. 25–50
  db.SetMaxIdleConns(cfg.MaxIdleConns)
  db.SetConnMaxLifetime(cfg.ConnMaxLifetime)  // 30m
  db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)  // 5m
  ```
- Make config-driven; default to safe production values.

### P0-4 — Cap password length + iteration floor (§10.2)
- `internal/game/auth/password.go:104-112`: add `len <= 1024` cap.
- `internal/game/auth/password.go:72-84`: enforce a minimum-iteration floor at verify.

---

## Phase 1 — Anti-cheat & economy safety (high risk, low–medium effort)

### P1-1 — Cumulative speed/distance anti-cheat (C6)
- `internal/game/server/handlers.go:388-414` `validateMoveIntentLocked`:
  - Compute `allowed = effectiveSpeed * timeSinceLastAcceptedPosition * toleranceFactor`.
  - Reject `entity.Position.Distance(intent.Target) > allowed` (replace the flat
    `defaultMaxMoveDistance` 1200 check at `:406`).
  - Track per-player cumulative displacement; flag sustained over-speed in telemetry.
  - On violation, emit `position.corrected` to snap the client back.
- Validate against `settledMovementPositionAt(now)` not the stale stored position (§5.2).
- **Test:** §14.2 (the 16,000 u/s test must now fail to exceed ship speed).

### P1-2 — Stop command-driven simulation ticks (C4)
- Remove `Worker.Tick()` calls from request handlers; handlers should only `Submit` intent.
  Sites: `handlers.go:307,361,478`; `runtime_sessions.go:55,70`; `runtime_metrics.go:66`;
  `portal_handlers.go:220,309,345`; `death_respawn.go:98`; `death_events.go:59`;
  `runtime_maps.go:80,186`; `runtime_players.go:113`.
  - For paths that need immediate reflection (portal transfer, session bootstrap), add a
    **non-simulating** `Worker.FlushCommands()` that drains the mailbox and applies movement
    intents without advancing the spawner/aggro/tick counter.
- Only the background loop ticks; remove the now-decorative `tickMu` or repurpose it.
- **Test:** §14.1 (tick count ≈ wall-clock / tickInterval under concurrent commands).

### P1-3 — Make economy mutations atomic across services (C12)
- Convert market buy / auction buy-now / craft-start into a **single DB transaction** by
  threading a `*sql.Tx` (or a tx-aware context) through `DebitWallet`/`CreditWallet`/
  `SystemMoveItem`/`ReleaseReservation`, so a mid-orchestration failure rolls back all DB
  writes atomically. Files: `market/service.go:339-390`, `auction/service.go:277-299` (mirror
  `PlaceBid`'s `:169-198` snapshot/restore as an interim minimal fix),
  `crafting/service.go:360-364` (release reservation + refund on duplicate-job).
- **Interim (if the tx refactor is too big for one change):** add compensating DB writes
  (reversal `CreditWallet` with a distinct reference key) on every rollback path, and assert
  in-memory == DB after each operation in tests.
- **Test:** §14.4 (fault-injection: second wallet call fails → no net money created/destroyed).

### P1-4 — Fix login timing oracle (C7)
- `internal/game/auth/service.go:172-185`: on the not-found path, run a dummy PBKDF2 verify
  against a precomputed hash so both branches take ~equal time.

### P1-5 — Brute-force / rate-limit hardening (C13, C14)
- Add an IP/ASN dimension to `auth/attempts.go` backed by a shared store (Redis) for
  multi-replica; escalate lockout on repeat; rate-limit by IP at the HTTP edge (middleware in
  `server.New`).
- Add a startup guard forbidding `AllowMissingOrigin=true` in production (mirror the
  `CookieSecure` guard at `server/config.go:219-220`). `auth/origin.go:25-29`.
- **Client:** add exponential backoff + jitter + max-retries to reconnect
  (`client/src/app/client-app-handlers.ts:165-175`) — prevents reconnect storms (§10.16).

---

## Phase 2 — Scalability & memory (high effort, removes the hard ceiling)

### P2-1 — Shard runtime state per map instance (C5)
- Move each `mapInstance`'s `ActiveSessions`/`LastAOI`/AOI computation behind a **per-instance
  mutex**; the tick sweep then runs per-map (and can be parallelized). The global `runtime.mu`
  shrinks to coordinating cross-map concerns (sessions, transfers, global cooldowns).
- `runtime_world_snapshot.go:185-225`: snapshot under the per-instance lock, compute AOI diffs
  outside the lock where possible.
- Replace `reflect.DeepEqual` AOI diff (`aoi/snapshot.go:144`) with a version/dirty field
  (§8 Finding 8.5). Compute per-NPC metadata once per map per tick, not per-viewer (§8.6).
- **Test:** §14.7 (tick-time scaling benchmark as a regression gate).

### P2-2 — Session→connection index + pre-serialized broadcasts (C9)
- `internal/game/server/server.go:23-25` + `transport.go:362-378`: replace
  `conns sync.Map` + `sessionConnCounts` with `map[SessionID][]*clientConnection` under
  `connMu`; deliver directly (no O(N) scan). `transport.go:299-311`: serialize shared
  broadcast events once per tick and send bytes per client.

### P2-3 — Prune unbounded maps (C8, §13.3)
- `realtime/rate_limiter.go`: add `ForgetSession(sessionID)`, call on `detachSession`; add a
  periodic sweeper evicting idle buckets (2× refill window).
- `runtime.go:143,148,150,152,153,214`: add a janitor (per-tick or 60s) evicting expired
  cooldowns; cap idempotency maps (LRU); sweep per-player entries on `detachSession`.
- **Test:** §14.5 (long-run session simulation asserts bounded maps).

### P2-4 — Remove DB I/O from under service mutexes (C10)
- Wallet/inventory/market/progression: persist via a write-ahead mutation queue or transactional
  outbox reconciled async, so a slow DB doesn't serialize all economy ops. Always pass a
  request-scoped context (replace `context.Background()` at `wallet_service.go:612,619`).
- Pair with P0-3 (pool limits) so a burst can't exhaust connections even if a write stalls.

### P2-5 — Fixed-timestep tick + independent outbox pump (§7.2, §7.3)
- `runtime.go:1335,1342`: drain `ticker.C` in a loop summing debt; step in fixed
  `tickInterval` chunks capped at e.g. 5/frame (no silent skip, no catch-up storm).
- Move `runDurableOutboxRealtimePumpTick` (`runtime.go:1343-1346`) to its own goroutine/ticker.

---

## Phase 3 — Resync, observability, client hardening (medium risk)

### P3-1 — Replay-gap resync signal (§4.1)
- `runtime_sessions.go:183-189`: on a ring gap, emit an explicit `event_resync_required` and
  force a full world snapshot rather than silently dropping events.

### P3-2 — Make release gates enforce in CI (§12.7)
- Wire a CI job invoking `NewReleaseGateReport`/`NewCommandSecurityReviewReport` that fails the
  build if the catalog isn't satisfied.

### P3-3 — Strip `?smoke` state publisher from production builds (§10.15)
- `client/src/app/client-app-handlers.ts:724-789`: build-time flag; never ship the
  `__SPACE_MORPG_SMOKE_STATE__` publisher (incl. `auth`/`adminInspection`) to production.

### P3-4 — Verify server-side enforcement of client-gated actions (§10.14)
- `repair.shield_tick`: confirm the server independently validates module possession, state,
  and durability (the client gate at `client-app-handlers.ts:809-833` is advisory).
- Confirm `cooldown_ready_at_ms` omission doesn't cause log spam (consider always sending it).

### P3-5 — Auth logging redaction clarity (§12.2)
- `observability/auth_transition_log.go:24-25`: remove misleading `json:"-"` or truly redact
  `player_id`/`session_id` from `MarshalJSON`.

---

## Verification checklist (run before closing each phase)

Per `AGENTS.md`:
```bash
go test ./...
git diff --check
cd client && npm --cache /tmp/gameproject-npm-cache run check
```
Plus the new simulation/property tests added in §14 for the phase's risk area.

---

## Risk-reduction summary

| Phase | Fixes | Risk removed |
|-------|-------|--------------|
| 0 | C1, C2, C3, C11 + password hardening | Process crashes; broken combat/loot economy; DB pool exhaustion |
| 1 | C4, C6, C7, C12, C13, C14, client reconnect | Speed hack; tick amplification; email enumeration; fund loss; brute force/CSRF; reconnect storms |
| 2 | C5, C8, C9, C10, tick timestep | Scalability ceiling; memory leaks; O(N²) fan-out; economy serialization |
| 3 | resync, release gates, smoke strip, server-enforce checks | Silent desync; unenforced gates; client state leak; log clarity |

After Phase 0 + Phase 1, the server is safe to put behind real (small-scale) players behind
auth. Phase 2 is required before scaling beyond a few hundred concurrent players per map.
