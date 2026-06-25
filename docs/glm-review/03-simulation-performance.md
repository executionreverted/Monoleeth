# §5 Movement/Combat/Projectiles · §7 Tick/Game Loop · §8 Entity System · §13 Load/Scalability/Memory

## §5 — Movement, Combat & Projectile Logic

### 5.1 Movement anti-cheat — the dominant cheating vector

**Finding 5.1 — No cumulative speed/distance anti-cheat (speed hack ~133×)** — *HIGH (C6)*
`internal/game/server/handlers.go:388-414` `validateMoveIntentLocked` only enforces a
**per-command** radius: `entity.Position.Distance(intent.Target) > defaultMaxMoveDistance`
(`:406`, `defaultMaxMoveDistance = 1200` at `runtime.go:46`). There is no cumulative
distance-traveled tracking. A client chaining move commands every `minMoveCommandInterval =
75ms` (`runtime.go:50`, enforced `handlers.go:410-411`) covers 1,200 units per command →
`1200 / 0.075s ≈ 16,000 units/s`. Against an intended ship speed of ~120 u/s, that's a
**~133× speed hack** that passes every server check. **Why it matters:** movement is the
core loop of a space MMO; a trivially exploitable speed hack breaks PvP positioning, loot
racing, and scanner kiting. **Fix:**
1. Validate each move target against the entity's **effective max speed × elapsed since
   last accepted position** (e.g. `allowed = speed * dt * toleranceFactor`), not a flat
   1,200-unit radius. Reject moves that exceed it (`CodeOutOfRange`).
2. Track per-player cumulative displacement and flag sustained over-speed for telemetry.
3. Optionally snap the player back to the last server-authoritative position on violation
   (emit `position.corrected`).

**Finding 5.2 — Distance check uses stored (tick-advanced) position, not extrapolated** — *LOW*
`handlers.go:402-406`: `PlayerEntity` returns the worker's `Position`, updated only on tick
advance. If the player is mid-transit, the check may be stale by up to one tick (50 ms).
Compounds with 5.1. **Fix:** validate against `settledMovementPositionAt(now)`.

### 5.2 Dual movement model — tick-step vs. time-interpolation

**Finding 5.3 — Two independent position systems can oscillate** — *MED*
`internal/game/world/worker/worker.go:800-812` `settledMovementPositionAt` reconciles a
tick-step position (`AdvanceMovement`, `movement.go:13`, moves `speed * tickDelta`) with a
time-interpolated one (`MovementPositionAt`, `movement.go:45`, from `StartedAtMS`/
`ArriveAtMS`). It picks whichever is closer to target. With irregular ticks (C4),
tick-step can overshoot then be corrected by the time-based path, causing **visible
position jitter / backward jumps**. **Fix:** pick a single source of truth. Prefer the
time-based model for client-visible position (deterministic regardless of tick jitter) and
use tick-step only for coarse NPC stepping; reconcile at arrival.

### 5.3 Combat — RNG is nil (C2)

**Finding 5.4 — Combat RNG is `nil`; every attack hits; Accuracy/Evasion/Tracking are inert** — *CRITICAL (C2)*
`internal/game/server/runtime.go:1000`: `combatService := combat.NewService(clock, nil)`.
In `internal/game/combat/service.go:273-285` `rollHit`:
```go
hitChance := attacker.Stats.Stats.Combat.Accuracy + attacker.Stats.Stats.Combat.Tracking - target.Stats.Stats.Combat.Evasion
...
if rng == nil {
    return true   // ALWAYS HIT
}
return rng.Float64() <= hitChance
```
A real `runtimeRNG` exists (`internal/game/server/runtime_rng.go`, created at
`runtime.go:740`) and is wired into `deathService` (`runtime.go:1069`) — but **not** into
combat or loot. **Impact:** hit/miss, dodge, and evasion are completely non-functional;
PvP has no defensive counterplay to an always-hitting attacker. **Fix:** pass the runtime
RNG into `combat.NewService(clock, rng)` at `runtime.go:1000`. Trivial one-line fix with
large gameplay correctness impact.

**Finding 5.5 — Combat is hitscan; no projectiles exist** — *NOTE*
There is no `projectile`/`Projectile` type anywhere. `ExecuteBasicAttack` resolves
instantly: range check → hit roll → damage → kill check, synchronously under
`combat.Service.mu`. This **eliminates projectile desync** (good for an MVP) but means
weapon range is the only spatial constraint and there's no projectile travel time to
dodge/intercept. If DarkOrbit-style traveling projectiles are a goal, this is a feature
gap, not a bug. Flagging because the review brief names "Projectile Logic".

