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

- [x] Add structured JSON logging for gameplay commands.
- [x] Include `request_id` in the command log primitive/wrapper.
- [x] Include `player_id` in the command log primitive/wrapper.
- [x] Include `session_id` in the command log primitive/wrapper.
- [x] Include `world_id` and `zone_id` where relevant in the command log primitive/wrapper.
- [x] Include operation name in the command log primitive/wrapper.
- [x] Include error code in the command log primitive/wrapper.
- [x] Include reference ID for value mutations in the command log primitive/wrapper.
- [x] Ensure command log primitives do not leak hidden gameplay data unnecessarily.

Implementation note 2026-06-18:
`internal/game/observability` now has safe `CommandLogEntry`,
`MemoryCommandLogger`, and JSON-line `JSONCommandLogger` primitives with
clone-safe deterministic snapshots where applicable.
`internal/game/realtime.ObservedCommandExecutor` records safe command logs,
command counts, and error-code metrics from server-resolved session/player
context while keeping payload details out of logs. `internal/game/runtime`
now exposes an observed realtime command gateway for single-process runtime
dispatch; a concrete external WebSocket server transport remains future work.

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
- [x] Simulate offline planet settlements.
- [x] Simulate route settlements.
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
Offline planet and route settlement runners now use the authoritative production
store and automation route service, record production faucets and route-loss
sinks, and retry settlement at the same server timestamp to assert duplicate
no-op behavior.

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

- [x] Inspect player inventory.
- [x] Inspect player wallet ledger.
- [x] Inspect item ledger.
- [x] Reverse bad transaction using compensating entry.
- [x] Disable suspicious market listing.
- [x] Mark intel listing stale.
- [x] Refund auction bid through ledger.
- [x] Repair stuck craft job.
- [x] Dry-run offline settlement.
- [x] Dry-run route settlement.

Implementation note 2026-06-18:
`internal/game/admin` now composes existing economy, market, auction, crafting,
and production services for read-side inspection and repair. Currency/item
repair writes admin compensation ledger entries through wallet/inventory
services, market listing disable returns escrow through market cancellation, and
production/route dry-runs execute against cloned production stores.

## TODO: Security Review Gates

For every command:

- [x] Client sends only intent.
- [x] Server loads player/session from auth.
- [x] IDs are ownership-checked.
- [x] Amounts are positive and bounded.
- [x] Visibility/range is checked for world interactions.
- [x] Mutable value has transaction lock.
- [x] Item/currency mutation writes ledger.
- [x] Request ID and domain idempotency are handled.
- [x] Error message does not leak hidden info.
- [x] Broadcast occurs after commit.

Implementation note 2026-06-18:
`NewCommandSecurityReviewReport` now fail-closes on missing command security
checks. `Phase12CommandSecurityCoverage` now records satisfied or
not-applicable evidence for every required security check on the currently
registered realtime operations (`move_to`, `stop`, `debug_spawn_npc`, and
`debug_snapshot`). Future operations must add coverage or the coverage report
and tests fail closed.

## TODO: Release Gates

Before enabling each module beyond local development:

- [x] Unit tests pass.
- [x] Integration transaction tests pass.
- [x] Abuse tests pass.
- [x] Metrics exist.
- [x] Admin inspection exists for value-changing module.
- [x] Error codes are mapped.
- [x] Ledger reason is added.
- [x] Load test exists for expected throughput.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

Implementation note 2026-06-18:
`NewReleaseGateReport` now fail-closes on missing release gates and lists stable
missing check names. `Phase12ReleaseGateCoverage` now records module-by-module
gate evidence for all 16 module specs, including explicit not-applicable notes
where a gate does not apply. `Phase12LoadTestTargets` records the local expected
throughput envelope from the module spec; production soak/load execution remains
a deployment-time activity beyond this local readiness primitive.

## Abuse Test Suite

- [x] Negative amounts.
- [x] Enormous amounts.
- [x] Duplicate request ID.
- [x] Same command with different request IDs.
- [x] Hidden entity interaction.
- [x] Out-of-range pickup.
- [x] Market buy/cancel race.
- [x] Auction bid/buy-now race.
- [x] Premium webhook replay.
- [x] Offline settlement repeated.
- [x] Route toggle around settlement.
- [x] Locked skill unlock.
- [x] Broken module still active.

Implementation note 2026-06-18:
`Phase12AbuseTestCoverage` records executable Go-test evidence for each abuse
case, and `NewAbuseTestCoverageReport` fail-closes if required coverage is
missing. The coverage test parses referenced packages so stale test-function
references fail during `go test`.

## Data Retention Guidance

Operational logs:

- [x] Keep around 30 days unless policy changes.

Economy and security ledger:

- [x] Keep long-term or summarize into archive.
- [x] Never silently delete money/item ledger needed for support or fraud review.

High-volume telemetry:

- [x] Aggregate after operational retention period.

Implementation note 2026-06-18:
`DefaultDataRetentionGuidance` records the Phase 12 retention posture:
30-day operational logs, long-term or summarized economy/security ledgers, and
post-window aggregation for high-volume telemetry. The guidance fail-closes if
wallet, item, premium purchase, or auction sale ledgers would be silently
dropped with short-lived operational logs.

## Done Criteria

- [x] Core game loop has command logs, metrics, and key dashboards.
- [x] Economy mutations are observable by reason.
- [x] Simulation tests can catch duplicate value creation.
- [x] Admin repair uses compensating transactions, not silent edits.
- [x] Every module has release gate checklist coverage.
- [x] `go test ./...` passes.
- [x] `git diff --check` passes.

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
