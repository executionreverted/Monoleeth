# §6 — Concurrency, Goroutines & Race Conditions

This section consolidates the concurrency hazards found across runtime, worker, economy,
and transport. Data-corruption races are rare (lock discipline is mostly consistent);
the dangerous classes here are **semantic races** (double simulation advance, stale reads
across lock boundaries), **contention/deadlock-adjacent patterns**, and
**goroutine-lifecycle leaks**.

## 6.1 Concurrent `Worker.Tick()` double-advances the simulation

**Severity: HIGH (see C4, §7.1 Finding 7.1)**

`Worker.Tick()` is safe from *memory corruption* (it takes `worker.mu` at `worker.go:227`),
but it can run from a request handler and the background loop concurrently. Each call
independently runs `advanceMovement()` (advancing every entity by `speed * tickDelta`) and
increments `worker.tick`. A single move command can therefore produce 2+ movement advances
and inflate the tick counter; `tickDelta = 50ms` becomes meaningless as a rate limiter.
The worker's `Run()` loop (`worker.go:253-265`) is never used in production — all ticking
is event-driven from 17+ call sites. **This is the central concurrency correctness bug.**
Fix in §7.1.

## 6.2 Mailbox drain is not atomic with the tick

**Severity: MEDIUM**
`worker.go:225-228`:
```go
func (worker *Worker) Tick() TickResult {
    commands := worker.mailbox.Drain()   // mailbox mutex
    worker.mu.Lock()                      // worker mutex — separate lock
    defer worker.mu.Unlock()
```
`Drain()` (`commands.go:51-63`) snapshots+clears under the mailbox mutex; `Submit()`
(`commands.go:38-48`) can interleave between `Drain()` and `worker.mu.Lock()`. A command
submitted in that window is **orphaned until the next Tick**, adding latency. Worse, two
back-to-back `Tick()` calls from different goroutines: one drains all commands, the other
processes an empty set but still advances movement and increments the tick. **Fix:**
remove command-driven ticks (C4), or merge drain+tick under one critical section.

## 6.3 `FlushCommands` races with periodic tick

**Severity: MEDIUM**
`worker.go:347-356` `RefreshPlayerMovementPosition` does `Submit()` then `FlushCommands()`,
called from `syncPlayerCombatActorLocked` (`combat_loot_helpers.go:62`) under `runtime.mu`.
If the periodic tick runs `Worker.Tick()` concurrently, `FlushCommands` waits for
`worker.mu` then drains whatever accumulated — potentially mixing movement commands from
other goroutines with the refresh intent. **Fix:** same as 6.1.

## 6.4 Stale worker snapshot across `runtime.mu` release in move/stop handlers

**Severity: MEDIUM (re-entrancy window)**
`handlers.go:293-310` (move) and `:347-364` (stop):
```go
runtime.mu.Lock()
... validate, get instance ...
runtime.mu.Unlock()                  // :303
if err := instance.Worker.Submit(...); ...
result := instance.Worker.Tick()     // :307  — runs WITHOUT runtime.mu
runtime.mu.Lock(); defer runtime.mu.Unlock()   // :309
```
During the unlocked window the background tick can run `aoiDiffEventsForInstanceLocked`,
which may **delete the session from `instance.ActiveSessions`/`LastAOI`** if the router
location no longer matches (`runtime_world_snapshot.go:278-290`). Handlers defend with
`current != instance` (`:319`,`:373`) and re-read AOI state under the lock — so no
corruption — but the captured `instance` pointer can go stale mid-handler; the movement
intent is still processed by a now-detaching instance. Functionally tolerated, fragile.
**Fix:** drop `runtime.mu` only for the worker submission and re-validate the instance
identity after re-locking before mutating runtime maps.

## 6.5 DB I/O under service mutexes → global serialization + pool exhaustion

