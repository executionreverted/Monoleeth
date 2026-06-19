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
- [ ] `Tab` cycles visible hostile targets, skipping self/friendly/loot/planet
      signals and including witnessed hidden hostiles when eligible.
- [ ] WASD is server-owned if enabled or documented as blocked.
- [ ] WASD decision documents tap/hold semantics, diagonal behavior, throttle,
      keyup behavior, and `move_to` vs `movement.set_input`.
- [ ] `stop` and any new movement input op have rate-limit posture.
- [ ] Modals/windows can open, drag, close, and click while ship is moving.
- [ ] Modal/window/HUD clicks do not send `move_to`.
- [ ] Radar contact click selects/opens detail.
- [ ] One radar/user click starts one navigation route with at most one
      immediate bounded `move_to`; later chunks wait for server reconciliation.
- [ ] Empty no-lock target UI does not show useless action buttons.
- [ ] Empty radar clicks no-op unless explicitly designed.

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
