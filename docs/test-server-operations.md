# Test Server Operations Runbook

Date: 2026-06-24

This runbook covers the first playable test server for the DarkOrbit-style
vertical slice. It is for private or controlled playtests while the game still
uses process-local runtime state. It is not a public production launch plan.

## Current Readiness

The supported deployment shape is one Go process serving:

- `/api`, `/ws`, and `/healthz` from `cmd/game-server`
- the built browser client from `client/dist`
- the playtest onboarding seed through `GAME_PLAYTEST_SEED=true`

Build and scan the candidate artifact first:

```bash
scripts/ci_playtest_artifact_gate.sh
```

The same gate is installed for hosted CI at:

```text
.github/workflows/playtest-artifact-gate.yml
```

It runs on pull requests, pushes to `master`, and manual dispatch.

To create a self-contained release directory with the built client, Go server
binary, manifest, README, and guarded `run.sh`, use:

```bash
scripts/package_playtest_release.sh
```

By default this writes under:

```text
output/playtest-release/<utc-timestamp>-<git-revision>/
```

To choose the directory explicitly:

```bash
GAME_PLAYTEST_RELEASE_DIR=/srv/gameproject/releases/2026-06-24-de64311 \
scripts/package_playtest_release.sh
```

Then run the packaged server with the real browser origin:

```bash
GAME_ALLOWED_ORIGINS=https://playtest.example.com \
/srv/gameproject/releases/2026-06-24-de64311/run.sh
```

The package test is:

```bash
scripts/test_playtest_release_package.sh
```

For source-tree runs without a package, run the server with an explicit static
artifact directory and externally reachable bind address:

```bash
GAME_SERVER_ADDR=0.0.0.0:8080 \
GAME_ALLOWED_ORIGINS=https://playtest.example.com \
GAME_CLIENT_STATIC_DIR=/srv/gameproject/client-dist.current \
GAME_PLAYTEST_SEED=true \
go run ./cmd/game-server
```

If the server is behind a reverse proxy, terminate TLS at the proxy and forward
HTTP/WebSocket traffic for `/`, `/api`, `/ws`, and `/healthz` to
`GAME_SERVER_ADDR`.

## Artifact Policy

The deployable browser artifact is `client/dist`.

Before publishing an artifact, run:

```bash
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
```

For a complete server+client release directory, prefer:

```bash
scripts/package_playtest_release.sh
```

For the exact staging directory that the deploy job will serve, use:

```bash
GAME_PLAYTEST_BUILD_ONLY=true \
GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR=/srv/gameproject/client-dist.next \
scripts/run_playtest_server.sh
```

The publish directory must be empty by default. This prevents old hashed assets
or stale source maps from surviving into the test-server bundle. If a deploy job
intentionally reuses a staging directory, it must opt in to cleaning:

```bash
GAME_PLAYTEST_BUILD_ONLY=true \
GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR=/srv/gameproject/client-dist.next \
GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR=true \
scripts/run_playtest_server.sh
```

The focused guard is:

```bash
scripts/test_playtest_publish_dir_guard.sh
```

After a staged build passes, promote the directory atomically at the host level,
for example by switching a symlink from `client-dist.current` to
`client-dist.next`.

## Seed Policy

Use `GAME_PLAYTEST_SEED=true` for the first private test server. It gives each
new player enough real server-owned state to reach the current playable loop:

- starter ship/loadout and normal authenticated spawn
- one real Inventory `x_core`
- claim rank eligibility through Progression
- two owned route-test production planets with source storage
- access to the seeded public maps and portals

This seed is an onboarding aid, not a production economy rule. Do not enable
`GAME_DEV_MODE`, `GAME_E2E_PLANET_CLAIM_SEED`, `GAME_E2E_ROUTE_SEED`, or
`GAME_E2E_SCAN_NO_PLANET_SEED` on the test server. Those flags are for local
E2E harnesses only and startup rejects them outside dev mode.

Production tuning must remain separate from playtest seeding. Scanner rarity,
enemy density, drop rates, PvP rewards, and route risk should be changed in
content/config code and then covered by focused tests.

## Reset Policy

The first test server can be reset by restarting the Go process because current
live gameplay state is process-local. This is acceptable only for controlled
playtests where wipes are expected.

Before each scheduled reset:

1. Announce the reset window to testers.
2. Stop the Go server.
3. Promote or roll back the intended `client/dist` artifact.
4. Start the Go server with the same `GAME_PLAYTEST_SEED=true` setting.
5. Run a login/spawn/portal sanity pass or the focused built-client gate before
   reopening the test link.

Do not call this persistent progression until durable DB-backed rows exist for
claims, production, routes, death/repair, inventory, wallet, and outbox events.

## Port And Origin Config

Use these server env vars:

- `GAME_SERVER_ADDR`: bind address, for example `0.0.0.0:8080` on a host or
  `127.0.0.1:8080` behind a local-only proxy.
- `GAME_ALLOWED_ORIGINS`: comma-separated browser origins allowed for cookie
  sessions and WebSocket upgrades, for example `https://playtest.example.com`.
- `GAME_CLIENT_STATIC_DIR`: absolute or repo-relative path to the built client
  artifact.
- `GAME_PLAYTEST_SEED`: `true` for the current private test-server onboarding
  seed.

Set `GAME_ALLOWED_ORIGINS` to the real browser URL. A mismatch will make login
or WebSocket connection fail even if the server is reachable.

## Rollback Steps

Rollback should restore both the server revision and the client artifact
together.

1. Stop the Go server or drain traffic at the reverse proxy.
2. Switch `GAME_CLIENT_STATIC_DIR` or the artifact symlink back to the previous
   scanned `client/dist` directory.
3. Check out or redeploy the matching previous server revision.
4. Start the server with the previous env set.
5. Verify `/healthz`, browser register/login, spawn, one NPC fight, one loot
   pickup, and one portal transfer.
6. Run the bundle scan against the active artifact:

```bash
cd client
GAME_ARTIFACT_SCAN_ROOTS=/srv/gameproject/client-dist.current node tests/bundle-scan.mjs
```

If rollback follows a gameplay-state bug, wipe the process-local server state
by restarting the process and tell testers the shard was reset.

## Pre-Open Checklist

Before sharing the test link:

- `scripts/ci_playtest_artifact_gate.sh` passes.
- The hosted `Playtest Artifact Gate` workflow has passed for the deployed
  commit, or the missing hosted run is recorded in the playtest status report.
- `scripts/test_playtest_publish_dir_guard.sh` passes.
- `scripts/test_playtest_release_package.sh` passes if using a packaged
  server+client release directory.
- `scripts/verify_playtest_vertical_slice.sh` has passed for the candidate
  build, or the exact skipped canaries are recorded in the status report.
- `GAME_ALLOWED_ORIGINS` matches the public URL.
- `GAME_CLIENT_STATIC_DIR` points at the scanned artifact.
- `GAME_PLAYTEST_SEED=true` is set.
- Dev-only E2E seed flags are absent.
- The packaged `manifest.json` revision matches the intended server revision
  if using `scripts/package_playtest_release.sh`.
- The current asset needs and known rollout gaps are copied from
  `docs/playtest-vertical-slice-status.md` into the playtest announcement.
