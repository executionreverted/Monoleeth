# Game Systems Code Review Report

> Scope: static review of the current repository under `/Users/canersevince/gameproject`, ignoring Symphony/orchestration code and ignoring the dirty CMS worktree as requested. This report focuses on production risks, multiplayer/desync bugs, race/concurrency risks, cheating vectors, server performance, persistence, and maintainability.  
> Date: 2026-06-25  
> Note: The project is still actively in progress; issues below distinguish MVP/in-progress limitations from production blockers.

---

## 1. Executive Summary

The project has a strong direction for an in-progress DarkOrbit-like MORPG: server-owned sessions, strict request envelopes, server-side command handlers, AOI filtering, visibility checks, idempotency keys, and domain-specific services for economy, combat, loot, production, quests, CMS/content, and auth.

The biggest production risks are not “missing features”; they are **boundary and ownership risks** that will become severe as soon as multiple real players connect:

1. **Most game state is still in-memory**: accounts, sessions, player runtime state, wallet, inventory, market, auction, production, map state, combat actors, route state, etc. A restart loses the live game and all player progress except CMS content DB.
2. **The runtime is a single-process monolith protected by one global mutex**. It works for a vertical slice, but it serializes ticks, commands, AOI building, world updates, economy calls, and event queues.
3. **WebSocket writes happen synchronously from the tick/event path**, so a slow client can stall simulation/event fanout.
4. **Market and auction settlement call wallet/inventory services in multiple steps without a durable transaction/outbox**, so mid-flow failures can create lost currency, item escrow mismatch, or double-settlement risks.
5. **CMS publish writes a new content version but does not update the live runtime catalog/services**, so admin may “publish” content that the game server does not actually use until restart or manual reload.
6. **Movement stop/interact logic can use stale positions**, causing server/client correction and possible range/desync edge cases.
7. **Rate limiting is mostly documented metadata, not enforced**, especially for login/register, market/search, quest, content admin, and many gameplay operations.
8. **Realtime reconnect/cursor semantics are snapshot-based, not event-replay-based**, so clients recover by snapshot but cannot reliably replay missed event streams.

This is acceptable for an active in-progress vertical slice, but before real production/playtest scale, the next hardening pass should prioritize:

- durable identity/progression/economy storage;
- transactional economy settlement;
- decoupled WebSocket writer queues;
- per-map actor ownership instead of global runtime lock;
- CMS runtime version application/reload semantics;
- movement/interact position settling;
- real rate limits and backpressure;
- race tests and simulation/load tests.

---

## 2. Game Server Architecture

### Current architecture

The runtime is composed mainly in:

- `internal/game/server/runtime.go`
  - `NewRuntime`
  - `Runtime`
  - `StartWithEventSink`
- `internal/game/server/server.go`
  - `Server`
  - `Run`
  - `Shutdown`
- `internal/game/server/handlers.go`
  - `commandHandlers`
  - realtime command dispatch

The server currently builds a **single in-process game runtime**:

- Auth service: `internal/game/auth`
- Realtime gateway: `internal/game/realtime`
- Map workers: `internal/game/world/worker`
- Combat: `internal/game/combat`
- Loot: `internal/game/loot`
- Economy/wallet/inventory: `internal/game/economy`
- Market/auction/premium: `internal/game/market`, `internal/game/auction`, `internal/game/premium`
- Discovery/scanner/planets: `internal/game/discovery`
- Production/routes/buildings: `internal/game/production`
- CMS/content DB: `internal/game/contentdb`, `internal/game/admin/content_service.go`

This is good for an integrated vertical slice. The code is domain-rich and the service boundaries are understandable.

### Issue: single global runtime mutex

**Severity:** High  
**Affected files/functions:**

- `internal/game/server/runtime.go`
  - `Runtime.mu`
  - `StartWithEventSink`
- `internal/game/server/runtime_world_snapshot.go`
  - `tickAndCollectAOIEvents`
  - `aoiSnapshotForPlayerLocked`
- `internal/game/server/handlers.go`
  - most command handlers lock `runtime.mu`
- Many `runtime.handle*` functions across `internal/game/server/*_handlers.go`

**Why it matters:**

The server has multiple map workers, but the outer runtime serializes almost everything through one `sync.Mutex`. Under load:

- one slow command blocks all other player commands;
- AOI building blocks economy/admin commands;
- ticks block WebSocket command handling;
- commands block ticks;
- map A activity blocks map B activity;
- CMS/admin requests can compete with real-time gameplay.

This also hides race conditions because everything is serialized at the top level. When you later split maps into independent workers/processes, many assumptions may break.

**Suggested fix:**

Move toward actor ownership:

1. Runtime-level lock only for session map and global routing metadata.
2. Per-map worker command queue owns movement/entities/AOI.
3. Economy services own their own transactional boundaries.
4. Use immutable snapshots or copy-on-write state for read projections.
5. Make command handlers submit intents to owning actor/service and return after authoritative result.
6. Add race tests that call runtime commands and tick loop concurrently without relying on the global lock.

---

## 3. Multiplayer Networking & Protocol Design

### Strengths

The realtime protocol has several good production-oriented choices:

- Strict request envelope validation:
  - `internal/game/realtime/envelope.go`
  - `DecodeRequestEnvelope`
  - `Validate`
- Operation registry:
  - `registeredOperations`
- Server resolves session identity server-side:
  - `internal/game/realtime/gateway.go`
  - `Gateway.HandleRequest`
  - `executeResolved`
- Handlers reject server-owned client payload keys:
  - `internal/game/server/handlers.go`
  - `trustedClientPayloadKeys`
  - `rejectTrustedPayload`
- Client also rejects forbidden server payload keys:
  - `client/src/protocol/envelope.ts`
  - `rejectForbiddenPayloadKeys`

This is a strong anti-cheat foundation.

### Issue: request cache keys only by session + request ID

**Severity:** Medium  
**Affected files/functions:**

- `internal/game/realtime/request_cache.go`
  - `RequestCache.GetOrRemember`
  - `newRequestCacheKey`
- `internal/game/realtime/gateway.go`
  - `HandleRequest`

**Why it matters:**

The cache key is:

```text
session_id + request_id
```

It does not include:

- operation;
- payload hash;
- protocol version;
- command semantic idempotency key.

If a buggy or malicious client reuses the same `request_id` with a different operation/payload, the server returns the first cached response. This probably will not mutate state twice, but it can create confusing client behavior, incorrect UI reconciliation, and hard-to-debug replay behavior.

**Suggested fix:**

Store request metadata with the cached response:

```text
session_id
request_id
op
payload_hash
protocol_version
```

On duplicate:

- if op/payload hash match, return cached response;
- if mismatch, return `ERR_INVALID_PAYLOAD` or `ERR_REQUEST_REPLAY_MISMATCH`.

### Issue: no durable event replay despite reconnect cursor

**Severity:** High  
**Affected files/functions:**

- `internal/game/server/runtime_sessions.go`
  - `bootstrapEvents`
  - `eventSeq`
  - `ReconnectCursor`
- `internal/game/server/runtime_sessions.go`
  - `eventAtLocked`
- `client/src/net/realtime-client.ts`
  - reconnect/status handling
- `client/src/state/reducer-events.ts`

**Why it matters:**

The client receives `ReconnectCursor`, and events have sequence numbers, but there is no durable per-session event log or replay path. On reconnect, the client gets snapshots, but any event-only details between disconnect and snapshot are lost.

For a game, that is usually acceptable for position snapshots, but risky for:

- combat feedback;
- loot created/removed;
- market/auction notifications;
- quest progress;
- route settlement;
- premium/admin events;
- death/repair transitions.

**Suggested fix:**

Decide explicitly:

- **Snapshot-only recovery:** remove or de-emphasize reconnect cursor, and make every important event reconciled by snapshot/query.
- **Replay recovery:** add bounded per-player event ring or durable outbox cursor. Client reconnects with last seq and receives missed events before latest snapshot.

For production, prefer:

```text
event stream for recent UX events + authoritative snapshot reconciliation
```

---

## 4. Real-Time State Synchronization

### Issue: WebSocket event writes can block the game tick loop

**Severity:** Critical  
**Affected files/functions:**

- `internal/game/server/server.go`
  - `Run`
  - calls `runtime.StartWithEventSink`
- `internal/game/server/runtime.go`
  - `StartWithEventSink`
- `internal/game/server/transport.go`
  - `writeEventsToSession`
  - `writeEvents`
  - `writeText`

**Why it matters:**

`StartWithEventSink` runs a ticker goroutine. After each tick, it calls the sink. The sink writes events to WebSocket connections synchronously:

```text
tick goroutine
  -> sink(sessionID, events)
    -> writeEventsToSession
      -> writeText
        -> conn.Write with SocketWriteTimeout
```

A slow or stuck client can block writes up to the configured write timeout. That blocks the simulation/event loop and can cause:

- tick drift;
- delayed AOI updates for all sessions;
- delayed durable outbox pumping;
- global latency spikes;
- soft DoS by slow WebSocket readers.

**Suggested fix:**

Use per-connection writer goroutines:

```text
simulation tick -> enqueue event batch into bounded channel -> writer goroutine writes socket
```

Rules:

- bounded queue per connection;
- drop/coalesce AOI updates if queue is full;
- always keep latest world snapshot/correction;
- disconnect slow clients after queue overflow;
- never block simulation on network writes.

### Issue: `queuedEvents` is process-local and loosely bounded

**Severity:** Medium  
**Affected files/functions:**

- `internal/game/server/combat_loot_helpers.go`
  - `queueEventLocked`
  - `drainQueuedEventsLocked`
  - `drainQueuedEventsBySessionLocked`
- `internal/game/server/runtime_durable_outbox_realtime.go`
  - durable event pump integration

**Why it matters:**

Events are appended into `runtime.queuedEvents[sessionID]`. They are drained after commands or by the runtime pump. In edge cases, stale sessions or command paths that fail before draining can leave events queued longer than expected. At production scale, unbounded per-session slices become memory risk.

**Suggested fix:**

- Put explicit max queued events per session.
- Use typed event priority:
  - latest snapshot/correction replaces previous;
  - combat/loot UX events bounded;
  - terminal auth/death events high priority.
- Clear queues on session detach/revoke.
- Add metrics for queued events per session and dropped/coalesced events.

