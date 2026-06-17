# Phase 12: Observability, Balancing, And Release Gates

## Status

- State: Not started
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
- [ ] Include `request_id`.
- [ ] Include `player_id`.
- [ ] Include `session_id`.
- [ ] Include `world_id` and `zone_id` where relevant.
- [ ] Include operation name.
- [ ] Include error code.
- [ ] Include reference ID for value mutations.
- [ ] Ensure logs do not leak hidden gameplay data unnecessarily.

## TODO: Metrics

- [ ] Add command count by op.
- [ ] Add command error count by op and code.
- [ ] Add zone tick duration metric.
- [ ] Add visible entity count metric.
- [ ] Add combat action metric.
- [ ] Add loot created/picked metric.
- [ ] Add wallet delta by reason metric.
- [ ] Add item delta by reason metric.
- [ ] Add craft job metric.
- [ ] Add quest reward metric.
- [ ] Add planet settlement metric.
- [ ] Add route settlement metric.
- [ ] Add market sale metric.
- [ ] Add auction bid metric.

## TODO: Simulation Tests

- [ ] Build deterministic simulation runner for NPC kills.
- [ ] Simulate many concurrent loot pickups.
- [ ] Simulate market buy/cancel races.
- [ ] Simulate auction bid/buy-now races.
- [ ] Simulate offline planet settlements.
- [ ] Simulate route settlements.
- [ ] Track total item faucets.
- [ ] Track total item sinks.
- [ ] Track total currency faucets.
- [ ] Track total currency sinks.
- [ ] Assert no duplicate value creation.

## TODO: Economy Dashboards

- [ ] Define dashboard for credits faucet/sink.
- [ ] Define dashboard for X Core supply.
- [ ] Define dashboard for top raw material supply.
- [ ] Define dashboard for top processed material supply.
- [ ] Define dashboard for market average prices.
- [ ] Define dashboard for auction clearing prices.
- [ ] Define dashboard for repair total.
- [ ] Define dashboard for craft fees.
- [ ] Define dashboard for route loss.
- [ ] Define dashboard for planet production.

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
