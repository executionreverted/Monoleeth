# Task 001 Goal

Date: 2026-06-20

## Objective

Task 001 turns the authenticated browser client from a mostly connected HUD into
a real playtestable game surface. It must find and close the missing UI to
server gameplay links, widen the server-owned world/radar visibility experience,
remove the current debug-looking UI copy, and rebuild the main game menus around
the DarkOrbit-style system layouts referenced by the local mockups. Use caveman skill for your subagents.

This goal is complete only when:

- every visible gameplay control is backed by a real server command/query/event
  or is absent/locked with non-debug game copy
- missing UI/server integration gaps are audited and either implemented or
  tracked as explicit blockers in the phase file
- the world/radar returns a larger server-owned visible window around the player
  without leaking hidden entities or procedural truth
- all known nearby entities, known planets, non-hidden players, enemies, and
  loot are visible in the map/radar surfaces when server rules allow it
- the current fog-of-war visual overlay is removed for this playtest, while
  server-side hidden-data filtering remains intact
- stealth is a real server-owned state with a movement speed penalty and a
  scanner witness window that can reveal a hidden player to one viewer for 15
  minutes
- Shop, Inventory, Cargo, Loadout, Hangar, Planets, and Quests surfaces feel like
  game systems instead of generic data renderers
- generic `Inspect` titlebar buttons are removed and replaced by contextual `?`
  tutorial/help access where useful
- player-facing UI no longer says debug/internal phrases such as
  `server recalculates`, `server-owned`, `server policy`, or
  `No server-owned routes`
- impossible actions are hidden or quiet, not displayed as primary disabled
  clutter
- modals/windows can be opened and clicked while moving, but world movement is
  blocked while a modal/window owns the interaction
- minimap/radar contacts are clickable and can navigate or open detail using
  server-returned coordinates
- quick hotkeys `1..6`, target cycling with `Tab`, and optional WASD movement
  work only when the world has focus
- every UI slice is checked against `output/mockups/final-mockup.png`,
  `output/mockups/darkorbit-magaza-ornek-mockup.png`, and
  `output/mockups/darkorbit-envanter-ornek-layout.png`

## Required Reading Before Every Slice

Read these first in every Task 001 implementation turn:

```text
AGENTS.md
docs/plans/task-001-goal.md
docs/plans/task-001/00-index.md
the matching docs/plans/task-001 phase file
docs/todo.md
```

For visual/UI work, inspect:

```text
output/mockups/final-mockup.png
output/mockups/darkorbit-magaza-ornek-mockup.png
output/mockups/darkorbit-envanter-ornek-layout.png
output/assets/hud-svg/
client/
```

For server contracts, gameplay, economy, world, or persistence integration,
start with:

```text
docs/plans/modules/00-index.md
docs/plans/modules/02-inventory-cargo-wallet-ledger.md
docs/plans/modules/03-ship-hangar-loadout.md
docs/plans/modules/04-module-stat-aggregation.md
docs/plans/modules/05-combat-damage-targeting.md
docs/plans/modules/06-loot-drop-ownership.md
docs/plans/modules/07-death-repair-respawn.md
docs/plans/modules/08-crafting-recipes-materials.md
docs/plans/modules/09-market-auction-premium.md
docs/plans/modules/10-quest-board-generation.md
docs/plans/modules/11-planet-production-offline-settlement.md
docs/plans/modules/12-automation-routes.md
docs/plans/modules/13-intel-coordinate-trading.md
docs/plans/modules/14-world-aoi-fog-security.md
docs/plans/modules/15-api-events-errors.md
docs/plans/modules/16-testing-observability-balancing.md
docs/2026-06-17-world-system-design.md
docs/2026-06-17-progression-economy-systems-design.md
docs/2026-06-16-space-morpg-architecture-notes.md
```

## External UI References

Use DarkOrbit only as layout and interaction research. Do not copy art, names,
exact economy, item values, or unimplemented gameplay truth.

Research links used for this planning pass:

```text
https://darkorbit.fandom.com/wiki/Hangar
https://darkorbit-archive.fandom.com/wiki/Hangar
https://darkorbit.fandom.com/wiki/Generators
https://darkorbit.fandom.com/wiki/Cloak
https://darkorbitwiki.com/equipment/
https://darkorbit-archive.fandom.com/wiki/Cargo_box
https://darkorbit-archive.fandom.com/wiki/Bonus_box
```

Adapted patterns:

- Hangar is an owned ship list, active ship selection, and equipment/manage
  surface.
- Equipment layout is ship preview plus slot board plus item inventory.
- Shop layout is category rail, item grid/list, selected product detail, and a
  clear purchase panel.
- Generator/equipment categories separate speed, shield, weapons, launchers,
  ammo, extras, CPUs/modules, boosters, and ships.
- Cloak/stealth is a gameplay visibility state and must be countered by scanner
  or minimap/radar rules instead of client-side guessing.

## Working Style

- Main session acts as conductor. Read `AGENTS.md` for info.
- Use subagents/Symphony for independent research or implementation slices when
  useful.
- Worker prompts must tell workers to follow `docs/symphony-worker-rules.md`.
- Worker prompts must not ask workers to read `AGENTS.md` or manage Symphony.
- Workers must not spawn subagents, manage queues, or commit.
- Implement one vertical slice at a time.
- Review diffs before applying.
- Update phase checklists only after verification.
- Every visual slice must inspect the three mockups before and after the
  change.

## Non-Negotiable Rules

- No fake gameplay values in real mode.
- Client requests are intents, not facts.
- The client must never author player id, position, speed, damage, cooldown,
  XP, loot, wallet, cargo, inventory, loadout, quest progress, market totals,
  planet ownership, visibility, or hidden world truth.
