# Testing, Observability, Balancing, And Security Checklist

Date: 2026-06-17

## Purpose

Bu dosya bütün modüller için ortak kalite, test, gözlemleme ve balans yaklaşımını tanımlar.

Bu oyun uzun ömürlü persistent ekonomi kurduğu için sadece "çalışıyor" yetmez.

Gerekli:

- correctness
- anti-duplication
- anti-abuse
- performance visibility
- economy visibility
- balancing knobs
- rollback/debug ability

## Test Layers

```text
unit tests
domain/service tests
transaction/integration tests
simulation tests
load tests
security/abuse tests
economy tests
reconciliation tests
```

## Must-Test Invariants

Economy:

```text
No item appears from nowhere except approved faucet.
No item disappears except approved sink.
No currency appears except approved faucet.
No currency disappears except approved sink.
Ledger delta equals balance change.
Escrow item cannot be used.
Duplicate request cannot duplicate value.
```

Gameplay:

```text
Hidden entity cannot be interacted with.
Out-of-range attack fails.
Cooldown cannot be skipped.
Energy cannot go negative.
Cargo cannot exceed capacity.
Storage cannot exceed capacity.
Dead ship cannot fight.
```

Progression:

```text
XP event grants once.
Rank up grants one skill point.
Skill unlock consumes point once.
Quest reward claims once.
```

## Simulation Tests

Create deterministic simulation runners:

```text
1000 players kill NPCs for 1 hour
1000 players pickup loot concurrently
1000 market buy/sell operations
1000 offline planet settlements
1000 route settlements
```

Track:

- total item faucets
- total item sinks
- total credits created
- total credits sunk
- average repair cost
- average craft cost
- market velocity
- X Core supply

## Load Test Targets

Per world target:

```text
1500-2000 online players
visible entities per player: 50-100
normal snapshot: 5-10Hz
combat tick: 20Hz sim
```

Metrics:

- tick duration p50/p95/p99
- websocket outbound bytes/player/sec
- command latency p50/p95/p99
- DB transaction latency
- Redis hit rate
- NATS/event lag
- GC pause
- CPU per zone worker
- memory per zone worker

## Observability

Metrics:

```text
active_players
active_entities
zone_tick_ms
commands_per_sec
errors_by_code
combat_actions_per_sec
loot_created_per_sec
loot_picked_per_sec
market_volume
auction_volume
craft_jobs_started
craft_jobs_completed
planet_settlements
route_settlements
wallet_delta_by_reason
item_delta_by_reason
```

Logs:

- structured JSON
- player_id
- session_id
- world_id
- zone_id
- request_id
- reference_id
- op
- error_code

Traces:

- websocket command
- DB transaction
- inventory mutation
- ledger insert
- event publish

## Economy Dashboards

Need dashboards:

- credits faucet/sink per day
- premium paid/earned traded
- X Core created/consumed
- top raw materials supply
- top processed materials supply
- market average price
- auction clearing prices
- repair cost total
- craft fee total
- route loss total
- planet production total

Balancing without dashboards is guessing.

## Balancing Knobs

Progression:

- XP table
- role XP rates
- rank requirements
- quest reward weights

Combat:

- weapon damage
- energy cost
- cooldown
- shield regen
- NPC HP/damage

Economy:

- drop rates
- craft input amounts
- craft duration
- market fee
- repair rate
- route loss chance
- planet production rate
- X Core fragment chance

World:

- planet scan chance
- planet density
- biome risk
- radar requirement
- distance scaling

## Config Strategy

Use versioned catalogs:

```text
ship_catalog_v1
module_catalog_v1
recipe_catalog_v1
quest_catalog_v1
loot_table_v1
production_catalog_v1
```

Runtime hot reload can come later.

MVP:

- load catalogs on server startup
- catalog version written to events/jobs

Important:

```text
Craft job should remember recipe version.
Auction lot should remember generated payload.
Quest should remember generated reward.
```

## Security Review Checklist

For every command ask:

1. Does client send only intent?
2. Does server load player/session from auth?
3. Are ids ownership-checked?
4. Are amounts positive and bounded?
5. Is visibility/range checked if world interaction?
6. Is transaction lock used for mutable value?
7. Is ledger written for item/currency?
8. Is request id/idempotency handled?
9. Is error message leaking hidden info?
10. Is event broadcast after commit?

## Common Bug Patterns

### Double Completion

Examples:

- craft complete twice
- quest reward twice
- route settlement twice
- auction close twice

Fix:

- state machine
- row lock
- unique reference

### Stale Cache

Examples:

- broken module still gives stat
- ship swap not invalidating stats
- market stale intel still active

Fix:

- explicit invalidation events
- versioned snapshots
- tests around invalidation

### Race Conditions

Examples:

- two pickups same drop
- buy and cancel same listing
- two repair requests
- cargo capacity concurrent add

Fix:

- lock target rows
- transaction boundaries
- retry on serialization conflict

### Hidden Information Leak

Examples:

- hidden entity serialized
- error says "target out of range" for unseen target
- client receives gameplay seed

Fix:

- server-side filter
- generic hidden errors
- no gameplay seed to client

## Abuse Test Cases

Write automated tests for:

- negative amounts
- enormous amounts
- duplicate request id
- same command with different request ids
- hidden entity interaction
- out of range pickup
- market buy/cancel race
- auction bid/buy-now race
- premium webhook replay
- offline settlement repeated
- route toggle around settlement
- skill unlock locked node

## Data Retention

User said:

```text
30 günden eski log tutmayız.
```

But some ledgers should persist longer:

- wallet ledger
- item ledger
- premium purchase ledger
- auction sale history

Recommendation:

```text
operational logs: 30 days
security/economy ledger: long-term or summarized archive
high-volume telemetry: aggregate after 30 days
```

Do not delete money/item ledger too early; support and fraud needs it.

## Rollback And Repair Tools

Admin tools needed:

- inspect player inventory
- inspect wallet ledger
- inspect item ledger
- reverse transaction by compensating entry
- disable suspicious market listing
- stale intel listing
- refund auction bid
- repair stuck craft job
- re-run offline settlement dry-run

Never edit balances silently.

Use compensating transactions:

```text
bad grant +1000
compensation -1000
```

## Release Gates

Before enabling a module in production:

- unit tests pass
- integration transaction tests pass
- abuse cases pass
- metrics exist
- admin inspection exists
- error codes mapped
- economy ledger reason added
- load test for expected throughput

## Implementation Notes

Minimum observability from day one:

- request_id in every log
- player_id/session_id in every command log
- ledger for currency/items
- metrics for command errors
- tick duration metrics
- economy faucet/sink metrics

This makes balancing and debugging possible before the game gets messy.

