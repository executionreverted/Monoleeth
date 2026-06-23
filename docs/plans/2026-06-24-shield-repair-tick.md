# Shield Repair Tick Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add DarkOrbit-style out-of-combat shield repair driven by client repair intents and validated by the server.

**Architecture:** The browser sends an empty `repair.shield_tick` intent while the authenticated ship is alive and shield is below max. The Go runtime owns combat lock, elapsed time, equipped/stat-derived repair rate, shield mutation, snapshots, and events. This slice keeps death repair separate from passive shield repair and adds new focused server/client tests.

**Tech Stack:** Go realtime server, in-memory playtest runtime, TypeScript browser client, Vitest/Node focused checks.

---

### Task 1: Server Operation And Focused Tests

**Files:**
- Modify: `internal/game/realtime/envelope.go`
- Modify: `internal/game/server/handlers.go`
- Create: `internal/game/server/shield_repair.go`
- Create: `internal/game/server/shield_repair_test.go`

**Steps:**
1. Add `repair.shield_tick` to realtime operations with intent-burst posture.
2. Register `runtime.handleShieldRepairTick`.
3. Add tests proving empty payload succeeds after combat lock expires, rejects/does not mutate during lock, rejects trusted client-authored repair fields, and never repairs hull.
4. Implement combat lock tracking in runtime memory with an 8 second delay.
5. Use `state.Ship.MaxShield`, `state.Ship.Shield`, and equipped module `shield_regen` modifiers as authoritative inputs.
6. Emit `ship.snapshot` and `player.snapshot` only when shield changes.

Focused verification:

```bash
go test ./internal/game/server -run 'TestShieldRepairTick|TestCombatUseSkillRefreshesShieldRepairCombatLock|TestRealtimeOperationRegistry' -count=1
```

### Task 2: Client Intent Tick

**Files:**
- Modify: `client/src/protocol/envelope.ts`
- Modify: `client/src/protocol/commands.ts`
- Modify: `client/src/protocol/envelope.test.ts`
- Modify: `client/src/app/client-app.ts` or `client/src/app/client-app-commands.ts`
- Add/update a focused client test if the existing app/protocol test seam covers timers safely.

**Steps:**
1. Add `OPERATIONS.shieldRepairTick = 'repair.shield_tick'`.
2. Add `CommandBuilder.shieldRepairTick()` with an empty payload.
3. Add a focused protocol assertion that the request payload is empty.
4. Add a browser interval that sends the command only when connected, authenticated, ship alive, shield below max, and no same op is pending.
5. Keep all visible shield values sourced from server snapshots/events.

Focused verification:

```bash
npm --cache /tmp/gameproject-npm-cache --prefix client run test -- protocol/envelope.test.ts
```

### Task 3: Docs And Slice Verification

**Files:**
- Modify: `docs/plans/ui-implementation/05-combat-loot-death-repair.md`
- Modify: `docs/playtest-vertical-slice-status.md`
- Modify: `docs/todo.md` only if a follow-up remains.

**Steps:**
1. Document `repair.shield_tick` as shield-only out-of-combat repair, distinct from `death.repair_ship`.
2. Record focused test evidence.
3. Run `git diff --check`.
4. Commit and push the completed slice.
