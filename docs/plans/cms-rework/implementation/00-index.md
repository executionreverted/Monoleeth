# CMS Rework Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement DB-backed CMS in small, reviewable phases.

**Architecture:** Postgres stores draft rows and immutable published snapshots. Runtime already depends on `content.Repository`; CMS adds a DB-backed repository that loads current published content, maps it into `content.GameplayContent`, validates it, then serves only safe projections.

**Tech Stack:** Go, `database/sql`, `github.com/jackc/pgx/v5/stdlib`, Postgres, Docker Compose, custom SQL migration runner, existing realtime/admin handlers.

---

## Order

1. [Postgres/Migrations](./01-postgres-migration-foundation.md)
2. [Schema/Snapshot](./02-content-snapshot-schema.md)
3. [Seed/Bootstrap](./03-seed-source-publish-bootstrap.md)
4. [Runtime Loader](./04-runtime-loader-catalog-assembly.md)
5. [Items/Modules/Ships/Shop](./05-items-modules-ships-shop.md)
6. [NPC/Pools/Loot](./06-npc-enemy-pools-loot.md)
7. [Craft/Production/Buildings](./07-crafting-production-buildings.md)
8. [Admin API](./08-admin-publish-rollback-audit-api.md)
9. [Admin UI](./09-admin-content-ui-safe-projections.md)
10. [Rollout/Hardening](./10-rollout-versioning-balancing.md)
11. [Quest Board/Rewards](./11-quest-board-reward-content.md)

## Global Worker Prompt

```text
Use $caveman. Read docs/symphony-worker-rules.md.
Work only on assigned cms-rework phase.
Do not spawn subagents. Do not commit.
Use Context7 before Docker/Postgres/pgx/API syntax changes.
Keep files small. No monolith. No giant tests.
Run phase tests plus git diff --check.
```

## Global Verification

Final main session runs:

```bash
go test ./...
git diff --check
```

Client phases also run:

```bash
cd client
npm --cache /tmp/gameproject-npm-cache run check
```