---

## 5. Movement, Combat & Projectile Logic

### Current movement model

Movement is server-owned:

- `internal/game/world/movement.go`
  - `AdvanceMovement`
  - `MovementPositionAt`
- `internal/game/world/worker/worker.go`
  - `movePlayerTo`
  - `advanceMovement`
- `internal/game/server/handlers.go`
  - `handleMoveTo`
  - `validateMoveIntentLocked`

The client sends a movement intent. The server checks bounds and max move distance.

### Issue: stop command does not settle current movement position

**Severity:** High  
**Affected files/functions:**

- `internal/game/world/worker/worker.go`
  - `stopPlayer`
- `internal/game/server/handlers.go`
  - `handleStop`
- `client/src/app/client-app-commands.ts`
  - `stopMovement`

**Why it matters:**

`movePlayerTo` settles the entity to current server time before starting a new timed movement. But `stopPlayer` only clears movement:

```go
entity.Movement = world.MovementState{}
worker.entities[entity.ID] = entity
```

It does **not** compute the current `MovementPositionAt`, and it does **not** update the spatial index.

Result:

- player can snap back to last tick position;
- client sees a correction that may feel like rubber-banding;
- range checks right after stopping can use stale position;
- spatial index may be stale until later mutation/tick.

**Suggested fix:**

When stopping:

1. compute settled position at `worker.clock.Now()`;
2. assign `entity.Position`;
3. clear movement;
4. update spatial index;
5. then store entity.

Add a regression test:

```text
start movement -> advance fake clock half ETA without worker tick -> stop -> entity position equals interpolated position, not old origin
```

### Issue: combat/loot interaction uses last worker position rather than exact server-time movement position

**Severity:** Medium-High  
**Affected files/functions:**

- `internal/game/server/combat_loot_helpers.go`
  - `syncPlayerCombatActorLocked`
  - `viewerForPlayerLocked`
- `internal/game/server/combat_loot_repair.go`
  - `handleCombatUseSkill`
  - `handleLootPickup`
- `internal/game/combat/service.go`
  - `validatedAttackActorsLocked`
- `internal/game/loot/service.go`
  - `PickupDrop`

**Why it matters:**

Combat and loot validation use `entity.Position` from the worker. If a command arrives between ticks while movement is active, that position is only as fresh as the last tick. At 20 Hz this is usually tolerable, but it creates edge cases:

- player sees target in range by interpolated state, server rejects;
- server accepts based on previous tick position while client thinks out of range;
- loot pickup near range threshold flickers;
- fast ships or future boosts magnify this.

**Suggested fix:**

Before interaction validation, settle the acting player position to `MovementPositionAt(now)` or send an explicit “settle actor” command to the map worker. Same for moving targets if combat should use continuous positions. Keep collision/range decisions in the map worker or an interaction actor so combat sees authoritative current positions.

### Current combat model

Combat is server-authoritative and hitscan-like:

- `internal/game/server/combat_loot_repair.go`
  - `handleCombatUseSkill`
- `internal/game/combat/service.go`
  - `ExecuteBasicAttack`
  - cooldown/energy/range/visibility checks
- `internal/game/server/combat_loot_helpers.go`
  - actor sync
  - target events

This is good for the MVP.

### Projectile logic risk

**Severity:** Medium  
**Affected files/functions:**

- Server:
  - `internal/game/combat/service.go`
- Client visual effects:
  - `client/src/render/world-renderer-effects.ts`
  - `client/src/render/world-renderer-*`

**Why it matters:**

The current server combat is effectively instant/hitscan. Client projectile/laser effects are presentation only. That is fine for basic lasers, but if future gameplay introduces rockets, mines, travel-time shots, AoE, or projectile dodging, the server must own projectile entities and collision. Do not let client-rendered projectiles become gameplay truth.

**Suggested fix:**

For future projectile weapons:

- represent projectile as server entity;
- tick position server-side;
- validate launcher, target, speed, TTL, collision, visibility;
- emit filtered projectile AOI events;
- client only interpolates.

---

## 6. Concurrency, Goroutines & Race Conditions

### Issue: `Worker` is not internally synchronized

**Severity:** High if used outside runtime lock; Medium in current single-runtime use  
**Affected files/functions:**

- `internal/game/world/worker/worker.go`
  - `Worker.Tick`
  - `Worker.Entity`
  - `Worker.Snapshot`
  - `Worker.InsertEntity`
  - `Worker.UpdateEntity`
  - `Worker.RemoveEntity`
  - `Worker.Run`
- `internal/game/world/worker/commands.go`
  - mailbox is synchronized, worker state is not

**Why it matters:**

The mailbox is thread-safe, but the worker state maps are not. Current server code mostly calls worker methods while holding `runtime.mu`, so it is hidden. But the worker exposes direct methods and has `Run(ctx)` that ticks independently. If any caller uses `Worker.Run` plus direct `Entity/Snapshot/InsertEntity`, data races are likely.

**Suggested fix:**

Pick one model:

1. **Strict actor model:** make direct mutation/query methods private or clearly test-only; all access through mailbox/snapshot requests.
2. **Mutex model:** add internal worker lock and consistently protect all maps.

