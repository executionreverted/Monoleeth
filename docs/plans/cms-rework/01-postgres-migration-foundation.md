# Phase 01 - Postgres And Migration Foundation

## Goal

Add local Postgres foundation for game content CMS.

No gameplay content moves yet. This phase only creates DB path, migration shape,
config, and testable connection boundary.

## Scope

- Docker Compose Postgres service.
- Named volume for local data.
- `.env.example` DB variables only, no secrets.
- Go DB config parsing.
- Migration runner contract.
- `ContentStore` package boundary.
- Minimal health/check command or server boot check.

## Out Of Scope

- Admin UI.
- Publish workflow.
- Runtime catalog replacement.
- Player state persistence.
- Market/inventory/wallet DB migration.

## Design

Create infra under small, clear files:

```text
docker-compose.yml
internal/game/contentdb/config.go
internal/game/contentdb/migrations.go
internal/game/contentdb/store.go
internal/game/contentdb/store_postgres.go
internal/game/contentdb/store_memory_test.go
```

`contentdb.Store` is CMS storage boundary. Gameplay packages should not import
Postgres driver directly.

Server config gets optional DB URL:

```text
GAME_CONTENT_DATABASE_URL
GAME_CONTENT_MIGRATIONS=auto|verify|off
GAME_CONTENT_MODE=required|dev_fallback|off
```

Local default can use Docker service name inside compose and localhost outside
compose, but secrets must stay env-based.

`required` mode is real gameplay mode. Missing DB URL or invalid content fails
boot. `dev_fallback` is only for local tests/dev and must be explicit.

## Docker Contract

Compose service:

```text
postgres image
POSTGRES_DB
POSTGRES_USER
POSTGRES_PASSWORD from env/default dev values
named volume /var/lib/postgresql/data
healthcheck pg_isready
```

If app service is added later, it must depend on DB health, not only container
start.

## Migration Contract

Migrations must be idempotent at runner level:

```text
schema_migrations(
  version text primary key,
  checksum text not null,
  applied_at timestamptz not null
)
```

Each migration:

- one reason to exist
- no seed data except schema bootstrap
- transaction-wrapped where possible
- fails closed on partial apply
- checksum mismatch fails closed

## Validation

Narrow tests:

```bash
go test ./internal/game/contentdb -count=1
git diff --check
```

Full later:

```bash
go test ./...
git diff --check
```

## Done

- `docker compose up postgres` starts healthy DB.
- migrations apply once, skip on repeat.
- server/test can open content DB connection.
- no gameplay runtime behavior changed.

## Implemented Slice

- Added `docker-compose.yml` Postgres service with named volume and
  `pg_isready` healthcheck.
- Added `.env.example` content DB vars:
  `GAME_CONTENT_DATABASE_URL`, `GAME_CONTENT_MODE`, and
  `GAME_CONTENT_MIGRATIONS`.
- Added `internal/game/contentdb` config parsing, pgx `database/sql` opener,
  migration runner, embedded `0001_schema_migrations.sql`, and small store
  boundary.
- Wired server/runtime config validation only. `NewRuntime` still loads
  `content.StaticRepository`; no runtime catalog source changed in Phase 01.

Verified:

```bash
docker compose config
go test ./internal/game/contentdb -count=1
go test ./internal/game/server -run 'Config|ContentDB' -count=1
git diff --check
```
