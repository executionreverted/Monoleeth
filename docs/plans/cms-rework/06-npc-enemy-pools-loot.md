# Phase 06 - NPC Enemy Pools And Loot Content

## Goal

Move monster/NPC templates, enemy pools, and loot/drop tables to DB-backed CMS.

Combat remains server-authoritative. Client never receives hidden loot chances
or spawn internals.

## Content Types

- NPC stat templates
- NPC aggro/leash profiles
- NPC drop profiles
- map enemy pools
- spawn areas needed by enemy pools
- NPC event spawns
- loot tables and loot rows

## Editable Fields

NPC template:

```text
npc_type
label_key
min_level/max_level
hp_max
shield_max
energy_max
weapon_range
weapon_damage
weapon_cooldown_ms
accuracy
radar_signature
speed
xp_value
risk/rank tags
```

Enemy pool:

```text
map_id
enemy_pool_id
npc_type
level band
spawn_area_refs
map_max_alive
pool_max_alive
initial_alive
spawn_interval_ms
kill_respawn_delay_ms
spawn_jitter_ms
spawn_mode
stat_template_id
drop_profile_id
aggro_profile_id
leash_profile_id
enabled
```

Loot table:

```text
loot_table_id
roll_mode: independent_chance for MVP
rows[item_id, min_quantity, max_quantity, chance]
owner_lock/public/despawn policy optional later
```

No free-form condition expressions in MVP. Future conditions must be typed enum
rules such as `risk_band`, `npc_rank`, or `event_tag`.

## Map Boundary

Do not move full map/portal/safe-zone catalog in this phase unless required for
enemy pool assembly.

Reason: map projections already hide server-only fields. Full map CMS needs
separate safety review.

## Validation

- NPC stats finite and bounded
- cooldown/range positive
- accuracy `0..1`
- drop chance `0..1`
- loot quantity positive and min <= max
- loot item exists
- enemy pool map exists
- enemy pool references existing spawn area/template/drop/aggro/leash
- pool caps valid: `initial <= pool <= map`
- spawn intervals positive
- spawn area radius finite and inside map bounds
- safe-zone and portal exclusions match current map catalog validation
- event spawn pool/drop refs valid and event cap within pool cap
- PvP/high-risk maps cannot use starter-only loot table unless allowlisted

## Runtime Touch Points

```text
internal/game/world/maps/enemy_catalog.go
internal/game/world/maps/catalog.go
internal/game/content/bundle.go
internal/game/content/validation.go
internal/game/server/combat_loot_catalog.go
internal/game/server/npc_loot_selector.go
internal/game/loot
internal/game/combat
```

## Safe Projection

Client may see visible NPC entity state after AOI/radar filtering:

```text
entity id
position
display label
hp/shield current
target state if visible
```

Client must not see:

```text
loot table id/chance
drop weight/roll internals
spawn interval
future spawn candidates
pool caps
hidden map ids
```

## Tests

- DB loot table controls drop selector
- invalid loot chance rejected
- enemy pool missing template rejected
- spawn area outside bounds/overlapping protected safe-zone rejected
- starter drone HP/damage from DB reaches runtime NPC projection
- client-safe map/NPC projection omits server-only pool/drop fields

## Done

- NPC/pool/loot normal runtime source is CMS snapshot
- old starter enemy code only default seed source
- changed published loot table affects new loot rolls after restart