For game-server scalability, actor model is better.

### Issue: cross-service locks can produce hidden deadlock risks later

**Severity:** Medium  
**Affected files/functions:**

- `internal/game/server/*handlers.go`
  - handlers hold `runtime.mu` and call services
- `internal/game/market/service.go`
  - holds market lock while calling wallet/inventory
- `internal/game/auction/service.go`
  - holds auction lock while calling wallet
- `internal/game/economy/wallet_service.go`
- `internal/game/economy/inventory_service.go`

**Why it matters:**

Today, service lock ordering mostly works because everything is in-process and simple. As soon as services call back into runtime, emit events, or become DB-backed with hooks, lock ordering can deadlock or create high tail latency.

**Suggested fix:**

Define lock hierarchy and avoid holding broad runtime locks across service calls. Prefer:

```text
validate under runtime/map ownership
release runtime lock
perform transactional domain mutation
reacquire only to publish cached snapshots/events
```

For economy, move to database transaction boundary instead of nested service locks.

---

## 7. Tick Loop / Game Loop Performance

### Issue: tick and AOI building happen under global runtime lock

**Severity:** High  
**Affected files/functions:**

- `internal/game/server/runtime_world_snapshot.go`
  - `tickAndCollectAOIEvents`
  - `aoiDiffEventsForInstanceLocked`
  - `aoiSnapshotForPlayerLocked`
- `internal/game/world/worker/worker.go`
  - `Tick`
- `internal/game/world/worker/enemy_aggro.go`
  - `tickEnemyAggroDefinition`
  - `nearestAggroTarget`

**Why it matters:**

Each tick:

1. locks runtime;
2. ticks all map instances;
3. syncs NPC combat projections;
4. builds AOI diffs for sessions;
5. queries spatial index;
6. applies visibility;
7. reads discovery/intel memory;
8. queues events.

All of this blocks command handlers.

At low player counts this is fine. At real MORPG counts it becomes the primary bottleneck.

**Suggested fix:**

- Per-map tick goroutine.
- Per-map event output channel.
- Runtime/router only handles session ownership and forwarding.
- AOI diff should be per-map and per-session, not global.
- Introduce tick duration metrics:
  - total tick ms;
  - command drain ms;
  - movement ms;
  - NPC aggro ms;
  - AOI ms;
  - event enqueue ms;
  - dropped/coalesced events.

### Issue: NPC aggro appears O(NPC × player) per tick

**Severity:** Medium-High  
**Affected files/functions:**

- `internal/game/world/worker/enemy_aggro.go`
  - `tickEnemyAggroDefinition`
  - `nearestAggroTarget`

**Why it matters:**

`nearestAggroTarget` iterates player entities for each NPC. This is fine with small starter maps. With many NPCs and players, this becomes expensive.

**Suggested fix:**

Use spatial index for player target acquisition:

```text
for npc:
  query players within aggro radius
  filter safe zone / stealth / eligibility
  choose nearest
```

Maintain separate spatial layers or entity type indexes to avoid scanning irrelevant entities.

---

## 8. Entity System & World Management

### Strengths

- Entities have world/zone ownership validation.
- Spatial index exists:
  - `internal/game/world/spatial`
- AOI filtering is separated:
  - `internal/game/world/aoi`
  - `internal/game/world/visibility`
- Map catalog/router exists:
  - `internal/game/world/maps`

### Issue: map worker ownership is not yet a true distributed ownership boundary

**Severity:** High for future multi-process deployment  
**Affected files/functions:**

- `internal/game/server/runtime.go`
  - `mapInstances`
  - single runtime owns all workers
- `internal/game/server/runtime_maps.go`
  - `activeMapInstanceLocked`
  - session attach/detach
- `internal/game/world/maps/router.go`

**Why it matters:**

Docs describe map actors/workers, but implementation is still a single process with multiple in-memory workers. Cross-map handoff is internal state mutation, not a durable/lease-backed transfer protocol.

This is fine for current development, but production will need:

- transfer state machine;
- source worker freeze/settle;
- destination worker insert;
- rollback if destination fails;
- session subscription epoch;
- durable player location;
- exactly-one live entity ownership.

Some subscription epoch protections already exist, which is good.

**Suggested fix:**

Before multi-process runtime:

1. persist active player map location;
2. add transfer table/state machine;
3. make source worker remove only after destination commit;
4. idempotency key for transfer;
5. recover in-progress transfer after crash;
6. add chaos tests for transfer failure at each step.

---

## 9. Database, Persistence & Player Progression

### Critical issue: core game state is volatile

**Severity:** Critical  
**Affected files/functions:**

- `internal/game/server/runtime.go`
  - `NewRuntime`
  - `auth.NewInMemoryStore`
  - `economy.NewInventoryService`
  - `economy.NewWalletService`
  - `market.NewMarketService`
  - `auction.NewService`
  - `quests.NewInMemoryQuestStore`
  - `discovery.NewInMemoryStore`
  - `production.NewInMemoryStore`
  - route/settlement/building in-memory stores
- `internal/game/auth/store.go`
  - `InMemoryStore`

**Why it matters:**

Production MORPG state must survive restart/crash. Currently a process restart loses:

