# §2 Game Server Architecture · §3 Networking & Protocol · §4 State Synchronization

## §2 — Game Server Architecture

### 2.1 Wiring & lifecycle (`cmd/game-server/main.go`, `internal/game/server/server.go`, `runtime.go`)

`server.Run` (`server.go:118-145`) starts one HTTP goroutine and the runtime tick
goroutine (`runtime.go:1336`). Dependency wiring in `NewRuntime`
(`runtime.go:724-1309`) is mostly sound: on each construction error, earlier-opened
stores are closed (e.g. `:788-808`), avoiding handle leaks on partial failure. The
dependency graph is constructor-injected (catalogs → services → runtime → gateway),
which is clean and testable.

**Finding 2.1 — Unclean shutdown on the HTTP-server-error path** — *MEDIUM*
`server.go:142-143`: when `ListenAndServe` returns (bind error, etc.), `Run` returns
**without** calling `server.Shutdown()` or `server.runtime.Close()`. The tick goroutine
(`runtime.go:1336`) is only cancelled via `ctx.Done()`, which isn't the path taken here,
so store closers in `Runtime.Close` (`runtime.go:334-379`) are skipped. Mitigated only
because `main.go` then `log.Fatalf`s, but DB handles / outbox leases get no graceful
close. **Fix:** in the `errc` branch, call `server.Shutdown(ctx)` then
`server.runtime.Close()` before returning.

**Finding 2.2 — Tick loop silently never starts if seeding produced no maps** — *LOW*
`runtime.go:1332`: `if runtime == nil || len(runtime.mapInstances) == 0 { return }`.
A silent seeding failure elsewhere makes the world never tick with no error surfaced.
**Fix:** log a warning when this guard triggers in a non-dev config.

### 2.2 The monolithic global lock (the central architectural finding)

`internal/game/server/runtime.go:107` declares `mu sync.Mutex` guarding ~25 maps
(`:135-216`): `players`, `stealthBaseSpeeds`, `eventSeq`, `eventRings`, `sessions`,
`sessionLocations`, `sessionEpochs`, `nextSessionEpoch`, `lastMove`, `queuedEvents`,
`activeTransfers`, `activeScanPulses`, `activePlanetClaims`, `portalCooldowns`,
`portalAttempts`, `playerProtections`, `pendingRespawns`, `combatLocks`,
`shieldRepairTicks`, `repairAttempts`, `repairQuotes`, `shopPurchases`,
`scanCooldowns`, `scanCapacitorSpends`, `nextPlayerEntity`, plus every `mapInstance`'s
`ActiveSessions`/`LastAOI`. **This is the single contention point for the whole server.**

**Finding 2.3 — One mutex serializes all commands and the per-tick AOI sweep** — *HIGH*
`runtime_world_snapshot.go:185-225`: after ticking, the loop holds `runtime.mu` while
computing AOI diffs for **every session on every map**
(`aoiSnapshotForPlayerLocked` does a radius query + visibility computation per session,
`:210-222`). Every gameplay handler also needs `runtime.mu` (e.g. `handleMoveTo:293,309`,
`handleSessionSnapshot:247`). So the **entire server's command throughput stalls for the
duration of the per-tick AOI sweep**. Throughput ≈ `1 / (avg handler latency)`,
process-wide. **Why it matters:** this is the hard scalability ceiling — you cannot add
players beyond what one mutex + one AOI sweep can carry per 50 ms. **Fix:** shard runtime
state per `mapInstance` (each instance owns its own mutex + AOI computation), so the tick
sweep runs per-map in parallel and commands only contend with their own map. Also consider
moving AOI diff computation off the locked section (snapshot under lock, diff unlocked).

### 2.3 Per-map instances are immutable — GOOD

`runtime.go:129-133`: `mapInstances`/`mapTickInstances` are populated at boot
(`:863-887`) and never mutated, and each `mapInstance.Worker` is fixed. This avoids a
whole class of map-lifetime races. (Minor: `sortedMapInstancesLocked` re-sorts
`mapInstances` every tick at `runtime_world_snapshot.go:312-334` — O(M log M)/tick.)

### 2.4 Secondary locks

**Finding 2.4 — `tickMu` is decorative; does not gate command-driven ticks** — *MED*
`runtime.go:112` `tickMu sync.Mutex` is taken only by `tickAndCollectAOIEvents`
(`runtime_world_snapshot.go:186`). But every command handler that calls `Worker.Tick()`
does so **without** `tickMu` (see C4). So `tickMu` only prevents the background loop from
re-entering itself; it does not prevent a background tick interleaving with a
command-driven tick. **Fix:** either gate all tick entry points through `tickMu`, or
(preferable) remove command-driven ticks entirely (see C4 fix).

**Finding 2.5 — Lock ordering is consistent (runtime.mu → worker.mu); no deadlock found** — *GOOD*
Every nesting takes `runtime.mu` before `worker.mu`. Command handlers release `runtime.mu`
before taking `worker.mu` for `Tick` (`handlers.go:303→307`), so they don't nest.
Worker-internal commands never take `runtime.mu`.

