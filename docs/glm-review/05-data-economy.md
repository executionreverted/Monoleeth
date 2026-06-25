# §9 Database, Persistence & Progression · §11 API / Backend Services

## §9 — Database, Persistence & Player Progression

### 9.1 Connection-pool limits (C11)

**Finding 9.1 — No `SetMaxOpenConns`/`SetMaxIdleConns`/`SetConnMaxLifetime` in production** — *HIGH (C11)*
`internal/game/contentdb/db.go:22-30`: `Open` calls `sql.Open` + `PingContext` and returns.
No pool tuning. (The only `SetMaxOpenConns` calls are in a smoke test
`postgres_smoke_test.go:158-159`.) Go's default allows unlimited connections. Combined with
DB I/O under service mutexes (§6.5), a burst of economy ops can open hundreds of PG
connections and exhaust `max_connections`. **Fix:** set sane production defaults in `Open`
(or a `ConfigurePool` caller): `SetMaxOpenConns` (e.g. 25–50), `SetMaxIdleConns`,
`SetConnMaxLifetime` (e.g. 30 min), `SetConnMaxIdleTime` (e.g. 5 min). Make them config-driven.

### 9.2 Persistence correctness — what's good

**Finding 9.2 — SQL is parameterized everywhere; no injection risk** — *GOOD*
All queries across `wallet_store.go`, `inventory_store.go`, `market_store.go`,
`economy_idempotency_outbox.go` use `$1, $2, …` placeholders. No string concatenation of
input into SQL.

**Finding 9.3 — Atomic multi-statement transactions for committed mutations** — *GOOD*
`wallet_store.go:224-234`, `inventory_store.go:428-461,463-477`: balance + ledger + reference
+ counter writes are within a single `BeginTx`/`Commit`.

**Finding 9.4 — Idempotency claim uses `ON CONFLICT DO NOTHING` + `RowsAffected`** — *GOOD*
`economy_idempotency_outbox.go:37-74`: atomic claim; losers load the existing row and resolve
via `ResolveIdempotencyClaim` (`idempotency_outbox.go:102-116`) which rejects same-key with
different `(op, player, requestHash)` → prevents collision attacks.

### 9.3 The rollback-vs-DB problem (C12)

The wallet/inventory services keep an **in-memory** truth and persist mutations to DB. Their
idempotency is keyed on `(player, operation, referenceKey)` and works *within a process*. The
danger is the interaction between cross-service orchestration and partial failures:

**Finding 9.5 — Market buy: in-memory rollback does not reverse committed DB writes** — *HIGH (C12a)*
`market/service.go:331-335`: `rollback` calls `restoreMarketBuyMutationLocked(snapshot)`
(restoring in-memory wallet/listing/escrow), but the sub-calls `wallet.DebitWallet` /
`wallet.CreditWallet` (`:339-372`) each **independently persist to DB** when they succeed. If
`buyerDebit` commits to DB and then `sellerCredit` fails, the in-memory wallet is restored to
the pre-buy snapshot but the **DB still holds the debit**. On the next operation the wallet
uses the (higher) in-memory balance (could spend money that was actually debited); on
restart/reload it resyncs to DB (money gone). This is a **balance/DB desync and potential
fund loss**. **Fix:** make the cross-service mutation a single DB transaction (pass a `*sql.Tx`
through debit/credit/move), or use a saga with compensating DB writes (explicit
`CreditWallet` with a *reversal* reference key) rather than in-memory-only restore.

**Finding 9.6 — Auction buy-now: buyer debit not rolled back on refund failure** — *HIGH (C12b)*
`auction/service.go:277-299`: `buyerDebit` commits; if the previous-bidder `refund`
(`CreditWallet`) then fails, the code returns the error **without** reversing the buyer's
debit. Contrast with `PlaceBid` (`:169-198`) which snapshots wallet state and restores on
refund failure. Buy-now has no such snapshot. Worse, the idempotency key is marked failed, so
the buyer **cannot retry** — they lose the funds with no recourse. **Fix:** mirror PlaceBid's
`snapshotWalletMutationState`/`restoreWallet` in buy-now, or (better) the shared-tx fix from 9.5.

