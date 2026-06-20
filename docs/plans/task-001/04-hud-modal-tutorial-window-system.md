# Phase 04 - HUD Window, Modal, Tutorial, And Dead-Control Cleanup

## Goal

Rebuild the HUD window/modal foundation so feature surfaces look like game
interfaces, not fixed debug panels. Remove generic `Inspect`, add contextual
`?` tutorial/help access, make windows content-sized, hide impossible actions,
and retheme toward the monochrome `final-mockup.png` direction.

## Problems Covered

- Generic `Inspect` buttons do not represent game actions.
- Everything looks fixed-size, blue/cyan, and panel-heavy.
- Modals should size to content with max-height and body scroll.
- Player-facing UI includes internal/debug copy.
- Target/action areas show useless controls when no action is possible.
- HUD still does not match the final mockup closely enough.

## Required Reading

```text
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
docs/todo.md
output/mockups/final-mockup.png
output/mockups/darkorbit-magaza-ornek-mockup.png
output/mockups/darkorbit-envanter-ornek-layout.png
output/assets/hud-svg/README.md
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
```

## Visual Rules

- Keep the final mockup as the main HUD target: black/near-black, white/cool
  gray text, sparse cyan selection, amber reward/warning, red hostile/error,
  green friendly/success.
- Use DarkOrbit references only for game-window layout logic.
- No generic titlebar `Inspect`.
- `?` opens tutorial/help only when that topic exists.
- Header and footer/action rows remain fixed; only window body scrolls.
- Window height is content-driven with `max-height`.
- Do not show internal/server implementation phrases in normal play UI.
- Normal player UI copy ban applies globally, not only to surfaces changed in
  this phase. Admin-only diagnostics are exempt only when role-gated.

## Subagent Review Additions - 2026-06-20

- Replace generic `Inspect` with real help topics. Add `helpTopic` metadata to
  window definitions, a small tutorial/help topic catalog, and render `?` only
  when a topic exists.
- `modalDefinition()` must not duplicate `definition.render(state)` as a fake
  detail view. Contextual help modals render help content; feature detail
  modals render their own system-specific body.
- Make sizing changes touch the full window pipeline: `renderWindows`,
  `defaultWindowPosition`, `clampWindowPosition`, window size metadata, and
  `.hud-window` / `.hud-window__body` CSS.
- Tighten dead-control cleanup around `targetPanel` and planet detail. Empty
  target state should be quiet; disabled `Aim`, `Fire`, `Gather`, and disabled
  primary planet actions should not appear as clutter.
- Expand forbidden-copy smoke to body text, `title`, and `aria-label` for
  `server-owned`, `server policy`, `server recalculates`, `server-side`,
  `server contract`, and similar internal phrases.
- Share the world-focus/input ownership rule with Phase 10 instead of relying
  only on a short HUD suppression timer.

## Second Subagent Review Additions - 2026-06-20

- Current code evidence still shows `Inspect` in the titlebar and generic modal
  duplication through `definition.render(state)`. This phase must delete the
  generic path, not only rename the button.
- Current window sizing still uses generated `--window-height` and fixed CSS
  `height`. The sizing plan must cover size metadata, default positions,
  clamping, header/footer behavior, and body-only scroll.
- Empty target UI and planet detail still show disabled primary clutter in code
  and smoke. The cleanup gate must fail on normal-player `No lock`, disabled
  `Aim`/`Fire`/`Gather`, and disabled future planet primary buttons.
- The `?` help system needs topic metadata, a real topic catalog, and smoke that
  proves no feature body is duplicated as fake help content.
- Forbidden-copy smoke must scan normal-player visible text and attributes,
  including `title`, `aria-label`, `alt`, and `placeholder`, with admin-only
  diagnostics excluded.

## Third Subagent Review Additions - 2026-06-20

