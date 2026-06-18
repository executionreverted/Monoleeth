# Quest Board And Quest Generation

Date: 2026-06-17

## Purpose

Bu modül oyuncuya yön veren notice/contract board sistemini yönetir.

Kural:

```text
Board shows 10 available quests.
Player can accept 3 active quests.
Player can reroll board for credits.
```

Quest type'lar board type yerine kullanılır.

## Owns

```text
QuestService
QuestGenerationService
QuestRewardService
```

## Does Not Own

- Combat kill validation
- Scan validation
- Craft completion
- Inventory primitive
- Wallet primitive
- XP primitive

Quest sistemi event tüketir ve progress günceller.

## Quest Types

```text
kill
scan
discover_planet
collect
deliver
craft
build
loot
travel
clear_anomaly
route_transfer
market_sell
```

MVP:

- kill
- scan
- collect
- craft
- build
- deliver

## Quest Template

```text
template_id
quest_type
title_key
description_key
difficulty_rules_json
objective_schema_json
reward_rules_json
expiration_rules_json
requirements_json
```

Generated quest:

```text
player_quest_id
player_id
template_id
generated_payload_json
state
progress_json
accepted_at
expires_at
completed_at
claimed_at
```

## Board Offer Data

```text
offer_id
player_id
template_id
generated_payload_json
created_at
expires_at
```

Offers are not active quests until accepted.

## Generate Board

Inputs:

- player rank
- main level
- role levels
- known planets
- recent activity
- current region
- daily seed

Pseudo:

```go
func GenerateBoard(player PlayerSnapshot, seed int64) []QuestOffer {
	pool := questCatalog.Filter(func(t Template) bool {
		return t.MinRank <= player.Rank && player.Rank <= t.MaxRank
	})
	weighted := WeightByPlayerNeeds(pool, player)
	return PickN(weighted, 10, seed)
}
```

Weight examples:

- low scout level -> more scan quests
- owns planet -> production/build quests
- no planet yet -> discovery quests
- combat-heavy player -> some non-combat nudges

## Accept Quest

Validation:

```text
offer exists
offer not expired
active quest count < 3
requirements still met
```

Transaction:

```text
lock active quest count
insert player_quest state accepted
remove offer or mark accepted
emit quest.accepted
```

## Progress From Events

QuestService consumes events:

```text
combat.npc_killed
scan.planet_discovered
loot.picked_up
craft.job_completed
building.completed
route.transfer_settled
market.sale_completed
movement.region_entered
```

Phase 07 implementation routes internal `events.EventEnvelope` values through
`QuestService.ConsumeDomainEvent` before calling objective-specific consumers.
The combat kill payload must include the server-owned killer/player and NPC
type, the loot pickup payload must include the claiming player, item id, and
quantity, and the craft completion payload must include the player, recipe or
output item id, and quantity. The client never submits these progress payloads.
Quest progress idempotency uses stable domain keys derived from the payload
identity, not only the envelope id:

```text
combat.npc_killed:<npc_entity_id>
loot.picked_up:<drop_id>
craft.job_completed:<job_id>
```

This lets at-least-once delivery retries no-op for the same domain event
without letting service-local envelope sequence ids suppress unrelated progress.

Example progress:

```go
func OnNPCKilled(event NPCKilled) {
	quests := repo.ActiveQuests(event.PlayerID, "kill")
	for _, q := range quests {
		if q.Objective.TargetNPCType == event.NPCType {
			q.Progress.Count++
			if q.Progress.Count >= q.Objective.RequiredCount {
				q.State = "completed"
			}
			repo.Save(q)
		}
	}
}
```

## Claim Reward

Claim validation:

```text
quest state completed
not already claimed
player owns quest
reward still valid
```

Player-facing claim errors must use a generic public message. Quest ids,
generated payload details, hidden targets, item ids, and internal service
diagnostics stay in the wrapped cause/server logs, not in realtime error
messages.

Reward transaction:

```text
lock quest
mark claimed
grant credits/items/xp via services
emit quest.reward_claimed
```

The quest XP grant uses source type `quest`, source id `<player_quest_id>`,
authority `quest_service`, and idempotency key
`quest_reward:<player_quest_id>`. Progression rank checks may use those
server-authorized quest XP records as completed quest milestone evidence only
when the idempotency key exactly matches `quest_reward:<source_id>`.

Idempotency:

```text
unique reward reference = quest_reward:<player_quest_id>
```

## Rewards

Reward types:

- credits
- main XP
- role XP
- raw materials
- processed materials
- X Core Fragment
- X Core, rare
- premium currency, rare
- recipe unlock
- blueprint
- title/badge
- coordinate intel packet

Rare reward chance should be server-side and generated at quest offer time or claim time.

Recommendation:

```text
Generate reward payload at offer generation time.
```

This avoids reroll/claim manipulation.

## Reroll

Reroll board:

```text
cost credits
cannot reroll accepted quests
new 10 offers generated
rare reward probability controlled
daily free reroll optional
```

Reroll flow:

```text
debit credits
expire old unaccepted offers
generate new offers
emit quest.board_rerolled
```

## Expiration

Offers expire.

Active quests may expire depending type.

MVP:

- offers expire daily
- accepted quests do not expire or expire weekly

## Events Emitted

```text
quest.board_generated
quest.board_rerolled
quest.accepted
quest.progressed
quest.completed
quest.abandoned
quest.reward_claimed
```

## Edge Cases

- Event arrives after quest completed; progress should not overflow badly.
- Reward claim retried.
- Board reroll while accept request concurrent.
- Quest requirement changes after accepted.
- Quest target planet gets colonized by another player.
- Delivery quest destination storage full.
- Market sell quest can be abused by self-trading.

## Abuse Vectors

### Reward Duplicate

Risk:

- Claim reward twice by retry.

Defense:

- quest state lock
- claimed_at
- ledger reference unique

### Reroll Rare Farming

Risk:

- Player rerolls until X Core/premium appears.

Defense:

- reroll cost scaling
- rare reward daily/weekly caps
- offer generation quota

Phase 07 implementation:

- Board generation and reroll generation can receive a server-side rare reward
  cap hook.
- Candidates rejected by the hook are skipped before offers are stored.
- Reroll generation runs the cap hook before wallet debit, so a blocked board
  does not charge credits or replace the current board.

### Fake Progress

Risk:

- Client sends "I killed 100 pirates".

Defense:

- progress only from server events
- client never increments quest

### Market Quest Wash Trade

Risk:

- Alt accounts sell/buy same item for quest progress.

Defense:

- avoid market quests MVP
- minimum economic value
- unique counterparty/recent relation checks

## Testing Checklist

- Board generates 10 offers.
- Accept max 3 active quests.
- Reroll charges credits.
- Kill event progresses only matching quest.
- Completed quest cannot progress further incorrectly.
- Reward claim exactly once.
- Rare reward cap respected.
- Expired offer cannot be accepted.

## Implementation Notes

MVP:

- deterministic generated payload
- 10 offers
- 3 active
- reroll for credits
- rewards generated upfront
- progress from server events only
