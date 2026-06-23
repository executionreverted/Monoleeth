#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/gameproject-playtest-release.XXXXXX")"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

reject_dir="$tmp_dir/reject"
mkdir -p "$reject_dir"
echo "stale" >"$reject_dir/stale.txt"

set +e
reject_output="$(
  GAME_PLAYTEST_RELEASE_DIR="$reject_dir" \
    scripts/package_playtest_release.sh 2>&1
)"
reject_status=$?
set -e

if [[ "$reject_status" -eq 0 ]]; then
  echo "expected non-empty playtest release directory to be rejected" >&2
  exit 1
fi

if [[ "$reject_output" != *"Playtest release directory is not empty"* ]]; then
  echo "expected release rejection output to explain the non-empty directory" >&2
  echo "$reject_output" >&2
  exit 1
fi

release_dir="$tmp_dir/release"
GAME_PLAYTEST_RELEASE_DIR="$release_dir" scripts/package_playtest_release.sh >/dev/null

for required in \
  "$release_dir/bin/game-server" \
  "$release_dir/client-dist/index.html" \
  "$release_dir/manifest.json" \
  "$release_dir/README.md" \
  "$release_dir/run.sh"
do
  if [[ ! -e "$required" ]]; then
    echo "expected playtest release file missing: $required" >&2
    exit 1
  fi
done

if [[ ! -x "$release_dir/bin/game-server" ]]; then
  echo "expected packaged game-server binary to be executable" >&2
  exit 1
fi

if [[ ! -x "$release_dir/run.sh" ]]; then
  echo "expected packaged run.sh to be executable" >&2
  exit 1
fi

if ! grep -q '"client_static_dir": "client-dist"' "$release_dir/manifest.json"; then
  echo "expected manifest to describe client-dist" >&2
  cat "$release_dir/manifest.json" >&2
  exit 1
fi

if ! grep -q 'GAME_ALLOWED_ORIGINS' "$release_dir/run.sh"; then
  echo "expected run.sh to require GAME_ALLOWED_ORIGINS" >&2
  exit 1
fi

(
  cd client
  GAME_ARTIFACT_SCAN_ROOTS="$release_dir/client-dist" node tests/bundle-scan.mjs >/dev/null
)

echo "playtest release package test passed"
