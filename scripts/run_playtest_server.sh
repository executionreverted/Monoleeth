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

cd "$ROOT_DIR"

if ! env_bool "${GAME_SKIP_CLIENT_BUILD:-false}"; then
  npm --cache "$NPM_CACHE" --prefix client run build
fi

if env_bool "${GAME_RUN_BUNDLE_SCAN:-true}"; then
  (
    cd client
    node tests/bundle-scan.mjs
  )
fi

export GAME_CLIENT_STATIC_DIR="${GAME_CLIENT_STATIC_DIR:-client/dist}"
export GAME_SERVER_ADDR="${GAME_SERVER_ADDR:-127.0.0.1:8080}"
export GAME_PLAYTEST_SEED="${GAME_PLAYTEST_SEED:-true}"

echo "Playtest client: http://${GAME_SERVER_ADDR}"
echo "Static dir: ${GAME_CLIENT_STATIC_DIR}"
echo "Playtest seed: ${GAME_PLAYTEST_SEED}"

if env_bool "${GAME_PLAYTEST_BUILD_ONLY:-false}"; then
  echo "GAME_PLAYTEST_BUILD_ONLY=true; build and artifact scan completed."
  exit 0
fi

go run ./cmd/game-server
