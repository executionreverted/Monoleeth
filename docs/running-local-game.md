# Running The Local Game Client

This is the local real-client path. It uses mail/password auth, server-owned
cookie sessions, and the Go WebSocket gateway. Do not use `?demo=1` unless you
are explicitly testing the isolated fixture/demo path.

## Start The Server

From the repo root:

```bash
go run ./cmd/game-server
```

The default server address is `:8080`, and the default allowed browser origins
are `http://localhost:5173` and `http://127.0.0.1:5173`.

Useful local overrides:

```bash
GAME_SERVER_ADDR=127.0.0.1:8080 \
GAME_ALLOWED_ORIGINS=http://127.0.0.1:5173 \
go run ./cmd/game-server
```

Optional local admin seed:

```bash
GAME_ADMIN_EMAIL=admin@example.com \
GAME_ADMIN_PASSWORD='replace-with-local-secret' \
GAME_ADMIN_CALLSIGN=Admin \
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
`GAME_CLIENT_STATIC_DIR=client/dist` and `GAME_PLAYTEST_SEED=true`, then starts
`go run ./cmd/game-server`.
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

Run the focused Phase09 bounded-map/portal browser proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase09-map
```

Run the single-process built-client playtest proof explicitly:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
```

That proof builds `client/dist`, starts `cmd/game-server` with
`GAME_CLIENT_STATIC_DIR=client/dist` and `GAME_PLAYTEST_SEED=true`, registers a
real browser user from the served app, verifies the playtest onboarding seed,
and clicks real HUD route create/settle controls without Vite.

That smoke starts its own real Go server and Vite dev server, then writes
screenshots under `output/screenshots/ui-implementation/09/`, including the
current desktop artifacts:

```text
output/screenshots/ui-implementation/09/map-origin-desktop.png
output/screenshots/ui-implementation/09/map-outer-ring-desktop.png
```

This smoke is not part of `npm run check` today.
