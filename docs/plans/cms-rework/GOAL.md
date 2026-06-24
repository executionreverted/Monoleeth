# CMS Rework Goal

Bu mesajı aktif goal olarak set edebilirsin.

Objective:
Implement a DB-backed, admin-editable game content CMS for the server-authoritative browser-first space MORPG.

Current baseline:
- Recent content-foundation commits already created `internal/game/content.GameplayContent`.
- Runtime already loads validated gameplay content through `content.Repository`.
- Current fallback implementation is `content.StaticRepository`, which returns the static bundle from `DefaultGameplayContent`.
- Shop, scanner, starter/playtest constants, route policy, production rules, combat rules, and complete enemy-map validation now live under the content bundle.
- Browser demo runtime mode was removed; CMS UI/smoke must use real authenticated server/admin flows, not fake client state.

Goal:
- Add Dockerized Postgres and `contentdb` migration/store boundary.
- Store admin-editable content rows and immutable published snapshots in Postgres.
- Seed default MVP content only when content tables are empty, using the current validated `GameplayContent` bundle as seed source.
- Implement a DB-backed `content.Repository` that loads the current published snapshot, maps it into `GameplayContent`, validates it, then lets runtime install the same server-owned catalogs/services it uses today.
- Keep stable content IDs for items, modules, ships, NPCs, loot tables, recipes, shop products, and production/building definitions.
- Move dynamic definitions to DB: items, equipment/module stats, ships, shop products, NPC templates, enemy pools, loot tables, craft recipes, production/building definitions, and later quest reward content.
- Add draft edit, validation, publish, rollback, diff, audit log, and balance notes/tags.
- Add safe player projections as allowlist DTOs; never leak hidden loot rows, spawn internals, procedural seeds, admin notes, or server-only map fields to normal players.
- Add admin UI slices for content versions, modules/LC1-style stat edits, items, ships, shop products, NPCs, pools, loot, craft recipes, and production buildings.
- Keep gameplay server-authoritative. Client sends intents only; content edits are admin-only and published content controls new runtime behavior after restart for MVP.
- Craft recipe editing must be staged: identity/inputs/outputs read-only first,
  `required_rank` safe edit first, broader economic/job-affecting fields only
  after active-job version/refund/completion policy is enforced.

Implementation rules:
- Follow `AGENTS.md`.
- Use `$caveman`.
- Use Symphony worker rules when dispatched by Symphony.
- Use Context7 before Docker/Postgres/pgx/API syntax work.
- Keep slices small. No monolith files. No giant tests.
- Treat `internal/game/content` as canonical content bundle, not a new competing catalog layer.
- Put DB details in `internal/game/contentdb`; gameplay packages must not import Postgres/pgx directly.
- Root runtime should depend on `content.Repository`; real mode uses DB repo, explicit dev/test mode may use `content.StaticRepository`.
- Static content remains seed/fallback/test fixture only after DB repo lands.

Done criteria:
- Empty DB boot seeds one current published version exactly once.
- Non-empty DB boot never overwrites admin content silently.
- Real mode fails closed if DB, migrations, published snapshot, or validation fails.
- DB-published content changes reach runtime catalogs after restart.
- Admin invalid content cannot publish.
- Rollback restores previous published version.
- Safe projection leak tests pass with forbidden field names and sentinel hidden values.
- Client admin CMS uses authenticated admin ops; no fake/demo runtime content.
- Client admin CMS depends on real `admin.content.*` operations; no local-only
  fake draft editor path.
- Full verification passes:
  - `go test ./...`
  - `cd client && npm --cache /tmp/gameproject-npm-cache run check`
  - `git diff --check`