### 9.4 Crafting: reservation+debit not rolled back on duplicate job

**Finding 9.7 — StartCraft leaves reservation + fee committed on duplicate job** — *MED*
`crafting/service.go:323-367`: the lock is released after allocating `jobID` (`:325`); the
reservation (`ReserveItems`) and fee debit (`DebitWallet`) happen lock-free; on re-lock
(`:360`) a duplicate-job check fails and returns — but the **reservation and fee are not
released/refunded** (release only happens on debit failure, `:354`). Materials reserved and
fee debited, no job created → player loses both. **Fix:** on the duplicate-job branch, call
`ReleaseReservation` and a refund `CreditWallet` (reversal reference key).

### 9.5 Loot: drop ownership & pickup races

**Finding 9.8 — Loot pickup can leak items on concurrent expiry** — *MED*
`loot/service.go:317-370`: pickup sets `pendingClaims[dropID]`, releases the lock, runs
`cargo.AddItem` (idempotency key `loot_pickup:<dropID>`), re-locks, then marks `ClaimedAt`.
If, between the cargo add and re-lock, the drop **expires** (`ExpireDrops` removes it), the
re-lock at `:358` finds `service.drops[input.DropID]` gone → returns `ErrUnknownDrop`. The
items are already in cargo but the drop disappears unconsumed. **Impact:** an item leak
(items granted but drop not recorded as claimed). Mitigated because the cargo idempotency
key means a duplicate pickup is a no-op, but the original grant stands. **Fix:** under the
re-lock, if the drop vanished *and* the cargo add succeeded, record the claim against the
drop's metadata (keep a short-lived `claimedDrops` tombstone) rather than erroring; or move
the claim mark to before the cargo add and roll back cargo on failure.

**Finding 9.9 — Loot RNG nil → all rows drop at max quantity (C3)** — *CRITICAL*
`runtime.go:990`: `loot.Config` has no `RNG` set; `loot/service.go:685-696` skips the chance
check and forces `quantity = row.MaxQuantity` when `rng == nil`. Every NPC drops every loot
row at max. **Fix:** `loot.Config{ RNG: rng, … }` at `runtime.go:990`. One-line fix; large
economy-correctness impact.

### 9.6 Progression

**Finding 9.10 — XP accumulation has no integer-overflow check** — *MED*
`progression/service.go:68`: `player.MainXP += input.Amount` with no overflow guard. If near
`math.MaxInt64`, the sum wraps negative → `MainLevelForXP` misbehaves (resets level). Same at
`store.go:154` `roleLevel.XP += grant.Amount`. `input.Amount` is validated positive but the
**sum** isn't. **Fix:** check `sum > 0 && sum > player.MainXP` (no wrap), else cap or error.

**Finding 9.11 — Concurrent XP grants serialized by store mutex; dual-key idempotency** — *GOOD*
`progression/service.go:38-39,46-64`: serialized; dedup on both `(player, source, sourceID)`
and explicit idempotency key. No lost updates. (Caveat: persist happens under the mutex,
§6.5.)

**Finding 9.12 — `TryRankUp` skips dedup for zero idempotency key** — *LOW*
`progression/service.go:156-157`: zero key bypasses the dedup map. Intended for trusted
server-initiated rank-ups; ensure no client-facing path passes a zero key.

### 9.7 Wallet/inventory — single-process correctness (GOOD) but multi-instance hazard

**Finding 9.13 — Negative-balance protection is correct in single-process** — *GOOD*
`wallet_service.go:310-313`: balance check + subtraction both under `service.mu`. No negative
balance possible. `WalletBalance.Validate()` (`wallet.go:97-99`) rejects negatives;
`addWalletAmount` (`:702-707`) checks overflow.

