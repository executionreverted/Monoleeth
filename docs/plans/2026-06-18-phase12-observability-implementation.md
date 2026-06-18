# Phase 12 Observability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the first gameplay-domain observability primitives for Phase 12 without coupling game packages to Symphony or external telemetry backends.

**Architecture:** Add `internal/game/observability` as a small, dependency-light package. It owns structured command logs, in-memory metrics, economy flow accounting, dashboard definitions, and release gate reports. Existing gameplay services are not instrumented in this first slice; later tasks can wire these APIs into gateway/runtime/domain services.

**Tech Stack:** Go standard library, existing `internal/game/foundation` primitives, existing `internal/game/economy` ledger primitives where useful.

---

## Task 1: Command Logs And Metrics

**Files:**

- Create: `internal/game/observability/doc.go`
- Create: `internal/game/observability/errors.go`
- Create: `internal/game/observability/command_log.go`
- Create: `internal/game/observability/command_log_test.go`
- Create: `internal/game/observability/metrics.go`
- Create: `internal/game/observability/metrics_test.go`

**Steps:**

1. Add package docs stating that gameplay observability must not import
   `internal/symphony`.
2. Define validation errors for blank operation, blank metric name, invalid
   duration, negative metric value, and unsafe label names.
3. Add `CommandLogEntry` with request/player/session/world/zone/op/error code,
   reference id, duration, status, and timestamp.
4. Add `MemoryCommandLogger` that records cloned entries and returns sorted
   snapshots.
5. Add `MetricRecorder` with counters, gauges, and duration summaries.
6. Add helpers for command count, command error count, zone tick duration,
   visible entity count, wallet delta, item delta, craft job, quest reward,
   planet settlement, route settlement, market sale, and auction bid metrics.
7. Tests:
   - command log rejects missing op/request/session/player where required
   - command log JSON contains safe fields and no internal detail field
   - snapshots are clone-safe and deterministic
   - counters/gauges/durations aggregate by stable sorted label sets
   - command error metrics use error codes, not free-form messages

**Validation:**

```bash
GOCACHE=/tmp/phase12-observability-go-build go test ./internal/game/observability -count=1
```

## Task 2: Economy Flow And Simulation Accounting

**Files:**

- Create: `internal/game/observability/economy_flow.go`
- Create: `internal/game/observability/economy_flow_test.go`

**Steps:**

1. Define `ValueFlowDirection` as faucet or sink.
2. Define `EconomyFlowEntry` with currency or item id, amount, reason,
   reference id, direction, and timestamp.
3. Add `EconomyFlowAccumulator` that rejects duplicate references for the same
   value kind/direction/reason and returns duplicate-safe summaries.
4. Add summaries for total currency faucets/sinks and item faucets/sinks by
   reason.
5. Tests:
   - duplicate reference does not double count
   - faucet and sink totals are separated
   - negative/zero amounts are rejected
   - snapshots are sorted and clone-safe

**Validation:**

```bash
GOCACHE=/tmp/phase12-observability-go-build go test ./internal/game/observability -count=1
```

## Task 3: Dashboards And Release Gates

**Files:**

- Create: `internal/game/observability/dashboard.go`
- Create: `internal/game/observability/dashboard_test.go`
- Create: `internal/game/observability/release_gate.go`
- Create: `internal/game/observability/release_gate_test.go`

**Steps:**

1. Define dashboard specs for credits faucet/sink, X Core supply, raw material
   supply, processed material supply, market average prices, auction clearing
   prices, repair totals, craft fees, route loss, and planet production.
2. Define release gate checks from Phase 12: unit tests, integration
   transaction tests, abuse tests, metrics, admin inspection, error codes,
   ledger reason, load test, `go test ./...`, and `git diff --check`.
3. Define command security review checks: intent-only client payload,
   server-side player/session, ownership, positive bounded amounts,
   visibility/range, transaction lock, ledger write, idempotency, leak-safe
   error, broadcast after commit.
4. Add report methods that fail closed when required checks are missing.
5. Tests:
   - all required dashboards are present and stable
   - missing release gates produce failed reports with missing check names
   - complete reports pass
   - command security review reports missing checks explicitly

**Validation:**

```bash
GOCACHE=/tmp/phase12-observability-go-build go test ./internal/game/observability -count=1
```

## Task 4: Roadmap And Full Verification

**Files:**

- Modify: `docs/roadmap/12-observability-balancing-release-gates.md`
- Modify: `docs/roadmap/00-index.md`
- Modify: `docs/todo.md` if follow-ups are intentionally deferred

**Steps:**

1. Check only Phase 12 TODOs backed by code/tests.
2. Note that this slice creates domain observability primitives but does not yet
   wire every service or build external dashboards/admin endpoints.
3. Run narrow package tests.
4. Run full verification.
5. Stage only Phase 12 files.
6. Commit.

**Validation:**

```bash
GOCACHE=/tmp/phase12-observability-go-build go test ./internal/game/observability -count=1
GOCACHE=/tmp/gameproject-go-build go test ./...
git diff --check
```