---

## §3 — Multiplayer Networking & Protocol Design

### 3.1 Connection model (`transport.go`)

Two goroutines per connection: the read loop runs in the HTTP-handler goroutine
(`transport.go:90-130`), the write loop in a spawned goroutine (`:170-187`).
`readMessage` uses a fresh 30 s `SocketReadTimeout` context per `Read`
(`:253-257`); `writeLoop` uses a 10 s per-write context (`:196`). Shutdown coordination
via `done`/`writerDone`/`closeOnce` (`:34`,`222-237`) is correct; `waitForWriter`
(`:239-251`) has a timeout so `Shutdown` (`server.go:158-163`) can't hang.

**Finding 3.1 — Outbound queue is bounded & non-blocking (good slow-consumer design)** — *GOOD*
`transport.go:19,160,204-219`: 64-deep buffer; `enqueue` uses a non-blocking `select` with
`default` → on a full queue it returns `enqueueResultDroppedSlowClient`, records telemetry
(`:326-327`), and **disconnects** the slow client (`:216`). The broadcaster never blocks.
This is the correct pattern for a realtime game. **Note:** `writeText`/`writeTextMessage`
(`:315,318`) double-copies bytes (`append([]byte(nil), payload...)` on top of an earlier
copy) — minor GC pressure at high event volume.

**Finding 3.2 — Read loop is fully synchronous → per-client head-of-line blocking** — *MED*
`transport.go:90-130`: one goroutine reads a message, runs `Gateway.HandleRequest` (full
handler + DB + combat lease), writes the response, then fans out post-command events to
all affected sessions — all before reading the next frame. A slow handler stalls that
client's inbound queue, inflating input latency under handler contention (compounded by
the global lock, Finding 2.3). **Fix:** decouple request execution from the read loop
(e.g. dispatch handler to a worker pool; keep read loop only framing + dispatching).

### 3.2 Event fan-out has no session→conn index

**Finding 3.3 — `writeEventsToSession` scans all connections per delivery** — *HIGH*
`transport.go:362-378`: every event batch for a session does `server.conns.Range(...)`
over **every** live connection to find matches. Called from the tick sink
(`server.go:122-124`) once per session per tick, and from `serveWebSocket` per command
(`transport.go:127-129`). With N connections and M sessions updated per tick, fan-out is
**O(M·N)** — ~1M map iterations/tick at 1k players, on the world-tick goroutine. A
`sessionConnCounts` map exists (`server.go:25`) but tracks counts only, not pointers.
**Fix:** add `map[SessionID][]*clientConnection` under `connMu` and index it on
register/unregister; deliver directly. **Also pre-serialize shared broadcast events once
per tick** instead of per-connection (`transport.go:299-311` re-marshals per client).

### 3.3 Protocol envelope (`realtime/envelope.go`)

`RequestEnvelope` (`:521`) = `request_id, op, payload(RawMessage), client_seq, v`;
`EventEnvelope` (`:554`) = `event_id, type, payload, server_time, seq, v`. Decode uses
`DisallowUnknownFields()` (`:575`) and rejects trailing data (`:587`) — solid
anti-injection. Payload must be a JSON object (`:675`).

**Finding 3.4 — JSON-text-only framing, one WS message per event, no compression** — *LOW/MED*
For a DarkOrbit-style high-frequency position/AOI stream, full JSON per event over WS text
frames is bandwidth-heavy; position corrections and AOI updates are sent individually
(`transport.go:299-311`). **Fix (later):** batch per-session tick events into a single WS
frame, and/or add a binary position-update channel + permessage-deflate. Not a correctness
issue.

**Finding 3.5 — Version negotiation is hard-fail, no overlap window** — *LOW*
`envelope.go:604` accepts only `0` or `CurrentVersion`; anything else is rejected. Fine for
controlled deploys; a rolling upgrade with no overlap will hard-disconnect half the clients.
Document the contract or support a one-version grace window.

### 3.4 Rate limiting (`realtime/rate_limiter.go`)

Token bucket per `(sessionID, playerID, operation, posture)` with operation-specific
overrides (e.g. combat 8/250 ms, loot 6/500 ms, scan 2/5 s) and a default 30/100 ms.

**Finding 3.6 — Rate-limiter `buckets` map is never pruned → unbounded memory** — *HIGH*
`rate_limiter.go:42,117`: a new entry is inserted on first use and **never deleted**
(no `delete(limiter.buckets, …)` anywhere). Keyed by operation too — with ~90 registered
operations (`envelope.go:19-98`), each player creates up to ~90 permanent entries. Session
teardown (`detachSession`, `forgetSessionReplay`) never notifies the limiter. **Fix:** add
`ForgetSession(sessionID)`, call it on `detachSession`, and run a periodic sweeper evicting
buckets idle past 2× their refill window. Consider sharding the map by playerID to reduce
the global mutex contention.

