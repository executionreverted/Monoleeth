#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_PHYSICAL="$(cd "$ROOT_DIR" && pwd -P)"
NPM_CACHE="${NPM_CACHE:-/tmp/gameproject-npm-cache}"

env_bool() {
  case "${1:-}" in
    1 | true | TRUE | yes | YES | on | ON) return 0 ;;
    *) return 1 ;;
  esac
}

publish_client_dist() {
  local target_dir="$1"

  mkdir -p "$target_dir"

  local target_physical
  target_physical="$(cd "$target_dir" && pwd -P)"

  case "$target_physical" in
    "/" | "/tmp" | "/private/tmp" | "$HOME" | "$ROOT_PHYSICAL" | "$ROOT_PHYSICAL/client")
      echo "Refusing unsafe published artifact directory: $target_physical" >&2
      exit 1
      ;;
  esac

  case "$target_physical/" in
    "$ROOT_PHYSICAL/client/dist/"*)
      echo "Refusing to publish into client/dist or its children: $target_physical" >&2
      exit 1
      ;;
  esac

  if [[ -n "$(find "$target_physical" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    if env_bool "${GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR:-false}"; then
      find "$target_physical" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
    else
      echo "Published artifact directory is not empty: $target_physical" >&2
      echo "Set GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR=true to clean it before publishing." >&2
      exit 1
    fi
  fi

  cp -R "$ROOT_PHYSICAL/client/dist/." "$target_physical"/
  published_artifact_dir="$target_physical"
}

cd "$ROOT_DIR"

if ! env_bool "${GAME_SKIP_CLIENT_BUILD:-false}"; then
  npm --cache "$NPM_CACHE" --prefix client run build
fi

artifact_scan_roots="${GAME_ARTIFACT_SCAN_ROOTS:-}"
if [[ -n "${GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR:-}" ]]; then
  published_artifact_dir="$GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR"
  case "$published_artifact_dir" in
    /*) ;;
    *) published_artifact_dir="$ROOT_DIR/$published_artifact_dir" ;;
  esac
  publish_client_dist "$published_artifact_dir"
  artifact_scan_roots="${artifact_scan_roots:+${artifact_scan_roots}:}${published_artifact_dir}"
fi

if env_bool "${GAME_RUN_BUNDLE_SCAN:-true}"; then
  (
    cd client
    GAME_ARTIFACT_SCAN_ROOTS="$artifact_scan_roots" node tests/bundle-scan.mjs
  )
fi

export GAME_CLIENT_STATIC_DIR="${GAME_CLIENT_STATIC_DIR:-client/dist}"
export GAME_SERVER_ADDR="${GAME_SERVER_ADDR:-127.0.0.1:8080}"
export GAME_DEV_MODE="${GAME_DEV_MODE:-true}"
export GAME_PLAYTEST_SEED="${GAME_PLAYTEST_SEED:-true}"
export GAME_DISABLE_AUTH_ATTEMPT_LIMIT="${GAME_DISABLE_AUTH_ATTEMPT_LIMIT:-true}"
export GAME_DEV_ACCOUNT_SEED="${GAME_DEV_ACCOUNT_SEED:-true}"
export GAME_DEV_ACCOUNT_PASSWORD="${GAME_DEV_ACCOUNT_PASSWORD:-dev-password}"
export GAME_DEV_ACCOUNT_CREDITS="${GAME_DEV_ACCOUNT_CREDITS:-100000}"

echo "Playtest client: http://${GAME_SERVER_ADDR}"
echo "Static dir: ${GAME_CLIENT_STATIC_DIR}"
echo "Dev mode: ${GAME_DEV_MODE}"
echo "Playtest seed: ${GAME_PLAYTEST_SEED}"
echo "Auth attempt limit disabled: ${GAME_DISABLE_AUTH_ATTEMPT_LIMIT}"
echo "Dev accounts seeded: ${GAME_DEV_ACCOUNT_SEED}"
if [[ -n "${GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR:-}" ]]; then
  echo "Published artifact dir: ${published_artifact_dir}"
fi

if env_bool "${GAME_PLAYTEST_BUILD_ONLY:-false}"; then
  echo "GAME_PLAYTEST_BUILD_ONLY=true; build and artifact scans completed."
  exit 0
fi

go run ./cmd/game-server
