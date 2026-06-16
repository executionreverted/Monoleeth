# AGENTS.md

## Project

This repo is a browser-first 2D space MORPG project.

Core direction:
- Go-first backend/server tooling.
- Server-authoritative game architecture.
- Browser-first client later.
- Avoid building gameplay logic that trusts the client.
- Keep Symphony/orchestration tooling in Go.
- Keep OpenAI/agent orchestration code separate from gameplay domain logic.

## Current Commands

Run before claiming changes are complete:

```bash
go test ./...
git diff --check
```

Use narrower commands first while developing, then run the full commands before final handoff.

## Required Reading

Before implementing gameplay or economy code, read the relevant module spec under:

```text
docs/plans/modules/
```

Start with:

```text
docs/plans/modules/00-index.md
```

For world, fog, procedural map, discovery, and planet logic, also read:

```text
docs/2026-06-17-world-system-design.md
```

For progression, economy, ships, modules, loot, craft, market, auction, premium, death, quests, and production, also read:

```text
docs/2026-06-17-progression-economy-systems-design.md
```

For stack and architecture context, read:

```text
docs/2026-06-16-space-morpg-architecture-notes.md
```

## Workflow

1. Read the issue/request and identify the exact module being touched.
2. Read the matching file in `docs/plans/modules/`.
3. Keep the change scoped to that module unless the request explicitly crosses boundaries.
4. Write down a short plan before large code changes.
5. Prefer small vertical slices over broad rewrites.
6. Add or update tests for server rules, transactions, and edge cases.
7. Run the narrowest useful test during development.
8. Run `go test ./...` and `git diff --check` before final handoff.
9. Report what changed, what was tested, and any remaining risk.

## Architecture Guardrails

Client requests are intents, not facts.

Never trust the client for:
- player id
- position
- speed
- damage
- hit/miss result
- cooldown
- energy
- XP
- loot
- item amount
- wallet amount
- quest progress
- craft completion
- market price totals
- planet ownership
- visibility/fog state

The server must validate:
- ownership
- range
- visibility / fog
- AOI/fog permission
- rank requirements
- inventory/cargo/storage capacity
- wallet balance
- cooldown
- energy
- item tradeability
- idempotency

## Economy Rules

Any item or currency mutation must go through the proper service and ledger.

Do not silently edit balances or inventories.

Use transaction-style flows:

```text
lock
validate
mutate
write ledger/event
commit
broadcast after commit
```

Protect against:
- duplicate reward claims
- double craft completion
- double loot pickup
- market buy/cancel races
- auction bid/buy-now races
- premium webhook replay
- negative amount exploits
- escrow item reuse

## Rate Limits And Spam

Cooldowns are gameplay rules. Rate limits are abuse and infrastructure protection. Use both where needed.

Every client operation should have an explicit rate-limit posture, especially:
- `combat.use_skill`
- `loot.pickup`
- `scan.pulse`
- `market.search`
- `chat.send`
- `mail.send`
- `quest.reroll`
- `inventory.move`

Server-timed systems must not be driven by client spam. Scanner pulses, production settlement, route settlement, cooldown recovery, and regeneration should be scheduled or validated by the server.

Prefer progressive handling:
- reject invalid/noisy requests cheaply
- throttle repeated spam
- temporarily mute non-critical channels when appropriate
- disconnect abusive sessions when needed

Do not let rate-limit code change gameplay truth. A rejected request must not partially mutate state.

## Duplicate Safety And Idempotency

Assume requests, events, webhooks, and workers can run more than once.

Use `request_id` for network retry safety, but use domain idempotency keys for state-changing operations:

```text
quest_reward:<player_quest_id>
craft_complete:<job_id>
loot_pickup:<drop_id>
auction_close:<auction_id>
premium_webhook:<provider_event_id>
offline_settlement:<planet_id>:<settlement_window>
```

Do not rely on "this should only happen once." Enforce it with:
- state machines
- row locks or ownership locks
- unique constraints
- ledger reference uniqueness
- idempotent event consumers

Internal event delivery should be treated as at-least-once. Every consumer that mutates state must tolerate duplicate events.

## Replication And Consistency

Gameplay write paths must go through the authoritative owner for that state.