- Generic `Inspect` remains live in the window titlebar, and modal rendering can
  still duplicate `definition.render(state)`. This must be removed, not renamed:
  window help uses `?` only when a real `helpTopic` exists.
- Window geometry is still fixed-height through per-window dimensions and CSS
  height variables. Convert the shell to size classes plus content height,
  measured clamping, fixed header/footer, and body-only scroll.
- World input suppression is still timer-based. Phase 04 must coordinate with
  Phase 10 on a shared input authority and add delayed canvas-click leak checks
  after modal/window focus.
- Empty target UI still shows `No lock` plus disabled action clutter. Normal
  player UI must hide disabled `Aim`, `Fire`, and `Gather` when no valid target
  or resource exists.

## Fourth Subagent Review Additions - 2026-06-20

- Define mobile/tablet window behavior explicitly. If small viewports convert
  windows into bottom sheets, mark them as non-draggable by design and remove
  drag affordance/copy; otherwise implement touch dragging with stable bounds.
- Browser smoke must cover moving plus modal/window interaction on desktop,
  tablet, and mobile. Touch/pointer paths should prove clicks, drags, and
  bottom-sheet interactions do not leak `move_to`.
- Coordinate with Phase 10 so `Tab` inside modals/windows follows focus-trap or
  native form behavior, while world target cycling happens only when world focus
  owns the keyboard.
- Modal/window focus tests must include delayed canvas clicks after any old
  timer-based HUD suppression would have expired.

## Implementation Evidence - 2026-06-20

- Generic titlebar `Inspect` was removed from HUD windows. Window definitions
  now carry optional `helpTopic` metadata and render a compact `?` help button
  only when a real topic exists.
- Tutorial/help modals use a dedicated topic catalog for Inventory, Shop,
  Quests, Planets, Hangar, and role-gated Ops. `modalDefinition()` no longer
  clones `definition.render(state)` as a fake feature detail modal.
- Browser smoke was updated to open the Shop help topic, assert the tutorial
  modal does not contain feature window bodies, verify Escape/backdrop/body
  modal behavior on that help modal, and fail if any window titlebar still
  exposes generic `Inspect`.
- Forbidden-copy smoke now checks visible `title`, `aria-label`, `alt`, and
  `placeholder` attributes in addition to body text.
- Target panel dead-control clutter was removed. Empty target state now renders
  a quiet select-contact state, the fake disabled `Aim` control is gone, NPC
  targets only expose `Fire`, loot targets only expose `Gather`/`Approach`, and
  quick action standby copy no longer says `No lock` or `No drop`.
- Planet catalog/detail disabled future-action clutter was removed for
  `Claim`, `Build`, `Upgrade`, `Route`, and `Auto`; only the real navigate
  action remains visible until the planet gameplay contracts are implemented.
- Browser smoke now fails on visible `No lock`/`No drop`, target-panel disabled
  `Aim`/wrong-target `Fire`/`Gather` clutter, and extra planet future-action
  buttons in catalog/detail surfaces.
- HUD windows no longer emit or consume `--window-height`. Window metadata now
  carries size classes plus width only, while CSS uses `height: auto`,
  viewport-capped `max-height`, a fixed header, and `.hud-window__body` as the
  scroll owner.
- Browser smoke now asserts visible windows have no fixed height var, do not
  scroll at the window shell, scroll only through the body when capped, avoid
  large empty fixed-height slack, and keep desktop/tablet/mobile horizontal
  overflow closed. Verified screenshots:
  `output/screenshots/task-001/04/windows-content-sized-{desktop,tablet,mobile}.png`.

## Implementation Plan

1. Replace window sizing.
   - Remove rigid per-window fixed heights where possible.
   - Introduce size classes: compact, dual-pane, triple-pane, full-system.
   - Use explicit `data-size` or size-class metadata instead of per-window fixed
     pixel heights.
   - Use `height: auto` plus `max-height: calc(100dvh - safe margins)`.
   - Scroll only `.hud-window__body` or equivalent.
   - Keep headers, footers, and action rows fixed while only the body scrolls.
   - Add measurable smoke checks for sparse Shop/Hangar/Inventory windows, but
     leave their system-specific layout redesign to phases 06 and 07.

