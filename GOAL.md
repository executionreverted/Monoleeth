Bu mesajı aktif goal olarak kullan ve tüm işler bitene kadar özerk devam et.

# Active Goal

DarkOrbit-feel vertical slice ile Kalaazu-derived DB default seed işini kalan
parçalarıyla tamamla. Biten commitleri tekrar açma; sadece aşağıdaki kalan
işleri uygula.

## Already Landed, Do Not Redo

Bu işler önceki commitlerde tamamlandı:

- `f73c8ea` docs: plan darkorbit feel and kalaazu seed work
- `e3cb7a3` game: add server combat engagement loop
- `ab54b03` client: wire combat stance feedback
- `ff6f984` content: add kalaazu seed input dumps
- `534c7b93` content: derive darkorbit ammo combat rules

Bunların kapsadığı aktif-goal dışı işler:

- Authenticated player target lock.
- Server-authoritative attack stance start/stop.
- Player hareket ederken server tick üzerinde otomatik ateş.
- NPC return fire.
- İlk combat HUD/renderer feedback.
- Compact topbar/HUD cleanup.
- Kalaazu SQL dump inputs, parser, mapper, report, snapshot builder.
- Kalaazu maps, portals, NPC templates, early-sector density, items, ships,
  modules, shop products, quests, production, combat rules.
- Runtime empty content DB first boot seed path.
- Static fallback/legacy bridge hardening.
- Ammo content catalog and LF laser damage normalization.

## Remaining Work

### 1. Server-Owned Ammo Settings

- Add `combat.select_ammo`.
- Runtime owns active ammo selection per player.
- Validate authenticated session, family, item id, selectable ammo definition,
  owned inventory quantity, and item catalog match.
- Client must never send damage, multiplier, cooldown, hit result, quantity, or
  fallback decision.
- Expose selected ammo and remaining quantity through safe combat state
  snapshot/event payloads.

### 2. Laser Ammo Consumption And Damage

- On each server laser tick, resolve selected laser ammo.
- If selected ammo is empty, fallback to `LCB-10` only when player owns it.
- If selected ammo and fallback are both unavailable, reject shot or stop active
  engagement with `not_enough_ammo`.
- Consume one laser ammo unit server-side through inventory/economy service and
  ledger.
- Apply server-side ammo multiplier to laser damage.
- Emit client-safe combat feedback with selected/fallback ammo and remaining
  quantity.
- Keep SAB/CBO/RSB special behavior conservative unless implemented with tests;
  cataloged/selectable is okay, fake effects are not.

### 3. Client Quickbar Ammo Slice

- Let inventory ammo be assignable to quick action bar slots.
- Selecting an ammo quickbar slot sends only `combat.select_ammo` intent.
- Laser attacks use server-selected ammo state from snapshots/events.
- If server says ammo unavailable, UI shows locked/empty/disabled state from
  real server response.
- No fake inventory, ammo, damage, target, cargo, wallet, NPC, map, or quest
  values.

### 4. DB-Backed Dense Early Sector Verification

- Keep early route `1-1 -> 1-2 -> 1-3` dense, risky, and rewarding through
  DB-published Kalaazu/default seed data, not static Go catalog edits.
- Verify runtime loads configured DB content as production truth.
- Invalid/missing published DB content must fail closed.
- Prove DB-published edits affect runtime and static fallback is not used.

### 5. Optional Browser Feel Gate

- If UI/browser behavior changes, run browser smoke/screenshot verification.
- Run or schedule the opt-in 10-minute observation loop:
  `DARKORBIT_FEEL_LONG_RUN_MS=600000`.
- Record human playtest notes if this loop runs.

## Required Context

Before implementation/resume:

- Read `AGENTS.md`.
- Read this `GOAL.md`.
- Read:
  - `docs/plans/2026-06-28-darkorbit-feel-design.md`
  - `docs/plans/2026-06-28-darkorbit-feel-implementation.md`
  - `docs/plans/2026-06-28-kalaazu-db-default-seed-design.md`
  - `docs/plans/2026-06-28-kalaazu-db-default-seed-implementation.md`
  - `docs/polish/00-index.md`
  - `docs/polish/10-kalaazu-reference-content-source.md`
  - `docs/polish/12-darkorbit-ammo-weapon-combat-plan.md`
- Check `git status --short`, current diff, and recent commits.

## Working Rules

- Current branch: `codex/darkorbit-feel-vertical-slice`.
- Use Caveman communication style.
- Use Context7 only for current library/framework/API/CLI/cloud docs.
- Do not revert user changes.
- Keep commits small and scoped.
- Static Go catalogs may remain only as explicit test/legacy helpers, not
  production truth.
- Unsupported Kalaazu rows must be counted in import reports, not silently
  dropped.
- Gameplay truth must come from server snapshot/event/query.
- Server validates ownership, range, visibility, cooldown, energy, inventory,
  wallet, and item mutations.

## Verification Before Handoff

Run narrow tests while developing. Before handoff:

```bash
go test ./...
cd client && npm --cache /tmp/gameproject-npm-cache run check
git diff --check
```

If UI/browser changed, also run browser smoke/screenshot verification.

Final report must briefly include:

- commits made
- tests run
- docs changed
- remaining risks
