# Phase 11 - Quest Board And Reward Content

## Goal

Move quest templates, objective definitions, reward tables, reroll/cooldown
knobs, and quest display metadata to DB-backed CMS.

Quest progress remains player state.

## Content Types

- quest templates
- objective schemas
- reward tables
- board generation weights
- reroll cost/cooldown policy
- rank/role requirements
- display metadata and art keys

## Editable Fields

Quest template:

```text
quest_id
display_name
description
category
objective_schema
reward_table_id
required_rank
required_roles
board_weight
reroll_group
enabled
```

Reward table:

```text
reward_table_id
credits
xp
items[item_id, quantity]
ship_unlocks
premium grant refs only if explicitly safe
```

## Validation

- quest IDs unique
- objective schema valid
- kill target NPC type exists in CMS NPC templates
- collect/deliver/craft item refs exist
- craft recipe refs exist
- build objective production/building refs exist
- reward item/ship refs exist
- reward amount positive
- rare/premium/X Core rewards gated by durable cap policy before enabled
- board weights finite and non-negative

## Runtime Touch Points

```text
internal/game/quests/catalog.go
internal/game/quests/board.go
internal/game/quests/service.go
internal/game/server/runtime.go
internal/game/server/quest_admin_observability_handlers.go
```

## Versioning Rule

Accepted player quests keep source quest definition/version and generated
payload. Publish affects new board generation only.

Reward claim uses accepted quest payload/source, not current draft.

## Tests

- DB quest catalog controls generated board offers
- quest objective refs validate against NPC/item/recipe/production CMS content
- accepted quest from old version can progress/claim after publish or publish
  blocks incompatible changes
- duplicate reward claim remains idempotent
- client quest payload uses safe display metadata only

## Done

- runtime no longer calls `quests.MustMVPQuestCatalog()` in real CMS mode
- old quest catalog is seed/fallback/test fixture only
- quest content participates in publish validation and rollback