2. Remove generic `Inspect`.
   - Delete titlebar inspect button generation.
   - Add optional contextual `?` icon button based on `helpTopic` metadata.
   - Add tutorial modal/catalog shell with real topic keys and placeholder-safe
     game help copy.
   - Do not duplicate the same body inside a modal.
   - `modalDefinition()` must not use `definition.render()` as a generic
     inspect/detail body.

3. Clean dead controls.
   - No visible `No lock` as a player-facing label.
   - Empty target panel becomes quiet and compact.
   - Hide `Aim`, `Fire`, `Gather` if there is no valid target/resource.
   - Locked future actions stay visually secondary or absent.
   - Add state-specific checks for empty target, hostile target, loot target,
     out-of-range loot, disabled ship, and planet future actions.
   - Planet Claim/Build/Upgrade/Route/Auto clutter is either hidden/quiet here
     or explicitly owned by Phase 08 before release.

4. Remove internal/debug copy.
   - Ban phrases such as `server recalculates`, `server policy`,
     `server-owned`, `No server-owned routes`.
   - Replace with game copy such as `Unavailable`, `Out of range`,
     `Requires claim`, `No routes`, or hide the section entirely.
   - Add `assertNoPlayerDebugCopy()` in smoke over visible non-admin text,
     `title`, and `aria-label`.
   - Keep protocol/state fields such as `server_recalculates` only if needed
     internally; they must not appear in normal player DOM or smoke output.

5. Retheme the HUD shell.
   - Move hard-coded accents into tokens.
   - Reduce blue dominance.
   - Reuse `output/assets/hud-svg` frames/icons where it improves parity.
   - Keep buttons/icon scale stable and compact.

## Likely Files

```text
client/src/ui/hud.ts
client/src/styles.css
client/tests/browser-smoke.mjs
output/assets/hud-svg/
docs/plans/task-001/04-hud-modal-tutorial-window-system.md
```

## Acceptance Criteria

- [x] No generic `Inspect` text appears in normal HUD windows.
- [x] Contextual `?` opens a tutorial/help modal with selected topic.
- [x] Windows are content-sized and only body-scroll after max height.
- [x] Shared window shell no longer forces large empty fixed panels when sparse.
- [x] Target panel does not show `No lock` plus dead action clutter.
- [ ] Internal/debug phrases are absent from all normal player UI, with
      role-gated admin diagnostics exempted.
- [x] Smoke asserts no generic `Inspect`, no forbidden debug copy, and no dead
      target/planet primary controls.
- [x] Smoke fails on visible normal-player `Inspect`, `No lock`, disabled
      `Aim`/`Fire`/`Gather`, and disabled future planet primary buttons.
- [ ] Modal focus trap, focus return, Escape close, backdrop close, and delayed
      modal/window canvas-click leak checks are covered by browser smoke.
- [ ] Mobile/tablet window behavior is explicit: either touch-draggable with
      bounds or bottom-sheet non-draggable with no fake drag affordance.
- [ ] Moving plus modal/window click/drag/touch isolation is smoke-tested on
      desktop, tablet, and mobile.
- [ ] Palette is closer to black/white monochrome with restrained accents.
- [x] Browser smoke checks no horizontal overflow on desktop/tablet/mobile.

## Verification

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
npm --cache /tmp/gameproject-npm-cache run smoke
```

Capture screenshots under:

```text
output/screenshots/task-001/04/
```

Required screenshot/check names:

```text
tutorial-help-{desktop,tablet,mobile}.png
windows-content-sized-{desktop,tablet,mobile}.png
target-empty-desktop.png
target-npc-desktop.png
target-loot-desktop.png
```