- accounts;
- sessions;
- players;
- ship state;
- wallet balances;
- inventory/cargo;
- market listings;
- auction lots/bids;
- production state;
- route state;
- quest state;
- discovery/intel;
- active deaths/repairs;
- combat/loot/world state.

CMS content has PostgreSQL support, but player/gameplay persistence is not yet durable.

**Suggested fix:**

Prioritize durable stores in this order:

1. accounts/sessions/player profiles;
2. wallet ledger and inventory ledger;
3. ship/hangar/loadout;
4. market/auction escrow;
5. planet/discovery/intel;
6. production/routes/buildings;
7. quest/player quest progress;
8. world location and transfer state.

Do not attempt to persist everything at once. Start with wallet/inventory because economy correctness depends on it.

### Issue: CMS publish does not update live runtime content

**Severity:** High  
**Affected files/functions:**

- `internal/game/server/content_admin_handlers.go`
  - `handleAdminContentPublish`
  - `handleAdminContentRollback`
- `internal/game/admin/content_service.go`
  - `PublishDraft`
  - `Rollback`
- `internal/game/server/content_projection_handlers.go`
  - `handleContentCatalog`
- `internal/game/server/runtime.go`
  - `contentCatalogProjection`
  - `contentCatalogVersion`
  - `contentBundle` consumed once during `NewRuntime`

**Why it matters:**

The CMS can publish a new content version to the content DB, but the live runtime continues using the content loaded at startup:

- item catalog;
- loot tables;
- combat rules;
- map catalog;
- shop products;
- crafting recipes;
- production rules;
- player content projection.

So admin can see “publish succeeded,” while the actual game continues with old values. This is especially dangerous for balancing, shop prices, loot tables, combat rules, and maps.

**Suggested fix:**

Introduce explicit runtime content version application:

```text
admin publish -> validates -> DB current version changes -> runtime reload request
```

Then either:

- require server restart and clearly report `pending_runtime_restart`;
- or hot-swap a `ContentRuntimeBundle` behind atomic pointer/version lock.

For hot reload, classify content:

- safe live reload: display names, shop visibility, some prices;
- requires quiescence/restart: map bounds, spawn pools, item schema, recipe/building definitions with active jobs, combat formulas.

The response should say:

```json
{
  "published": true,
  "runtime_applied": false,
  "runtime_version": "old",
  "published_version": "new"
}
```

until live reload exists.

### Issue: CMS publish safety checks are narrow

**Severity:** Medium-High  
**Affected files/functions:**

- `internal/game/admin/content_service.go`
  - `validatePublishSafety`

**Why it matters:**

The current safety checks cover changed craft recipes with active craft jobs and changed production buildings with active buildings. Good start.

But content changes can also affect:

- active market listings for item definitions;
- equipped modules / loadout stat definitions;
- active NPC combat actors using old stat templates;
- active loot drops using old item definitions/loot tables;
- shop products in active purchase flows;
- route rules/storage definitions;
- premium/auction grant payloads.

**Suggested fix:**

Add content dependency readers by domain:

```text
ActiveMarketListingsByItemID
ActiveEquippedModulesByModuleID
ActiveLootDropsByItemID
ActiveNPCsByTemplateID
ActiveRoutesByResourceID
ActiveShopPurchaseLocks
```

At minimum, classify changed content IDs and mark publish as:

- safe;
- requires no active references;
- requires restart;
- requires migration.

---

## 10. Security, Anti-Cheat & Abuse Prevention

### Strengths

- Server resolves `player_id` from session, not payload:
  - `internal/game/realtime/gateway.go`
- Many server-owned fields are rejected in payloads:
  - `internal/game/server/handlers.go`
- Strict JSON decoding:
  - `decodeStrict`
  - `DecodeRequestEnvelope`
- WebSocket auth uses cookie session resolution:
  - `internal/game/server/transport.go`
- Origin checks exist:
  - `internal/game/auth/origin.go`
- Passwords are hashed and cookies are HttpOnly:
  - `internal/game/auth/password.go`
  - `internal/game/auth/http.go`

### Issue: rate limits are mostly metadata, not enforcement

**Severity:** High  
**Affected files/functions:**

- `internal/game/realtime/envelope.go`
  - `RateLimitPosture`
  - comment says metadata only
- `internal/game/auth/http.go`
  - `HTTPRouteSpecs`
  - comment says concrete enforcement can be wired later
- `internal/game/server/handlers.go`
  - movement has `lastMove`, but most ops do not
- `internal/game/server/scanner_providers.go`
  - scanner cooldown exists
- `internal/game/server/shield_repair.go`
  - shield repair tick interval exists

**Why it matters:**

The most abusable operations are public and authenticated:

- login/register brute force;
- session endpoint spam;
- market search;
- market buy/cancel;
- auction bid;
- quest reroll/claim;
- scan pulse;
- world snapshot;
- content admin list/get/update/publish if admin compromised;
- observability endpoints.

Some have gameplay cooldowns; many do not have infrastructure rate limits.

**Suggested fix:**

Add server-boundary rate limit middleware:

- unauthenticated IP limit for `/api/auth/login` and `/api/auth/register`;
- per-account/session limits for realtime ops;
- separate expensive-op buckets:
  - market search;
  - content admin validation/publish;
  - world snapshot;
  - route settle;
  - scan pulse;
