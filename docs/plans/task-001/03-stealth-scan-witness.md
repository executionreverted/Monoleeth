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
- Hidden movement speed is multiplied by `0.70`.
- Public activation uses `stealth.toggle` with intent-only payload
  `{ "enabled": boolean }`.
- `stealth.toggle` success returns `accepted`, safe `stealth.enabled`, and the
  reconciled stats snapshot, then emits `stats.updated` plus self AOI diff.
- The local hidden player may receive self-only public
  `status_flags: ["stealthed"]`. Enemy hidden truth is never serialized; a
  revealed enemy may receive only safe public flags such as `scan_revealed`.
- If stealth changes while moving, the server settles the current movement to
  now and recalculates movement state with the new speed.
- The hidden player always sees self, own movement, and own stealth state.
- Unauthorized public payloads never include `hidden`, witness expiry, hidden
  target metadata, scan rolls, or target-player ids that would reveal hidden
  truth.
- Current playtest activation is free after normal ship-movement validation.
  Module ownership, energy cost, cooldown, and anti-spam rate enforcement remain
  explicit follow-up gates before this becomes a balanced production ability.

## Subagent Review Additions - 2026-06-20

- Add viewer/target identity before witness work. Visibility checks need the
  viewer player/entity id and the target player/entity id so self visibility and
  viewer-specific witness reveals can be represented without leaking hidden
  truth.
- Do not reuse the generic runtime `hidden` map as the final player stealth
  state without splitting reasons. That map also covers seeded hidden signals,
  removed NPCs, looted drops, or other non-player visibility suppression.
- Add a worker speed mutation command for stealth toggles. The command must
  settle current movement to server time, change authoritative `entitySpeeds`,
  and recalculate an active movement route to the same target with the new
  speed.
- Add a separate live-player scanner branch. Current scanner flow is
  planet-oriented; player reveal must not materialize planet intel, grant planet
  scan XP, or expose scan-roll internals.
- Add witness-specific leak tests before payload work. Forbidden public payload
  fields include `hidden`, `target_player_id`, `witness_expires_at`, hidden
  target metadata, detection rolls, and procedural scan internals.
- Client reveal presentation must react only to server-sent visible entities and
  safe public flags such as `status_flags: ["scan_revealed"]`. The client must
  not infer or display hidden/cloaked truth for enemies.

## Second Subagent Review Additions - 2026-06-20

- Do not treat the runtime harness as playable stealth. Add an explicit public
  activation decision: either implement a real `stealth.toggle`/module command,
  handler, reducer state, UI affordance, and tests, or keep stealth activation
  hidden with a named blocker while scanner witness work proceeds.
- Wire scanner-created witnesses. `scan.pulse` currently follows the planet
  discovery path; live-player reveal needs its own server branch that returns a
  safe `player_revealed`-style result, stores the viewer/target witness, emits
  viewer-only AOI updates, and never creates planet intel or scan XP.
- Bring leak guards to the client side as well as Go tests. Forbidden public
  payload fields include `hidden`, `target_player_id`, `witness_expires_at`,
  hidden target metadata, detection rolls, scan candidate data, and procedural
  internals.
- Unify witness-aware visibility helpers. The same viewer id, witness list, and
  observed-at time should drive render AOI and server-side interaction
  validation so a witnessed target is not visible in one path and rejected in
  another for the wrong reason.
- Add safe reveal presentation. Renderer, minimap, target panel, and combat
  affordances may use only public flags such as `scan_revealed`; they must not
  show enemy hidden status or exact witness expiry.
- Add a real two-session browser scenario: target hidden and absent, scanner
  pulses, target appears with safe flag, unrelated viewer still cannot see,
  expiry removes visibility again.

## Third Subagent Review Additions - 2026-06-20

- Public activation must not remain only a runtime test harness. It now needs a
  documented browser contract: `stealth.toggle` request `{enabled:boolean}`,
  response `{accepted, stealth.enabled, stats}`, queued `stats.updated`, and a
  self AOI update carrying only safe `stealthed`.
- The client must derive own cloak state from server-owned self AOI flags, not
  from optimistic local truth. The response can reconcile stats, but the HUD
  active state comes from self `status_flags`.
- `stealthed` is self-only. If a hostile or friendly non-self entity payload
  contains `stealthed`, the reducer must drop it; enemies use only
  `scan_revealed` after witness.
- Slot 4 quick action is no longer guarded `shield`; smoke must treat
  `stealth` as a real enabled action with `data-command-op="stealth.toggle"`.
