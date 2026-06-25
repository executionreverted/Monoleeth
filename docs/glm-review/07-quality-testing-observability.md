# §12 Error Handling, Logging & Observability · §14 Testing Gaps · §15 Code Quality & Go Idioms

## §12 — Error Handling, Logging & Observability

### 12.1 Structured logging — solid, with one misleading-redaction bug

**Finding 12.1 — Command log is structured, no secrets, JSON-escaped (no log injection)** — *GOOD*
`observability/command_log.go:33-45,149-175,230-242`: `request_id`, `player_id`,
`session_id`, `op`, `result`, `error_code`, `idempotency_key`, `duration_ms`, `timestamp`.
`WorldID`/`ZoneID` are `json:"-"`. `json.Marshal` escapes control chars and appends `\n`, so
newline/log-forgery via player-controlled `idempotency_key` is mitigated. No request bodies,
headers, cookies, or payloads are logged.

**Finding 12.2 — `auth_transition_log.go`: `json:"-"` tags are misleading; MarshalJSON re-emits player_id/session_id** — *MED*
`observability/auth_transition_log.go:24-25`: struct tags say `json:"-"` for `PlayerID`/
`SessionID`, but the custom `MarshalJSON` (`:95-119`) re-emits them as `player_id`/`session_id`.
So despite the `json:"-"` appearance, **player_id and session_id ARE serialized** to JSON log
lines on successful auth. The `json:"-"` will fool future maintainers into thinking they're
redacted; a successful login also writes the freshly-minted `SessionID` to the auth log.
**Fix:** either truly redact them (drop from MarshalJSON) or remove the misleading `json:"-"`
and document that session_id is intentionally logged. If session handles are sensitive in your
log pipeline, redact.

### 12.2 Error surfacing

**Finding 12.3 — `commandErrors` reports only the first of multiple errors** — *LOW*
`runtime_sessions.go:483-491` (used at `handlers.go:312,366`): returns
`result.CommandErrors[0].Err` / `ScheduledTaskErrors[0].Err`. A tick producing multiple
command/task errors surfaces only one; the rest are swallowed (telemetry records enemy
telemetry but not these). **Fix:** aggregate via `errors.Join`, or log the full error set to
observability while returning the first to the client.

**Finding 12.4 — `DomainError.Public()` strips detail; internal errors collapse** — *GOOD*
`foundation/errors.go:119-140`: `Public()` exposes only `Code`+`Message`; `detail`+`cause`
stripped. Internal errors collapse to `CodeInternal`/`"Request failed."` (`http.go:228`); the
realtime gateway additionally masks internal codes (`gateway.go:157`). No stack/internal path
leakage to clients.

**Finding 12.5 — Observability errors are fire-and-forget** — *LOW*
`realtime/observability.go:139,161`: `_ = ...Record(...)`. Telemetry failures never surface;
silent metric loss is only visible via `TelemetryErrorMetricWrite` (`:154`) which itself is
recorded only if the recorder implements `telemetryErrorRecorder`. Acceptable (don't fail the
request on metric errors), but worth a debug metric.

**Finding 12.6 — `commandLogReferenceID` re-unmarshals payload on the hot path** — *LOW*
`realtime/observability.go:164`: parses `request.Payload` with `json.Unmarshal` per command to
extract idempotency keys for shop/market/portal/death ops. Minor CPU; re-decodes bytes the
handler already decoded. **Fix:** pass the extracted reference id through the command context.

### 12.3 Release gates / abuse coverage — checklists, not enforcement

**Finding 12.7 — Release-gate/abuse-coverage types are reporting only, not runtime gates** — *MED*
`observability/release_gate.go`, `abuse_coverage.go`: they catalog required tests (negative
amounts, duplicate request IDs, hidden-entity interaction, out-of-range pickup, market/auction
races, webhook replay, etc.). Useful as documentation, but **do not block deployment** — a CI
step must invoke `NewReleaseGateReport`/`NewCommandSecurityReviewReport` for them to matter.
**Fix:** wire a CI job that fails the build if the release-gate report isn't satisfied.

---

## §14 — Testing Gaps & Simulation Tests

The project has ~1,750 test functions and good unit coverage of economy idempotency,
reservation lifecycle, loot pickup dedup, route settlement replay, reconnect replay, and
combat/loot/death vertical slices. The **gaps are exactly where the production risks live**:

**Finding 14.1 — No concurrency/simulation test for the command-driven tick amplification (C4)** — *HIGH*
There are worker-concurrency tests (`worker_concurrency_test.go`) but none assert that
**concurrent commands cannot inflate the tick count or double-advance movement** under a
fixed wall-clock budget. This semantic race (§6.1) is untested because it produces wrong
*results*, not a data race. **Fix:** add a property test: drive N concurrent move/stop/stealth
commands + background ticks for T seconds, assert `worker.tick` increments by ≈ `T/tickInterval`
(±tolerance) and total entity displacement ≤ `speed * T * tolerance`. It will fail today.

**Finding 14.2 — No speed-hack/anti-cheat movement test (C6)** — *HIGH*
No test asserts that chaining max-radius moves at the rate limit cannot exceed ship speed.
**Fix:** a test that issues `defaultMaxMoveDistance` moves every `minMoveCommandInterval` and
asserts rejection once cumulative speed exceeds the ship's effective speed. It will fail today
(16,000 u/s accepted).

