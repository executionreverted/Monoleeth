# API, Realtime Events, Error Model, And Contracts

Date: 2026-06-17

## Purpose

Bu dosya client-server command/query/event biçimini standartlaştırır.

Hedef:

```text
Her modül aynı contract disiplinini kullansın.
Client intent göndersin.
Server authoritative result dönsün.
Realtime event'ler commit sonrası aksın.
```

## Owns

```text
ApiGateway
RealtimeProtocol
EventBusContracts
ErrorModel
```

## Protocol Style

MVP:

```text
WebSocket + JSON
```

Production:

```text
WebSocket + MessagePack or Protobuf
```

JSON MVP debug için iyi. Contract disiplini baştan doğru olmalı.

## Message Envelope

Client request:

```json
{
  "request_id": "uuid",
  "op": "combat.use_skill",
  "payload": {},
  "client_seq": 1234
}
```

Server response:

```json
{
  "request_id": "uuid",
  "ok": true,
  "payload": {},
  "server_time": 182736123
}
```

Error:

```json
{
  "request_id": "uuid",
  "ok": false,
  "error": {
    "code": "ERR_OUT_OF_RANGE",
    "message": "Target is out of range.",
    "retryable": false
  },
  "server_time": 182736123
}
```

## Event Envelope

Realtime event:

```json
{
  "event_id": "uuid",
  "type": "loot.created",
  "payload": {},
  "server_time": 182736123,
  "seq": 99122
}
```

AOI events:

- include visible entity only
- no hidden metadata

## Request Rules

All requests:

- request_id required
- op required
- payload schema validated
- authenticated session required
- player/world/session resolved server-side
- rate limit by op

Never trust:

- player_id from client payload
- world_id from client payload unless cross-checked
- position if movement model server-authoritative
- damage
- price total
- XP
- reward
- cooldown
- timestamp

## Idempotency

Use `request_id` for network retry safety, but economy operations also need domain reference keys.

Example:

```text
quest_reward:<player_quest_id>
market_buy:<listing_id>:<buyer_id>:<request_id>
craft_complete:<job_id>
```

## Command vs Query

Command mutates state:

```text
combat.use_skill
loot.pickup
market.buy
craft.start
quest.accept
```

Query reads state:

```text
player.snapshot
market.search
hangar.list
planet.production_summary
```

Commands go through validation + transaction.

Queries can use cache but must respect visibility/security.

## Error Codes

Common:

```text
ERR_UNAUTHENTICATED
ERR_FORBIDDEN
ERR_NOT_FOUND
ERR_INVALID_PAYLOAD
ERR_RATE_LIMITED
ERR_INTERNAL
```

Gameplay:

```text
ERR_OUT_OF_RANGE
ERR_NOT_VISIBLE
ERR_COOLDOWN
ERR_NOT_ENOUGH_ENERGY
ERR_NOT_ENOUGH_CARGO
ERR_NOT_ENOUGH_FUNDS
ERR_RANK_TOO_LOW
ERR_ITEM_NOT_TRADEABLE
ERR_SHIP_DISABLED
ERR_STORAGE_FULL
```

Error messages to client should not leak hidden truth.

Bad:

```text
"Planet level 9 hidden at x,y requires radar 4"
```

Good:

```text
"No valid signal found."
```

## Event Bus

Internal events:

```text
combat.npc_killed
loot.picked_up
quest.reward_claimed
market.sale_completed
planet.production_settled
```

Internal event can be more detailed than client event.

Client event must be filtered.

## Commit Then Publish

Rule:

```text
DB transaction commits before realtime broadcast.
```

If broadcast fails:

- state remains correct
- client reconciles via snapshot

For critical events, store outbox:

```text
event_outbox
- event_id
- type
- payload
- status
- created_at
```

## Snapshot/Reconciliation

Client receives:

- periodic visible world snapshot
- player stat/wallet/cargo snapshots after mutations
- correction messages after rejected actions

Movement/combat prediction can exist visually, but server state wins.

## Rate Limits

Per op:

```text
combat.use_skill: based on cooldown + small command burst limit
loot.pickup: small burst
market.search: stricter
chat/send mail: strict
quest.reroll: cost + rate
scan.pulse: server-timed, not client-spammed
```

## Versioning

Envelope can include protocol version:

```json
{"v": 1, "op": "..."}
```

Breaking changes:

- bump version
- support old client for short window or force update

## Events Emitted To Client

Examples:

```text
player.snapshot
ship.changed
stats.updated
combat.damage
loot.created
loot.removed
quest.updated
market.sale_completed
planet.production_summary
route.updated
fog.updated
```

## Edge Cases

- Client reconnects after command accepted but response lost.
- Request arrives twice.
- Server processed DB commit but event broadcast lost.
- Client has old protocol version.
- Payload valid JSON but semantically invalid.
- Query asks for hidden object.

## Abuse Vectors

### Operation Flood

Risk:

- Client spams cheap requests.

Defense:

- per-op rate limit
- session backpressure
- disconnect abusive sessions

### Error Oracle

Risk:

- Player probes hidden entity ids and reads error differences.

Defense:

- same generic error for hidden/not found where needed
- no hidden metadata in errors

### Replay Attack

Risk:

- Old request replayed.

Defense:

- request id cache
- domain idempotency keys
- state validation

### Client Version Exploit

Risk:

- Old client bypasses new validation path.

Defense:

- validation server-side shared
- protocol version gate

## Testing Checklist

- Invalid payload rejected.
- Hidden query returns generic not found/not visible.
- Duplicate request id idempotent.
- DB commit before event publish.
- Lost response can be reconciled by snapshot.
- Rate limits trigger.
- Error messages do not leak hidden data.
- Client player_id ignored.

## Implementation Notes

MVP:

- JSON envelope
- op registry
- schema validation
- request id cache
- common error codes
- commit-then-broadcast
- periodic snapshot

Current Phase 04 implementation note:

- `internal/game/realtime.Gateway` is transport-agnostic request handling. It
  decodes JSON envelopes, resolves authenticated session/player/world/zone
  identity through a server-side resolver, executes handlers with that resolved
  context, and caches responses by session/request id.
- Client payload identity fields such as `player_id` are not trusted by the
  gateway boundary.