- Base speed must be preserved when toggling. Enabling cloak records the
  current server effective speed and applies the 30 percent penalty; disabling
  restores that server effective speed instead of hard-coding starter speed.
- Remaining blockers: module/rank/energy/cooldown requirements, explicit
  toggle anti-spam enforcement, witnessed-target interaction validation, and
  a two-session browser witness smoke.

## Fourth Subagent Review Additions - 2026-06-20

- Browser coverage is still not the same as server coverage. Add a real
  multi-session smoke: target stealths, scanner viewer cannot see it,
  `scan.pulse` resolves `player_revealed`, AOI/minimap shows `scan_revealed`,
  unrelated viewer still cannot see it, and expiry removes it.
- Add UI assertions for `scan_revealed` treatment in renderer, minimap, and
  target panel. The UI may show a safe reveal marker, but must never show
  non-self `stealthed` or exact witness expiry.
- Document the current playtest scanner reveal rule honestly. The bridge is
  deterministic nearest eligible hidden player by server-owned scan
  power/radius, not final production detection scoring by scanner strength,
  stealth signature, distance, and roll.
- Keep final stealth balancing open: module/rank/energy/cooldown/anti-spam
  checks are not implemented by `stealth.toggle` yet and must remain named
  blockers before production balancing.

## Fifth Subagent Review Additions - 2026-06-20

- `scan.pulse` must return hidden-player `player_revealed` only when the target
  can also be serialized through the current server AOI projection for that
  viewer. If the hidden target is within scan radius but outside the safe AOI
  projection, the response should be a safe miss/no-signal state instead of
  claiming a reveal the UI cannot show.
- Align scanner reveal radius with projection policy. Current planning must
  explicitly handle scan radius `2000` versus live projection half-extent
  `1000` so reveal success, AOI enter, minimap contact, and target selection
  stay consistent.
- When base speed changes while stealth is active, reconcile
  `stealthBaseSpeeds`, player `Stats.Speed`, worker `entitySpeeds`, and any
  active movement ETA together. Ship activation, loadout/stat changes, repair,
  and future buffs must not leave the worker moving at stale cloaked speed.
- Client reducer and scan-mode state should treat a safe `player_revealed`
  response as a resolved scan result even if the follow-up AOI event is delayed
  or lost, while still relying on AOI/entity payloads for actual target render.
- Browser smoke fixtures for `stealth.toggle` must emit the self
  `aoi.entity_updated` safe `stealthed` flag on enable/disable; tests should
  not create local cloak truth that the real client would never receive.

## Implementation Plan

1. Model hidden state.
   - Add or surface server-owned player/entity hidden state.
   - Decide and document the activation path before implementation:
     `stealth.toggle`/module activation with real protocol and handlers, or a
     test-only harness that keeps player stealth gameplay blocked.
   - If real, add `internal/game/realtime/envelope.go`,
     `internal/game/server/handlers.go`, `client/src/protocol/envelope.ts`, and
     `client/src/protocol/commands.ts` coverage.
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

## Implementation Evidence - 2026-06-20

- Visibility inputs now carry server-owned viewer player id, target player id,
  observation time, and internal witness records. Public AOI payload fields are
  unchanged.
- Hidden self visibility is supported: the hidden local player can still receive
  their own entity and movement state.
- Runtime player stealth state is split from the generic `runtime.hidden`
  entity suppression map. `runtime.hidden` remains for non-player suppression
  such as hidden planet signals.
- Runtime witness storage is keyed by viewer player id plus target player id
  and expires by server time. Witnessed hidden players receive only the safe
  public `scan_revealed` status flag.
- Witness visibility is viewer-specific: the scanner owner can see the hidden
  player while an unrelated viewer cannot. Expiry removes visibility again.
- Worker speed mutation now has a server-owned `SetPlayerSpeedCommand` that
  settles in-flight movement to the worker clock, updates authoritative
  `entitySpeeds`, and recalculates the active route to the same target.
- Runtime player stealth applies the 30 percent speed penalty as
  current server effective speed `* 0.70`, updates the player stat snapshot and
  worker speed, avoids stacking on duplicate enable, and restores that server
  effective speed on disable.
- Public `stealth.toggle` is registered in the realtime protocol and server
  handler map. The command accepts only `{enabled:boolean}`, rejects trusted
  payload fields such as `hidden`, resolves the player from the authenticated
  session, and queues safe stat/AOI reconciliation.
