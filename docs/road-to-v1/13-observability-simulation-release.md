# Phase 13 — Observability, Simulation & Release Gate

## Status
- State: Done (100%)
- Wave: 4 (runs alongside others), finalizes at v1
- Depends on: P02, P05 (and consumes signals from all phases)
- Unlocks: v1 sign-off

## Goal
Add external metrics, OpenTelemetry trace instrumentation, race/simulation/load
tests, and a release gate that must be green before v1, covering the critical
multiplayer/economy paths.

## Why (report refs)
- Code review §12, §13, §14: missing external observability, load/sim tests, race coverage.
- Feature-gap §9, §15: production observability + release evidence are required for v1.

## Scope
- Prometheus export and OpenTelemetry spans from the existing observability domain.
- Deterministic simulation tests for combat/loot/economy/production/routes.
- Lightweight load test for concurrent sessions + tick stability.
- Release gate aggregating passed/missing/evidence/freshness.

## Out Of Scope
- Full Grafana dashboards/alerts (can follow v1).
- Collector/exporter deployment config; runtime accepts an injected OTel provider,
  and tests use an in-memory recorder.

## Tasks
- [x] `[P:wave4/lane-D]` Export metrics via Prometheus endpoint + OTel traces for command/tick paths.
  - [x] Prometheus-compatible `GET /metrics` endpoint exports runtime
    `MetricRecorder` counters/gauges/duration summaries. Production startup now
    requires `GAME_METRICS_TOKEN`, and configured endpoints require
    `Authorization: Bearer <token>`.
  - [x] OTel spans cover command execution and runtime tick/AOI phases:
    `TestObservedCommandExecutorRecordsOTelCommandSpan` and
    `TestPhase13RuntimeTickRecordsOTelSpans`. Server startup now passes an
    injected `TracerProvider` into runtime/gateway wiring; no external collector
    is required for the proof.
- [x] `[P:wave4/lane-D]` Add deterministic simulation tests (extend `observability/simulations`) per critical loop.
  - [x] Combat/loot simulation now proves identical summaries across two runs,
    and release-gate evidence references that deterministic smoke.
  - [x] Economy simulation now proves one planet-production scenario reports a
    nonzero balanced source/sink item flow.
  - [x] Production and route settlement simulations now prove deterministic
    summaries across identical runs:
    `TestPhase13ProductionSettlementSimulationSummaryIsDeterministic` and
    `TestPhase13RouteSettlementSimulationSummaryIsDeterministic`.
- [x] `[P:wave4/lane-E]` Add a concurrent-session load harness asserting tick stability under N sessions.
  - [x] P13/P15 load envelope smoke now runs 1500 concurrent AOI viewers
    against 1552 simulated entity states and proves the configured visible
    payload budget remains bounded.
  - [x] Worker aggro load smoke now runs 1500 players and proves one NPC tick
    checks one spatial-radius candidate instead of scanning all players.
  - [x] Runtime AOI tick wall-clock stability smoke covers 128 active sessions
    within the 3s budget under the targeted race command:
    `TestPhase13P15RuntimeAOITickStabilityKeepsDurationBudget`.
- [x] `[P:wave4/lane-E]` Add `-race` integration test across command + tick + economy mutation.
  - [x] Command + tick + economy mutation race target passed:
    `go test -race ./internal/game/observability/... ./internal/game/server/... ./internal/game/production/... ./internal/game/world/... -run 'Load|Tick|AOI|Aggro|Race|Command|Economy|Route|Production|Phase13|Phase15|OTel|Trace' -count=1`.
- [x] `[P:wave4/lane-D]` Extend release gate to require persistence/economy/rate-limit/social/gate evidence.
  - [x] Release-gate coverage now fails closed when one required module/check
    evidence item is missing.
  - [x] Release-gate coverage references metrics, OTel trace, deterministic
    simulation, load, race, `go test ./...`, and `git diff --check` evidence,
    and `TestPhase12ReleaseGateCoverageCoversRequiredModulesAndChecks` passes.

## Server Ownership
- Observability must stay separate from Symphony orchestration (AGENTS.md).

## Smoke Tests (one assertion each)
- [x] Metrics endpoint exposes command count for one op.
- [x] Combat/loot simulation is deterministic across two runs.
- [x] Economy simulation reports balanced source/sink for one scenario.
- [x] Production settlement simulation is deterministic across two runs.
- [x] Route settlement simulation is deterministic across two runs.
- [x] Load harness keeps AOI payload and aggro candidate work inside the configured envelope for N sessions.
- [x] Runtime AOI tick wall-clock stability stays within the configured smoke budget.
- [x] OTel command and runtime tick spans record server-owned attributes through
  injected provider wiring.
- [x] Command + tick + economy mutation race target passes under `-race`.
- [x] Release gate fails when a required evidence item is missing.
- [x] Release gate coverage is green with referenced evidence.

## Done Criteria
- [x] External metrics endpoint and OTel trace instrumentation/provider injection
  available. Collector/exporter deployment config is out of scope here.
- [x] Simulation + load + race evidence feed a green release gate.

## Verification
```bash
go test ./internal/game/realtime/... ./internal/game/server/... ./internal/game/observability/... -run 'OTel|Trace|Load|Tick|AOI|Aggro|Simulation|Route|Production|ReleaseGate|Race|Phase13|Phase15|GateEvidence|Coverage' -count=1
go test -race ./internal/game/observability/... ./internal/game/server/... ./internal/game/production/... ./internal/game/world/... -run 'Load|Tick|AOI|Aggro|Race|Command|Economy|Route|Production|Phase13|Phase15|OTel|Trace' -count=1
go test ./...
git diff --check
```
