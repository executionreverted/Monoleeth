# Task 001 Plan Index

Date: 2026-06-20

## Purpose

Task 001 is a hardening and game-system pass for the authenticated browser game.
The current client can authenticate, move, scan, render the world, open windows,
and use some real server contracts, but the next playtest problems show three
large gaps:

- many UI controls are not fully connected to server gameplay contracts
- radar/map/visibility is too narrow or visually misleading for playtesting
- the main menu surfaces still feel like generic panels instead of game systems

This plan converts those complaints into implementation phases with explicit
server-ownership, UI layout, and verification gates.

## Primary References

```text
docs/plans/task-001-goal.md
output/mockups/final-mockup.png
output/mockups/darkorbit-magaza-ornek-mockup.png
output/mockups/darkorbit-envanter-ornek-layout.png
output/assets/hud-svg/
docs/todo.md
docs/plans/ui-implementation/00-index.md
docs/plans/ui-implementation/04-live-world-aoi-movement.md
docs/plans/ui-implementation/06-progression-inventory-loadout-crafting.md
docs/plans/ui-implementation/07-discovery-planets-production-routes.md
docs/plans/ui-implementation/08-market-auction-premium-economy.md
docs/plans/ui-implementation/10-final-mockup-parity-hardening.md
docs/plans/modules/00-index.md
docs/2026-06-17-world-system-design.md
docs/2026-06-17-progression-economy-systems-design.md
```

## DarkOrbit Reference Notes

Use these only for interaction and layout references:

```text
https://darkorbit.fandom.com/wiki/Hangar
https://darkorbit-archive.fandom.com/wiki/Hangar
https://darkorbit.fandom.com/wiki/Generators
https://darkorbit.fandom.com/wiki/Cloak
https://darkorbitwiki.com/equipment/
https://darkorbit-archive.fandom.com/wiki/Cargo_box
https://darkorbit-archive.fandom.com/wiki/Bonus_box
```

Key patterns to adapt:

- Hangar is owned ships, active ship, and manage/equipment layout.
- Equipment has ship preview, slot groups, filters, and available inventory.
- Shop has category rail, product grid/list, selected detail, and buy panel.
- Equipment categories should map to this game as ships, weapons/lasers,
  launchers, ammo, shield generators, speed generators, scanners, stealth,
  cargo, repair/support, boosters, and modules.
- Cloak/stealth must be server-owned and countered through scanner/radar rules.

## Current Audit Snapshot

Server/UI connection gaps found during the planning audit:

- `planet.storage_summary` is requested and handled, but reducer handling is
  weak or missing for `planet_storage`.
- `route.snapshot` exists server-side, but UI entry and singular route parsing
  are weak.
- `death.ship_disabled` logs but does not reliably surface a repair/disabled
  ship UI unless a separate ship snapshot arrives.
- Market, auction, and premium events are mostly log-only for passive clients.
- `auction.bid` can be double-sent from one click path and needs a smoke guard.
- `inventory.move`, `progression.unlock_skill`, crafting mutations, planet
  claim/build/upgrade, route mutations, intel share, and coordinate item flows
  are not exposed as browser gameplay loops.

World/radar findings:

- Current live AOI feel is based on `defaultRadarRange = 420`.
- Movement max distance and client navigation chunking are separate concerns.
- Scanner discovery radius is already `2000`, but it is not the live AOI/radar
  window.
- Minimap is built from live AOI plus remembered planet intel, but remembered
  entries need stable ids and click behavior.
- Client fog overlay is visual only and can be removed without exposing hidden
  server data.

UI findings:

- Windows use fixed dimensions that create empty space or cramped scroll.
- Generic `Inspect` duplicates window bodies and feels like a debug button.
- Target panels show `No lock` plus disabled actions as clutter.
- Shop says internal implementation copy such as `Server: recalculates`.
- Inventory/Cargo/Loadout are stacked in one surface instead of distinct game
  layouts.
- Planet detail displays internal lock copy and disabled controls regardless of
  whether the player can use them.
- The minimap/radar is visually weak, inert, and does not feel like a circular
  sector map.

## User Issue Coverage

