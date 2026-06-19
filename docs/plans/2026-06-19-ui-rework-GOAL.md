# Browser UI Rework Goal

Date: 2026-06-19

## Objective

Rebuild the authenticated browser client into a real full-screen space MORPG
HUD that matches `output/mockups/final-mockup.png` as the visual target and
keeps every gameplay value server-owned.

This goal is complete only when the logged-in default client no longer feels
like a vertical debug/form page and instead plays like a cockpit-style game
surface with panels, modals, parallax world motion, entity selection, combat
feedback, loot feedback, and continuous server-authoritative movement.

## Primary Reference

Every UI implementation turn must inspect:

```text
output/mockups/final-mockup.png
```

Mockup properties recorded at goal creation:

```text
1672 x 941 PNG
```

The implementation does not need to be pixel-perfect in a single commit, but
each slice must deliberately copy the relevant mockup region it touches. Do not
invent unrelated layouts when the mockup already answers the composition.

## Required Reading Before Work

Before starting each UI rework slice, read:

```text
AGENTS.md
docs/plans/2026-06-19-ui-rework-brief.md
docs/plans/2026-06-19-ui-rework-GOAL.md
docs/plans/ui-implementation/00-index.md
docs/plans/ui-implementation/10-final-mockup-parity-hardening.md
docs/todo.md
```

For slices touching movement, world rendering, entities, combat, loot, or
server contracts, also read the relevant module/source files first.

## Working Style

Use the same manager/worker style as the todo cleanup wave:

- The main Codex session acts as project manager.
- Use Symphony/subagents for independent research or implementation slices.
- Worker prompts must tell workers to follow `docs/symphony-worker-rules.md`.
- Worker prompts must not ask workers to read `AGENTS.md` or
  `docs/symphony-operating-model.md`.
- Workers must not spawn subagents, manage Symphony, or commit.
- Review every worker diff before applying it.
- Apply, verify, and commit small vertical slices.
- Keep `docs/todo.md` truthful. Only close work that was actually implemented
  and verified.

## Done Means

The UI rework is not done until all of these are true.

### Visual Shell

- [x] Login lands in a fixed full-viewport game HUD, not a page stack.
- [x] Desktop gameplay has no default body/page scroll.
- [x] The first desktop viewport visibly resembles `final-mockup.png`.
- [x] Central world/canvas is the dominant surface.
- [x] Top status bar resembles the mockup structure:
  - sector/status
  - danger/contested indicator
  - energy
  - cargo
  - credits
  - capacitor
  - mail/social/menu affordances only when backed or locked safely
- [x] Left side resembles the mockup structure:
  - ship status card
  - icon menu stack
- [x] Right side resembles the mockup structure:
  - planets/list panel
  - selected detail panel
  - sector map/minimap panel
- [x] Bottom resembles the mockup structure:
  - log panel
  - centered action rail
- [x] Typography, borders, spacing, icon scale, panel density, and color accents
  are intentionally copied from the mockup.

### Panel And Modal System

- [ ] Existing always-visible form/section content is moved into a panel/window
  model.
- [ ] Panels open, close, focus, and preserve state predictably.
- [ ] Secondary systems do not dump into the central game surface.
- [ ] At least these panels exist as real UI concepts:
  - target
  - inventory/cargo
  - quests
  - intel/scanner
  - market/auction/premium
  - systems/loadout/crafting read models
  - admin/ops only for admin users
- [ ] A reusable modal primitive exists.
- [ ] Modal behavior supports close button, escape, and backdrop behavior.
- [ ] Modal content is compact and does not create nested-card clutter.
- [ ] Mobile uses drawer/tabs/sheets instead of a broken stacked desktop.

### World Background And Parallax

- [x] Background has layered starfield/parallax depth.
- [ ] There is visible motion/depth during player/camera movement.
- [x] The world surface includes sector grid/radar treatment similar to the
  mockup.
- [ ] Planets/signals/loot/NPCs are visually distinct and readable.
- [x] Canvas rendering remains nonblank and correctly framed on desktop and
  mobile.

### Server-Authoritative Movement

