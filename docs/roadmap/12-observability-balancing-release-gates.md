# Phase 12: Observability, Balancing, And Release Gates

## Status

- State: In progress
- Owner: Production readiness and live operations
- Depends on: all prior phases incrementally
- Unlocks: safer playtests, balancing, fraud/debug workflows

## Goal

Add the metrics, logs, traces, simulation tests, economy dashboards, admin inspection tools, and release gates needed to operate a persistent economy game without guessing.

## Source Specs

Read before implementation:

- `docs/plans/modules/16-testing-observability-balancing.md`
- `docs/plans/modules/15-api-events-errors.md`
- `docs/2026-06-17-progression-economy-systems-design.md`
- `docs/2026-06-16-space-morpg-architecture-notes.md`

## Why This Is A Phase And A Cross-Cutting Rule

Some observability should be added from day one, especially request IDs and ledger reasons. This phase collects the broader production-readiness work after the core loop exists.

Do not wait until production to add economy visibility. Balancing without dashboards is guessing.

## Required Metrics

Gameplay:

- active players
- active entities
- zone tick duration p50/p95/p99
- commands per second
- errors by code
- combat actions per second
- loot created and picked per second

Economy:

- wallet delta by reason
- item delta by reason
- credits faucet and sink per day
- premium paid/earned traded
- X Core created and consumed
- craft jobs started and completed
- market volume
- auction volume
- repair costs paid
- route loss totals
- planet production totals

Infrastructure:

- websocket outbound bytes per player per second
- command latency p50/p95/p99
- DB transaction latency
- Redis hit rate later
- NATS event lag later
- GC pause
- memory per worker
- CPU per worker

## TODO: Structured Logs

- [ ] Add structured JSON logging for gameplay commands.
- [x] Include `request_id` in the command log primitive/wrapper.
- [x] Include `player_id` in the command log primitive/wrapper.
- [x] Include `session_id` in the command log primitive/wrapper.
- [x] Include `world_id` and `zone_id` where relevant in the command log primitive/wrapper.
- [x] Include operation name in the command log primitive/wrapper.
- [x] Include error code in the command log primitive/wrapper.
- [x] Include reference ID for value mutations in the command log primitive/wrapper.
- [x] Ensure command log primitives do not leak hidden gameplay data unnecessarily.

Implementation note 2026-06-18:
`internal/game/observability` now has safe `CommandLogEntry` and
`MemoryCommandLogger` primitives with clone-safe deterministic snapshots.
`internal/game/realtime.ObservedCommandExecutor` records safe command logs,
command counts, and error-code metrics from server-resolved session/player
context while keeping payload details out of logs. Authenticated gateway/runtime
command paths do not invoke this wrapper yet, so runtime gameplay command
logging remains open.

## TODO: Metrics

- [x] Add command count by op.
- [x] Add command error count by op and code.
- [x] Add zone tick duration metric.
- [x] Add visible entity count metric.
- [x] Add combat action metric.
- [x] Add loot created/picked metric.
- [x] Add wallet delta by reason metric.
- [x] Add item delta by reason metric.
- [x] Add craft job metric.
- [x] Add quest reward metric.
- [x] Add planet settlement metric.
- [x] Add route settlement metric.
- [x] Add market sale metric.
- [x] Add auction bid metric.

Implementation note 2026-06-18:
`MetricRecorder` now supports deterministic counters, gauges, and duration
summaries with p50/p95/p99, stable sorted label sets, and label-value safety.
Market sale and auction clearing helpers include item identity and quantity so
price dashboards can compute averages from local sources. `CombatService`
optionally emits combat action metrics after successful authoritative attacks,
and `LootService` optionally emits created/picked loot metrics after committed
drop creation or pickup.

## TODO: Simulation Tests

- [x] Build deterministic simulation runner for NPC kills.
- [x] Simulate many concurrent loot pickups.
- [x] Simulate market buy/cancel races.
- [x] Simulate auction bid/buy-now races.
- [ ] Simulate offline planet settlements.
- [ ] Simulate route settlements.
- [x] Track total item faucets.
- [x] Track total item sinks.
- [x] Track total currency faucets.
- [x] Track total currency sinks.
- [x] Assert no duplicate value creation.

Implementation note 2026-06-18:
`EconomyFlowAccumulator` now tracks duplicate-safe currency/item faucets and
sinks by stable value identity, reason, and reference. Deterministic simulation
runners for NPC kills and concurrent loot pickups now live under
`internal/game/observability/simulations`; the combat/loot runner uses the
authoritative combat, loot, cargo, and progression services, retries each NPC
death drop creation once, and fans out pickup attempts against each drop.
Market buy/cancel and auction bid/buy-now race runners now use the authoritative
market, auction, wallet, and inventory services and fail closed if item
conservation, escrow cleanup, refund, grant, or terminal-state checks drift.