- metrics for rejected rate limits;
- ensure rate-limit rejection happens before mutation.

### Issue: production cookie security can be misconfigured silently

**Severity:** Medium  
**Affected files/functions:**

- `internal/game/server/config.go`
  - `CookieSecure`
  - `ConfigFromEnv`
- `internal/game/auth/http.go`
  - `setSessionCookie`

**Why it matters:**

`CookieSecure` defaults false, which is fine for local development. But if deployed over HTTPS without setting `GAME_COOKIE_SECURE=true`, the session cookie is less safe.

**Suggested fix:**

At startup:

- if allowed origins include `https://` or addr is public/prod mode, require `CookieSecure=true`;
- add explicit `GAME_ENV=production` guard;
- fail startup if production and insecure cookies.

### Issue: debug operations are registered in the main protocol

**Severity:** Low-Medium  
**Affected files/functions:**

- `internal/game/realtime/envelope.go`
  - `OperationDebugSpawnNPC`
  - `OperationDebugSnapshot`
- `internal/game/server/handlers.go`
  - `handleDebugSnapshot`
  - `handleDebugSpawnNPC`

**Why it matters:**

Handlers correctly check `devMode`, so this is not currently an open exploit. But registering debug ops in the production protocol increases attack surface and test/prod drift.

**Suggested fix:**

- Keep handler guard.
- Also conditionally register debug handlers only in dev config.
- Add startup log/metric when dev mode is enabled.
- Consider build tag or separate admin-only namespace for debug.

---

## 11. API / Backend Services Review

### Realtime API

Good:

- strict envelope;
- operation registry;
- server-owned session resolution;
- command logging hooks;
- client/server forbidden payload checks.

Needs hardening:

- enforce rate limits;
- request cache mismatch detection;
- event replay/snapshot contract clarity;
- binary protocol later for high-frequency AOI.

### Auth API

Good:

- password hashing;
- HttpOnly cookie;
- origin check for mutating endpoints;
- max auth request body size;
- strict JSON.

Needs hardening:

- durable account/session store;
- brute-force rate limit;
- production secure-cookie enforcement;
- session pruning;
- admin seed operational docs and audit.

### CMS/Admin API

Good:

- admin role checks through `requireAdmin`;
- draft validation;
- publish audit log;
- PostgreSQL transaction/advisory lock for publish;
- audit scrubbing for sensitive keys.

Needs hardening:

- runtime content application status;
- broader active-reference safety checks;
- publish/rollback rate limits;
- optimistic version check exposed clearly in UI;
- audit around who changed what and when is good, but operational alerts are still needed.

---

## 12. Error Handling, Logging & Observability

### Strengths

- Domain errors with public-safe messages:
  - `internal/game/foundation/errors.go`
- Command logger and metric recorder:
  - `internal/game/observability`
- Metrics are wired into several paths.
- Admin observability endpoints exist.

### Issue: metric/log failures are often ignored

**Severity:** Medium  
**Affected files/functions:**

- `internal/game/combat/service.go`
  - `recordCombatActionMetric`
- `internal/game/server/runtime_world_snapshot.go`
  - `recordAOITickErrorLocked`
- multiple `runtime.record*Metric` patterns

**Why it matters:**

Ignoring metrics is fine for not breaking gameplay, but it means observability pipeline failure is silent. During production incidents, you need to know when telemetry is broken.

**Suggested fix:**

- Keep gameplay non-blocking.
- Add a fallback error counter/log for telemetry failure.
- Track:
  - metric write errors;
  - event encode errors;
  - event queue drops;
  - websocket slow-client disconnects;
  - tick overruns.

### Issue: no structured operational log around critical state transitions

**Severity:** Medium  
**Affected areas:**

- auth login/logout/session revoke;
- map transfer;
- market/auction settlement;
- CMS publish/rollback;
- death/repair;
- route settlement;
- premium provider events.

**Why it matters:**

When players report item/currency loss, you need traceable event IDs and ledger references across services.

**Suggested fix:**

Adopt structured logs with fields:

```text
player_id
session_id
request_id
operation
idempotency_key
entity_id
map_id
listing_id
auction_id
route_id
content_version
result
error_code
duration_ms
```

---

## 13. Load, Scalability & Memory Usage

### Issue: synchronous write fanout + global lock limits concurrency

**Severity:** Critical/High  
**Affected files/functions:**

- `internal/game/server/transport.go`
  - `writeEventsToSession`
- `internal/game/server/runtime.go`
  - `StartWithEventSink`
- `internal/game/server/runtime_world_snapshot.go`
  - `tickAndCollectAOIEvents`

**Why it matters:**

These combine into a severe production scaling risk:

```text
global runtime lock + tick loop + synchronous socket writes
```

A small number of slow clients or expensive AOI sessions can degrade every player.

**Suggested fix:**

- bounded writer queues;
- per-map tick loops;
- per-map AOI;
- metrics for fanout latency;
- slow-client disconnect policy.

### Issue: AOI snapshots are rebuilt per session

**Severity:** Medium-High  
**Affected files/functions:**

