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

if grep -q 'GAME_DEV_MODE.*:-true' "$release_dir/run.sh"; then
  echo "expected run.sh not to default GAME_DEV_MODE on packaged releases" >&2
  exit 1
fi

set +e
missing_mode_output="$(
  GAME_ALLOWED_ORIGINS=https://playtest.example.com "$release_dir/run.sh" 2>&1
)"
missing_mode_status=$?
set -e

if [[ "$missing_mode_status" -eq 0 ]]; then
  echo "expected run.sh to reject missing state mode" >&2
  exit 1
fi

if [[ "$missing_mode_output" != *"Choose a playtest state mode"* ]]; then
  echo "expected run.sh to explain missing state mode" >&2
  echo "$missing_mode_output" >&2
  exit 1
fi

set +e
mixed_mode_output="$(
  GAME_ALLOWED_ORIGINS=https://playtest.example.com \
  GAME_DEV_MODE=true \
  GAME_CONTENT_DATABASE_URL=postgres://gameproject:pw@db:5432/gameproject?sslmode=disable \
  "$release_dir/run.sh" 2>&1
)"
mixed_mode_status=$?
set -e

if [[ "$mixed_mode_status" -eq 0 ]]; then
  echo "expected run.sh to reject mixed dev and durable state modes" >&2
  exit 1
fi

if [[ "$mixed_mode_output" != *"Choose exactly one playtest state mode"* ]]; then
  echo "expected run.sh to explain mixed state modes" >&2
  echo "$mixed_mode_output" >&2
  exit 1
fi

set +e
non_required_mode_output="$(
  GAME_ALLOWED_ORIGINS=https://playtest.example.com \
  GAME_CONTENT_DATABASE_URL=postgres://gameproject:pw@db:5432/gameproject?sslmode=disable \
  GAME_CONTENT_MODE=off \
  GAME_CORE_STORE_MODE=dev_fallback \
  "$release_dir/run.sh" 2>&1
)"
non_required_mode_status=$?
set -e

if [[ "$non_required_mode_status" -eq 0 ]]; then
  echo "expected run.sh to reject non-required durable modes" >&2
  exit 1
fi

if [[ "$non_required_mode_output" != *"Durable package mode requires GAME_CONTENT_MODE=required and GAME_CORE_STORE_MODE=required"* ]]; then
  echo "expected run.sh to explain durable required modes" >&2
  echo "$non_required_mode_output" >&2
  exit 1
fi

if ! grep -q 'GAME_DEV_MODE=true' "$release_dir/README.md"; then
  echo "expected README to document explicit local no-DB dev-mode opt-in" >&2
  exit 1
fi

(
  cd client
  GAME_ARTIFACT_SCAN_ROOTS="$release_dir/client-dist" node tests/bundle-scan.mjs >/dev/null
)

echo "playtest release package test passed"