**Finding 3.7 — Single global mutex on the hottest realtime path** — *MED*
`rate_limiter.go:112-113` serializes **every** realtime command across **all** sessions.
Under load this is a serialization point. **Fix:** shard by session/playerID, or use a
sharded map.

**Finding 3.8 — Rate-limited (429) responses are cached & replayed on retry** — *MED*
`gateway.go:160` wraps the limiter error with `retryable: true`; `cacheableResponse`
(`request_cache.go:199-201`) caches non-retryable errors only — but 429 *is* retryable, so
it gets cached and replayed on retry, returning a stale 429 even after tokens refill.
**Fix:** mark the rate-limit error non-cacheable, or special-case it in
`cacheableResponse`.

**Finding 3.9 — Process-local only; no cross-instance coordination** — *MED (ops)*
`rate_limiter.go:70-71`: documented limitation. With >1 gateway pod and sticky sessions
off, burst limits are effectively multiplied by pod count. **Fix:** wrap with a shared
store (Redis token bucket) for multi-replica, or guarantee sticky sessions at the edge.

---

## §4 — Real-Time State Synchronization

### 4.1 Event ring / replay (`runtime_event_ring.go`, `runtime_sessions.go`)

Bounded ring per session (cap 128, `runtime_event_ring.go:5`). Replay access is via
`*Locked` helpers under `runtime.mu` (`runtime_sessions.go:342-372`) — no internal lock,
consistent discipline. `forgetSessionReplay` (`runtime_sessions.go:128-134`) correctly
takes `runtime.mu`.

**Finding 4.1 — Replay gap is silent; no "resync required" signal** — *MED*
`runtime_event_ring.go:42-66`: `replayAfter` returns `(nil, false)` on a gap
(`lastSeq+1 < oldest`, `:51`). In `bootstrapEvents` this is handled at
`runtime_sessions.go:183-189` by just returning the fresh bootstrap set — **no indication
that intervening events were lost**. For a chatty session with a 128-capacity ring, a brief
network drop silently discards combat/loot/AOI events. **Fix:** emit an explicit
`event_resync_required` / force a full world snapshot when a gap is detected, so the client
can reconcile.

**Finding 4.2 — Reconnect delivers replay (old seq) before bootstrap (new seq) — non-monotonic** — *LOW/MED*
`runtime_sessions.go:186-188`: `result = append(result, replay...); result = append(result, events...)`.
Replay events carry old seqs while bootstrap (`session.ready`, `world.snapshot`) carry new
monotonic seqs. The client receives a high→low→high stream. **Fix:** document the client
contract (seq is for gap detection, not strict monotonicity), or skip replay when bootstrap
is issued and rely on snapshot reconciliation.

**Finding 4.3 — `eventSeq`/`eventRings`/`queuedEvents` not cleared on normal disconnect** — *LOW/MED*
`detachSession` (`runtime_sessions.go:111-126`) does **not** clear these; only
`forgetSessionReplay` (`:128`) does, and that's called only on terminal auth error
(`transport.go:102`) and map transfer (`runtime_maps.go:168` calls `ForgetSessionCache`,
not `forgetSessionReplay`). Normal disconnect leaves entries keyed by sessionID; if the
session never reconnects, they leak (bounded by 30 s read-timeout eventually dropping the
conn). **Fix:** sweep these on `detachSession` for fully-expired sessions.

### 4.2 Map-scoped event epoch tagging — allocation-heavy

**Finding 4.4 — Double JSON round-trip per map-scoped event** — *LOW*
`runtime_sessions.go:374-389` marshals payload → unmarshal to `map[string]any` → inject
`map_subscription_epoch`; then `filterEventsForActiveEpochLocked` (`:430-448`) re-parses
JSON per event via `eventMapSubscriptionEpoch` (`:456-481`). Each AOI/combat event is
JSON-marshalled/unmarshalled multiple times purely for epoch tagging. **Fix:** carry the
epoch as a typed field on the event struct instead of round-tripping through JSON.

### 4.3 Post-command fan-out happens in the actor's read goroutine

**Finding 4.5 — One player's command pays O(C) scan per affected observer session** — *MED*
`transport.go:122-129`: after a combat/loot command, `postCommandEventsBySession` returns
events for *multiple* sessions (nearby players see the AOI/combat diff) and the actor's
read loop synchronously calls `writeEventsToSession` for each — each scanning all
connections (Finding 3.3). **Fix:** combine the session→conn index (Finding 3.3 fix) with
moving fan-out off the read loop (Finding 3.2 fix).

### 4.4 Client-side reconciliation (cross-ref)

The client is well-disciplined: position is interpolated **only** along server-provided
`origin/target/started_at_ms/arrive_at_ms` (`client/src/state/movement.ts:44-50`), the
`visibleEntities` map is mutated only by server events, and rejected moves revert to server
truth (`client/src/state/reducer.ts:253-255`). No optimistic position. See
`06-security-anticheat.md` §10.8 for client-specific gaps (reconnect storms, pending-request
leaks).