- `internal/game/server/runtime_world_snapshot.go`
  - `aoiSnapshotForPlayerLocked`
  - `aoiDiffEventsForInstanceLocked`
- `internal/game/world/aoi`

**Why it matters:**

Each session builds visibility and diffs individually. This is correct, but expensive. With radar/stealth/fog it cannot be trivially shared, but spatial query and static map data can be optimized.

**Suggested fix:**

- collect per-map entity snapshot once per tick;
- pre-index player viewers;
- cache stat/visibility inputs per player per tick;
- use entity version numbers to avoid serializing unchanged payloads;
- coalesce AOI updates under backpressure.

---

## 14. Testing Gaps & Simulation Tests

The repo already has many focused tests, which is a strength. The remaining gaps are mostly **production simulation tests**, not ordinary unit tests.

### Recommended high-value tests

#### 1. Slow WebSocket client simulation

**Severity covered:** Critical  
**Targets:**

- `server.Run`
- `StartWithEventSink`
- `writeEventsToSession`

**Test idea:**

Connect a client that stops reading. Verify ticks still proceed and other clients receive events. This test should fail with the current synchronous writer design.

#### 2. Movement stop settlement test

**Severity covered:** High  
**Targets:**

- `worker.stopPlayer`
- `handleStop`

**Test idea:**

Start movement, advance fake clock half the ETA without normal tick, stop, assert server position is interpolated current position.

#### 3. Market/auction injected failure tests

**Severity covered:** Critical  
**Targets:**

- `market.BuyListing`
- `auction.PlaceBid`
- `auction.BuyNow`

**Test idea:**

Use fake wallet/inventory that fails after debit or after seller credit. Assert no lost funds/items, or assert recovery/outbox record exists. Current code likely cannot satisfy this fully without transactional redesign.

#### 4. Runtime CMS publish/reload test

**Severity covered:** High  
**Targets:**

- `handleAdminContentPublish`
- `handleContentCatalog`
- runtime item/module/shop/combat catalogs

**Test idea:**

Publish changed module damage/shop price. Then query `content.catalog` and perform combat/shop action. Assert whether runtime applied new version or clearly reports pending restart. Current code appears to keep old runtime projection.

#### 5. Race tests around runtime commands and tick

**Severity covered:** High  
**Targets:**

- runtime handlers;
- worker methods;
- map transfer;
- combat/loot pickup.

**Command:**

Eventually run:

```bash
go test -race ./internal/game/...
```

#### 6. Load simulation

**Severity covered:** High  
**Targets:**

- AOI;
- NPC aggro;
- event fanout;
- command latency.

**Test idea:**

Simulate:

- 100 players;
- 500 NPCs;
- 1,000 loot drops;
- 20Hz ticks;
- random move/combat/loot commands;
- measure tick p95/p99 and command p99.

---

## 15. Code Quality, Maintainability & Go Idioms

### Strengths

- Good package naming by domain.
- Strong use of value objects and validation.
- Domain errors are public-safe.
- Tests are abundant.
- Idempotency keys are domain-specific in many places.
- Client/server payload guard tests appear to exist.
- CMS audit scrubbing is thoughtful.

### Main maintainability issue: `Runtime` is too large

**Severity:** High  
**Affected file/type:**

- `internal/game/server/runtime.go`
  - `Runtime`

**Why it matters:**

`Runtime` owns everything:

- auth;
- sessions;
- map workers;
- player state;
- combat;
- death;
- loot;
- economy;
- market;
- auction;
- premium;
- quests;
- crafting;
- production;
- discovery;
- CMS;
- metrics;
- content catalogs;
- event queues.

This makes small changes risky and encourages broad locking. It also makes it harder to split runtime into real production services later.

**Suggested fix:**

Split into narrower runtime coordinators:

```text
SessionRuntime
WorldRuntime / MapRuntime
CombatRuntime
EconomyRuntime
DiscoveryRuntime
ProductionRuntime
AdminContentRuntime
RealtimeEventBus
```

The top-level `Runtime` can compose them, but should not own all mutation logic directly.

### Issue: “in-memory MVP” services are production-shaped but not production-safe

**Severity:** Medium-High  
**Affected packages:**

- `auth`
- `economy`
- `market`
- `auction`
- `production`
- `discovery`
- `quests`

**Why it matters:**

The code often looks production-like because it has locks/idempotency/domain errors. That is good, but there is a risk of accidentally deploying in-memory services as if they were durable.

**Suggested fix:**

Add explicit runtime startup mode checks:

```text
GAME_ENV=production requires durable stores for auth/economy/progression/world
```

Fail fast if production mode uses in-memory stores.

---

## 16. Critical Issues & Suggested Fix Plan

### Critical / High findings table