- [ ] Client clicks remain movement intents only.
- [ ] Server movement state exposes enough timing data for continuous rendering:
  origin, destination, speed, start time, and arrival/target time or equivalent.
- [ ] Server computes current position from elapsed server time, not from
  client-authored position.
- [ ] Re-clicking while in transit starts from the server-computed current
  position.
- [ ] Client interpolates movement visually from server snapshots/events.
- [ ] Movement no longer appears as instant teleporting.
- [ ] Move spam is rate-limited, coalesced, or rejected clearly without
  corrupting authoritative movement state.

### Selection And Targeting

- [ ] Clicking visible NPCs, players, loot, planets/signals, or known objects
  selects them.
- [ ] Selected object gets a visible bracket/reticle/highlight.
- [ ] Target panel updates from server-safe visible data.
- [ ] Selection does not reveal hidden/fogged data.
- [ ] Distance/status/type/name are visible when allowed by server truth.

### Combat Feedback

- [ ] Selecting an enemy shows HP/shield/status in the target panel.
- [ ] Combat action buttons show cooldown/energy availability.
- [ ] Firing produces visible feedback:
  - weapon line/projectile/flash
  - target hit/miss/damage indicator
  - log entry
- [ ] HP/shield changes are visible after server events/snapshots.
- [ ] Death/disabled/loot-spawn feedback is visible when it happens.
- [ ] No client-authored damage or hit result is trusted.

### Loot Feedback

- [ ] Loot/resource objects are visibly identifiable.
- [ ] Clicking loot either moves toward it, picks it up if in range, or explains
  why pickup cannot happen.
- [ ] Successful pickup shows what was gained.
- [ ] Cargo/inventory updates only from server snapshots/events.
- [ ] Pickup errors are compact and visible.

### Real State Rules

- [ ] No fake HP/shield/energy/cargo/wallet/quest/inventory/planet/NPC/loot
  values are shown as real state.
- [ ] Offline/unauthenticated states show login, locked, empty, loading, or
  disconnected state.
- [ ] Demo fixtures remain behind explicit dev/test mode only.
- [ ] Client never sends trusted player id, position, damage, XP, loot, wallet,
  quest progress, or hidden world truth.

### Verification Artifacts

- [ ] Each implementation slice captures or updates evidence for the specific
  mockup region it touched.
- [ ] Desktop screenshot is compared against `final-mockup.png` after each
  visible shell/panel/world slice.
- [ ] Mobile screenshot verifies no incoherent overlap or page-scroll collapse.
- [ ] Browser smoke covers real login and server-owned gameplay state.
- [ ] Any generated screenshots committed are intentional; incidental smoke
  artifacts are restored before commit.

## Required Final Verification

Before claiming this goal is complete:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

For visual slices, also verify in a browser with:

- desktop viewport near the mockup aspect
- tablet viewport
- mobile viewport
- authenticated real-server session
- screenshots saved or inspected

## Suggested Milestone Slices

1. Replace page stack with fixed full-screen HUD shell.
2. Rebuild top bar, left ship/menu, right panel column, bottom log/action rail
   from the mockup.
3. Add panel/window registry and migrate existing systems into panels.
4. Add modal primitive and first real modal flow.
5. Add starfield/nebula/grid/parallax world background.
6. Add server-time movement state and client interpolation.
7. Add entity selection and target panel.
8. Add combat visual feedback and target HP/shield.
9. Add loot approach/pickup feedback and reward summary.
10. Run final mockup parity pass across desktop/mobile.

## Non-Goals For This Goal

- Do not add fake gameplay to fill the UI.
- Do not expose loadout/crafting/planet/route mutations before real
  server-owned contracts exist.
- Do not implement durable database/outbox work unless a slice explicitly
  targets that backend blocker.
- Do not replace the active gameplay architecture with a client-authoritative
  prototype.

## Final Handoff Report Must Include

- Completed slices and commits.
- Which mockup regions were implemented.
- Remaining deviations from `final-mockup.png`, if any.
- Screenshots or exact browser verification notes.
- Tests run and outcomes.
- Any server-contract blockers left open in `docs/todo.md`.