**Finding 9.14 — Wallet balance upsert has no optimistic concurrency (last-writer-wins)** — *MED (multi-instance)*
`contentdb/wallet_store.go:289-301`: upsert blindly overwrites `balance = EXCLUDED.balance`
(the in-memory-computed value). No `WHERE balance = $old` / version column. Safe
single-process (mutex serializes); in a multi-instance deployment two wallet services both
debiting → last writer wins, lost update. **Fix:** add a `version`/`balance_version` column
and `UPDATE … WHERE version = $expected` (CAS), or route all wallet writes through one owner.

**Finding 9.15 — Transfer idempotency keyed on sender only** — *LOW*
`wallet_service.go:386-389`: keyed on `FromPlayerID`. Same reference key with a different
`ToPlayerID` silently returns the original. Intended (key is unique per transfer), but no
validation that `ToPlayerID` matches. **Fix:** validate `ToPlayerID` on dedup hit.

---

## §11 — API / Backend Services Review

### 11.1 Realtime command gateway

**Finding 11.1 — `RealtimeCommandGateway.Handle` bypasses rate limit AND request cache** — *MED*
`runtime/realtime_gateway.go:50-64`: dispatches straight to `executor.Execute` with **no
limiter and no `RequestCache`** — a different boundary than `realtime.Gateway` (which has
both). The production WS path uses `realtime.Gateway` (`transport.go:100`), so this may be
dormant, but its existence is a footgun: any transport path routed through it gets no abuse
protection or retry dedup. **Fix:** confirm it's not wired into any live transport; if it is,
wire the same limiter + cache; otherwise remove it or mark it test-only.

**Finding 11.2 — Unknown op yields `CodeInternal` instead of `NotFound`** — *LOW*
`runtime/realtime_gateway.go:58-61`: unregistered op → `missingRealtimeCommandHandler` →
`CodeInternal`. Misleading (envelope decode already rejects unregistered ops at
`envelope.go:610`, so this should be unreachable). **Fix:** return `CodeNotFound`.

### 11.2 Combat gateway

**Finding 11.3 — Attacker identity is server-resolved; lease fences active ship** — *GOOD*
`runtime/combat_gateway.go:85,90`: `ActiveCombatActor(ctx)` derives attacker from
`CommandContext`; `WithActiveShipCombatLease` fences active-ship state. Payload only carries
`TargetID`. Solid anti-cheat posture.

**Finding 11.4 — Combat lease acquired synchronously in the read-loop goroutine** — *MED*
`runtime/combat_gateway.go:90`: if the lease blocks (contended lock/transaction), it
head-of-line-blocks that client's inbound stream (§3.2). Combat is the most latency-sensitive
op — verify the lease is short-held. **Fix:** measure lease hold time; if non-trivial, move
execution off the read loop (§3.2 fix).

**Finding 11.5 — Attacker-ship errors mislabeled as target errors** — *LOW*
`combat_gateway.go:183-186`: `ErrNoActiveShip`/`ErrShipNotUnlocked`/`ErrShipUnavailable`/
`ErrShipDisabled` all collapse to "Target is not available." These are *attacker-ship*
failures shown as target failures → confusing client message. **Fix:** separate
attacker-vs-target error codes.

### 11.3 Economy entry points are server-owned (GOOD)

`economy/inventory_service.go:24`, `wallet_service.go:29-48`: `PlayerID` is a struct field
populated from `CommandContext`, not from JSON. Amounts go through `NewMoney`/`NewQuantity`
(int64-only, positive-bounded, `foundation/amounts.go:115-123`). Market/auction/loot all take
identity from server context. Idempotency + `RequestCache` prevent replay/duplicate-request
duplication (`realtime/gateway.go:90-96`).

### 11.4 Observability fanout/backends

Outbox replay worker has lease-based dedup (`economy/outbox_replay_worker.go:56-100`):
at-least-once delivery with idempotent consumers. Market state-mutation fanout and premium
fanout have dedicated tests (`server_market_state_mutation_fanout_test.go`,
`server_premium_fanout_test.go`). The durable outbox pump runs every tick
(`runtime.go:1343-1346`) — see §7.3 for the stall risk.
