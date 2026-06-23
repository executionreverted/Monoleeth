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

cd "$ROOT_DIR"

if env_bool "${GAME_PLAYTEST_VERIFY_BUILD_GATE:-true}"; then
  run_step \
    "Build deployable playtest artifact and scan bundle" \
    env GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
fi

if env_bool "${GAME_PLAYTEST_VERIFY_MAIN_LOOP:-true}"; then
  run_step \
    "Built-client playtest loop: auth, combat, loot, scan, claim, production, route, portal, destination loot" \
    npm --cache "$NPM_CACHE" --prefix client run e2e:playtest-server
fi

if env_bool "${GAME_PLAYTEST_VERIFY_PVP_LOOP:-true}"; then
  run_step \
    "Built-client PvP/death/repair loop" \
    npm --cache "$NPM_CACHE" --prefix client run e2e:playtest-server-pvp
fi

if env_bool "${GAME_PLAYTEST_VERIFY_PVP_MAP_DROP:-true}"; then
  run_step \
    "Built-client destination/PvP scanner and Border Skirmish drop canary" \
    npm --cache "$NPM_CACHE" --prefix client run e2e:phase10-pvp-map-drop
fi

echo
echo "Playtest vertical slice verification complete."
