#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_CACHE="${NPM_CACHE:-/tmp/gameproject-npm-cache}"

cd "$ROOT_DIR"

if [[ ! -d client/node_modules ]]; then
  npm --cache "$NPM_CACHE" --prefix client ci
fi

npm --cache "$NPM_CACHE" --prefix client run test:bundle-scan-extra-root
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