| ID | Severity | Affected file/function | Why it matters | Suggested fix |
|---|---:|---|---|---|
| CR-01 | Critical | `internal/game/server/runtime.go` / `NewRuntime`; `internal/game/auth/store.go` / `InMemoryStore`; in-memory economy/market/auction/production stores | Restart loses accounts, sessions, wallet, inventory, market, progress, routes, world state | Add durable Postgres-backed stores starting with auth, wallet ledger, inventory ledger, ship/loadout, market/auction escrow |
| CR-02 | Critical | `internal/game/server/transport.go` / `writeEventsToSession`, `writeText`; `runtime.StartWithEventSink` | Slow WebSocket clients can block tick/event loop | Per-connection bounded writer goroutines; drop/coalesce AOI; disconnect slow readers |
| CR-03 | Critical | `internal/game/market/service.go` / `BuyListing`; `internal/game/auction/service.go` / `PlaceBid`, `BuyNow` | Wallet/inventory mutations are multi-step and not durable atomic transactions | Move settlement into DB transaction/outbox with row locks and recovery; inject-failure tests |
| HI-01 | High | `internal/game/server/runtime.go` / `Runtime.mu`; most handlers | Global mutex serializes all commands, ticks, AOI, maps, economy | Per-map actor loops and smaller locks; avoid runtime lock across service calls |
| HI-02 | High | `content_admin_handlers.go` / `handleAdminContentPublish`; `content_projection_handlers.go` / `handleContentCatalog`; `runtime.go` content fields | CMS publish succeeds but live runtime keeps old content | Add runtime content reload/apply contract or return `pending_restart/runtime_applied=false` |
| HI-03 | High | `internal/game/world/worker/worker.go` / `stopPlayer` | Stop clears movement without settling current position; causes rubber-band/desync | Settle position at current server time, update spatial index, then clear movement |
| HI-04 | High | `runtime_sessions.go` / `eventSeq`, `ReconnectCursor`; event queue | Reconnect cursor has no durable replay; missed events are lost | Snapshot-only contract or durable/recent event replay ring |
| HI-05 | High | `realtime/envelope.go` rate posture; `auth/http.go` route specs | Rate limits are documented but mostly not enforced | Add per-IP/per-session/per-op limiters before mutation |
| HI-06 | High | `world/worker/worker.go` public methods | Worker state maps are unsafe if accessed concurrently outside runtime lock | Enforce actor-only access or internal locking; race tests |
| HI-07 | Medium-High | `world/worker/enemy_aggro.go` / `nearestAggroTarget`; AOI snapshot builders | NPC aggro and AOI can become O(N×M) bottlenecks | Use spatial queries for target acquisition; per-tick visibility caches |
| HI-08 | Medium-High | `admin/content_service.go` / `validatePublishSafety` | CMS safety only checks active craft/production definitions | Add active-reference checks for market, loadout, loot, NPCs, routes, shop |
| MD-01 | Medium | `realtime/request_cache.go` / `GetOrRemember` | Reused request ID with different op/payload returns stale cached response | Store op/payload hash with request cache and reject mismatches |
| MD-02 | Medium | `server/config.go`, `auth/http.go` | Secure cookie defaults are local-dev friendly but production can be misconfigured | Production mode must require `CookieSecure=true` |
| MD-03 | Medium | `combat_loot_helpers.go` / `syncPlayerCombatActorLocked`, `viewerForPlayerLocked` | Combat/loot validation uses last tick position, not exact movement time | Settle movement before interaction validation |
| MD-04 | Medium | `server/handlers.go`, `realtime/envelope.go` debug ops | Debug ops exist in main protocol, gated by dev mode only | Register only in dev; add startup warning |
| MD-05 | Medium | `observability` integration and ignored metric errors | Telemetry failures invisible during incidents | Add fallback telemetry-error counters/logs |

### Suggested staged fix plan

#### Phase 1 — Do immediately before larger playtests

1. Fix `stopPlayer` movement settlement.
2. Add request cache op/payload mismatch rejection.
3. Add basic auth and realtime operation rate limits.
4. Add slow-client writer queue architecture.
5. Make CMS publish response clearly report whether runtime applied the version.

#### Phase 2 — Economy correctness

1. Build durable wallet ledger and inventory ledger.
2. Move market buy/cancel/create into transaction or outbox.
3. Move auction bid/buy-now/close into transaction or outbox.
4. Add injected-failure tests for every economy settlement step.
5. Add escrow reconciliation admin report.

#### Phase 3 — Runtime scaling

1. Split map worker tick loops from global runtime lock.
2. Add per-map event output queues.
3. Move AOI diff building out of global lock.
4. Use spatial query for NPC aggro target selection.
5. Add tick p95/p99 metrics and load simulation.

#### Phase 4 — Persistence and recovery

1. Durable auth/session/player profile.
2. Durable ship/hangar/loadout.
3. Durable quest progress.
4. Durable discovery/intel/planet ownership.
5. Durable production/routes/buildings.
6. Durable player active map/location and transfer state.

#### Phase 5 — CMS live operations

1. Content version dependency graph.
2. Runtime hot-reload or explicit restart-required workflow.
3. Active-reference publish blockers.
4. Rollback safety by content type.
5. Runtime/content version dashboard.

---

## Final assessment

The project is in a good state for an actively developed vertical slice. The main design direction is correct: the client sends intents, server owns truth, visibility is filtered, and most gameplay systems already have domain-specific boundaries.

The production blocker is that the current server is still a **single-process, in-memory, globally locked integration runtime**. That is normal for this stage, but it should now be treated as the next major hardening boundary. The highest-impact fixes are not UI polish or new mechanics; they are durable economy/auth state, non-blocking networking, per-map ownership, CMS runtime version correctness, and simulation/load/race tests.