- The browser client exposes slot 4 as `stealth.toggle`, builds only
  `{enabled:boolean}`, blocks duplicate pending toggles, and derives HUD cloak
  state from server-sent self `status_flags`.
- Client reducer keeps `stealthed` only when `self` is also present, while
  continuing to drop unsafe flags like `hidden` for all entities.
- Browser smoke recognizes `stealth` as an implemented enabled action, verifies
  slot 4 click/key `4` send `stealth.toggle` without movement leakage, and
  continues to fail unknown enabled actions.
- Scanner service now has an optional live-player reveal bridge that runs before
  planet candidate generation. A successful hidden-player reveal returns only
  `status: "player_revealed"`, emits safe scan events, creates no planet intel,
  materializes no planet, grants no scan XP, and exposes no target id or witness
  expiry.
- Runtime wires the scanner reveal bridge to hidden player witness storage. The
  bridge picks an eligible hidden player by server-owned world/zone/range rules,
  stores a 15-minute viewer-specific witness, and relies on AOI diff events to
  deliver a safe `scan_revealed` visible entity to that viewer only.
- Server tests now cover the real `scan.pulse` gateway path: hidden target is
  absent before scan, scan response is `player_revealed` without
  `known_planets`/`progression`, AOI enters with `scan_revealed`, unrelated
  viewers still cannot see the target, and expiry removes visibility.
- Scanner hidden-player reveal now also respects the current live projection
  window. A hidden target inside scanner radius but outside the
  `2000x2000` projection does not create a witness, does not return
  `player_revealed`, and does not enter AOI.
- Client protocol/reducer tests reject hidden-player witness leak fields and
  sanitize public `status_flags` so `scan_revealed` can enter UI state while
  unsafe values such as `hidden` are dropped.
- Server observability command-security coverage includes `stealth.toggle` with
  intent-only payload, server-resolved session ownership, safe visibility
  payload, and no item/currency ledger mutation.

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
docs/plans/task-001/03-stealth-scan-witness.md
```

## Acceptance Criteria

- [ ] Non-hidden players appear when server visibility rules allow.
- [x] Hidden players do not appear without witness.
- [x] Hidden local player still sees self, own movement, and own stealth state.
- [x] Hidden movement speed is reduced by 30 percent server-side.
- [x] Hidden toggle while moving settles/recalculates the route safely.
- [x] Scanner can reveal a hidden player through the current server-owned
      playtest rule.
- [x] Witness lasts 15 minutes and is viewer-specific.
- [x] Witness expiry removes visibility again.
- [x] Client payload never includes hidden truth for unauthorized viewers.
- [x] Scanner live-player reveal does not create planet intel, scan XP, or
      serialized scan-roll leaks.
- [x] Safe reveal UI uses public status flags only.
- [x] Public stealth activation is implemented, or stealth activation remains
      hidden with a named blocker and no misleading UI control.
- [x] Client parser/smoke leak guards include `target_player_id`,
      `witness_expires_at`, scan-roll internals, and hidden target metadata.
- [x] Scanner witness is verified through a real scan command path, not only
      runtime harness setters.
- [ ] `stealth.toggle` has final module/rank/energy/cooldown/anti-spam rules,
      or those rules remain named blockers before production balancing.
- [ ] Two-session browser smoke proves hidden target absent, scanner reveal,
      unrelated viewer absent, and expiry removal.
- [ ] Renderer, minimap, and target panel show `scan_revealed` safely without
      non-self `stealthed` or witness expiry leaks.
- [ ] Server-side interaction validation for combat/target cycle/radar actions
      uses the same witness-aware visibility rule as AOI.
- [x] Hidden-player scan success is limited to targets that can be serialized
      through the current safe AOI projection; outside-projection targets return
      safe no-signal/miss state.
- [ ] Stealth speed remains consistent when base stats change while cloaked,
      including worker speed and active movement ETA.
- [ ] `player_revealed` responses resolve scan-mode state without requiring the
      AOI event to arrive first, while render still waits for safe AOI payload.
- [ ] Browser smoke mocks self `stealthed` through AOI update events, not local
      optimistic state.

## Verification

```bash
go test ./internal/game/world/visibility -count=1
go test ./internal/game/server -run 'Test.*(Hidden|Witness|Scan|Movement|Visibility)' -count=1
go test ./internal/game/discovery -run 'Test.*Scan' -count=1
```
