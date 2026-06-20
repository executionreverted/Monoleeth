# Phase 10 - Controls, Hotkeys, Radar Clicks, And Input Rules

## Goal

Make controls feel intentional. Quick hotkeys should work, target cycling should
select useful targets, WASD movement should be server-authoritative if enabled,
radar clicks should navigate/select correctly, and modal/window interaction must
never leak into world movement.

## Problems Covered

- Hotkeys beyond quick actions are missing.
- Movement/modal interaction rules are inconsistent.
- User should be able to open/click modals while moving.
- User should not be able to start new world movement while a modal/window owns
  focus.
- Sector map/radar clicks should auto-navigate or open detail.
- No-lock target controls create useless clutter.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/plans/task-001/02-aoi-radar-map-visibility.md
docs/plans/task-001/03-stealth-scan-witness.md
docs/todo.md
docs/plans/ui-implementation/04-live-world-aoi-movement.md
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/15-api-events-errors.md
client/src/app/client-app.ts
client/src/render/world-renderer.ts
client/src/state/movement.ts
client/src/ui/hud.ts
client/tests/browser-smoke.mjs
internal/game/server/handlers.go
```

## Control Contract

- `1..6` activate quick actions only when world focus is valid.
- `Tab` cycles visible hostile targets from server-owned AOI/radar state,
  including scan-witness revealed hidden targets from Phase 03.
- WASD is optional. If enabled, it must send throttled bounded movement intents
  or a new server-owned input command. The client never sends authoritative
  position or speed.
- Modal/window/HUD focus blocks world movement clicks and hotkeys.
- If a HUD window/modal is open or focused, a canvas click should not move the
  ship; it may only refocus world if explicitly designed and tested.
- Moving does not block opening, dragging, closing, or clicking windows.
- Radar click behavior:
  - live hostile/player target: select/lock if allowed
  - known planet/contact: open detail first
  - navigable coordinate: one user click starts one navigation route with at
    most one immediate bounded `move_to`; follow-up bounded steps happen only
    after server reconciliation
  - empty radar space: no accidental movement unless explicitly supported

## Subagent Review Additions - 2026-06-20

- Define one shared world-focus authority used by HUD shortcuts, renderer click
  filtering, minimap/radar clicks, and modal/window drag interactions. A timer
  suppression heuristic is not enough as the long-term rule.
- Add `Tab` target cycling through visible hostile AOI entities. It skips self,
  friendly players, loot, planet signals, unavailable/out-of-range targets, and
  includes Phase 03 `scan_revealed` hostile players when server-sent.
- Add radar/minimap click contracts to the DOM and app layer: live contacts need
  stable `data-entity-id`/`data-entity-type`; remembered planets need
  `planet_id` or `detail_id`; empty radar clicks no-op unless explicitly
  designed.
- Decide WASD in this phase, not later by accident. Either document it as
  blocked, or implement throttled bounded `move_to`/`movement.set_input` with
  diagonal normalization, keyup semantics, and rate-limit tests.
- Browser smoke must prove valid `1` and `6` hotkeys emit exactly one command,
  locked/focused hotkeys emit none, `Tab` cycles targets, radar click selects or
  opens detail, and modal/window interaction never leaks `move_to`.

## Second Subagent Review Additions - 2026-06-20

- Replace timer-only HUD suppression with a single world-focus/input authority.
  It must be shared by HUD shortcuts, renderer click filtering, minimap/radar
  clicks, modal/window focus, drag interactions, and focused form elements.
- Current shortcut handling covers `1..6` only. Add `Tab` target cycling tests
  and implementation, including `scan_revealed` hostile players from Phase 03.
- Decide WASD in this phase. If implemented, add movement semantics,
  diagonal normalization, keyup/stop behavior, command cadence, and rate-limit
  tests. If deferred, the blocker must keep WASD UI/copy absent.
- Radar live contacts need type-specific behavior. Loot should not accidentally
  start navigation from a target-select click; planet/signal contacts need a
  server-owned detail id path or must no-op with player-safe copy.
- Smoke must test delayed clicks after modal/window focus, not only immediate
  suppression inside the old timer window.

## Third Subagent Review Additions - 2026-06-20

- Current shortcut handling still covers only `1..6`; `Tab` target cycling is
  missing from the input path and smoke.
- WASD remains undecided. This phase must either implement server-owned,
  throttled movement semantics with keyup/diagonal/rate tests, or keep WASD
  absent and document the blocker.
- Minimap click behavior is too generic. Split live contact actions by entity
  type: hostile/player select, loot select without accidental navigation,
  remembered planet detail, and empty radar no-op unless explicit navigation
  mode is implemented.
- Timer-only suppression must not be the authority. Smoke needs delayed clicks
  after modal/window focus, after any old suppression timeout would expire, and
  must prove no `move_to` leaks.

## Fourth Subagent Review Additions - 2026-06-20

- `Tab` target cycling happens only when world focus owns the keyboard and must
  call `preventDefault` in that mode. Inside modals, windows, forms, and focus
  traps, `Tab` keeps native/focus-trap behavior and must not change targets.
- Ignore `KeyboardEvent.repeat` for quick actions. Holding `1`, `3`, `4`, or
  `6` must emit at most one intent until keyup or server/pending state makes a
  new action meaningful.
- Add real limiter or named blocker coverage for hotkey-emitted operations:
  `combat.use_skill`, `loot.pickup`, `scan.pulse`, `stealth.toggle`, and
  `stop`, not only `move_to`.
- Define mobile/tablet behavior with Phase 04: draggable windows on touch, or
  explicit bottom-sheet non-draggable exception with matching smoke.
- Browser smoke must simulate repeated/held quick-action keys and moving plus
  modal/window click/drag/touch isolation across desktop, tablet, and mobile.

## Fifth Subagent Review Additions - 2026-06-20

- Minimap live contacts must expose type-specific UI intent, not only a generic
  `target-select`. Loot contact clicks are radar selection only and must never
  auto-approach or auto-pickup; world/canvas loot clicks and explicit gather
  remain the active pickup paths.
- Minimap contact stacking is part of the interaction contract. Loot, hostile,
  friendly/player, and remembered planet points can overlap; the z-order must
  keep important contacts clickable without hiding remembered planet details.
- Stale live contacts after server responses that replace `entities` without a
  `minimap` payload remain a backend/UI integration risk. Either include
  `minimap` in those responses or rebuild `minimap.live_contacts` from
  replacement entities while preserving remembered contacts.
- Hostile player contact selection needs a PvP contract decision. Until player
  combat is implemented server-side, player contacts should not imply that fire
  is valid just because selection is possible.
- Empty radar background clicks are expected to no-op unless an explicit
  navigation mode is implemented, and this still needs direct smoke coverage.

## Implementation Plan

1. Harden focus and click ownership.
   - Ensure HUD/window/modal controls mark events as HUD input.
   - Ensure renderer ignores world clicks that start inside HUD/modal/window.
   - Ensure movement state does not disable modal controls.
   - Define world focus precisely: input/HUD/modal/window focus blocks world
     hotkeys and movement clicks; empty canvas only moves when world focus is
     valid.

2. Quick hotkeys.
   - Keep `1..6` mapped to quick actions.
   - Ignore hotkeys inside inputs, textareas, selects, modals, and windows unless
     the window explicitly handles that shortcut.
   - Locked actions do not emit commands.

3. Target cycling.
   - Build ordered list of visible hostile NPC/player targets from current AOI.
   - `Tab` selects next valid target.
   - Cycling ignores hidden/unavailable/out-of-range targets.
   - Selection is client-local unless this phase adds a real `target.set`
     command. Combat/gather still re-validates target server-side at use time.

4. WASD movement decision.
   - Choose tap-to-step or hold-to-thrust before implementation.
   - Define diagonal normalization, key repeat throttle, keyup behavior, and
     command cadence.
   - Preferred MVP: throttled `move_to` intents to a short bounded destination
     in the key direction, using server speed/position reconciliation.
   - Alternative: introduce `movement.set_input` as a real server contract with
     rate-limit tests.
   - If not implemented, document blocker in this phase and do not fake it.
   - Define rate limits for `stop` and any new movement input op so key
     down/up cannot flood the server.

5. Radar clicks.
   - Add click handlers to minimap contacts.
   - Use stable ids from Phase 02: live `entity_id/type` and remembered
     `planet_id/detail_id`.
   - Open detail/select first; navigate only when explicit and server-known.
   - Convert passive minimap spans into pointer-enabled buttons/elements with
     data-action handlers.

## Implementation Evidence - 2026-06-20

- Server `move_to` retarget now settles the active route to the worker's
  authoritative timed in-flight position before creating the next movement
  state. Mid-route reclicks no longer restart from a stale tick position.
- Worker unit coverage locks the server-time retarget case with a fake clock.
- Browser smoke now checks reclick origin against the previous route at the
  next route's `started_at_ms`, instead of comparing against a stale snapshot
  position.
- `Tab` now cycles a client-local target selection over server-visible hostile
  NPC/player entities only. The helper skips self, friendly contacts, loot,
  planet signals, destroyed targets, hidden non-witnessed targets, and
  out-of-weapon-range targets; combat still revalidates server-side.
- Browser smoke verifies world-focus `Tab` selects the live training NPC without
  sending a server command, repeated quick-action keydown is ignored, and
  focused HUD windows/modals do not hijack native `Tab`.
- HUD keyboard gating and renderer canvas-click gating now share
  `world-input-authority`. Focused windows, open modals, auth/forms, and
  drag/HUD interaction state block world input after the old short suppression
  timeout has expired; the timer remains only as immediate same-event debounce.
- Browser smoke waits beyond the HUD suppression timeout and proves focused
  windows/modals do not emit `move_to` or new move-debug logs from canvas
  clicks while movement continues and UI stays usable.
- Minimap live loot contacts now use an explicit `loot-select` action and
  source-aware target selection. Radar loot selection only selects the drop; it
  does not call the world/canvas loot approach or pickup path.
- Browser smoke covers a server-spawned loot contact after combat: minimap
  click selects the drop while emitting zero `move_to` and zero `loot.pickup`,
  then a normal world/canvas loot click still gathers the same drop.
- Focused standalone HUD controls, such as nav/quick/topbar buttons, now share
  the same world-input authority as windows and modals after the short
  suppression timer expires. The renderer swallows the first canvas click,
  releases transient HUD control focus, and requires a later explicit world
  click before movement can resume.
- Browser smoke now runs moving HUD window/modal isolation on desktop, tablet,
  and mobile. Tablet uses touch-pointer drag under the desktop window policy;
  mobile asserts bottom-sheet/non-draggable header policy. Delayed canvas clicks
  after window/modal focus, drag/touch, and standalone HUD focus must emit zero
  `move_to` and zero new movement debug logs.

## Likely Files

```text
client/src/app/client-app.ts
client/src/render/world-renderer.ts
client/src/state/movement.ts
client/src/state/types.ts
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
internal/game/server/handlers.go
internal/game/server/server_test.go
internal/game/realtime/envelope.go
docs/plans/task-001/10-controls-hotkeys-input-rules.md
```

## Acceptance Criteria

- [ ] `1..6` quick action hotkeys work only with valid world focus.
- [ ] Locked quick actions emit no commands.
- [x] Held/repeated quick-action keys emit no duplicate command flood.
- [ ] Hotkey-emitted combat, loot, scan, stealth, and stop operations have
      limiter tests or named blockers.
- [x] World focus/input authority is shared by HUD, renderer, minimap/radar,
      modal/window drag, and form focus paths; timer-only suppression is gone.
- [x] `Tab` cycles visible hostile targets, skipping self/friendly/loot/planet
      signals and including witnessed hidden hostiles when eligible.
- [ ] `Tab` preserves native/focus-trap behavior in modals, windows, and form
      fields, and only cycles targets when world focus owns keyboard input.
- [ ] WASD is server-owned if enabled or documented as blocked.
- [ ] WASD decision documents tap/hold semantics, diagonal behavior, throttle,
      keyup behavior, and `move_to` vs `movement.set_input`.
- [ ] `stop` and any new movement input op have rate-limit posture.
- [x] Modals/windows can open, drag, close, and click while ship is moving.
- [x] Modal/window/HUD clicks do not send `move_to`.
- [x] World reclick movement starts from the server-timed in-flight position
      instead of a stale snapshot/tick origin.
- [x] Radar contact click selects/opens detail for hostile NPC contacts,
      remembered planet contacts, and server-spawned loot contacts without
      leaking movement or pickup commands.
- [ ] One radar/user click starts one navigation route with at most one
      immediate bounded `move_to`; later chunks wait for server reconciliation.
- [ ] Empty no-lock target UI does not show useless action buttons.
- [ ] Empty radar clicks no-op unless explicitly designed.
- [x] Smoke proves modal/window focused canvas clicks still emit no `move_to`
      after any short HUD suppression timeout would have expired.
- [ ] Browser smoke covers minimap contact behavior by type: hostile/player,
      loot, remembered planet, and empty radar. Current coverage includes
      hostile NPC, remembered planet, and server-spawned loot; hostile player
      and empty radar no-op remain open.
- [x] Browser smoke covers moving plus modal/window click/drag/touch isolation
      on desktop, tablet, and mobile.

## Verification

```bash
go test ./internal/game/server -run 'Test.*Movement' -count=1
cd client
npm --cache /tmp/gameproject-npm-cache run test -- --run src/state
npm --cache /tmp/gameproject-npm-cache run smoke
```

Browser smoke must include:

- moving ship plus modal open/click/drag
- modal click emits no movement
- quick action hotkeys
- `1` and `6` hotkeys emit exactly one expected command when world focus is valid
- `Tab` target cycling
- radar contact click
- radar navigate
- WASD smoke, if enabled, proves no input/window/modal command leakage and no
  rate-limit flood
