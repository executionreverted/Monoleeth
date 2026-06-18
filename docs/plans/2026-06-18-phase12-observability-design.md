# Phase 12 Observability Design

Date: 2026-06-18

## Goal

Add the first production-readiness layer for the game domain: structured command
logs, in-memory metrics, economy flow summaries, dashboard definitions, release
gate checklists, and deterministic simulation accounting primitives.

This is not a Prometheus/Grafana integration yet. Phase 12 starts with stable
domain-facing contracts that later adapters can export to Prometheus,
OpenTelemetry, dashboards, admin tools, or CI gates.

## Approaches Considered

1. Instrument every gameplay service directly now.
   - Pros: immediate real metrics from live service methods.
   - Cons: broad cross-package churn, high conflict risk, and premature coupling
     before the gateway/runtime integration is complete.

2. Build shared observability primitives first.
   - Pros: small package, testable in isolation, and safe for future services to
     adopt without importing Symphony or external telemetry dependencies.
   - Cons: metrics are not automatically emitted until later integration tasks.

3. Add a Prometheus/OpenTelemetry stack now.
   - Pros: close to production shape.
   - Cons: heavy infrastructure work before the game has a stable runtime
     gateway and persistence boundary.

Chosen approach: option 2.

## Package Shape

Create `internal/game/observability`.

Responsibilities:

- Command log entries with safe fields: request id, player id, session id, world
  id, zone id, operation, error code, reference id, duration, and timestamp.
- In-memory command log recorder for tests and local runtime slices.
- Metric recorder that tracks counters, gauges, and duration summaries by stable
  metric name and label set.
- Convenience helpers for command count/error metrics, zone tick duration,
  visible entity counts, wallet/item deltas, and module-specific event counts.
- Economy flow accumulator for faucet/sink accounting in deterministic
  simulation tests.
- Dashboard definition structs for Phase 12 economy dashboards without binding
  to a frontend or Grafana JSON format.
- Release gate checklist structs for module readiness and command security
  review. These should report missing gates explicitly instead of silently
  passing.

Non-responsibilities:

- No external metrics library.
- No HTTP endpoints.
- No admin mutation tools yet.
- No direct gameplay service imports unless a type is already a stable domain
  primitive such as foundation IDs, foundation codes, or economy ledger fields.
- No Symphony imports.

## Data Flow

Future gateway/service code will call observability helpers after validation or
mutation:

```text
request received
  -> validate auth/session/player
  -> execute domain command
  -> record command log and metrics
  -> commit state
  -> broadcast after commit
```

Economy services can later convert wallet/item ledger rows into faucet/sink
metrics by reason. The Phase 12 package should provide these aggregation APIs,
but this slice should not rewrite existing services.

## Error And Safety Rules

- Logs must not include hidden coordinates, procedural seeds, loot rolls, or
  full internal error details.
- Player-visible error text is not a metric label. Use stable error codes.
- Metrics label sets must be deterministic and sorted in snapshots.
- Negative economy delta inputs are rejected; direction is represented by
  explicit faucet/sink or ledger action.
- Release gates fail closed when a required check is missing.

## Testing Strategy

- Unit tests for command log validation and JSON shape.
- Unit tests for counter/gauge/duration snapshots and deterministic label
  ordering.
- Unit tests for economy faucet/sink accumulation by reason and no duplicate
  reference accounting.
- Unit tests for release gate missing-check reports.
- `go test ./internal/game/observability -count=1`.
- Full `go test ./...` and `git diff --check` before committing.
