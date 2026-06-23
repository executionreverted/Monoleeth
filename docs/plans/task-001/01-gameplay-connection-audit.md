# Phase 01 - Gameplay Connection Audit And Contract Parity

## Goal

Detect and fix the missing UI to server gameplay connections before the UI pass
turns placeholders into better-looking placeholders. This phase creates the
truth table for every visible menu/control and closes the small known reducer,
protocol, and double-send gaps that are safe to fix first.

## Problems Covered

- Missing UI/server gameplay links.
- Enabled or visible controls that do not perform real gameplay.
- Reducer drift for existing server responses/events.
- Double command emission from one UI click.
- Debug panels that imply real loops where the server contract is absent.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
docs/plans/ui-implementation/07-discovery-planets-production-routes.md
docs/plans/ui-implementation/08-market-auction-premium-economy.md
docs/plans/modules/15-api-events-errors.md
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
internal/game/realtime/envelope.go
internal/game/server/handlers.go
```

## Current Findings

- `planet.storage_summary` is requested and server-handled, but client state
  handling for `planet_storage` is weak or missing.
- `route.snapshot` exists, but reducer/UI handling expects route lists more than
  a singular route response.
- `death.ship_disabled` now updates client disabled state when received, and
  runtime maps domain `ship.disabled` events to client-safe
  `death.ship_disabled`, `ship.snapshot`, `player.snapshot`, and
  `movement.stopped` events. Full combat/zone-worker death E2E remains tracked
  in `docs/todo.md`.
- Market, auction, and premium events now reconcile reducer state when delivered
  to a client. Cross-session passive fanout is still session-scoped in backend
  handlers and remains an explicit backend blocker for true two-client passive
  updates.
- `auction.bid` can be double-sent from one click path.
- `admin.repair_craft_job` exists, but crafting job creation is not exposed in
  the normal browser loop.
- Missing browser loops include `inventory.move`, `progression.unlock_skill`,
  `crafting.start`, `crafting.complete`, `crafting.cancel`,
  `discovery.claim_planet`, `planet.building_build`,
  `planet.building_upgrade`, route mutations, `intel.share`, coordinate item
  create/use, and social/mail controls.
- Session/transport state transitions need audit: WebSocket open,
  `session.ready`, reconnect, auth expiry, pending-command cleanup, and command
  gating must prove the authenticated real client reaches a command-enabled
  state only after server-resolved session readiness.
- App command gating now has an explicit `canSendRealtimeCommand` unit test:
  real mode blocks `authenticated_pending_socket`, `connecting`,
  `reconnecting`, `offline`, `auth_expired`, and `error` states, and enables
  commands only after `session.ready` promotes state to `connected`.
- The control/contract matrix and envelope parity table now live in
  `docs/plans/task-001/01-control-contract-matrix.md`; later UI phases must
  keep it current when they enable or hide gameplay controls.
- Reducer-level pending command cleanup now covers socket loss and
  response-lost/event-delivered recovery for movement and scanner commands;
  browser-level reconnect smoke still needs to prove the full session loop.
- Event `seq` now rejects lower stale events at the reducer boundary; event-id
  dedupe still needs a broader replay policy, while reducer and server tests now
  cover reconnect cursor/snapshot refresh behavior.
- Coordinate/intel operation names drift between older docs and Task 001; guard
  tests must reject both short forms and `intel.coordinate_item.*` forms until
  the owner phase standardizes the contract.

## Passive Economy Event Contract

| Event | Client-safe payload | Reducer behavior | Remaining blocker |
| --- | --- | --- | --- |
| `market.listing_created` / `market.listing_updated` | listing payload | upsert listing, recalculate known counts | backend currently queues only to command session |
| `market.sale_completed` | `{ listing, quantity, server_total, server_fee }` | upsert listing from `listing` | two-client fanout/refresh policy |
| `market.listing_cancelled` | listing payload | upsert cancelled listing, recalculate known counts | two-client fanout/refresh policy |
| `auction.bid_placed` / `auction.lot_updated` | auction lot payload | upsert lot | backend currently queues only to bidder session |
| `auction.closed` | `{ lot, grant }` | upsert lot and grant | two-client fanout/refresh policy |
| `premium.entitlement_created` / `premium.entitlement_claimed` | entitlement payload | upsert entitlement | backend currently queues only to command session |
| `premium.stock_consumed` | stock payload | upsert stock row | two-client fanout/refresh policy |
| `economy.flow_updated` | observability/event summary | log only until payload contract is public | define safe player-facing payload or admin-only scope |

## Implementation Plan

1. Build a control/contract matrix.
   - List every visible control and intent source in `client/src/ui/hud.ts`,
     `client/src/app/client-app.ts`, `client/src/render/world-renderer.ts`, and
     `client/src/render/world-view.ts`.
   - Map it to operation, query, event, reducer state, server handler, and tests.
   - Mark each as `real`, `locked`, `hide`, or `phase-owned`.
   - Add an envelope/protocol parity table for every browser operation/event:
     kind, request payload schema, success payload schema, follow-up events,
     reducer target, public error codes, retryable/idempotency posture,
     rate-limit posture, server handler, and tests.
   - Verify Go/TypeScript operation registry parity or document intentional
     server-only/client-only exceptions.
   - Cover protocol version, malformed JSON, unknown fields, non-object
     payloads, max-size/read-limit behavior, and public error code mapping.

2. Fix existing drift before adding new gameplay.
   - Parse and store `planet_storage` responses.
   - Define exact merge rules for `planet.storage_summary`: standalone storage
     state or merge into the selected planet production/storage model.
   - Parse and upsert singular `{ route: ... }` from `route.snapshot`
     responses/events.
   - Add the backend bridge from domain `ship.disabled` to public
     `death.ship_disabled`, or record it as a named blocker before UI depends
     on it. The bridge is now implemented; full death E2E remains a named
     follow-up.
   - Make passive economy/auction/premium events update state, trigger explicit
     refresh, or document backend fanout/session-scope policy.
   - Fix `auction.bid` so one click sends one command.
   - Define general event reconciliation rules for duplicate/stale `seq`,
     unknown events, response-lost/event-delivered recovery, and reconnect
     snapshot refresh.

3. Strengthen operation guard tests.
   - Extend Go registry tests for unimplemented browser mutations.
   - Extend TypeScript protocol tests for unimplemented or forbidden browser
     operations.
   - Include `inventory.move`, skill unlock/respec, `intel.share`, coordinate
     item create/use, both documented coordinate op-name forms, and mail/social
     operations in guard tests or named deferrals.
   - Keep future operations rejected until implemented in their phase.

4. Remove or downgrade misleading controls.
   - Any visible enabled button must have a real handler and server contract.
   - Future actions can be locked only if the UI copy is player-facing and not
     internal/debug.

5. Audit diagnostics and public errors.
   - For every operation, document the user-facing message, retryable behavior,
     toast/log destination, hidden/not-found generic handling, and redaction
     requirements.
   - Never expose passwords, tokens, session ids, provider refs, hidden world
     truth, or internal/debug copy.
   - Admin diagnostics remain role-gated and excluded from normal player copy
     bans.

## Likely Files

```text
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/protocol/envelope.test.ts
client/src/app/client-app.ts
client/src/render/world-renderer.ts
client/src/render/world-view.ts
client/src/state/types.ts
client/src/state/reducer.ts
client/src/state/reducer.test.ts
client/src/ui/hud.ts
client/tests/browser-smoke.mjs
internal/game/realtime/envelope.go
internal/game/realtime/envelope_test.go
internal/game/server/*_handlers.go
internal/game/server/discovery_production_handlers.go
internal/game/server/economy_handlers.go
internal/game/server/combat_loot_helpers.go
internal/game/server/server_test.go
internal/game/death/service.go
docs/plans/task-001/01-gameplay-connection-audit.md
docs/plans/task-001/01-control-contract-matrix.md
```

## Acceptance Criteria

- [x] A committed matrix exists in this phase file or a linked audit doc.
- [x] An envelope/protocol parity table exists for browser operations/events.
- [x] Session/reconnect tests prove real mode blocks gameplay commands before
      `session.ready`, enables them after readiness, clears pending commands on
      logout/auth expiry, and reconciles reconnect bootstrap/cursor state.
- [x] Socket drop with in-flight `move_to`, `scan.pulse`, and economy commands
      clears or reconciles pending state without leaving controls blocked.
- [x] Stale/duplicate event `seq` payloads do not mutate state after a newer
      event has been applied.
- [ ] No visible enabled control lacks a real server-backed operation.
- [x] `planet.storage_summary` updates client state.
- [x] `route.snapshot` updates client state.
- [x] `death.ship_disabled` makes disabled/repair state visible or triggers a
      documented refresh path.
- [x] Passive market/auction/premium events reconcile, refresh, or document an
      explicit backend fanout blocker.
- [x] One auction bid click emits exactly one `auction.bid`.
- [x] Guard tests name every unimplemented browser mutation still blocked.
- [x] Reducer/event tests cover duplicate or stale `seq`, unknown events,
      response-lost/event-delivered recovery, and reconnect snapshot refresh.
- [x] Every passive event either carries enough client-safe payload to reconcile
      state or names the exact refresh query the client must issue.
- [ ] Crafting, progression unlock, intel/coordinate, mail/social, and menu
      affordances are implemented in their owner phase or explicitly deferred
      with guard tests and hidden/locked UI.

## Verification

```bash
go test ./internal/game/realtime -run 'TestOperationRegistry' -count=1
go test ./internal/game/server -run 'Test.*(Route|PlanetStorage|Auction|Repair|Death)' -count=1
go test ./internal/game/server -run 'TestShipDisabledDomainEventQueuesClientSafeRealtimeEvents' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/app/command-gate.test.ts
npm --cache /tmp/gameproject-npm-cache run test -- --run src/protocol src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```
