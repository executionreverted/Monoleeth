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

## Subagent Review Additions - 2026-06-20

- Add concrete reducer tasks for `planet.storage_summary` and
  `route.snapshot`. Server payloads that return `planet_storage` or singular
  `route` must reconcile into client state without requiring a full list
  refresh.
- Remove disabled primary placeholders from normal planet UI. `Claim`, `Build`,
  `Upgrade`, `Route`, and `Auto` appear only when meaningful and backed by a
  real contract; otherwise use quiet game copy or hide the section.
- Remove internal copy from planet detail, including `server policy`,
  `server-owned routes`, and implementation-lock explanations.
- Add display metadata to planet storage/routes. Resource rows need display
  name/category/art key or catalog reference data; UI must not render raw
  `item_id` or `resource_item_id` as player-facing labels.
- Planet detail opening should issue or schedule refreshes for detail,
  production, storage, and routes, and claim/route success must reconcile all
  related snapshots.

## Second Subagent Review Additions - 2026-06-20

- Align smoke with the plan. Browser smoke must fail on disabled primary
  placeholders such as `Claim Locked`, `Build`, `Upgrade`, `Route`, and `Auto`
  when their actions are not meaningful or backed by real contracts.
- Add an owner-checked production settlement wrapper before login/detail-open
  settlement is exposed. The detail open flow should settle, then refresh
  production, storage, and routes in deterministic order.
- Selecting/opening a planet must issue or schedule `discovery.planet_detail`,
  `planet.production_summary`, `planet.storage_summary`, and route refreshes
  without triggering movement.
- Storage and route payloads currently risk raw resource ids. Require display
  metadata or catalog refs for resource rows, route endpoints, route cargo, and
  production outputs.
- Claim/build/route success must refresh known planets, selected detail,
  production, storage, routes, wallet/inventory where costs apply, and any
  stale market/intel listings if those adapters are present.

## Third Subagent Review Additions - 2026-06-20

- Claim/build/route browser mutations are still not registered. Runtime exposes
  read ops for planet detail, production/storage summaries, and routes, but no
  `discovery.claim_planet`, `planet.building_*`, or route mutation handlers.
  UI must hide those primary actions until authenticated services are wired.
- Current planet UI still shows disabled primary placeholders for `Claim`,
  `Build`, `Upgrade`, `Route`, and `Auto`. These must become absent or quiet
  game status copy unless the action is meaningful and backed by a real
  contract.
- Planet detail open should trigger a deterministic refresh chain:
  `discovery.planet_detail` -> `planet.production_summary` ->
  `planet.storage_summary` -> `route.list`, without movement side effects.
- Backend planet inspection needs an owner-checked production settlement wrapper
  before returning fresh production, storage, and route views.
- Storage and route payloads must include display metadata/catalog refs for
  resources, route endpoints, buildings, and outputs instead of raw ids.

## Fourth Subagent Review Additions - 2026-06-20

- Resolve `planet.storage_summary` payload shape. If the server returns a
  collection such as `{ planet_storage: { planets: [] } }`, the reducer must
  merge every planet entry; if the UI expects a singular selected planet
  payload, the server contract and tests must change to match.
- Selected planet routes must include outbound and inbound routes. Direction,
  source label, destination label, endpoint ids, route cargo, and resource
  display metadata must be server-owned.
- Planet action visibility should use server `available_commands` or explicit
  `can_claim` / `can_build` / `can_route` capability state. Hard-coded disabled
  primary buttons remain a release blocker.
- Browser smoke must cover storage collection merge, inbound route rendering,
  and action visibility driven by server capabilities rather than local
  placeholders.

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
docs/plans/task-001/08-planets-production-routes-claim.md
```

## Acceptance Criteria

- [ ] Planet catalog opens a distinct detail/modal surface.
- [ ] Runtime wires real claim and route services before browser mutations are
      registered.
- [ ] Claim uses real `discovery.claim_planet` or remains hidden with a named
      blocker.
- [ ] Claim/build/route primary controls are hidden until their authenticated
      runtime handlers are registered and tested.
- [ ] Claim validates visibility/range/rank/cost/idempotency server-side.
- [ ] Claim transaction/recovery risk is resolved or browser claim remains
      blocked with a named durable-state blocker.
- [ ] Unclaimed planets do not show Build/Route as primary actions.
- [ ] Disabled primary placeholders for unavailable planet actions are absent
      from normal player UI and browser smoke asserts absence.
- [ ] Claimed planets show Production, Storage, and Routes sections/tabs.
- [ ] `planet.storage_summary`, `route.list`, and `route.snapshot` reconcile.
- [ ] `planet.storage_summary` collection and singular payload behavior is
      chosen, documented, and reducer-tested.
- [ ] Selected planet route UI includes inbound and outbound routes with
      direction and endpoint display metadata.
- [ ] Planet action visibility is driven by server capability state such as
      `available_commands` or `can_*` fields.
- [ ] Planet storage/routes render display metadata or catalog refs, never raw
      `item_id` / `resource_item_id` as player-facing labels.
- [ ] `planet.building_build`, `planet.building_upgrade`, and route mutations
      are real with server-owned wrappers or explicitly blocked and hidden.
- [ ] Planet detail open and claim/route/settlement success refresh related
      production/storage/route snapshots.
- [ ] Production settlement is exposed through an owner-checked wrapper before
      login/detail-open settlement is enabled.
- [ ] Planet select/detail open refreshes detail, production, storage, and
      routes in deterministic order without triggering movement.
- [ ] Internal server-policy copy is gone from player UI.
- [ ] Browser smoke verifies claim or documented locked blocker path.

## Verification

```bash
go test ./internal/game/server -run 'Test.*(Planet|Claim|Production|Storage|Route)' -count=1
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
```

Capture screenshots under:

```text
output/screenshots/task-001/08/
```
