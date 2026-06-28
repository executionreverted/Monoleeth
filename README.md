# Server-Authoritative 2D Space MORPG

Browser-first 2D space MORPG prototype with a Go authoritative game server and
a TypeScript/PixiJS client.

The current build is a playable vertical slice, not a finished game. It focuses
on the hard parts first: real auth, server-owned movement/combat/economy state,
safe WebSocket contracts, seeded content, and browser playtest proofs. The game
direction is inspired by classic browser space MMOs: sector maps, portals,
NPC farming, ship/loadout progression, ammo, loot, riskier maps, and a cockpit
HUD. It is not affiliated with DarkOrbit or Bigpoint.

## What Works Today

- Mail/password auth with server-owned cookie sessions.
- Authenticated WebSocket gameplay gateway.
- Server-authoritative movement, AOI snapshots, target lock, and combat stance.
- Automatic server tick laser fire while moving.
- NPC return fire, damage events, death/repair foundations, loot drops, and
  cargo pickup.
- Inventory, loadout, ammo selection, quickbar ammo assignment, wallet, shop,
  quests, hangar, crafting, planets, routes, portals, market/auction/premium
  surfaces in varying prototype depth.
- Kalaazu-derived default content seed path for maps, portals, NPC templates,
  items, ships, modules, shop products, quests, production, and combat rules.
- Runtime content can be loaded from a published Postgres content snapshot.
- Browser e2e canaries for the playable loop, PvP/death/repair, enemy aggro,
  scanner flows, and DarkOrbit-feel vertical slice.

## Current Status

This repo is in active prototype/playtest development.

What is solid:

- server authority and trusted-payload rejection
- real browser/server integration
- local playtest boot path
- focused automated proofs
- content seeding and migration foundations

What still needs product work:

- the moment-to-moment game feel is not there yet
- HUD still feels too much like a dense web app
- sectors need stronger visual identity, density, and danger
- progression needs more upgrade hunger and long-session pacing

See `docs/polish/00-index.md` and `docs/playtest-vertical-slice-status.md` for
the honest current-state notes.

## Tech Stack

- Backend: Go
- Client: TypeScript, PixiJS, Vite
- Realtime: WebSocket command/event protocol
- Persistence/content: optional Postgres content/core store
- Tests: Go tests, Vitest, Playwright browser e2e

## Quick Start

Prerequisites:

- Go matching `go.mod`
- Node.js/npm
- Optional: Docker, if you want durable Postgres mode

Install client dependencies:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client ci
```

Start a single-process local playtest server:

```bash
scripts/run_playtest_server.sh
```

Open:

```text
http://127.0.0.1:8080
```

The playtest script builds the client, scans the artifact, serves `client/dist`
from the Go server, enables local dev/playtest seed mode, and seeds unsafe
local-only accounts:

```text
pilot1@example.com / dev-password
pilot2@example.com / dev-password
```

These accounts are only for local/private playtests. Do not use these defaults
for shared or production deployments.

Useful local override:

```bash
GAME_SERVER_ADDR=127.0.0.1:8081 scripts/run_playtest_server.sh
```

## Durable Local Mode

For resettable local playtests, `scripts/run_playtest_server.sh` defaults to
`GAME_DEV_MODE=true` and process-local stores.

For durable content/core-store testing, start Postgres:

```bash
docker compose up -d postgres
```

Then run with explicit durable env:

```bash
GAME_CONTENT_DATABASE_URL=postgres://gameproject:gameproject_dev_password@127.0.0.1:5432/gameproject?sslmode=disable \
GAME_CONTENT_MODE=required \
GAME_CORE_STORE_MODE=required \
GAME_CONTENT_MIGRATIONS=auto \
scripts/run_playtest_server.sh
```

The first boot can seed default gameplay content into the DB. Runtime gameplay
truth should come from server snapshots/events/queries, not client fixtures.

## Development Commands

Backend tests:

```bash
go test ./...
```

Client checks:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run check
```

Full local vertical-slice verification:

```bash
scripts/verify_playtest_vertical_slice.sh
```

DarkOrbit-feel browser canary:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:darkorbit-feel
```

Build and scan the playtest artifact without starting the server:

```bash
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
```

Package a scanned playtest release:

```bash
scripts/package_playtest_release.sh
```

## Project Layout

```text
cmd/game-server/          Go server entrypoint
internal/game/            Gameplay domain, runtime, auth, economy, combat, world
internal/game/contentseed Default content builders and Kalaazu-derived seed input
client/                   Browser client, Pixi renderer, HUD, tests
docs/                     Plans, audits, runbooks, playtest status
scripts/                  Local playtest, verification, packaging helpers
```

## Architecture Principles

- The client sends intents, not facts.
- The server owns player identity, position, combat, cooldowns, damage, loot,
  wallet, inventory, cargo, quests, and map state.
- Visible gameplay values must come from authenticated server snapshots,
  server events, server queries, or clearly pending local UI state.
- Hidden gameplay truth must not be sent to the browser.
- Economy and inventory mutations must go through transaction-style service
  flows with ledgers/idempotency where needed.

## More Docs

- `docs/running-local-game.md` - local run modes
- `docs/test-server-operations.md` - private/shared playtest server runbook
- `docs/playtest-vertical-slice-status.md` - current playable proof status
- `docs/polish/00-index.md` - game-feel audit and polish backlog
- `docs/plans/2026-06-28-darkorbit-feel-implementation.md` - current feel slice
- `docs/plans/2026-06-28-kalaazu-db-default-seed-implementation.md` - content seed plan

## License

No license file is currently included. Treat the code as all rights reserved
unless a license is added.
