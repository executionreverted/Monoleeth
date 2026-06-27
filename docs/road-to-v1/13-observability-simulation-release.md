# Phase 13 — Observability, Simulation & Release Gate

## Status
- State: In progress (60%)
- Wave: 4 (runs alongside others), finalizes at v1
- Depends on: P02, P05 (and consumes signals from all phases)
- Unlocks: v1 sign-off

## Goal
Add external observability, race/simulation/load tests, and a release gate that
must be green before v1, covering the critical multiplayer/economy paths.

## Why (report refs)
- Code review §12, §13, §14: missing external observability, load/sim tests, race coverage.
- Feature-gap §9, §15: production observability + release evidence are required for v1.

## Scope
- Prometheus/OpenTelemetry export from the existing observability domain.
- Deterministic simulation tests for combat/loot/economy/production/routes.
- Lightweight load test for concurrent sessions + tick stability.
- Release gate aggregating passed/missing/evidence/freshness.

## Out Of Scope
- Full Grafana dashboards/alerts (can follow v1).

## Tasks
- [ ] `[P:wave4/lane-D]` Export metrics via Prometheus endpoint + OTel traces for command/tick paths.
  - [x] Prometheus-compatible `GET /metrics` endpoint exports runtime
    `MetricRecorder` counters/gauges/duration summaries. Production startup now
    requires `GAME_METRICS_TOKEN`, and configured endpoints require
    `Authorization: Bearer <token>`.
  - [ ] OTel traces for command/tick paths remain open.
- [ ] `[P:wave4/lane-D]` Add deterministic simulation tests (extend `observability/simulations`) per critical loop.
  - [x] Combat/loot simulation now proves identical summaries across two runs,
    and release-gate evidence references that deterministic smoke.
  - [x] Economy simulation now proves one planet-production scenario reports a
    nonzero balanced source/sink item flow.
  - [ ] Production/route deterministic coverage remains open beyond existing
    route settlement accounting smokes.
- [ ] `[P:wave4/lane-E]` Add a concurrent-session load harness asserting tick stability under N sessions.
  - [x] P13/P15 load envelope smoke now runs 1500 concurrent AOI viewers
    against 1552 simulated entity states and proves the configured visible
    payload budget remains bounded.
  - [x] Worker aggro load smoke now runs 1500 players and proves one NPC tick
    checks one spatial-radius candidate instead of scanning all players.
  - [ ] Wall-clock tick duration/stability budget remains open.
- [ ] `[P:wave4/lane-E]` Add `-race` integration test across command + tick + economy mutation.
  - [x] Narrow P13/P15 race target passed:
    `go test -race ./internal/game/observability/... ./internal/game/server/... ./internal/game/world/... -run 'Load|Tick|AOI|Aggro|Race|Phase13|Phase15' -count=1`.
  - [ ] Full command + tick + economy mutation race coverage remains open.
- [ ] `[P:wave4/lane-D]` Extend release gate to require persistence/economy/rate-limit/social/gate evidence.
  - [x] Release-gate coverage now fails closed when one required module/check
    evidence item is missing.

## Server Ownership
- Observability must stay separate from Symphony orchestration (AGENTS.md).

## Smoke Tests (one assertion each)
- [x] Metrics endpoint exposes command count for one op.
- [x] Combat/loot simulation is deterministic across two runs.
- [x] Economy simulation reports balanced source/sink for one scenario.
- [x] Load harness keeps AOI payload and aggro candidate work inside the configured envelope for N sessions.
- [x] Release gate fails when a required evidence item is missing.

## Done Criteria
- [ ] External metrics/traces available. Prometheus metrics are available; OTel
  traces remain open.
- [ ] Simulation + load + race evidence feed a green release gate. Load evidence
  now feeds the gate; OTel, production/route simulation, deeper race coverage,
  and final green gate remain open.

## Verification
```bash
go test ./internal/game/observability/... -count=1
go test ./... -race && git diff --check
```
