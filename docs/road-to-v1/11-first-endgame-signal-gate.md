# Phase 11 — First Endgame Loop (Signal Gate)

## Status

- State: Not started
- Wave: 5
- Depends on: P06 (combat), P07 (equipment), benefits from P10 (party)
- Unlocks: repeatable endgame, material sinks

## Goal

Add one repeatable Galaxy-Gate-style endgame: collect/build a Signal Gate, enter
an instanced wave map, clear waves + a boss, earn deterministic rewards.

## Why (report refs)

- Feature-gap §7 P1, §8.1: Galaxy Gates are the highest-value DarkOrbit loop to add.

## Scope

- Gate fragments from NPC/quest/scan.
- Build one gate from parts (server-validated).
- Instanced wave map: 5 waves + 1 boss.
- Server-owned rewards via ledger/outbox.
- Some kind of timeout system if player does not finish in time that closes instance record.

## Out Of Scope

- Multiple gate types, group matchmaking depth (start solo or single-party).

## Tasks

- [ ] `[P:wave5/lane-A]` Add gate fragment items + server-owned drop/quest sources.
- [ ] `[P:wave5/lane-A]` Add `gate.build` (consume fragments once, idempotent).
- [ ] `[P:wave5/lane-B]` Add instanced wave map profile + server-owned wave spawner + boss.
- [ ] `[P:wave5/lane-B]` Add `gate.enter`/`gate.exit` with lives/repair policy server-owned.
- [ ] `[P:wave5/lane-C]` Grant deterministic rewards via wallet/inventory ledger + outbox on clear.
- [ ] `[P:wave5/lane-C]` Client: gate build UI + instance HUD state.

## Server Ownership

- Fragment counts, wave logic, boss, rewards are server-owned; entry validates ownership/cost.
- Idempotency key: `gate_clear:<instance_id>`.

## Smoke Tests (one assertion each)

- [ ] `gate.build` consumes fragments exactly once.
- [ ] Entering a built gate creates one instance for the owner/party.
- [ ] Final wave/boss clear grants rewards exactly once.
- [ ] Duplicate clear/replay does not double-grant rewards.
- [ ] Instance state is not visible to non-participants (no leak).

## Done Criteria

- [ ] Build -> enter -> clear -> reward works end-to-end with one focused browser proof.

## Verification

```bash
go test ./internal/game/gates/... ./internal/game/server/... -run 'Gate|Wave|Boss|Reward' -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