- UI-only state may track selected tabs, selected rows, window positions,
  tutorial topics, drag/drop hover, focused panel, and pending presentation
  state.
- Server state owns movement, AOI, visibility, stealth, scan witness, combat,
  loot, inventory, wallet, active ship, loadout, crafting, quests, planets,
  production, routes, market, auction, premium, and repair.
- Disabling fog for Task 001 means removing the client visual fog overlay and
  playtest fog UX, not disabling hidden-data security filters.
- Every mutation must validate auth, ownership, range, visibility, capacity,
  rank, cooldown/rate limit, idempotency, and ledger/transaction rules where
  applicable.
- Player-facing UI must not display internal implementation copy. Examples to
  remove: `server recalculates`, `server policy`, `server-owned`,
  `No server-owned routes`, raw item ids, and debug lock reasons.

## Done Means

### Gameplay Integration

- [ ] Task 001 has a current UI/server gap audit.
- [ ] `planet.storage_summary`, `route.snapshot`, `death.ship_disabled`, and
      passive economy/auction/premium events reconcile in client state or
      trigger explicit refresh paths.
- [ ] Auction bid sends exactly one `auction.bid` command per click.
- [ ] Missing mutations are either implemented or guarded: `inventory.move`,
      `progression.unlock_skill`, `crafting.start`, `crafting.complete`,
      `crafting.cancel`, `discovery.claim_planet`, `planet.building_build`,
      `planet.building_upgrade`, route mutations, intel share, and coordinate
      item flows.

### World, Radar, Visibility

- [ ] The live/radar world window is widened by server policy beyond the current
      `420` radar-only feel, using a safe capped query or snapshot projection.
- [ ] A `2000x2000` playtest window or equivalent server-owned radius policy is
      documented and enforced consistently.
- [ ] Visible nearby planets, enemies, loot, and non-hidden players render on
      both map and radar surfaces.
- [ ] Minimap contacts and remembered known planets are clickable.
- [ ] Fog visual overlay is disabled for this playtest.
- [ ] Hidden entities, procedural seeds, future spawn candidates, scan rolls,
      and exact undiscovered data are still not serialized.
- [ ] Hidden players are invisible by default, slower while hidden, and
      revealable to one scanner viewer through a 15-minute witness.

### Game Menus

- [ ] Inventory, Cargo, Loadout, Hangar, Shop, Planets, and Quests have distinct
      game-system layouts.
- [ ] Inventory/Cargo uses tabs, and Loadout shows ship slots with drag/drop or
      explicit equip/unequip backed by real server contracts.
- [ ] Hangar shows owned ships and active ship management.
- [ ] Shop uses category rail, product/listing grid, selected detail, and buy
      panel based on server catalog/listings.
- [ ] Planets detail supports real claim flow and only shows build/route
      controls when they are meaningful.
- [ ] Quest menu is a selectable quest board, not a generic single-card panel.
- [ ] Generic `Inspect` buttons are gone; contextual `?` opens a tutorial/help
      modal when real help content exists.

### Interaction

- [ ] Movement can continue while windows/modals are opened and clicked.
- [ ] World movement clicks are blocked while a modal/window/HUD control owns
      pointer or keyboard focus.
- [ ] No impossible action is shown as primary clutter in target/action panels.
- [ ] Hotkeys `1..6` trigger quick actions only when world focus is valid.
- [ ] `Tab` cycles valid visible hostile targets.
- [ ] WASD movement is implemented as throttled server-owned movement intents or
      remains explicitly out of scope with a documented blocker.

### Visual And Verification

- [ ] HUD shell, panels, palette, and spacing are checked against
      `final-mockup.png` every phase.
- [ ] Shop is checked against `darkorbit-magaza-ornek-mockup.png`.
- [ ] Inventory/loadout is checked against `darkorbit-envanter-ornek-layout.png`.
- [ ] Windows are content-sized with max height and body scroll only when needed.
- [ ] Desktop/tablet/mobile screenshots are captured under
      `output/screenshots/task-001/`.
- [ ] No fake real-mode gameplay values appear.
- [ ] No internal/debug copy appears anywhere in normal player UI.

## Phase Plan

Start at:

```text
docs/plans/task-001/00-index.md
```

Recommended order:

1. `01-gameplay-connection-audit.md`
2. `02-aoi-radar-map-visibility.md`
3. `03-stealth-scan-witness.md`
4. `04-hud-modal-tutorial-window-system.md`
5. `05-seeded-game-content-catalog.md`
6. `06-inventory-cargo-loadout-hangar.md`
7. `07-shop-market-catalog-rework.md`
8. `08-planets-production-routes-claim.md`
9. `09-quest-board-game-layout.md`
10. `10-controls-hotkeys-input-rules.md`
11. `11-verification-release-gate.md`

## Required Final Verification

Before claiming this goal complete:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
find output/screenshots/task-001 -maxdepth 1 -type f | sort
```

Also verify in browser:

- desktop viewport near `1440x900`
- tablet viewport near `1024x768`
- mobile viewport near `390x844`
- authenticated real-server session
- moving ship while opening/clicking modals
- widened radar/map window with known planets, NPCs, loot, and players
- hidden player reveal and witness expiry path, unless Phase 03 records a named
  blocker
- shop category/detail/buy flow
- inventory/cargo/loadout tabs and equip/unequip flow
- hangar ship selection
- planet claim/detail/build/route visibility rules
- quest board selection, accept/claim/reroll availability, and reward
  reconciliation
- quick action hotkeys, target cycling, and WASD behavior if enabled
