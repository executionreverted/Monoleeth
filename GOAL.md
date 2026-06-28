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
- `74a3a404` game: add server combat ammo selection
- `608b6730` game: consume laser ammo during combat
- `03fb2a65` client: wire combat ammo selection
- `957a3abc` client: add quickbar ammo assignment
- `83d6f609` test: cover combat ammo security gate

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
- Server-owned `combat.select_ammo` intent.
- Runtime-owned active ammo selection per authenticated player.
- Ammo selection validation for family, item id, selectable ammo definition,
  account inventory quantity, and item catalog match.
- Safe combat state snapshots/events for selected ammo and remaining quantity.
- Server-side laser ammo resolution, selected-ammo fallback to owned `LCB-10`,
  no-ammo stop with `not_enough_ammo`, inventory/economy consumption ledger,
  and ammo multiplier damage application.
- Client inventory ammo selection intent and HUD display of server-selected ammo
  state.
- Client quickbar ammo assignment from inventory stacks, assigned ammo slot
  selection intent, and empty/selected/pending UI driven by server inventory and
  combat state.
- DB-published dense `1-1 -> 1-2 -> 1-3` proof, no static production fallback
  proof, fail-closed invalid published content proof, and browser smoke proof.

## Remaining Work

No required implementation work remains in this focused DarkOrbit-feel +
Kalaazu default seed goal.

Optional manual/human feel pass remains available but is not required for this
goal: run `DARKORBIT_FEEL_LONG_RUN_MS=600000 npm run e2e:darkorbit-feel` from
`client/` to produce a 10-minute observation artifact.

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
