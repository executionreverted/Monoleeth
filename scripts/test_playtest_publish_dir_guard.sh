#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -d client/dist ]]; then
  GAME_PLAYTEST_BUILD_ONLY=true \
    GAME_RUN_BUNDLE_SCAN=false \
    scripts/run_playtest_server.sh >/dev/null
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/gameproject-publish-guard.XXXXXX")"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

publish_dir="$tmp_dir/publish"
mkdir -p "$publish_dir"
echo "stale" >"$publish_dir/stale.txt"

set +e
reject_output="$(
  GAME_SKIP_CLIENT_BUILD=true \
    GAME_RUN_BUNDLE_SCAN=false \
    GAME_PLAYTEST_BUILD_ONLY=true \
    GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR="$publish_dir" \
    scripts/run_playtest_server.sh 2>&1
)"
reject_status=$?
set -e

if [[ "$reject_status" -eq 0 ]]; then
  echo "expected non-empty published artifact directory to be rejected" >&2
  exit 1
fi

if [[ "$reject_output" != *"Published artifact directory is not empty"* ]]; then
  echo "expected rejection output to explain the non-empty publish directory" >&2
  echo "$reject_output" >&2
  exit 1
fi

GAME_SKIP_CLIENT_BUILD=true \
  GAME_RUN_BUNDLE_SCAN=false \
  GAME_PLAYTEST_BUILD_ONLY=true \
  GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR="$publish_dir" \
  GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR=true \
  scripts/run_playtest_server.sh >/dev/null

if [[ -e "$publish_dir/stale.txt" ]]; then
  echo "expected clean publish mode to remove stale files" >&2
  exit 1
fi

if [[ ! -f "$publish_dir/index.html" ]]; then
  echo "expected clean publish mode to copy client/dist/index.html" >&2
  exit 1
fi

echo "playtest publish directory guard passed"
