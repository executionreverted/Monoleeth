# Phase 03 - Stealth, Scanner Witness, And Hidden Player Visibility

## Goal

Implement the playtest stealth rule: normal players are visible when in range,
hidden players are invisible, hiding reduces movement speed by 30 percent, and
a scanner can reveal a hidden player to the scanner owner for 15 minutes.

## Problems Covered

- Players need explicit visibility behavior.
- Hidden players need a real cost.
- Scanner needs a counter-stealth role.
- Visibility must be viewer-specific and expire.
- Expired witness state must actively remove client target/radar visibility,
  not merely stop future reveals.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/plans/task-001/02-aoi-radar-map-visibility.md
docs/todo.md
docs/plans/modules/04-module-stat-aggregation.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
internal/game/world/visibility/visibility.go
internal/game/world/worker/worker.go
internal/game/server/runtime.go
internal/game/discovery/scanner.go
client/src/protocol/commands.ts
client/src/state/reducer.ts
client/src/ui/hud.ts
```

## Design Contract

- Hidden/stealth state is server-owned.
- Hidden players are filtered from other players unless the viewer has an
  active witness.
- Witness key is viewer player id plus target player id.
- Witness expires 15 minutes after scanner success.
- Witness permits visibility only while the target is otherwise inside the
  allowed server range/window.
- Witness does not grant permanent intel, target lock after expiry, or fog/map
  ownership.
- Witness expiry, range/zone loss, or target stealth-state change must emit a
  viewer-only removal/refresh path that clears selected target and minimap
  contact.
- Hidden movement speed is multiplied by `0.70`.
- If stealth changes while moving, the server settles the current movement to
  now and recalculates movement state with the new speed.
- The hidden player always sees self, own movement, and own stealth state.
- Unauthorized public payloads never include `hidden`, witness expiry, hidden
  target metadata, scan rolls, or target-player ids that would reveal hidden
  truth.

## Implementation Plan

1. Model hidden state.
   - Add or surface server-owned player/entity hidden state.
   - Decide and document the activation path before implementation:
     `stealth.toggle`/module activation with real protocol and handlers, or a
     test-only harness that keeps player stealth gameplay blocked.
   - If real, add `internal/game/realtime/envelope.go`,
     `internal/game/server/handlers.go`, `client/src/protocol/envelope.ts`, and
     `client/src/protocol/commands.ts` coverage.
   - If not real in this phase, keep `stealth.toggle` and module activation
     browser controls hidden/guarded with a named blocker.
   - Keep command payload intent-only; client cannot declare hidden truth.
   - Add module/stat keys needed for stealth and speed penalty if stealth is
     catalog/module-driven.

2. Apply movement penalty.
   - Add stat aggregation or movement speed modifier for hidden state.
   - Settle and recalculate movement when hidden toggles.
   - Include ETA changes in server-owned movement payloads.
   - Update authoritative worker `entitySpeeds`, not only player stat display.
   - Emit movement/AOI/stat updates so client ETA and debug logs reconcile with
     the slowed route.

3. Add scanner witness.
   - Add witness storage with expiry.
   - Scanner pulse can roll or evaluate reveal chance server-side.
   - On success, store witness and emit safe event to the viewer.
   - Visibility filtering checks viewer id, target id, range/window, zone, and
     expiry before filtering hidden players.
   - Add a live-player scanner branch that does not materialize planet intel,
     grant planet scan XP, or expose scan rolls.
   - On reveal, send viewer-only AOI/minimap updates for the hidden target.
   - On witness expiry, zone/range loss, or hidden-state change, emit
     viewer-only `aoi.entity_left`/minimap refresh or an equivalent safe
     removal event.
   - Stale combat/target commands against expired witnesses must fail with a
     generic not-visible/not-found error and no hidden-truth leak.

4. Surface in UI.
   - Show hidden/scan reveal state as compact game copy, not debug text.
   - Show revealed hidden target like a normal target while witness is active,
     possibly with a subtle scan-mark visual.
   - Use a safe public marker such as `status_flags: ["scan_revealed"]`; do not
     serialize `hidden`.
   - Do not show exact expiry for enemies unless design wants it; local player
     can see own scanner intel duration.

5. Add tests.
   - Viewer-specific reveal.
   - Expiry after 15 minutes.
   - Speed penalty and movement ETA.
   - No hidden leak without witness.

## Likely Files

```text
internal/game/world/visibility/visibility.go
internal/game/world/visibility/*_test.go
internal/game/world/aoi/snapshot.go
internal/game/world/aoi/snapshot_test.go
internal/game/world/worker/worker.go
internal/game/world/worker/worker_test.go
internal/game/server/runtime.go
internal/game/server/server_test.go
internal/game/discovery/scanner.go
internal/game/discovery/scanner_test.go
internal/game/modules/catalog.go
internal/game/modules/definitions.go
internal/game/stats/model.go
internal/game/realtime/envelope.go
client/src/state/types.ts
client/src/state/reducer.ts
client/src/protocol/envelope.ts
client/src/protocol/commands.ts
client/src/ui/hud.ts
client/src/render/world-renderer.ts
client/tests/browser-smoke.mjs
docs/plans/task-001/03-stealth-scan-witness.md
```

## Acceptance Criteria

- [ ] Non-hidden players appear when server visibility rules allow.
- [ ] Hidden players do not appear without witness.
- [ ] Hidden local player still sees self, own movement, and own stealth state.
- [ ] Hidden movement speed is reduced by 30 percent server-side.
- [ ] Hidden toggle while moving settles/recalculates the route safely.
- [ ] Scanner can reveal a hidden player with server-owned chance/rules.
- [ ] Witness lasts 15 minutes and is viewer-specific.
- [ ] Witness expiry removes visibility again.
- [ ] Witness expiry/range loss/stealth change clears selected target and
      minimap contact for the viewer.
- [ ] Revealed hidden targets use viewer-only AOI/minimap updates with
      `scan_revealed` or equivalent public status only.
- [ ] Stale actions against expired witnesses fail generically without hidden
      truth leakage.
- [ ] `stealth.toggle`/module activation is implemented as a real server
      contract or remains named-blocked and absent from normal UI.
- [ ] Client payload never includes hidden truth for unauthorized viewers.
- [ ] Scanner live-player reveal does not create planet intel, scan XP, or
      serialized scan-roll leaks.
- [ ] Safe reveal UI uses public status flags only.

## Verification

```bash
go test ./internal/game/world/visibility -count=1
go test ./internal/game/server -run 'Test.*(Hidden|Witness|Scan|Movement|Visibility)' -count=1
go test ./internal/game/discovery -run 'Test.*Scan' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run smoke
```
