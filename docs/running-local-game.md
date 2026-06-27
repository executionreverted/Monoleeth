# Running The Local Game Client

This is the local real-client path. It uses mail/password auth, server-owned
cookie sessions, and the Go WebSocket gateway. Do not use `?demo=1` unless you
are explicitly testing the isolated fixture/demo path.

For the current playable-build status, remaining rollout tasks, test-server
commands, and asset needs, see `docs/playtest-vertical-slice-status.md`. For
private test-server operations, see `docs/test-server-operations.md`.

## Start The Server

From the repo root:

```bash
GAME_DEV_MODE=1 go run ./cmd/game-server
```

The default server address is `:8080`, and the default allowed browser origins
are `http://localhost:5173` and `http://127.0.0.1:5173`.

Useful local overrides:

```bash
GAME_SERVER_ADDR=127.0.0.1:8080 \
GAME_ALLOWED_ORIGINS=http://127.0.0.1:5173 \
GAME_DEV_MODE=1 \
go run ./cmd/game-server
```

Optional local admin seed:

```bash
GAME_ADMIN_EMAIL=admin@example.com \
GAME_ADMIN_PASSWORD='replace-with-local-secret' \
GAME_ADMIN_CALLSIGN=Admin \
GAME_DEV_MODE=1 \
go run ./cmd/game-server
```

The admin seed is for reproducible local/dev setup only. Never use a shared or
production password here.

## Start A Single-Process Playtest Server

To build the browser client and serve the built app from the same Go process:

```bash
scripts/run_playtest_server.sh
```

Open:

```text
http://127.0.0.1:8080
```

The script runs `npm --prefix client run build`, sets
`GAME_CLIENT_STATIC_DIR=client/dist`, `GAME_DEV_MODE=true`, and
`GAME_PLAYTEST_SEED=true`, scans the built bundle for fixture/server-only leak
tokens, then starts `go run ./cmd/game-server`.
Override the bind address or static dir when needed:

```bash
GAME_SERVER_ADDR=127.0.0.1:8081 \
GAME_CLIENT_STATIC_DIR=/absolute/path/to/dist \
scripts/run_playtest_server.sh
```

The playtest seed is an explicit test-server onboarding aid. It keeps the
normal server-authoritative flow, but gives each new player one X Core, claim
rank eligibility, and two owned route-test production planets with source
storage so the browser loop can reach claim and route actions without admin
setup. Disable it with:

```bash
GAME_PLAYTEST_SEED=false scripts/run_playtest_server.sh
```

`GAME_DEV_MODE=true` is the no-DB local/playtest path. For durable local runs,
start Postgres and provide `GAME_CONTENT_DATABASE_URL`,
`GAME_CONTENT_MODE=required`, and `GAME_CORE_STORE_MODE=required` instead.

For CI or deploy artifact preparation, run the same build and artifact scan
without starting the long-running server:

```bash
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
```

Useful build/scan toggles:

```bash
GAME_SKIP_CLIENT_BUILD=true scripts/run_playtest_server.sh
GAME_RUN_BUNDLE_SCAN=false scripts/run_playtest_server.sh
GAME_ARTIFACT_SCAN_ROOTS="/path/to/published:/path/to/staging" \
  GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
```

To stage the built client into the exact directory a deploy job will publish,
set `GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR`. The directory must be empty by
default so stale files cannot survive into a test-server artifact. For a reused
staging directory, opt in to cleaning it first:

```bash
GAME_PLAYTEST_BUILD_ONLY=true \
GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR=/path/to/publish \
GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR=true \
scripts/run_playtest_server.sh
```

In this mode `/api`, `/ws`, and `/healthz` remain server routes. Other browser
routes fall back to `index.html`, so reloading a client route works without
Vite. Missing asset files and unknown `/api/*` paths still return `404`.

## Start The Client

In another terminal:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run dev
```

Open:

```text
http://127.0.0.1:5173
```

Vite proxies `/api` and `/ws` to the Go server on `127.0.0.1:8080`.
Override the proxy for isolated local runs with:

```bash
GAME_CLIENT_PROXY_TARGET=http://127.0.0.1:8081 \
npm --cache /tmp/gameproject-npm-cache run dev -- --port 5174
```

## Bounded Multi-Map Dev Behavior

The local in-memory/dev runtime already routes the real authenticated client
through bounded multi-map behavior: server-owned active map membership,
`10000x10000` map snapshots, per-map workers/AOI, and portal handoff between
the starter `1-1` Origin Fringe map and the `1-2` Outer Ring map.

There is no production feature flag for this today. If DB persistence or a
production rollout path is introduced later, use
`GAME_FEATURE_BOUNDED_MULTI_MAP` as the proposed flag name and add explicit
backfill/quarantine docs before enabling it. Old durable coordinates should be
backfilled to a known map only when valid, and out-of-bounds or unassigned rows
should be quarantined instead of silently clamped.

## Verification

Before handing off changes:

```bash
go test ./...
cd client
npm --cache /tmp/gameproject-npm-cache run check
cd ..
git diff --check
```

The client check runs lint, typecheck, unit tests, and bundle scan. It does not
run Playwright smoke.

The bundle scan always checks `client/dist`. To also scan deployed, staging, or
published artifact directories, pass them as path-delimited roots:

```bash
cd client
GAME_ARTIFACT_SCAN_ROOTS="/path/to/published:/path/to/staging" node tests/bundle-scan.mjs
```

Extra roots can also be passed as positional arguments after `bundle-scan.mjs`.
The extra-root scanner behavior has a focused regression:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run test:bundle-scan-extra-root
```

