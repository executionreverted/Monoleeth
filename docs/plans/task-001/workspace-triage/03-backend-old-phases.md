# Workspace Triage 03 - Backend Old Phases

Date: 2026-06-21

Scope: TASK-0081, TASK-0082, TASK-0083, TASK-0084, TASK-0088, TASK-0089, TASK-0091, TASK-0092, TASK-0093, TASK-0094, TASK-0095, TASK-0099, TASK-0100, TASK-0101, TASK-0107, TASK-0174, TASK-0175, TASK-0176, TASK-0177, TASK-0178, TASK-0179, TASK-0180, TASK-0181, TASK-0182, TASK-0183, TASK-0184.

Authority: `/Users/canersevince/gameproject` `master` at `9447f6b`.

Method: inspected each workspace `git status --short`, `git diff --stat`, tracked changed files, untracked files, then compared each changed path against current main repo content. Some older workspaces have no local `master` ref, so file content from authoritative main repo was used as comparison source.

Docs read: `docs/symphony-worker-rules.md`, `docs/plans/task-001-goal.md`, `docs/plans/task-001/00-index.md`, `docs/todo.md`, `docs/plans/task-001/07-shop-market-catalog-rework.md`, `docs/plans/modules/09-market-auction-premium.md`, `docs/roadmap/00-index.md`.

## Counts

| Classification | Count |
| --- | ---: |
| APPLIED | 7 |
| SUPERSEDED | 19 |
| AUDIT_ONLY | 0 |
| LOST_PATCH | 0 |
| NEEDS_HUMAN_REVIEW | 0 |

## Classification Rules

APPLIED means intended code/test content is already exact in main, or only stale planning doc deltas remain.

SUPERSEDED means workspace code is older than current main. Applying it would remove newer server-authoritative checks, events, idempotency, fanout, tests, or auth/privacy guards.

No LOST_PATCH was found. No workspace should be merged from this batch.

## Per Workspace

| Workspace | Classification | Evidence | Recommended next action |
| --- | --- | --- | --- |
| TASK-0081 | SUPERSEDED | Scanner files differ, but main has newer scanner energy provider, player reveal provider, duplicate input matching, stationary checks, and many more tests. Workspace would remove current scan/reveal coverage. | Do not merge. Archive/ignore. |
| TASK-0082 | SUPERSEDED | Claim service differs, but main has production initializer, stale-listing marker, retry repair paths, and more claim tests. Workspace is older Phase08 claim slice. | Do not merge. Archive/ignore. |
| TASK-0083 | APPLIED | `internal/game/discovery/share.go` and `share_test.go` are exact matches to main. Only roadmap doc delta is stale. | No code action. |
| TASK-0084 | SUPERSEDED | Workspace exports `CoordinateScrollMetadata`; main intentionally keeps `coordinateScrollMetadata` unexported behind auth boundary. | Do not merge public method. |
| TASK-0088 | SUPERSEDED | Production foundation is older combined building catalog. Main split production catalog/state/storage/events and added route/settlement event payloads. | Do not merge. |
| TASK-0089 | APPLIED | `production/catalog.go`, `catalog_test.go`, and `types.go` are exact matches to main; doc/errors differ only because main has later additions. | No code action. |
| TASK-0091 | SUPERSEDED | Route/production settlement exists in main with event envelopes and later settlement tests. Workspace would drop newer settlement event behavior. | Do not merge. |
| TASK-0092 | SUPERSEDED | `claim_adapter.go` and test are exact in main, but workspace claim service/test/errors are older than current claim retry and stale-marker logic. | No merge; adapter already present. |
| TASK-0093 | SUPERSEDED | Route create/store exists in main, plus owner wrappers, settlement, and much broader route tests. Workspace is older route base. | Do not merge. |
| TASK-0094 | SUPERSEDED | Route settlement exists in main with later event payload and owner-route work. Workspace would remove newer route tests. | Do not merge. |
| TASK-0095 | SUPERSEDED | Route controls exist in main, and later TASK-0176 owner-checked wrappers are exact in main. This older patch lacks current owner settle/update coverage. | Do not merge. |
| TASK-0099 | SUPERSEDED | Inventory move is older. Main has `ToPlayerID`, batch system moves, cargo transfer guard hooks, duplicate result checks, and expanded tests. | Do not downgrade inventory code. |
| TASK-0100 | SUPERSEDED | Market service is older. Main has expire/stale flows, suspicious trade policy, extra idempotency helpers, broader tests, and Task001 shop hooks. | Do not merge. |
| TASK-0101 | SUPERSEDED | Premium service is older. Main adds provider risk lock/revoke, paid-premium listing policy, and more tests. | Do not merge. |
| TASK-0107 | SUPERSEDED | Observability primitives are older. Main adds JSON command logger, duration percentiles, economy metrics, release gates, retention, dashboards, and simulations. | Do not merge. |
| TASK-0174 | SUPERSEDED | Wallet files are exact in main, but repair service is older than current scoped in-flight duplicate coordination. | No merge; current main is safer. |
| TASK-0175 | APPLIED | `internal/game/discovery/claim_test.go` is exact match to main. | No code action. |
| TASK-0176 | APPLIED | `route_controls.go`, `route_service.go`, and `route_test.go` are exact matches to main. | No code action. |
| TASK-0177 | SUPERSEDED | Browser smoke patch is much older than main current smoke/auth/Task001 coverage. | Do not merge. |
| TASK-0178 | APPLIED | `death/repair.go` and `repair_service_test.go` are exact matches to main. | No code action. |
| TASK-0179 | SUPERSEDED | Scanner runtime energy-spend intent is present in main, but main `runtime.go`, `scanner_providers.go`, and `server_test.go` are much newer. | Do not merge. |
| TASK-0180 | APPLIED | Quest store/service/reroll/consumer files are exact matches to main. | No code action. |
| TASK-0181 | SUPERSEDED | Economy/quest observability runtime metrics are present in main, but current handlers/runtime/server tests have later expansions. | Do not merge. |
| TASK-0182 | APPLIED | Quest module doc, `model_test.go`, and `types.go` are exact matches to main. | No code action. |
| TASK-0183 | SUPERSEDED | Quest reward inventory adapter coverage is present in main; workspace docs/server test are older than current main. | Do not merge. |
| TASK-0184 | SUPERSEDED | Unimplemented mutation guard coverage exists in main with broader current client/realtime/browser smoke. Workspace is older. | Do not merge. |

## Recommendation

No lost backend patch exists in this batch. Close triage as no-merge. Smallest safe next action is cleanup/archive of stale workspaces after human confirms no external references depend on them.