Examples:
- active combat and movement through the zone/world worker
- wallet and inventory through their transaction services
- planet production through planet/production settlement ownership
- market and auction through escrow-backed services

Redis and other caches are acceleration layers, not truth.

Read replicas may be used for non-critical queries, but critical state changes must use authoritative storage or the owning worker:
- combat
- loot pickup
- wallet
- inventory
- market
- auction
- craft completion
- death
- repair
- planet claim

Broadcast after commit. If broadcast fails, clients must reconcile from snapshots or queries.

Cross-zone or cross-worker handoff must be an explicit state machine. Do not duplicate live entity ownership across workers without a handoff protocol.

## Visibility And Fog Rules

Hidden gameplay data must not be sent to the client.

Never leak:
- hidden planets
- hidden loot
- hidden NPC/player coordinates
- gameplay procedural seeds
- future spawn candidates
- loot table rolls
- scan results before server resolution

Every interaction must re-check visibility server-side.

This includes:
- attack
- pickup
- scan
- planet panel open
- route creation
- intel sharing
- coordinate item usage

## Gameplay Module Boundaries

Use the module specs as ownership boundaries.

Examples:
- `CombatService` calculates combat results, but does not mutate inventory directly.
- `LootService` creates drops, but inventory transfer goes through `InventoryService`.
- `DeathService` decides cargo drop and ship disable, but uses loot/inventory services for item movement.
- `CraftingService` validates recipes, but wallet/item movement goes through wallet/inventory services.
- `MarketService` uses escrow and ledger; it should not bypass inventory/wallet primitives.
- `StatAggregationService` is the source of effective server-side stats.

## Symphony And Tooling

Keep Symphony/orchestration code in Go.

Symphony code should stay separate from game server domain code. Do not mix issue orchestration, OpenAI client logic, or workflow runner concerns into gameplay modules.

When changing Symphony:
- keep prompts/templates explicit and testable
- avoid hidden global state
- keep config parsing covered by tests
- avoid committing secrets or local workspace output

## Code Shape

Prefer small, readable files with clear ownership.

Avoid large files. As a soft rule, when production code grows beyond 300-500 lines, consider splitting by responsibility.

Use domain-specific names instead of vague names:
- prefer `wallet_ledger.go` over `utils.go`
- prefer `loot_pickup.go` over `helpers.go`
- prefer `route_settlement.go` over `manager.go`
- prefer `quest_reward.go` over `common.go`

Avoid duplicate business rules. Good candidates for shared helpers:
- positive amount validation
- ownership checks
- idempotency checks
- ledger writes
- transaction/outbox patterns
- rate-limit middleware
- visibility/range validation primitives

Do not abstract too early. Small local duplication is acceptable while a rule is still changing, but once it becomes a gameplay or economy invariant, centralize it.

Keep functions focused:
- validation should be easy to find
- mutation should be transaction-scoped
- event publishing should happen after commit
- formatting/UI concerns should not live in domain services

## Documentation Rules

When adding a new gameplay system:
- add or update the matching module spec first
- document server ownership
- document data model
- document commands/events
- document edge cases
- document abuse vectors
- document testing checklist

When a design decision changes, update the docs in the same change as the code.

## Library And Cloud Documentation

Use Context7 MCP to fetch current documentation whenever the task asks about a library, framework, SDK, API, CLI tool, or cloud service.

This includes well-known tools such as React, Next.js, PixiJS, Tauri, Prisma, Redis, NATS, Docker, Kubernetes, OpenAI APIs, and Go libraries.

Do not rely on memory for current API syntax or setup instructions.

## Git And Safety

The worktree may contain user changes.

Do not revert, overwrite, or reformat unrelated files.

Keep commits minimal and clean.

Good commits have one reason to exist:
- one feature slice
- one bug fix
- one refactor
- one doc update
- one test update

Avoid mixed commits such as feature + refactor + formatting + docs unless the pieces are inseparable.

Before committing:
- inspect `git status --short`
- inspect the staged diff
- stage only files related to the task
- use a clear commit message

Prefer commit prefixes that describe intent:
- `docs:`
- `test:`
- `symphony:`
- `game:`
- `infra:`
- `refactor:`

Do not commit:
- secrets
- `.env`
- local logs
- `.symphony` workspaces
- generated dependency folders
- large temporary outputs
