# Phase 10 — Social MVP (Chat, Party, Clan)

## Status
- State: Done (chat/party/clan runtime + durable clan + real client panels + moderation redaction/logging + contribution read models)
- Wave: 4
- Depends on: P01 (durable identity), P04 (rate limits)
- Unlocks: P11 group content, retention

## Goal
Add the minimum MMO social layer: server-authoritative chat, party/group, and a
clan foundation, all with moderation and rate limits from day one.

## Why (report refs)
- Feature-gap §5, §7 P0/P2: chat/clan/party are major missing MMO layers.

## Scope
- Chat channels: system, local-map, party, clan.
- Party invite/accept/leave + shared target/contribution hook.
- Clan create/join/leave/ranks/tag + clan chat.

## Out Of Scope
- Clan outposts/territory war (post-v1), alliances (post-v1).

## Tasks
- [x] Domain package foundation: `internal/game/social/` chat, party, clan,
  channel membership, rate-limit/moderation seams, in-memory stores, unit tests.
- [x] `[P:wave4/lane-A]` Add `chat.send` + channel resolution server-side; enforce `chat.send` rate limit.
- [x] `[P:wave4/lane-A]` Add chat moderation hooks + redaction/logging policy (no PII leaks). Runtime defaults to PII/secret redaction before store/fanout and records only keyed HMAC fingerprints plus length/action metadata in moderation logs.
- [x] `[P:wave4/lane-B]` Add `party.invite/accept/leave` with server-owned membership.
- [x] `[P:wave4/lane-B]` Add party shared-target realtime foundation.
- [x] `[P:wave4/lane-B]` Add contribution event foundation.
- [x] `[P:wave4/lane-C]` Add `clan.create/join/leave` + ranks + tag (durable rows).
- [x] `[P:wave4/lane-C]` Add clan chat channel bound to clan membership.
- [x] `[P:wave4/lane-A]` Client: chat panel + party panel + clan panel (real state only).

## Server Ownership
- Channel membership, party/clan membership, ranks are server-owned; client sends intent only.
- Runtime now owns `chat.send`, `party.invite`, `party.accept`, `party.leave`,
  `party.target.set`, `clan.create`, `clan.join`, and `clan.leave`; clients
  send channel kind/content, invitee callsign, selected target id, or clan
  name/tag intent, never trusted player/session ids.
- Core-store DB mode uses Postgres-backed clan/membership rows and emits
  per-recipient clan read models on join/leave/bootstrap so clients do not
  infer membership or rank from another player's snapshot.
- Runtime records NPC-kill party/clan contribution snapshots from
  server-owned combat contribution maps, uses opaque occurrence ids instead of
  NPC entity ids in social payloads, and broadcasts read models only to
  current party/clan members.

## Smoke Tests (one assertion each)
- [x] Local-map chat reaches same-map members and not others.
- [x] Chat over rate limit is throttled without mutation.
- [x] Runtime moderation rejection blocks chat without queued mutation.
- [x] Default chat moderation redacts PII/secrets before storage and logs no raw content or dictionary-checkable raw hashes.
- [x] Party/clan contribution read models publish server-owned contribution totals with opaque occurrence ids.
- [x] Respawned NPC entity ids do not collapse later contribution events.
- [x] Realtime `party.invite/accept` adds exactly one membership using server-owned identity.
- [x] Realtime `party.leave` publishes a leaver event after mutation.
- [x] Realtime `party.target.set` rejects hidden targets and publishes a shared-target update after mutation.
- [x] Realtime `clan.create/join/leave` publishes durable per-recipient clan snapshots.
- [x] Runtime restart reloads durable clan rows and bootstraps a `clan.updated` read model.
- [x] Client social panel parses social responses/events with server-owned ids and sends intent-only chat/party/clan commands.
- [x] Party invite/accept adds exactly one membership.
- [x] Non-member cannot read/send clan chat.
- [x] Clan create assigns the creator the owner rank once.
- [x] Leaving a clan removes membership and clan-chat access.

## Done Criteria
- [x] Chat + party + clan MVP usable with moderation/rate limits.

## Verification
```bash
go test ./internal/game/social ./internal/game/server -run 'Chat|Party|Clan|Contribution' -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