## TODO: Economy Dashboards

- [x] Define dashboard for credits faucet/sink.
- [x] Define dashboard for X Core supply.
- [x] Define dashboard for top raw material supply.
- [x] Define dashboard for top processed material supply.
- [x] Define dashboard for market average prices.
- [x] Define dashboard for auction clearing prices.
- [x] Define dashboard for repair total.
- [x] Define dashboard for craft fees.
- [x] Define dashboard for route loss.
- [x] Define dashboard for planet production.

Implementation note 2026-06-18:
`RequiredDashboardSpecs` now defines stable dashboard keys and local source
references. These are specs only; no external Grafana/admin UI has been wired.

## TODO: Admin And Repair Tools

- [ ] Inspect player inventory.
- [ ] Inspect player wallet ledger.
- [ ] Inspect item ledger.
- [ ] Reverse bad transaction using compensating entry.
- [ ] Disable suspicious market listing.
- [ ] Mark intel listing stale.
- [ ] Refund auction bid through ledger.
- [ ] Repair stuck craft job.
- [ ] Dry-run offline settlement.
- [ ] Dry-run route settlement.

## TODO: Security Review Gates

For every command:

- [ ] Client sends only intent.
- [ ] Server loads player/session from auth.
- [ ] IDs are ownership-checked.
- [ ] Amounts are positive and bounded.
- [ ] Visibility/range is checked for world interactions.
- [ ] Mutable value has transaction lock.
- [ ] Item/currency mutation writes ledger.
- [ ] Request ID and domain idempotency are handled.
- [ ] Error message does not leak hidden info.
- [ ] Broadcast occurs after commit.

Implementation note 2026-06-18:
`NewCommandSecurityReviewReport` now fail-closes on missing command security
checks. Per-command reviews have not been run across every gameplay command yet.

## TODO: Release Gates

Before enabling each module beyond local development:

- [ ] Unit tests pass.
- [ ] Integration transaction tests pass.
- [ ] Abuse tests pass.
- [ ] Metrics exist.
- [ ] Admin inspection exists for value-changing module.
- [ ] Error codes are mapped.
- [ ] Ledger reason is added.
- [ ] Load test exists for expected throughput.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

Implementation note 2026-06-18:
`NewReleaseGateReport` now fail-closes on missing release gates and lists stable
missing check names. Module-by-module release gate reports have not been run yet.

## Abuse Test Suite

- [ ] Negative amounts.
- [ ] Enormous amounts.
- [ ] Duplicate request ID.
- [ ] Same command with different request IDs.
- [ ] Hidden entity interaction.
- [ ] Out-of-range pickup.
- [ ] Market buy/cancel race.
- [ ] Auction bid/buy-now race.
- [ ] Premium webhook replay.
- [ ] Offline settlement repeated.
- [ ] Route toggle around settlement.
- [ ] Locked skill unlock.
- [ ] Broken module still active.

## Data Retention Guidance

Operational logs:

- [ ] Keep around 30 days unless policy changes.

Economy and security ledger:

- [ ] Keep long-term or summarize into archive.
- [ ] Never silently delete money/item ledger needed for support or fraud review.

High-volume telemetry:

- [ ] Aggregate after operational retention period.

## Done Criteria

- [ ] Core game loop has command logs, metrics, and key dashboards.
- [ ] Economy mutations are observable by reason.
- [ ] Simulation tests can catch duplicate value creation.
- [ ] Admin repair uses compensating transactions, not silent edits.
- [ ] Every module has release gate checklist coverage.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.

## Resume Notes

If resuming here, start by asking: "Which production bug would be impossible to diagnose today?" Add the smallest metric, log, or admin inspection tool that would answer it.

2026-06-18: Phase 12 Task 1 added command log and metric primitives under
`internal/game/observability`. Continue with economy flow accounting, dashboard
definitions, release gates, and then runtime/domain instrumentation.

2026-06-18: Phase 12 Task 2 added duplicate-safe economy flow accounting under
`internal/game/observability`. Continue with dashboard definitions and release
gate reports before full runtime/domain instrumentation.

2026-06-18: Phase 12 Task 3 added dashboard specs plus release/security gate
report primitives under `internal/game/observability`. Continue with roadmap
verification and Phase 12 review/fixes.

2026-06-18: Phase 12 core observability wiring added a realtime observed-command
executor plus optional combat and loot metric hooks. Continue with deterministic
simulation runners, admin/repair tools, and module-by-module gate coverage.
