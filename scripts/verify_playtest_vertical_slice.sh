#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_CACHE="${NPM_CACHE:-/tmp/gameproject-npm-cache}"

env_bool() {
  case "${1:-}" in
    1 | true | TRUE | yes | YES | on | ON) return 0 ;;
    *) return 1 ;;
  esac
}

run_step() {
  local label="$1"
  shift
  echo
  echo "==> ${label}"
  if env_bool "${GAME_PLAYTEST_VERIFY_DRY_RUN:-false}"; then
    printf 'DRY RUN:'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

run_client_canary() {
  local label="$1"
  shift
  local npm_script="$1"
  shift

  if env_bool "${GAME_PLAYTEST_VERIFY_BUILD_GATE:-true}"; then
    run_step "$label" "$@"
    return 0
  fi

  run_step "$label" npm --cache "$NPM_CACHE" --prefix client run "$npm_script"
}

cd "$ROOT_DIR"

if env_bool "${GAME_PLAYTEST_VERIFY_BUILD_GATE:-true}"; then
  run_step \
    "Build deployable playtest artifact and scan staged publish bundle" \
    scripts/ci_playtest_artifact_gate.sh
fi

if env_bool "${GAME_PLAYTEST_VERIFY_MAIN_LOOP:-true}"; then
  run_client_canary \
    "Built-client playtest loop: auth, combat, loot, scan, claim, production/building, route, portal, destination loot" \
    e2e:playtest-server \
    node client/tests/e2e/playtest-server-flow.mjs
fi

if env_bool "${GAME_PLAYTEST_VERIFY_PVP_LOOP:-true}"; then
  run_client_canary \
    "Built-client PvP/death/repair loop" \
    e2e:playtest-server-pvp \
    env PHASE10_BUILT_CLIENT=1 node client/tests/e2e/phase10-pvp-death-flow.mjs
fi

if env_bool "${GAME_PLAYTEST_VERIFY_ENEMY_AGGRO:-true}"; then
  run_client_canary \
    "Built-client Border Skirmish enemy aggro/leash canary" \
    e2e:phase10-enemy-aggro-built \
    env PHASE10_BUILT_CLIENT=1 node client/tests/e2e/phase10-enemy-aggro-flow.mjs
fi

if env_bool "${GAME_PLAYTEST_VERIFY_PVP_MAP_DROP:-true}"; then
  run_client_canary \
    "Built-client destination/PvP scanner, claim, and Border Skirmish drop canary" \
    e2e:phase10-pvp-map-drop \
    node client/tests/e2e/phase10-pvp-map-drop-flow.mjs
fi

if env_bool "${GAME_PLAYTEST_VERIFY_SCAN_NO_SIGNAL:-true}"; then
  run_client_canary \
    "Built-client hidden-player scanner no-signal canary" \
    e2e:phase10-scan-no-signal \
    node client/tests/e2e/phase10-scan-no-signal-flow.mjs
fi

echo
echo "Playtest vertical slice verification complete."
