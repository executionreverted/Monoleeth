# AGENTS.md

## Project

This repo is a browser-first 2D space MORPG project.

Current active run:

-docs/road-to-v1/GOAL.md

Core direction:

- Go-first backend/server tooling.
- Server-authoritative game architecture.
- Browser-first client.
- Keep OpenAI/sakana/agent orchestration code separate from gameplay domain logic.

## Active Planning Docs

The active implementation plan lives under:

```text
docs/plans/ui-implementation/
```

Start with:

```text
docs/plans/ui-implementation/00-index.md
```

The old `docs/roadmap/` files are historical backend implementation records.
Do not use them as the active execution roadmap for this run.

## Current Commands

Run before claiming changes are complete:

```bash
go test ./...
git diff --check
```

For client work, also run:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
```

Use narrower commands first while developing, then run the full commands before
final handoff.

## Required Reading

Before starting a UI implementation phase, read:

```text
GOAL.md
docs/plans/ui-implementation/00-index.md
the matching docs/plans/ui-implementation phase file
docs/todo.md
```

For visual/UI work, also inspect:

```text
output/mockups/final-mockup.png
client/
```

For server contracts, gameplay, economy, world, or persistence integration,
read the relevant backend module spec under:

```text
docs/plans/modules/
```

If you need information about core modules lookup with:

```text
docs/plans/modules/00-index.md
```

For world, fog, procedural map, discovery, and planet logic, also read:

```text
docs/2026-06-17-world-system-design.md
```

For progression, economy, ships, modules, loot, craft, market, auction,
premium, death, quests, and production, also read:

```text
docs/2026-06-17-progression-economy-systems-design.md
```

For stack and architecture context, read this old mmd:

````text
docs/2026-06-16-space-morpg-architecture-notes.md
``` .

## Workflow

1. Read the user request and identify the exact UI implementation phase touched.
2. Read `docs/plans/ui-implementation/00-index.md` and the matching phase file. Read agents.md and refresh your whole context if you need to be sure.
3. Read relevant backend module specs and source files.
4. Keep changes scoped to the active phase unless the request explicitly crosses
   boundaries.
5. Write a short implementation plan before large code changes.
6. Prefer small vertical slices over broad rewrites.
7. Add or update tests for server rules, transactions, client reducers,
   protocol parsing, UI state, and edge cases.
8. Run the narrowest useful test during development.
9. For UI changes, verify in a browser with real server state, screenshots, and
   responsive viewports.
10. Update the relevant UI implementation phase checklist only for work actually
    completed and verified.
11. Run full verification before final handoff:
    - `go test ./...`
    - `npm --cache /tmp/gameproject-npm-cache run check` in `client/`
    - `git diff --check`
12. Report what changed, what phase was touched, what was tested, and any
    remaining risk.
13. Arayüzde çalışırken amacımız `/Users/canersevince/gameproject/output/mockups/final-mockup.png` tasarımındaki HUD'u birebir kopyalamak. Oyun için objeler ve diğer assetler (iconlar - map arkaplanı vs.) subagent spawnlatıp asset ürettirebilirsin. Tek kriter mockup dosyasında ilgili alanla birebir / çok benzer olması assetin. Olabildiğince ona yakın tutacağız. Ayrıca @output/assets/hud-svg klasöründe icon-marker vs gibi şeyler var. İşine yararsa kullanırsın.

## Real UI Rules

The browser client must not present fake gameplay as real gameplay.

Default client behavior:

- no fake HP/shield/energy
- no fake cargo
- no fake wallet
- no fake quest counts
- no fake inventory/loadout counts
- no fake planets
- no fake NPCs
- no fake loot
- no fake market/auction/premium data
- no fake!

* EVERYTHING MUST BE REAL - SERVER AUTHORITATIVE - REAL GAME STATE !

If the client is offline or unauthenticated, show login, disconnected, empty,
locked, or loading states. Demo fixtures may exist only behind explicit dev/test
switches such as `?demo=1` or a test-only harness.

Every visible gameplay value must come from:

- authenticated server snapshot
- server event
- server query response
- client-local pending UI state that is clearly pending and reconciled by server
  truth

## Auth And Session Rules

Mail/password auth is required for the real client run.

Server must own:

- account id
- player id
- session id
- session expiry
- admin role
- password hash
- WebSocket session resolution

Never store plaintext passwords. Never log passwords, password hashes, session
tokens, cookies, or reset secrets.

Admin account seeding must be explicit and reproducible. Prefer environment or
seed command inputs over hard-coded credentials. If a local dev default is added,
it must be documented as unsafe for production and easy to override.

The WebSocket handshake must resolve authenticated session state server-side.
The client must not send trusted player identity in command payloads.

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

- authentication
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
````

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

Cooldowns are gameplay rules. Rate limits are abuse and infrastructure
protection. Use both where needed.

Every client operation should have an explicit rate-limit posture, especially:

- `auth.login`
- `combat.use_skill`
- `loot.pickup`
- `scan.pulse`
- `market.search`
- `chat.send`
- `mail.send`
- `quest.reroll`
- `inventory.move`

Server-timed systems must not be driven by client spam. Scanner pulses,
production settlement, route settlement, cooldown recovery, regeneration, and
session expiry should be scheduled or validated by the server.

Do not let rate-limit code change gameplay truth. A rejected request must not
partially mutate state.

## Duplicate Safety And Idempotency

Assume requests, events, webhooks, and workers can run more than once.

Use `request_id` for network retry safety, but use domain idempotency keys for
state-changing operations:

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

Internal event delivery should be treated as at-least-once. Every consumer that
mutates state must tolerate duplicate events.

## Replication And Consistency

Gameplay write paths must go through the authoritative owner for that state.

Examples:

- active combat and movement through the zone/world worker
- wallet and inventory through their transaction services
- planet production through planet/production settlement ownership
- market and auction through escrow-backed services

Redis and other caches are acceleration layers, not truth.

Read replicas may be used for non-critical queries, but critical state changes
must use authoritative storage or the owning worker:

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

Broadcast after commit. If broadcast fails, clients must reconcile from
snapshots or queries.

Cross-zone or cross-worker handoff must be an explicit state machine. Do not
duplicate live entity ownership across workers without a handoff protocol.

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

## UI Implementation Boundaries

Use the UI implementation phase files as ownership boundaries.

Examples:

- Auth phase may add account/session infrastructure, but should not also build
  market UI.
- Gateway phase may wire transport and session resolution, but should not fake
  gameplay state to make panels look full.
- Combat/loot phase may expose attack and pickup controls, but inventory
  browsing belongs to the inventory/loadout phase.
- Market/auction/premium phase must use escrow and ledger-backed services; it
  must not create client-side price totals as truth.

## Symphony And Tooling

If you are running inside a Symphony-managed worker workspace, do not use this
file as your operating guide. Follow `docs/symphony-worker-rules.md` and the
task prompt instead. Do not spawn subagents, create Symphony tasks, dispatch
agents, or manage the Symphony queue.

Keep Symphony/orchestration code in Go.

Symphony code should stay separate from game server domain code. Do not mix
issue orchestration, OpenAI client logic, or workflow runner concerns into
gameplay modules.

### Symphony Agent Backends

Symphony can run worker tasks on more than one agent backend:

- `codex` (default): the Codex app-server backend (`codex` config).
- `crush`: the `crush` CLI backend (`crush` config), which can target external
  model providers, including:
  - GLM models on z.ai, e.g. model `zai/glm-5.2` with endpoint
    `https://api.z.ai/api/coding/paas/v4`.
  - Sakana models via provider, e.g. `sakana/fugu-ultra` with the
    matching compatible endpoint. ~/.config/crush/crush.json has it.

Selecting a backend/model:

- Global default: `agent.backend` plus `crush.{command,model,endpoint}` in the
  Symphony config.
- Per-task override: `agent_backend`, `agent_model`, `agent_endpoint` on
  `POST /api/v1/tasks` (also exposed in the `/tasks` dashboard form).
