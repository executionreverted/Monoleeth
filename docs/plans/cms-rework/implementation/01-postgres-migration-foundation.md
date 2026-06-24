# Postgres Migration Foundation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Dockerized Postgres, DB config, pgx stdlib connection, and custom migration runner.

**Architecture:** `internal/game/contentdb` owns DB config, connection, migration state. Gameplay packages do not import pgx or SQL directly.

**Tech Stack:** Docker Compose, Postgres, Go `database/sql`, `github.com/jackc/pgx/v5/stdlib`, embedded SQL migrations.

---

### Task 1: Add Docker Compose Postgres

**Files:**
- Create: `docker-compose.yml`
- Modify: `.env.example`

**Steps:**
1. Add `postgres` service with image, env vars, port `5432:5432`, named volume, `pg_isready` healthcheck.
2. Add `.env.example` vars:
   - `POSTGRES_DB=gameproject`
   - `POSTGRES_USER=gameproject`
   - `POSTGRES_PASSWORD=gameproject_dev_password`
   - `GAME_CONTENT_DATABASE_URL=postgres://gameproject:gameproject_dev_password@localhost:5432/gameproject?sslmode=disable`
   - `GAME_CONTENT_MODE=required`
   - `GAME_CONTENT_MIGRATIONS=auto`
3. Run `docker compose config`.
4. Expected: compose renders valid config.

### Task 2: Add pgx Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Steps:**
1. Run:
   ```bash
   go get github.com/jackc/pgx/v5
   ```
2. Expected: pgx v5 deps added.
3. Run:
   ```bash
   go test ./internal/game/catalog -count=1
   ```

### Task 3: Add Content DB Config

**Files:**
- Create: `internal/game/contentdb/config.go`
- Test: `internal/game/contentdb/config_test.go`

**Steps:**
1. Test env parsing:
   - empty URL -> disabled config
   - URL present -> enabled config
   - migrations mode from env; local example uses `auto`
   - content mode valid values `required|dev_fallback|off`
   - valid modes `auto|verify|off`
2. Implement `Config`, `FromEnv`, `Validate`.
3. Run:
   ```bash
   go test ./internal/game/contentdb -run Config -count=1
   ```

### Task 4: Add DB Opener

**Files:**
- Create: `internal/game/contentdb/db.go`
- Test: `internal/game/contentdb/db_test.go`

**Steps:**
1. Add `Open(ctx, Config) (*sql.DB, error)` using driver name `pgx`.
2. Ping with context before returning.
3. Unit-test disabled/missing URL behavior without live DB.
4. Integration DB test must be opt-in via `GAME_CONTENT_DATABASE_URL`.

### Task 5: Add Migration Runner

**Files:**
- Create: `internal/game/contentdb/migrations.go`
- Create: `internal/game/contentdb/migrations/0001_schema_migrations.sql`
- Test: `internal/game/contentdb/migrations_test.go`

**Steps:**
1. Add embedded migration list.
2. Create `schema_migrations(version text primary key, checksum text not null, applied_at timestamptz not null default now())`.
3. Runner applies pending migrations in transaction and fails on checksum mismatch.
4. Test with fake/in-memory runner abstraction where possible; live Postgres test opt-in.
5. Run:
   ```bash
   go test ./internal/game/contentdb -count=1
   git diff --check
   ```

### Task 6: Wire Server Config Only

**Files:**
- Modify: `internal/game/server/config.go`
- Modify: `internal/game/server/runtime.go`

**Steps:**
1. Parse content DB config.
2. If `GAME_CONTENT_MODE=required`, missing DB URL must return config/runtime error.
3. Do not alter runtime catalogs.
4. Add boot log/error path only.

### Status

Implemented:

- Dockerized local Postgres service and `.env.example` content DB vars.
- `contentdb.Config`, `Open`, migration runner, embedded
  `schema_migrations` bootstrap, and `Store` wrapper.
- `server.Config` and `RuntimeConfig` carry `ContentDB`.
- `NewRuntime` validates required content DB config early.
- Runtime content still uses `content.StaticRepository`; DB loading waits for
  Phase 04.

Verified:

```bash
docker compose config
go test ./internal/game/contentdb -count=1
go test ./internal/game/server -run 'Config|ContentDB' -count=1
git diff --check
```

### Commit

```bash
git add docker-compose.yml .env.example go.mod go.sum internal/game/contentdb internal/game/server/config.go internal/game/server/runtime.go
git commit -m "infra: add content postgres foundation"
```
