# Task 001 Status Report - 2026-06-20

Superseded by
[`task-001-status-report-2026-06-21.md`](./task-001-status-report-2026-06-21.md).
This file is historical and still records the state seen on 2026-06-20, but it
must not be used as the current Task 001 execution state.

## Summary

Task001 is not done.

Doc checklist progress:

```text
118 / 227 checked = 52.0%
```

Equal phase average is about 55%, but player-facing readiness is closer to
45-50% because the biggest visible systems are still open:

- Phase 05 content/catalog: 0%
- Phase 07 shop/catalog rework: 13%
- Phase 08 planets/claim/production/routes: 0%
- Phase 11 release/verification gate: 0%

## Commit And Push State

Commits do exist locally.

Current branch:

```text
master...origin/master [ahead 15]
```

Remote `origin/master` is still at:

```text
85b86d4 doc: task-001 start
```

Current local `HEAD` is:

```text
7aa8a14 game: limit stealth scan reveals to projection
```

So the real problem is not "no commits"; the problem is local commits were not
pushed to remote. Push is currently blocked by GitHub auth on this machine:

```text
HTTPS credential helper: failed to get credentials
SSH: Permission denied (publickey)
```

Until credentials are fixed, GitHub will not show these commits.

## Symphony State

The work drifted away from the intended Symphony style for part of the session.
That was a process miss.

Symphony has been restarted for the current review pass on port `4010`, and
three fresh worker reviews were spawned:

- `TASK-0213` - Phase05/Phase07 server catalog and `shop.catalog` plan.
- `TASK-0214` - Phase07 client/HUD/reducer/protocol plan.
- `TASK-0215` - Phase05/Phase07 test and smoke guard plan.

Worker findings:

- Repo currently has no `shop.catalog` realtime operation.
- Server still has the old `listing-raw-ore-1` market fixture.
- Client has no first-class `shopCatalog` reducer state yet.
- HUD shop code still centers the old `Market / Sell / Auction / Premium`
  section model instead of game equipment categories.
- Browser smoke still proves some old shop assumptions instead of failing on
  them.

## Done

Phase 01 - gameplay connection audit: 22/22.

- Server/client connection gaps were mapped and many contract guards were added.
- Client no longer relies on several fake/demo truth paths.

Phase 02 - AOI/radar/map visibility: 20/20.

- Radar projection, remembered intel, hostile markers, square radar projection,
  and long-range planet navigation got real tests/guards.

Phase 09 - quest board layout: 17/17.

- Quest board state/layout/action handling is complete for the current Task001
  scope.

Phase 10 - controls/hotkeys/input rules: 20/20.

- Hotkeys, modal input isolation, movement/modal click ownership, and smoke
  coverage are closed for this phase.

Several supporting commits also landed:

- modal input isolation
- content-sized HUD windows
- help topic replacement for old Inspect buttons
- dead target-action clutter removal
- radar/intel projection fixes
- stealth scan reveal projection limiting

## Partly Done

Phase 03 - stealth/scan/witness: 14/22.

- Hidden player projection and reveal lifetime logic progressed.
- Still needs stronger server/UI coverage for full witness behavior and edge
  cases.

Phase 04 - HUD modal/tutorial/window system: 10/13.

- Modal/input isolation and content sizing improved.
- Still needs final DarkOrbit-style polish and all panel behavior locked.

Phase 06 - inventory/cargo/loadout/hangar: 12/23.

- Some inventory/cargo/hangar surfaces exist.
- Still missing real game-grade slot/loadout equip flows, ship list/active ship
  UX depth, and stronger server-owned item metadata.

Phase 07 - shop/market/catalog: 3/23.

- `server_recalculates` player-facing copy was removed/guarded.
- Premium purchase identity became product-specific/server-owned.
- But real shop catalog contract is still missing.

## Not Done

Phase 05 - seeded content catalog: 0/19.

Missing:

- canonical `ContentRegistry`
- broad server-owned shop products
- cross-catalog validation
- original game item/ship/module/NPC/loot catalog breadth
- display metadata that prevents raw id UI fallback
- removal of raw ore as primary shop truth

Phase 08 - planets/production/routes/claim: 0/20.

Missing:

- planet modal as real catalog/detail surface
- claim flow
- production UI cleanup
- route UI cleanup
- player-facing copy instead of backend-policy copy

Phase 11 - verification/release gate: 0/28.

Missing:

- full end-to-end gate pass
- final browser smoke against real server state
- responsive screenshot verification
- full checklist audit

## Next Correct Slice

Do this next, using Symphony workers for review/parallel checks:

1. Add server-owned `ContentRegistry` and `ShopProductDefinition`.
2. Build registry from current item/module/ship catalogs.
3. Add validation: no dangling refs, no duplicate products, no raw id display.
4. Add read-only `shop.catalog` realtime op.
5. Add client protocol builder, reducer state, and unit tests.
6. Replace HUD shop primary categories with server categories:
   Ships, Weapons, Ammo, Launchers, Shield Generators, Speed Generators,
   Extras/Modules, Scanner/Radar, Stealth, Cargo/Utility, Boosters, Resources.
7. Update browser smoke so it fails on old primary `Market/Sell/Auction/Premium`
   shop truth and `raw_ore` primary shop purchase truth.
8. Only after that, implement `shop.buy_product` with wallet/inventory/hangar
   ledger-backed mutation.

## Risk

Main risk is UI work running ahead of backend truth again. Phase05 must happen
before more serious shop/inventory/hangar UI polish, otherwise the UI keeps
rendering temporary data in nicer clothes.