- The crush backend forwards the endpoint via `CRUSH_BASE_URL` /
  `OPENAI_BASE_URL`; provider API keys come from the environment and must never
  be committed or logged. Crush config already has required API keys and data;
  spawn tasks through Symphony without hard-coding secrets or custom provider
  URLs unless a task explicitly needs an endpoint override.

Agent selection guide:

- Use GLM 5.2 (`agent_backend: "crush"`, `agent_model: "zai/glm-5.2"`) for
  easy, mechanical, repetitive, or output-heavy implementation work.
- Use Sakana Fugu through Crush for serious reasoning, architecture decisions,
  planning, code plan review, research, snippet design, and complex debugging.
  Keep Fugu reasoning at `high` by default unless the task requires deeper
  analysis.
- For code plans and snippets, prefer Fugu first. Use its output as the task
  contract for worker agents.
- For pure execution or bulk file work, delegate to GLM 5.2, then have Fugu or
  the manager review the diff, tests, and assumptions before merging or
  claiming completion.
- For mixed work, split the job: Fugu plans and reviews; GLM 5.2 implements the
  narrow slices; Codex can still be used for baseline/default local tasks.
- Keep task prompts explicit about scope, files, tests, and "Do not commit".
  Provider choice does not relax any project rule.
- If a Sakana model override fails because Crush reports duplicate provider
  names, use the configured Crush default Sakana model for Fugu tasks instead
  of editing secrets or provider config during gameplay work.
- Always use Caveman skill for saving tokens.

This is orchestration-only configuration. It does not change any
server-authoritative gameplay rule, and gameplay domain code must not depend on
which agent backend or model produced a change. Coding rules applies for everyone.

## Code Shape

Prefer small, readable files with clear ownership.

Avoid large files. As a soft rule, when production code grows beyond 300-500
lines, consider splitting by responsibility.

Use domain-specific names instead of vague names:

- prefer `auth_session.go` over `utils.go`
- prefer `wallet_ledger.go` over `helpers.go`
- prefer `loot_pickup_handler.go` over `manager.go`
- prefer `route_settlement.go` over `common.go`

Avoid duplicate business rules. Good candidates for shared helpers:

- positive amount validation
- ownership checks
- idempotency checks
- ledger writes
- transaction/outbox patterns
- rate-limit middleware
- visibility/range validation primitives
- client-safe snapshot filtering
- no monolith - spaghetti code! small chunks of code for each module, readable good code!

Do not abstract too early. Small local duplication is acceptable while a rule is
still changing, but once it becomes a gameplay or economy invariant, centralize
it.

Keep functions focused:

- validation should be easy to find
- mutation should be transaction-scoped
- event publishing should happen after commit
- formatting/UI concerns should not live in domain services

## Documentation Rules

When exposing a backend system through the UI:

- update the matching UI implementation phase file first or in the same change
- document server ownership
- document command/query/event contracts
- document empty/loading/error states
- document abuse vectors
- document testing checklist

When a design decision changes, update docs in the same change as the code.

When implementation progress changes, update the matching phase file in
`docs/plans/ui-implementation/` in the same change as the code.

## Library And Cloud Documentation

Use Context7 MCP to fetch current documentation whenever the task asks about a
library, framework, SDK, API, CLI tool, or cloud service.

This includes well-known tools such as React, Next.js, PixiJS, Tauri, Prisma,
Redis, NATS, Docker, Kubernetes, OpenAI APIs, and Go libraries.

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

Avoid mixed commits such as feature + refactor + formatting + docs unless the
pieces are inseparable.

Before committing:

- inspect `git status --short`
- inspect the staged diff
- stage only files related to the task
- use a clear commit message

Prefer commit prefixes that describe intent:

- `docs:`
- `test:`
- `client:`
- `game:`
- `auth:`
- `infra:`
- `refactor:`

Do not commit:

- secrets
- `.env`
- local logs
- `.symphony` workspaces
- generated dependency folders
- large temporary outputs
