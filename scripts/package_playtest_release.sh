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

release_revision() {
  git -C "$ROOT_DIR" rev-parse --short=12 HEAD 2>/dev/null || printf "unknown"
}

release_timestamp() {
  date -u +"%Y%m%dT%H%M%SZ"
}

release_dir="${GAME_PLAYTEST_RELEASE_DIR:-}"
if [[ -z "$release_dir" ]]; then
  release_dir="$ROOT_DIR/output/playtest-release/$(release_timestamp)-$(release_revision)"
elif [[ "$release_dir" != /* ]]; then
  release_dir="$ROOT_DIR/$release_dir"
fi

mkdir -p "$release_dir"
release_physical="$(cd "$release_dir" && pwd -P)"

case "$release_physical" in
  "/" | "/tmp" | "/private/tmp" | "$HOME" | "$ROOT_PHYSICAL" | "$ROOT_PHYSICAL/client" | "$ROOT_PHYSICAL/client/dist")
    echo "Refusing unsafe playtest release directory: $release_physical" >&2
    exit 1
    ;;
esac

if [[ -n "$(find "$release_physical" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  if env_bool "${GAME_PLAYTEST_CLEAN_RELEASE_DIR:-false}"; then
    find "$release_physical" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
  else
    echo "Playtest release directory is not empty: $release_physical" >&2
    echo "Set GAME_PLAYTEST_CLEAN_RELEASE_DIR=true to clean it before packaging." >&2
    exit 1
  fi
fi

client_dir="$release_physical/client-dist"
bin_dir="$release_physical/bin"
binary_path="$bin_dir/game-server"

mkdir -p "$bin_dir"

cd "$ROOT_DIR"

GAME_PLAYTEST_BUILD_ONLY=true \
  GAME_PLAYTEST_PUBLISHED_ARTIFACT_DIR="$client_dir" \
  GAME_PLAYTEST_CLEAN_PUBLISHED_ARTIFACT_DIR=false \
  NPM_CACHE="$NPM_CACHE" \
  scripts/run_playtest_server.sh

go build -o "$binary_path" ./cmd/game-server

revision="$(release_revision)"
created_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

cat >"$release_physical/run.sh" <<'RUN_SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

RELEASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

export GAME_CLIENT_STATIC_DIR="${GAME_CLIENT_STATIC_DIR:-$RELEASE_DIR/client-dist}"
export GAME_SERVER_ADDR="${GAME_SERVER_ADDR:-0.0.0.0:8080}"
export GAME_PLAYTEST_SEED="${GAME_PLAYTEST_SEED:-true}"

env_enabled() {
  case "${1:-}" in
    1 | true | TRUE | yes | YES | on | ON) return 0 ;;
    *) return 1 ;;
  esac
}

if [[ -z "${GAME_ALLOWED_ORIGINS:-}" ]]; then
  echo "GAME_ALLOWED_ORIGINS is required, for example https://playtest.example.com" >&2
  exit 1
fi

dev_mode_enabled=false
if env_enabled "${GAME_DEV_MODE:-false}"; then
  dev_mode_enabled=true
fi
durable_mode_enabled=false
if [[ -n "${GAME_CONTENT_DATABASE_URL:-}" ]]; then
  durable_mode_enabled=true
fi

if [[ "$dev_mode_enabled" == false && "$durable_mode_enabled" == false ]]; then
  echo "Choose a playtest state mode before starting this package." >&2
  echo "For resettable no-DB local/private playtests set GAME_DEV_MODE=true." >&2
  echo "For durable shared playtests set GAME_CONTENT_DATABASE_URL plus GAME_CONTENT_MODE=required and GAME_CORE_STORE_MODE=required." >&2
  exit 1
fi

if [[ "$dev_mode_enabled" == true && "$durable_mode_enabled" == true ]]; then
  echo "Choose exactly one playtest state mode." >&2
  echo "Use GAME_DEV_MODE=true only for resettable no-DB local/private playtests." >&2
  echo "Use GAME_CONTENT_DATABASE_URL only with durable shared playtest mode." >&2
  exit 1
fi

if [[ "$durable_mode_enabled" == true ]]; then
  if [[ "${GAME_CONTENT_MODE:-required}" != "required" || "${GAME_CORE_STORE_MODE:-required}" != "required" ]]; then
    echo "Durable package mode requires GAME_CONTENT_MODE=required and GAME_CORE_STORE_MODE=required." >&2
    exit 1
  fi
fi

exec "$RELEASE_DIR/bin/game-server"
RUN_SCRIPT
chmod +x "$release_physical/run.sh"

cat >"$release_physical/manifest.json" <<MANIFEST
{
  "revision": "$revision",
  "created_at": "$created_at",
  "server_binary": "bin/game-server",
  "client_static_dir": "client-dist",
  "run_script": "run.sh",
  "required_env": ["GAME_ALLOWED_ORIGINS"],
  "default_env": {
    "GAME_SERVER_ADDR": "0.0.0.0:8080",
    "GAME_PLAYTEST_SEED": "true"
  },
  "state_mode_policy": "Set either GAME_DEV_MODE=true for resettable no-DB local/private playtests or GAME_CONTENT_DATABASE_URL with required content/core-store modes for durable shared playtests.",
  "verification": {
    "client_build": "scripts/run_playtest_server.sh",
    "bundle_scan": "client/tests/bundle-scan.mjs",
    "server_build": "go build ./cmd/game-server"
  }
}
MANIFEST

cat >"$release_physical/README.md" <<README
# Playtest Release $revision

Run from this directory:

\`\`\`bash
GAME_ALLOWED_ORIGINS=https://playtest.example.com \\
GAME_CONTENT_DATABASE_URL=postgres://gameproject:pw@db:5432/gameproject?sslmode=disable \\
GAME_CONTENT_MODE=required \\
GAME_CORE_STORE_MODE=required \\
GAME_CONTENT_MIGRATIONS=auto \\
./run.sh
\`\`\`

Defaults:

- GAME_SERVER_ADDR=0.0.0.0:8080
- GAME_CLIENT_STATIC_DIR=./client-dist
- GAME_PLAYTEST_SEED=true

Production-like durable runs must provide the required Postgres content/core
store env. For a local resettable no-DB playtest only, explicitly opt in with
GAME_DEV_MODE=true.
README

echo "Playtest release packaged: $release_physical"
echo "Revision: $revision"
echo "Run: GAME_ALLOWED_ORIGINS=https://playtest.example.com $release_physical/run.sh"