The CI/deploy wrapper `scripts/ci_playtest_artifact_gate.sh` installs client
dependencies when needed, runs that extra-root scanner regression, and then
runs the same build-only playtest artifact gate. This proves the checked-in
deployable client package and default `client/dist` leak scan. A future
external deploy job should still pass its final staging or published artifact
directory through `GAME_ARTIFACT_SCAN_ROOTS`. The publish directory guard has a
focused regression:

```bash
scripts/test_playtest_publish_dir_guard.sh
```

The ready GitHub Actions workflow template is checked in at
`docs/ci/playtest-artifact-gate-github-actions.yml`. Copy it to
`.github/workflows/playtest-artifact-gate.yml` with a credential that has
GitHub `workflow` scope to activate the hosted artifact gate.

Run the full built-client playtest vertical-slice gate explicitly:

```bash
scripts/verify_playtest_vertical_slice.sh
```

This is intentionally not part of the routine `npm run check`. It runs the
playtest build/artifact scan gate once, then reuses that scanned `client/dist`
artifact for the single-process browser playtest loop, the single-process
PvP/death/repair proof, the destination/PvP scanner plus Border Skirmish drop
canary, and the scanner no-signal canary. If the build gate is disabled, the
wrapper falls back to the npm E2E scripts that build before running. To inspect
the command list without launching the browser proofs:

```bash
GAME_PLAYTEST_VERIFY_DRY_RUN=true scripts/verify_playtest_vertical_slice.sh
```

Each step can be skipped with `GAME_PLAYTEST_VERIFY_BUILD_GATE=false`,
`GAME_PLAYTEST_VERIFY_MAIN_LOOP=false`, `GAME_PLAYTEST_VERIFY_PVP_LOOP=false`,
`GAME_PLAYTEST_VERIFY_PVP_MAP_DROP=false`, or
`GAME_PLAYTEST_VERIFY_SCAN_NO_SIGNAL=false`.

Run the focused Phase09 bounded-map/portal browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase09-map
```

Run the single-process built-client playtest proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
```

That proof builds `client/dist`, starts `cmd/game-server` with
`GAME_CLIENT_STATIC_DIR=client/dist`, `GAME_DEV_MODE=1`, and
`GAME_PLAYTEST_SEED=true`, registers a real browser user from the served app,
verifies the playtest onboarding seed, completes a starter NPC fight and loot
pickup, clicks the real HUD scanner and planet claim controls, verifies X Core
consumption plus production initialization, clicks real HUD route create/settle
controls, then transfers through `east_gate` to public `1-2` and completes a
destination-map NPC fight and loot pickup without Vite.

Run the single-process built-client PvP/death/repair proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server-pvp
```

That proof builds `client/dist`, serves it from `cmd/game-server` with
`GAME_CLIENT_STATIC_DIR=client/dist`, opens two real browser sessions from the
same served app, moves both through `1-1` -> `1-2` -> `1-3`, verifies protected
PvP rejection, proves lethal PvP death cargo drop visibility for the attacker,
then runs `death.repair_quote` and `death.repair_ship` to reconcile the target
at the public `1-3` checkpoint with respawn protection and strict leak
canaries, all without Vite.

Run the single-process built-client Border Skirmish NPC drop proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-map-drop
```

That proof registers a normal browser player, travels through `1-1` -> `1-2` ->
`1-3`, resolves browser `scan.pulse` successfully on public `1-2` and public
`1-3`, kills a public Border Skirmish NPC, picks up the server-created
`carbon_shards` drop, and scans DOM/state/storage/WebSocket/process-log
surfaces for hidden map/scan/drop internals without Vite.

The separate Phase09 map smoke starts its own real Go server and Vite dev
server, then writes screenshots under `output/screenshots/ui-implementation/09/`,
including the current desktop artifacts:

```text
output/screenshots/ui-implementation/09/map-origin-desktop.png
output/screenshots/ui-implementation/09/map-outer-ring-desktop.png
```

This smoke is not part of `npm run check` today.
