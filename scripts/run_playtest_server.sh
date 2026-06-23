#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_CACHE="${NPM_CACHE:-/tmp/gameproject-npm-cache}"

cd "$ROOT_DIR"

npm --cache "$NPM_CACHE" --prefix client run build

export GAME_CLIENT_STATIC_DIR="${GAME_CLIENT_STATIC_DIR:-client/dist}"
export GAME_SERVER_ADDR="${GAME_SERVER_ADDR:-127.0.0.1:8080}"
export GAME_PLAYTEST_SEED="${GAME_PLAYTEST_SEED:-true}"

go run ./cmd/game-server
