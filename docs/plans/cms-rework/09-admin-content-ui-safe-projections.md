# Phase 09 - Admin Content UI And Safe Projections

## Goal

Expose CMS controls in browser admin UI without leaking hidden gameplay content
to non-admin players.

## UI Sections

MVP admin slice:

- Content Versions
- Equipment/Modules
- Diff View
- Publish/Rollback

Later slices:

- Items
- Ships
- Shop Products
- NPC Templates
- Enemy Pools
- Loot Tables
- Craft Recipes
- Production Buildings

## UX Contract

Admin UI edits draft rows. Normal player UI only sees safe published
projections.

No fake gameplay values. Empty/missing content renders locked/loading/error
states.

## LC1 Edit Surface

LC1 form fields:

```text
display name
rarity
rank requirement
attack damage
shield damage
range
cooldown ms
energy cost
trade flags
shop price
enabled
```

Save draft does not affect gameplay until publish and server restart for MVP.

## Admin Payload Gate

Current client/server payload guards reject trusted gameplay fields globally.
CMS admin payloads need operation-aware parsing:

- `admin.content.*` may send typed content patches with damage/rank/cooldown
  fields after admin auth
- normal gameplay commands still reject trusted damage/player/server fields
- admin responses may include draft/server-only content only inside admin-only
  response channels
- normal player stream remains strict

## Diff View

Diff compares:

```text
current published snapshot
draft row/snapshot
previous published version
rollback target
```

Show field paths and old/new values. Do not show hidden data to non-admin.

## Safe Player Projection

Player-visible catalog payload may include:

```text
content_version
display_name
description
category
subcategory
art_key
rarity
tier
visible stats
shop price/availability
```

Player payload must not include:

```text
loot chance
drop weight
enemy pool caps
spawn timers
NPC aggro/leash internals
future spawn config
hidden map ids
procedural seeds
audit data
admin notes
```

Implement player projections as allowlist DTOs, not "remove forbidden fields"
filters. Leak tests must use sentinel values as well as field names.

## Client Touch Points

```text
client/src/state/
client/src/ui/
client/src/net/
internal/game/server/quest_admin_observability_handlers.go
internal/game/server/handlers.go
```

Exact files can change with current UI layout.

## Tests

- non-admin cannot open/fetch CMS admin payloads
- admin list renders versions/content rows
- save draft shows validation errors
- publish action returns content version
- player catalog payload omits hidden loot/spawn fields by allowlist and
  sentinel-value leak tests
- client parser accepts admin CMS payload only for admin op and keeps player
  payload guard strict
- no raw IDs used as visible labels where display metadata exists

Browser smoke:

```text
admin versions list
admin LC1 draft edit form
player shop/inventory uses safe projection
```

## Done

- admin can inspect/edit/publish content from UI
- normal player sees only safe published catalog metadata
- validation errors visible and actionable
