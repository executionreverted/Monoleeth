# UI Patch 2 Goal

Date: 2026-06-19

## Objective

Patch the authenticated browser HUD so the game surface feels stable,
interactive, and much closer to `output/mockups/final-mockup.png` while keeping
all gameplay truth server-owned.

This goal is complete only when:
- discovered planets and planet memory markers stay anchored to server
  coordinates, can be selected, and never masquerade as live AOI truth
- menu-driven feature surfaces open as centered, draggable HUD windows/modals
  instead of stacked panels
- HUD, modal, and form interactions never leak clicks into the world canvas
- implemented quick actions are wired to real commands or explicit client-local
  modes, while unimplemented actions stay locked
- scan mode can be toggled on, automatically sends real `scan.pulse` intents
  as server timing permits, and renders a clear scanning indicator
- movement logs show from/to coordinates, and an ETA pill displays remaining
  server-time travel
- combat fire shows a visible projectile traveling from the player to the
  target before disappearing
- `output/assets/mockup-hud/background/starfield_2048x1152.png` is used as the
  world background with endless mirrored parallax
- desktop, tablet, and mobile screenshots visibly move toward the mockup
  composition without fake gameplay data

## Required Reading Before Every Slice

Read these before starting any UI Patch 2 implementation turn:

```text
AGENTS.md
docs/plans/ui-patch-2-goal.md
docs/plans/ui-patch-2/00-index.md
the matching docs/plans/ui-patch-2 phase file
docs/plans/2026-06-19-ui-rework-GOAL.md
docs/plans/ui-implementation/10-final-mockup-parity-hardening.md
docs/todo.md
```

For visual work, inspect:

```text
output/mockups/final-mockup.png
output/assets/mockup-hud/background/starfield_2048x1152.png
output/assets/mockup-hud/background/grid_overlay_2048x1152.png
output/assets/hud-svg/
client/
```

For world, scan, discovery, planet, movement, or combat work, also read:

```text
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/2026-06-17-world-system-design.md
```

## Working Style

- Main session is the project manager.
- Use Symphony/subagents for independent slices when useful.
- Worker prompts must tell workers to follow `docs/symphony-worker-rules.md`.
- Worker prompts must not ask workers to read `AGENTS.md` or
  `docs/symphony-operating-model.md`.
- Workers must not spawn subagents, manage Symphony, or commit.
- Review every diff before applying.
- Apply, verify, and commit one vertical slice at a time.
- Every visual slice must inspect `output/mockups/final-mockup.png` before and
  after the change.

## Non-Negotiable Rules

- No fake gameplay values.
- No client-authored position, damage, loot, wallet, XP, quest progress,
  cooldown, scanner result, or hidden world truth.
- UI-only state may track window positions, focused panels, scan mode enabled,
  local animations, and pending presentation state.
- Server state owns world positions, visible entities, combat results, scan
  results, loot rewards, cargo, wallet, planets, quests, and market state.
- Do not enable rocket, shield, warp, build, upgrade, route, equip, craft, or
  claim mutations unless real server contracts are implemented in the same
  slice.

## Done Means

### Planet And Map Stability

- [x] Discovered planet list entries can request/open safe planet detail.
- [x] Planet detail coordinates render as map memory markers when available.
- [x] Memory planet markers remain at fixed world coordinates while the player
      moves.
- [x] Clicking a planet marker selects/opens the planet panel instead of
      sending a move intent.
- [x] Clicking empty world space still sends only a `move_to` intent.
- [x] Known planet data does not reveal hidden planets or future spawn data.

### Windows, Modals, And Input Isolation

- [x] Inventory/cargo, hangar/systems, quests, intel/scanner, economy, and ops
      open centered by default.
- [x] HUD windows are draggable by their headers and clamped to the viewport.
- [x] Window focus/z-order is predictable.
- [x] Escape, close buttons, and backdrop/modal behavior remain predictable.
- [x] Main feature screens are not dumped into the central game surface.
- [x] Clicking or focusing HUD, modal, button, input, or form elements cannot
      trigger world movement or world selection.
- [x] Mobile remains usable as sheets/drawers with no horizontal body scroll.

### Quick Actions And Scan Mode

- [x] Laser, scan, and gather quick actions call real command paths or explicit
      client-local modes.
- [x] Locked action slots remain visibly locked and disabled.
- [x] Keyboard shortcuts, if added, are ignored while typing in inputs or while
      a modal owns focus.
- [ ] Scan button toggles scan mode instead of firing a one-off pulse only.
- [ ] Scan mode automatically sends `scan.pulse` when server timing allows.
- [ ] Scan mode shows a ship-centered scanning wave and an animated action
      button state.
- [ ] Server cooldown/rate-limit/energy/movement rejection stops or backs off
      the client loop without mutating gameplay truth.

### Movement Debug UX

- [ ] Move logs show from coordinate, to coordinate, distance, and estimated
      travel time.
- [ ] Move rejection logs remain compact and visible.
- [ ] A top-center ETA pill shows destination and remaining time while the
      server-owned route is active.
- [ ] ETA uses the same server-time interpolation model as the renderer.
- [ ] Arrival pill disappears cleanly when movement stops or reconnect
      reconciles state.

### Combat Projectile Feedback

- [ ] Firing at a visible hostile creates a projectile from self to target.
- [ ] Projectile timing is presentation-only and derived from server events.
- [ ] Hit/miss/damage/death feedback remains server-owned.
- [ ] Projectile, target flash, and damage text are visible in screenshots and
      smoke checks.

### Starfield And Mockup Parity

- [ ] The world uses `starfield_2048x1152.png` as the primary background.
- [ ] Background is endless through mirrored tiling.
- [ ] Background parallax has at least far/mid/near motion depth.
- [ ] Grid/radar overlay remains readable over the starfield.
- [ ] HUD borders, spacing, icon scale, action slots, topbar density, and panel
      placement are rechecked against `final-mockup.png`.
- [ ] Desktop/tablet/mobile screenshots are captured under
      `output/screenshots/ui-patch-2/`.

## Required Final Verification

Before claiming this goal complete:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

Also verify in browser:

- desktop viewport near `1440x900`
- tablet viewport near `1024x768`
- mobile viewport near `390x844`
- authenticated real-server session
- selected discovered planet
- draggable modal/window
- scan mode active
- movement ETA pill during movement
- combat projectile during fire
- starfield parallax while moving

## Phase Plan

Start at:

```text
docs/plans/ui-patch-2/00-index.md
```

Recommended order:

1. `01-planet-map-selection.md`
2. `02-window-modal-input-isolation.md`
3. `03-quick-actions-input-contracts.md`
4. `04-scan-mode-automation.md`
5. `05-movement-debug-eta.md`
6. `06-projectile-combat-feedback.md`
7. `07-starfield-parallax-background.md`
8. `08-mockup-parity-verification.md`
