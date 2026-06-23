#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NPM_CACHE="${NPM_CACHE:-/tmp/gameproject-npm-cache}"

cd "$ROOT_DIR"

if [[ ! -d client/node_modules ]]; then
  npm --cache "$NPM_CACHE" --prefix client ci
fi

cleanup_publish_dir=""
if [[ -z "${GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR:-}" ]]; then
  cleanup_publish_dir="$(mktemp -d "${TMPDIR:-/tmp}/gameproject-playtest-publish.XXXXXX")"
  export GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR="$cleanup_publish_dir"
fi
cleanup() {
  if [[ -n "$cleanup_publish_dir" ]]; then
    rm -rf "$cleanup_publish_dir"
  fi
}
trap cleanup EXIT

npm --cache "$NPM_CACHE" --prefix client run test:bundle-scan-extra-root
GAME_PLAYTEST_BUILD_ONLY=true scripts/run_playtest_server.sh
