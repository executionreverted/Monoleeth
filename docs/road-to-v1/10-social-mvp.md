# Phase 10 — Social MVP (Chat, Party, Clan)

## Status
- State: In progress (domain package done; runtime/client wiring pending)
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
- [ ] `[P:wave4/lane-A]` Add `chat.send` + channel resolution server-side; enforce `chat.send` rate limit.
- [ ] `[P:wave4/lane-A]` Add chat moderation hooks + redaction/logging policy (no PII leaks).
- [ ] `[P:wave4/lane-B]` Add `party.invite/accept/leave` with server-owned membership.
- [ ] `[P:wave4/lane-B]` Add party shared-target + contribution event foundation.
- [ ] `[P:wave4/lane-C]` Add `clan.create/join/leave` + ranks + tag (durable rows).
- [ ] `[P:wave4/lane-C]` Add clan chat channel bound to clan membership.
- [ ] `[P:wave4/lane-A]` Client: chat panel + party panel + clan panel (real state only).

## Server Ownership
- Channel membership, party/clan membership, ranks are server-owned; client sends intent only.

## Smoke Tests (one assertion each)
- [x] Local-map chat reaches same-map members and not others.
- [x] Chat over rate limit is throttled without mutation.
- [x] Party invite/accept adds exactly one membership.
- [x] Non-member cannot read/send clan chat.
- [x] Clan create assigns the creator the owner rank once.
- [x] Leaving a clan removes membership and clan-chat access.

## Done Criteria
- [ ] Chat + party + clan MVP usable with moderation/rate limits.

## Verification
```bash
go test ./internal/game/social/... ./internal/game/server/... -run 'Chat|Party|Clan' -count=1
go test ./... && cd client && npm --cache /tmp/gameproject-npm-cache run check
```