**Finding 14.3 — No test that combat RNG is non-nil / that misses occur (C2)** — *HIGH*
No test verifies the production runtime wires a real RNG into combat (and that evasion can
produce a miss). The unit tests inject RNGs; the wiring (`runtime.go:1000`) is untested.
**Fix:** an integration test that builds the runtime and inspects
`combatService`'s RNG, plus a statistical test that high-evasion targets sometimes miss.
Same for loot RNG (C3).

**Finding 14.4 — No test for market/auction rollback-vs-DB desync (C12)** — *HIGH*
The fanout tests exist but none inject a failing `sellerCredit`/refund and assert the buyer's
DB balance is restored (or the operation is fully atomic). **Fix:** a fault-injection test
that makes the second wallet call fail mid-buy and asserts no net money was created/destroyed
(in-memory == DB).

**Finding 14.5 — No memory-growth/leak test for rate-limiter buckets (C8) / cooldown maps (§13.3)** — *MED*
No test simulates many connect/disconnect cycles and asserts the rate-limiter `buckets` map
and runtime cooldown maps are bounded. **Fix:** a long-run simulation (e.g. 10k sessions over
a fake clock) asserting the maps don't grow unbounded after a sweeper runs.

**Finding 14.6 — No replay-gap / resync test (§4 Finding 4.1)** — *MED*
The reconnect-replay test (`server_reconnect_replay_test.go`) exists but doesn't cover the
case where the ring has evicted the needed seq (gap → silent loss). **Fix:** overflow the
128-cap ring and assert the client is told to resync (currently it isn't).

**Finding 14.7 — No load/scalability test for the global lock + O(M·N) fan-out (C5/C9)** — *MED*
No test measures tick time as players/sessions scale. **Fix:** a benchmark that scales
sessions per map and reports tick duration; it will show the super-linear cost and serve as a
regression gate after the per-map-shard fix.

**Finding 14.8 — Simulation harnesses exist but aren't wired to assert the above** — *NOTE*
`observability/simulations/` (combat_loot, market_auction, production_routes) and
`internal/game/testutil/` (fake_clock, fake_rng, event_recorder) are good infrastructure.
Extend them to cover 14.1–14.6.

---

## §15 — Code Quality, Maintainability & Go Idioms

### 15.1 What's good

**Finding 15.1 — Domain decomposition is clean and matches AGENTS.md guidance** — *GOOD*
Files are small and ownership-focused: `wallet_service.go`, `wallet_ledger.go`,
`loot_pickup_handler.go`, `route_settlement.go`, `auth_session.go`. Doc.go files per package
document ownership. The build + `go vet` pass on core packages.

**Finding 15.2 — Strong typing for money/quantities/IDs** — *GOOD*
`foundation.Money`/`foundation.Quantity` (int64), typed `PlayerID`/`SessionID`/`EntityID`.
Constructors enforce invariants. This is idiomatic and prevents a class of bugs.

### 15.2 Maintainability concerns

**Finding 15.3 — `NewRuntime` is a 580-line constructor with sequential close-on-error** — *MED*
`runtime.go:724-1309`: a single function wires ~40 dependencies with verbose, duplicated
close-on-error handling. It's correct but a maintenance burden and a review hazard (the C2/C3
nil-RNG bug lives here precisely because the wiring is so long). **Fix:** extract into smaller
builders (`buildEconomy(...)`, `buildCombat(...)`, `buildWorld(...)`) each returning a closer;
this also makes the RNG wiring obvious.

**Finding 15.4 — Global `runtime.mu` guards ~25 maps (§2.3)** — *HIGH (maintainability + perf)*
A single mutex for this many responsibilities violates single-responsibility and makes every
new gameplay feature a new contender for the hot lock. **Fix:** the per-map shard refactor
(§16) naturally splits this into per-instance locks + a thin coordination lock.

**Finding 15.5 — gopls "modernize" hints (~200+) suggest the codebase predates Go 1.21+ idioms** — *LOW*
E.g. `auth/http.go:171`, `combat/service.go:275,289,296` (use `max`), `auth/model.go:174`
(`slices.Contains`), `combat/types.go:234` (`maps.Copy`). Not bugs. **Fix (optional):** a
one-time `gofmt -s` / modernize pass once on a quiet commit; don't mix with feature work.

**Finding 15.6 — Dead client code: `serverCorrection` action never dispatched** — *LOW*
`client/src/state/reducer.ts:320-321`: defined and handled but never dispatched. Position
reconciliation happens only via `position.corrected`. **Fix:** remove or implement.

**Finding 15.7 — Client `request_id` fallback is non-CSPRNG** — *LOW*
`client/src/protocol/request-id.ts:1-10`: falls back to `Math.random()` when
`crypto.randomUUID` is absent. `request_id` is only response correlation (not a secret), so
risk is low — **but verify** no server logic treats `request_id` as unguessable (idempotency
keys, replay windows). Idempotency keys should be server-derived or client-UUID-based, never
`Math.random`.

**Finding 15.8 — Client `client_seq` is client-monotonic but never validated; resets on reload** — *LOW*
`client/src/protocol/commands.ts:21,553`: increments client-side, sent on every envelope, but
the client doesn't use it for ordering/replay, and it resets per page load. **Fix:** document
that the server must not trust `client_seq` for replay defense across reconnects; or drop it.

### 15.3 Consistency of idempotency (GOOD)

Across wallet/inventory/market/auction/quest/craft/premium, idempotency keys are
server-derived from `(player, operation, referenceKey)` and dedup maps reject mismatched
replays. The idempotency outbox uses `ON CONFLICT DO NOTHING` + conflict resolution
(`§9.4`). This is a consistent, well-applied pattern — the gaps are the rollback-vs-DB issues
(C12), not the dedup itself.