**Severity: HIGH (C10, detail in §9)**
`economy/wallet_service.go:615-620` (`persistWalletMutationLocked` under `service.mu`),
`economy/inventory_move.go:189`, `economy/inventory_service.go:328`,
`market/service.go:339-390`, `progression/service.go:102-104`. A slow/stalled DB write
blocks **every** wallet/inventory/market/progression operation globally, and (with no
`SetMaxOpenConns`) can exhaust PG connections. Repositories also use `context.Background()`
(`wallet_service.go:612,619`) so writes can't be cancelled on disconnect/shutdown — a hung
write holds the mutex indefinitely. **Fix:** persist outside the in-memory lock with a
write-ahead mutation queue, or use a transactional outbox and reconcile async; always pass
a request-scoped context.

## 6.6 Nested locks (cargo.mu → inventory.mu) during DB I/O

**Severity: MEDIUM**
`economy/cargo_service.go:121-123,219-221`: cargo locks its own mutex then inventory's.
`ReservationService` does the same (`reservation.mu → inventory.mu`). Lock ordering is
consistent (outer → `inventory.mu` always last) so **no deadlock**, but holding two
mutexes during DB I/O compounds 6.5's contention. **Fix:** remove DB I/O from under the
locks (see 6.5 fix).

## 6.7 Combat actor position desyncs under concurrent tick

**Severity: MEDIUM (see §5 Finding 5.6)**
`combat/service.go:248` validates range against positions projected at sync time
(`combat_loot_helpers.go:97-101`), while the periodic tick advances NPC positions outside
`runtime.mu` (`runtime_world_snapshot.go:189`). A concurrent tick can move the target
between sync and range-check. Microsecond window; fixed by unifying tick/command lock
ownership (C4/C5).

## 6.8 Single-flight request-cache waiters block forever

**Severity: MEDIUM**
`realtime/request_cache.go:163`: `<-flight.done` with **no timeout**. A duplicate retry
joins the in-flight request; if the handler (DB lock, combat lease) hangs, the waiter — and
its transport read goroutine (§3.2 Finding 3.2) — blocks indefinitely. Panic recovery exists
(`request_cache.go:174-183`) but re-pansics. **Fix:** select on `flight.done` + a context
deadline; on timeout return `CodeRequestTimeout`.

## 6.9 `Runtime.conns` (`sync.Map`) vs `connMu`+`sessionConnCounts` — dual structures

**Severity: LOW (clarity/perf)**
`server.go:23-25`: `conns sync.Map` keyed by `*clientConnection`; `sessionConnCounts` keyed
by SessionID under `connMu`. `writeEventsToSession` uses only `conns` (O(N) scan,
§3.3 Finding 3.3); the count map exists solely to detect "last connection for session" →
`runtime.detachSession` (`transport.go:355`). There's no `sessionID → []*clientConnection`
index. **Fix:** replace the dual structure with `map[SessionID][]*clientConnection` under
`connMu` — fixes both clarity and the O(N) scan.

## 6.10 Goroutine lifecycle

**Severity: LOW**
Per-connection: 2 goroutines, correctly joined via `waitForWriter` (`transport.go:239-251`)
with a timeout; `Shutdown` (`server.go:158-163`) can't hang. The tick goroutine
(`runtime.go:1336`) is cancelled by `ctx.Done()` on the clean path but orphaned on the
errc path (§2.1 Finding 2.1). No goroutine leak under normal operation; the orphan only
matters on the unclean-shutdown path (process exits anyway). **Fix:** §2.1.

## 6.11 No `go test -race` failures inferred, but specific races are plausible

The codebase has strong per-module locking, but the **semantic** races above (6.1, 6.2,
6.4, 6.7) won't be caught by `-race` because they don't touch the same memory unsynchronized
— they produce wrong *results*, not data races. **Recommendation:** add simulation-level
property tests that run many concurrent move/tick/combat operations and assert tick count
monotonicity and bounded per-tick displacement (see §14).
