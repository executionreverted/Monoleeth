# Phase 03 — Realtime Hardening

## Status
- State: In progress
- Wave: 1
- Depends on: none
- Unlocks: scale for all later phases

## Goal
Stop slow clients from stalling the simulation, add bounded event replay on
reconnect, and make the request cache key safe against replay mismatch.

## Why (report refs)
- Code review §4 (Critical): synchronous WS writes block the tick loop.
- Code review §3: request cache keys only by session+request_id; no durable replay.

## Scope
- Per-session async writer goroutine + bounded buffer + drop/disconnect policy.
- Bounded per-session event ring for reconnect replay using existing `seq`.
- Request cache key includes op + payload hash + protocol version.

## Out Of Scope
- Binary protocol (later).

## Tasks
- [x] `[P:wave1/lane-B]` Add per-session outbound write queue in `transport.go`; tick path enqueues, never blocks.
- [x] `[P:wave1/lane-B]` Add bounded buffer policy: slow client gets disconnected, not the whole loop.
- [ ] `[P:wave1/lane-B]` Add bounded per-session event ring keyed by `seq`; replay missed events on reconnect before latest snapshot.
- [x] `[P:wave1/lane-C]` Extend request cache key with op + payload hash + version; mismatch returns `ERR_REQUEST_REPLAY_MISMATCH`.

## Server Ownership
- Events still publish after commit; writer queue is delivery only.

## Smoke Tests (one assertion each)
- [x] A blocked/slow socket does not delay another session's tick events.
- [x] Overflowing client buffer disconnects only that session.
- [ ] Reconnect with last `seq` replays missed events in order.
- [x] Same request_id + different op returns replay-mismatch error, not stale cached payload.

## Done Criteria
- [x] Tick/event loop never blocks on a single client write.
- [ ] Reconnect recovers missed bounded events deterministically.

## Verification
```bash
go test ./internal/game/realtime/... ./internal/game/server/... -run 'Transport|Reconnect|RequestCache|WriterQueue' -count=1 -race
go test ./... && git diff --check
```