**Finding 5.6 — Range check can use a stale NPC position under concurrent tick** — *MED*
`combat/service.go:248`: `attacker.Position.Distance(target.Position) > WeaponRange`. Actor
positions are projected from worker entities at sync time (`combat_loot_helpers.go:97-101`).
The periodic tick advances worker NPC positions **outside** `runtime.mu`
(`runtime_world_snapshot.go:189`), while combat runs **inside** `runtime.mu`. A concurrent
tick can move an NPC between sync and range-check → stale range validation. Window is
microseconds, impact minor, but it's a real desync vector. **Fix:** run combat under the
same lock ownership as the worker tick, or re-read target position atomically inside the
combat call (C4/C5 fix removes the window).

**Finding 5.7 — NPC aggro ignores stealth detection** — *MED*
`enemy_aggro.go:271-273`: `playerAggroEligibleByEntityID` only checks the
`playerAggroIneligible` map (set on stealth toggle); it does **not** compute detection
scores. An NPC with aggro radius R targets any "eligible" player within R regardless of
stealth/detection (`visibility.DetectionForEntity` is used only for AOI visibility).
Stealth does not hide you from NPCs. **Fix:** fold detection into aggro eligibility, or
document that stealth is PvP-only.

**Finding 5.8 — NPC never deals damage (aggro is movement-only)** — *NOTE*
`enemy_aggro.go:208`: NPCs chase players but never attack. There is no NPC→player damage
loop; `combat.Service` has no NPC-initiated attack. NPCs are positional/kiteable threats.
This eliminates NPC damage desync but trivializes PvE. Likely intentional MVP scope —
flagging for completeness.

### 5.4 Combat event timing & damage clamp

**Finding 5.9 — Damage clamp caps resist at 0.95 (5% min damage); penetration clamped at 0** — *NOTE (design)*
`combat/service.go:287-298`: `effectiveResist` clamped to `[0, 0.95]`. Over-penetration
(negative resist) gives no bonus damage. Design choice; document it.

**Finding 5.10 — Events emitted after combat mutex release** — *LOW*
`combat/service.go:128-133`: events emitted after `service.mu.Unlock()`. A concurrent
combat command could mutate actor state between mutation and emission. Safe (events carry
snapshots) but consumers may see slightly out-of-order state transitions. Acceptable.

---

## §7 — Tick / Game Loop Performance

### 7.1 Command handlers drive extra simulation ticks (C4)