| User issue | Covered by |
| --- | --- |
| Detect all missing UI/server gameplay links | Phase 01 |
| Pull wider nearby objects, around `2000x2000` | Phase 02 |
| Revise menus from DarkOrbit examples and content-sized windows | Phase 04, 06, 07 |
| Replace `Inspect` with `?` tutorial modal | Phase 04 |
| Market catalog should be real game catalog, not raw ore | Phase 05, 07 |
| Map/radar should show known nearby objects, players, enemies | Phase 02 |
| Hidden player, speed penalty, scan witness for 15 minutes | Phase 03 |
| Seed DarkOrbit-style dummy content with renamed data | Phase 05 |
| Use shop/inventory local mockups | Phase 06, 07 |
| Remove internal planet copy, wire claim | Phase 08 |
| Inventory/Cargo tabs and final mockup monochrome style | Phase 04, 06 |
| Quest board must be a real game surface | Phase 09 |
| Moving while modal open, modal blocks movement clicks | Phase 10 |
| Hide impossible target actions | Phase 04, 10 |
| Sector map/radar shape, contents, click navigation | Phase 02, 10 |
| Remove fog of war for now | Phase 02 |
| Hotkeys `1..6`, `Tab`, WASD | Phase 10 |

## Phase List

1. [Gameplay Connection Audit And Contract Parity](./01-gameplay-connection-audit.md)
2. [AOI, Radar, Map Visibility, And Fog Removal](./02-aoi-radar-map-visibility.md)
3. [Stealth, Scanner Witness, And Hidden Player Visibility](./03-stealth-scan-witness.md)
4. [HUD Window, Modal, Tutorial, And Dead-Control Cleanup](./04-hud-modal-tutorial-window-system.md)
5. [Seeded Game Content Catalog](./05-seeded-game-content-catalog.md)
6. [Inventory, Cargo, Loadout, And Hangar Game Layouts](./06-inventory-cargo-loadout-hangar.md)
7. [Shop, Market, Auction, And Catalog Rework](./07-shop-market-catalog-rework.md)
8. [Planets, Claim, Production, And Routes](./08-planets-production-routes-claim.md)
9. [Quest Board Game Layout](./09-quest-board-game-layout.md)
10. [Controls, Hotkeys, Radar Clicks, And Input Rules](./10-controls-hotkeys-input-rules.md)
11. [Verification And Release Gate](./11-verification-release-gate.md)

## Suggested Subagent Split

- Contract agent: phase 01 protocol/reducer/server operation audit.
- World agent: phases 02 and 03 AOI/radar/fog/stealth/witness.
- UX shell agent: phase 04 modal/window/tutorial/dead-control cleanup.
- Content/catalog agent: phase 05 canonical content registry and seed data.
- Systems UI agent: phase 06 inventory/cargo/loadout/hangar.
- Economy agent: phase 07 shop/catalog/market/auction/premium.
- Planet agent: phase 08 claim/production/routes.
- Quest agent: phase 09 quest board/progression reward surface.
- QA agent: phases 10 and 11 smoke, screenshots, interaction gates.

Workers must follow `docs/symphony-worker-rules.md`, must not spawn subagents,
must not manage Symphony, and must not commit.

## Global Implementation Rules

- No fake real-mode gameplay values.
- No hidden world or procedural truth leaks.
- Client requests are intents only.
- Unsupported actions are absent, quiet, or locked with player-facing game copy.
- Do not show internal implementation copy to players.
- Newly enabled mutations must include server validation, protocol command,
  handler, reducer reconciliation, tests, and browser smoke in the same slice.
- Every phase inherits the goal requirement to read `docs/todo.md` before
  implementation, even when its local reading list is shorter.
- UI state may hold presentation state only: tabs, selection, windows, drag
  hover, local animation, tutorial topic.
- Server state owns all gameplay truth.

## Verification Commands

Use narrow commands while implementing. Before final handoff:

```bash
GOCACHE=/tmp/gameproject-go-cache go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
find output/screenshots/task-001 -maxdepth 1 -type f | sort
```

UI screenshots for this task should live under:

```text
output/screenshots/task-001/
```

## Done Criteria

- Every phase file checklist is updated only after verified work.
- Every user-reported problem is fixed or tracked as a named blocker.
- Full verification passes.
- Desktop/tablet/mobile screenshots show visible progress toward the mockups.
- No client fake data or debug/internal player copy remains in normal player UI.
