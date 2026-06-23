# Playtest Vertical Slice Status

Date: 2026-06-24

This report tracks the current test-server readiness of the server-authoritative
browser game. It is a status snapshot, not a completion claim. The active goal
is still open until the remaining rollout and asset gaps below are closed and
the final verification gate is run.

## Current Playable Build

The local single-process playtest path can build the browser client, serve it
from the Go server, seed a new player, and drive a real authenticated browser
loop with server-owned state:

- register/login
- starter spawn with real ship/loadout/cargo/wallet state
- movement and NPC combat
- loot drop and pickup
- scanner pulse and planet discovery
- planet detail and claim with X Core consumption
- production initialization and a real `planet.building_build`
- route create/settle
- portal travel from public `1-1` to public `1-2`
- destination-map NPC combat/drop/pickup

Separate built-client canaries cover:

- public `1-3` PvP/death/repair with two authenticated browser sessions
- public `1-2` and `1-3` scanner/detail/claim/drop matrix proof
- scanner `no_signal` for a no-planet candidate and hidden player outside radar
- deployable `client/dist` artifact scanning, including a staged publish copy

## Run Commands

Start a playable local test server:

```bash
scripts/run_playtest_server.sh
```

Build and scan the deployable artifact without starting the server:

```bash
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
```

Run the full local vertical-slice verification gate:

```bash
scripts/verify_playtest_vertical_slice.sh
```

Print the full gate without launching browser proofs:

```bash
GAME_PLAYTEST_VERIFY_DRY_RUN=true scripts/verify_playtest_vertical_slice.sh
```

Run the hosted-CI/deploy artifact gate locally:

```bash
scripts/ci_playtest_artifact_gate.sh
```

Verify the publish-directory guard for reused staging directories:

```bash
scripts/test_playtest_publish_dir_guard.sh
```

A ready GitHub Actions workflow template exists at:

```text
docs/ci/playtest-artifact-gate-github-actions.yml
```

Activating it under `.github/workflows/` requires a GitHub credential with
`workflow` scope.

## Evidence

Current focused proof commands:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:playtest-server-pvp
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-pvp-map-drop
npm --cache /tmp/gameproject-npm-cache --prefix client run e2e:phase10-scan-no-signal
scripts/ci_playtest_artifact_gate.sh
```

The playtest asset screenshot proof writes:

```text
output/screenshots/ui-implementation/playtest/asset-sprites-desktop.png
```

The renderer currently loads the first world asset set from:

```text
client/src/assets/world/
```

## Remaining Work Before A Public Test Server

1. Run the full `scripts/verify_playtest_vertical_slice.sh` gate after the next
   gameplay/content pass and record the exact date/result.
2. Activate the hosted artifact workflow or wire the same
   `scripts/ci_playtest_artifact_gate.sh` into the external deploy pipeline.
3. Finish broader Phase10 rollout canaries:
   - additional PvP rollout canaries beyond the focused `1-3` death/repair proof
   - fuller browser scanner/claim/drop matrix variants
   - broader per-map/risk/rank loot balance coverage
   - production-log/admin-response leak canaries beyond the current harnesses
4. Decide whether the first public test server accepts process-local in-memory
   persistence, or whether durable DB-backed claim/production/route/death rows
   must land first.
5. Prepare test-server operations notes: seed policy, reset policy, port/origin
   config, artifact location, rollback steps, and whether the deploy job cleans
   a reused `GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR` before publishing.

## Asset Needs

The current asset set is good enough for proof-of-play, but not final art.
Needed before a more public-facing test:

- final player ship variants or 3D-rendered sprite sheets
- distinct NPC silhouettes for starter, outer-ring, and PvP-map enemies
- stronger loot crate/material icons for `raw_ore`, `carbon_shards`, and later
  crafted materials
- planet visuals for unknown/known/claimed/owned states
- portal gate variants per route/map tier
- safe-zone and PvP danger markers that match the final HUD style
- optional impact/projectile/repair effects for readability in combat footage

The canvas can already accept asset replacements through
`client/src/assets/world/` and `client/src/render/world-renderer-assets.ts`.
