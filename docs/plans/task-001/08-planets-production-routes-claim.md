# Phase 08 - Planets, Claim, Production, And Routes

## Goal

Turn Planets from a flat readout into a real planet catalog/detail/action
system. Wire claim mechanics, show production/storage/routes as game state, and
hide irrelevant actions until the planet state makes them meaningful.

## Problems Covered

- Planet detail contains internal copy such as `Production is locked by server
  policy` and `No server-owned routes for this planet`.
- Claim/Build/Route buttons do not follow the planet state.
- Claim is not usable from the browser.
- Build/route actions show even when the planet is unclaimed.
- Planet modal feels like a radar expansion, not a planet catalog.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/plans/task-001/05-seeded-game-content-catalog.md
docs/todo.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/12-automation-routes.md
docs/plans/modules/13-intel-coordinate-trading.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
internal/game/server/discovery_production_handlers.go
internal/game/server/runtime.go
client/src/ui/hud.ts
client/src/state/reducer.ts
```

## Design Contract

- Planets window is a catalog of discovered/known planets.
- Selecting a planet opens detail in modal/window, not inline expansion.
- Unclaimed planet shows overview, coordinates if known, requirements, and
  `Claim` if claim is possible.
- Claimed planet shows tabs/sections: Overview, Production, Storage, Routes.
- Build/Upgrade/Route controls appear only after claim and only when the server
  contract is real or clearly unavailable as secondary game copy.
- Navigate uses server-returned known coordinates only.
- Stale intel is visually distinct from live/current detail.
- Claim/build/route browser mutations must not be exposed until runtime has
  real `ClaimService`/`AutomationRouteService` wiring, idempotency, recovery,
  and authenticated handlers.

## Implementation Plan

1. Wire claim contract.
   - Construct and inject `ClaimService` into runtime; do not rely only on raw
     `Discovery`/`Production` stores.
   - Map request IDs to domain idempotency references.
   - Add browser op `discovery.claim_planet` only if server service validates
     intel, range, unowned state, rank, X Core/currency, idempotency, and
     production init.
   - Add command builder, handler registration, reducer reconciliation, and
     smoke.
   - Resolve the current claim transaction/recovery risk, or mark browser claim
     as blocked: X Core consume, owner mutation, production init, and stale
     listing marking must not leave partial state.

2. Clean planet detail UI.
   - Remove internal lock/policy copy.
   - Hide Build/Route if unclaimed.
   - Show only meaningful actions for the selected state.
   - Use game copy for unavailable states.
   - Remove disabled primary Claim/Build/Upgrade/Route/Auto placeholders and
     any `server policy`, `server-owned`, or `not enabled in this slice` copy.

3. Production/storage/routes.
   - Ensure `planet.storage_summary` and production summary parse into state.
   - Ensure `route.list` and `route.snapshot` parse into state.
   - Add route create/update/enable/disable/settle only when backed by server
     route services and tests.
   - Construct and inject `AutomationRouteService` and a runtime
     `RouteCreatePolicyProvider` before registering route mutation ops.
   - Browser handlers must fill owner player id and route id server-side; never
     trust those fields from the client.
   - Define server-generated route ids, idempotency references, source/dest
     ownership, capacity, energy/upkeep policy, unsupported destination behavior,
     and duplicate settlement handling.
   - If durable route event/outbox support is still missing, keep route
     mutations blocked and leave read-only route summaries.

4. Planet catalog.
   - Build a browsable list with filters/status chips: unclaimed, claimed,
     stale, producing, route active.
   - Selected detail should not become one giant modal that mixes every system
     at once.
   - Opening detail refreshes planet detail, production, storage, and routes.
   - Claim success, route mutation, and settlement refresh all related snapshots.

5. Add tests.
   - Claim success and failure.
   - Wrong owner/range/stale intel rejection.
   - Production/storage route reconciliation.
   - UI hides impossible actions.
   - Claim initializes production and publishes/reconciles known planets,
     detail, production, and storage snapshots.
   - Route create/update/enable/disable/settle tests cover wrong owner,
     duplicate settlement, source empty, destination full, and energy/upkeep.

## Likely Files

```text
internal/game/server/discovery_production_handlers.go
internal/game/server/runtime.go
internal/game/discovery/claim.go
internal/game/production/route_service.go
internal/game/realtime/envelope.go
internal/game/realtime/envelope_test.go
internal/game/server/server_test.go
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/reducer.ts
client/src/state/types.ts
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
docs/plans/task-001/08-planets-production-routes-claim.md
```

## Acceptance Criteria

- [ ] Planet catalog opens a distinct detail/modal surface.
- [ ] Runtime wires real claim and route services before browser mutations are
      registered.
- [ ] Claim uses real `discovery.claim_planet` or remains hidden with a named
      blocker.
- [ ] Claim validates visibility/range/rank/cost/idempotency server-side.
- [ ] Claim transaction/recovery risk is resolved or browser claim remains
      blocked with a named durable-state blocker.
- [ ] Unclaimed planets do not show Build/Route as primary actions.
- [ ] Claimed planets show Production, Storage, and Routes sections/tabs.
- [ ] `planet.storage_summary`, `route.list`, and `route.snapshot` reconcile.
- [ ] `planet.building_build`, `planet.building_upgrade`, and route mutations
      are real with server-owned wrappers or explicitly blocked and hidden.
- [ ] Planet detail open and claim/route/settlement success refresh related
      production/storage/route snapshots.
- [ ] Internal server-policy copy is gone from player UI.
- [ ] Browser smoke verifies claim or documented locked blocker path.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Planet|Claim|Production|Storage|Route)' -count=1
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```

Capture screenshots under:

```text
output/screenshots/task-001/08/
```