**Finding 7.1 — `Worker.Tick()` called from 17+ handler sites → variable, client-amplifiable sim rate** — *HIGH (C4)*
`Worker.Tick()` (`worker.go:224-250`) is the **full** step: advances movement, the **enemy
spawner**, **enemy aggro**, scheduled tasks, and increments `worker.tick`. It is invoked
from the background loop (`runtime_world_snapshot.go:243`) **and** synchronously from many
request paths:
`handlers.go:307,361,478`; `runtime_sessions.go:55,70`; `runtime_metrics.go:66`;
`portal_handlers.go:220,309,345`; `death_respawn.go:98`; `death_events.go:59`;
`runtime_maps.go:80,186`; `runtime_players.go:113`. Move/stop are throttled to 75 ms
(`handlers.go:410`), but stealth toggle, portal, hangar activate, crafting, session
bootstrap, and **session detach** also tick and are *not* subject to that gate. A client
reconnecting or spamming stealth/crafting injects unbounded extra enemy-AI/spawner ticks.
The worker's internal mutex prevents data corruption, but the **semantic** effect is:
NPC spawn cadence and aggro ticking scale with aggregate command throughput, and the tick
counter inflates. This is both a balance/determinism bug and a CPU amplification vector
affecting every player sharing a map. **Fix:**
- **Preferred:** make handlers *submit* an intent and let only the background loop tick
  (remove all command-driven `Tick()` calls; for cases needing immediate reflection, use a
  dedicated non-simulating `applyCommands` flush that doesn't advance spawner/aggro).
- **Minimal:** gate every tick entry through `tickMu` and a per-map "last tick" timestamp,
  skipping ticks that fire within `tickInterval` of the last.

**Finding 7.2 — Non-fixed timestep: ticker coalescing drops ticks under load** — *MED*
`runtime.go:1335,1342`: `time.NewTicker(50ms)` with a bare `<-ticker.C`. Go's ticker drops
ticks that fire while the receiver is blocked (keeps only the latest). If a tick body
exceeds 50 ms (likely under load given C5), ticks are silently skipped → simulation runs
slower than realtime with **no catch-up / accumulator**. Combined with C4, net behavior is
unpredictable (loses background ticks, gains command ticks). **Fix:** use a fixed-timestep
accumulator (drain `ticker.C` in a loop summing debt; step in fixed `tickInterval` chunks
capped at e.g. 5 steps/frame) so the sim neither lags silently nor double-advances.

**Finding 7.3 — Durable-outbox DB pump runs every 50 ms tick, inside the tick goroutine** — *MED*
`runtime.go:1343-1346`: `runDurableOutboxRealtimePumpTick()` drains up to 100 rows/store
(`runtime_durable_outbox_realtime.go:17,77-82`) on every tick → up to ~20 DB round-trips/s
just for outbox leasing, and a slow DB directly stalls the tick (compounds 7.2). **Fix:**
run the outbox pump on its own goroutine/ticker independent of the sim tick.

### 7.2 No panic recovery in the tick goroutine (C1)

**Finding 7.4 — Missing `recover()` → any panic crashes the process** — *CRITICAL (C1)*
`runtime.go:1336-1361`: the tick goroutine (`go func() { defer ticker.Stop(); for {...} }()`)
has **no `recover()`**. Any panic in `runDurableOutboxRealtimePumpTick`, `tickAndCollectAOIEvents`,
an AOI-diff computation, or a worker `Tick()`/scheduled-task handler crashes the entire
server. The only `recover()` that touches execution is in the request cache
(`realtime/request_cache.go:174-183`) and it **re-pansics** after recording. **Fix:** wrap
the tick loop body in `defer func() { if r := recover(); r != nil { log/telemetry; } }()`
(record, don't exit) so one bad tick doesn't take down the world. Pair with structured
logging of the recovered panic + stack.

---

## §8 — Entity System & World Management

### 8.1 Worker entity storage & spatial index

Entities stored in `map[world.EntityID]world.Entity` (`worker.go:69-70`). `Entity()`
(`:288-294`) returns a struct copy under `RLock` — safe. The spatial index
(`spatial/index.go`) is worker-owned (no internal locking, `:27-31`), all access under
`worker.mu`; `CellCoord` uses `math.Floor` so negative coords are correct (`spatial/hash.go:44-49`).

**Finding 8.1 — O(N×M) aggro scan bypasses the spatial index** — *MED*
`enemy_aggro.go:244-291` `nearestAggroTarget`: for **every NPC without an aggro target**
(`:109`) it linearly scans **all players** (`worker.playerEntities`), not the spatial index.
With N NPCs and M players, aggro scanning is O(N×M) per tick. **Fix:** query the spatial
index for players within the aggro radius (O(N × k) where k = nearby players).

**Finding 8.2 — `eventAliveCount`/`poolReservedCount`/`mapReservedCount` are O(n), called O(pools×rows)/tick** — *LOW*
`enemy_spawner.go:1014,1024,1047`: linear scans over all rows, called per event/pool during
periodic fill (`tickEnemySpawnerDefinition`, `:445-525`). **Fix:** maintain counters (some
already exist like `aliveByPool`); or cap row scans.

**Finding 8.3 — `aliveByPool` counter can drift from actual alive count** — *LOW*
`enemy_spawner.go:314-318`: manually maintained alongside row mutations; any path that
mutates `record.Alive` without `markEnemyKilled` drifts it. There's no consistency
validation. **Fix:** derive counts from rows on demand, or add an invariant check in tests.

**Finding 8.4 — Spawn entity ID collisions abort the entire spawner tick** — *LOW*
`enemy_spawner.go:506-508`: deterministic IDs `entity_npc_{hash}_{spawnIndex+1}`; a
collision returns an error that aborts all other pool processing for that tick. **Fix:**
continue other pools on a single collision (log + skip).

### 8.2 AOI diff — O(S×N) per tick with reflect.DeepEqual

**Finding 8.5 — `reflect.DeepEqual` in AOI diff per entity per session per tick** — *MED*
`aoi/snapshot.go:144`: `if !reflect.DeepEqual(previousEntity, entity)`. `reflect.DeepEqual`
on `EntityPayload` (nested pointers/slices) is far more expensive than field-by-field.
Called for every entity in every session's AOI every tick, under the global lock (C5).
**Fix:** add a cheap version/`dirty` field or a hand-written `equalEntityPayload`, and
increment it on mutation; diff on the version.

**Finding 8.6 — O(S×N) NPC combat-actor upsert per tick** — *MED*
`runtime_world_snapshot.go:83-121`: for each active session, `publicEntityMetadataLocked`
calls `upsertNPCCombatActorProjectionLocked` (`npc_actor_projection.go:184`) per NPC per
session per tick, each taking the combat service mutex. **Fix:** compute per-NPC metadata
once per map per tick (it's map-global, not per-viewer), then the per-session work is just
the AOI radius filter + visibility.

### 8.3 Visibility/fog — clean client-safe boundary (GOOD)

`aoi/snapshot.go:89-121` `BuildVisibleSnapshot` calls `visibility.CanSendEntityToClient`
per entity and copies only public fields into `EntityPayload`; `InternalMetadata`,
`GameplaySeed`, `FutureSpawnData` are never serialized. `CanInteract` returns a single
generic `ErrNotVisible` (no info leak). Hidden entities are always visible to themselves
(`visibility.go:318-338`).

**Finding 8.7 — Witness expiry requires `viewer.ObservedAt` to be set** — *LOW*
`visibility.go:332`: if a caller forgets to set `viewer.ObservedAt`, the `IsZero()` check
makes witnesses with non-zero expiry **always pass**, leaking hidden player positions.
**Fix:** default `ObservedAt` to "now" at construction, or fail-closed when zero.

---

## §13 — Load, Scalability & Memory Usage

### 13.1 Scalability ceiling = the global lock + O(M·N) fan-out

The two structural limits (detailed in §2.3 and §3.3) compound: the global `runtime.mu`
serializes all commands *and* the AOI sweep, and each delivered event scans all
connections. Together they cap the server at roughly **single-core command throughput ×
(connections²) fan-out cost**. Neither is a memory bug — both are CPU/latency ceilings.

**Finding 13.1 — Expected single-map population ceiling is low (hundreds, not thousands)** — *HIGH*
Given C5 + C9 + 8.5 + 8.6, a single map with a few hundred concurrent players will spend
most of each 50 ms tick under the lock doing per-session AOI work and re-scanning
connections. **Fix roadmap:** (1) per-map mutex + per-map AOI computation (parallel ticks);
(2) session→conn index; (3) pre-serialized broadcast events; (4) AOI versioning instead of
`reflect.DeepEqual`. This is the work that turns "single shard" into "scalable shards".

### 13.2 Unbounded memory growth surfaces

**Finding 13.2 — Rate-limiter buckets never pruned (~90 entries/player, permanent)** — *HIGH (C8)*
See §3.3 Finding 3.6.

**Finding 13.3 — Cooldown/idempotency maps have no janitor** — *MED*
`runtime.go:143,148,150,152,153,214`: `lastMove`, `portalCooldowns`,
`playerProtections`, `combatLocks`, `shieldRepairTicks`, `scanCooldowns` are keyed by
`foundation.PlayerID`. Most are checked lazily at read time but **never deleted on a
timer**. `lastMove` is only deleted on death/respawn (`death_respawn.go:106`) — otherwise
grows forever. A player who triggers cooldowns and never returns leaves entries behind.
`repairAttempts`/`shopPurchases`/`scanCapacitorSpends`/`portalAttempts` are idempotency
stores by design (unbounded typical), but with no TTL/limit they're an unbounded surface.
**Fix:** add a periodic sweeper (e.g. on each tick, or a 60 s janitor) that evicts expired
cooldowns and caps idempotency maps (LRU). On `detachSession`, sweep per-player entries.

**Finding 13.4 — `queuedEvents` per-session slice is unbounded between drains** — *LOW*
`combat_loot_helpers.go:473-474` appends with no cap; drained on next qualifying command
(`runtime_sessions.go:273`) / outbox tick, cleared on `forgetSessionReplay`. A handler
queuing many events without draining could grow it. Low risk given drain triggers. **Fix:**
cap the slice; drop/telemetry on overflow.

### 13.3 Connection-pool exhaustion (cross-ref)

DB I/O under service mutexes (§9) + no `SetMaxOpenConns` (`contentdb/db.go:22-30`) means a
burst of economy ops can open hundreds of PG connections and exhaust `max_connections`.
See `05-data-economy.md` §9.2.
