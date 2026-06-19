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

- [ ] No generic `Inspect` text appears in normal HUD windows.
- [ ] Contextual `?` opens a tutorial/help modal with selected topic.
- [ ] Windows are content-sized and only body-scroll after max height.
- [ ] Shared window shell no longer forces large empty fixed panels when sparse.
- [ ] Target panel does not show `No lock` plus dead action clutter.
- [ ] Internal/debug phrases are absent from all normal player UI, with
      role-gated admin diagnostics exempted.
- [ ] Smoke asserts no generic `Inspect`, no forbidden debug copy, and no dead
      target/planet primary controls.
- [ ] Palette is closer to black/white monochrome with restrained accents.
- [ ] Browser smoke checks no horizontal overflow on desktop/tablet/mobile.

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
