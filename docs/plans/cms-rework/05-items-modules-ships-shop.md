# Phase 05 - Items Modules Ships And Shop Content

## Goal

Move economy-facing core definitions to DB-backed CMS.

This phase gives LC1-style balancing path: admin changes equipment values in DB,
published snapshot changes runtime stats after restart.

## Content Types

- item definitions
- module/equipment definitions
- ship definitions
- shop categories/products

## Editable Fields

Items:

```text
display_name
description
category/subcategory
art_key
item_type
rarity
max_stack
weight
trade_flags
bind_rules
metadata_schema
```

Modules/equipment:

```text
slot_type
category
tier
rarity
required_rank
required_role_levels
stat_modifiers
energy_profile
cooldowns
durability
compatible_slots/categories
trade_flags
bind_rules
```

Ships:

```text
display_name
tier
rank_requirement
slot_layout
base_stats
cargo_capacity
credit_price
premium_price
auction_buy_now_price
craft_recipe_id
role_tag
repair_cost_multiplier_bps
passive_bonus_id
acquisition_blocker
```

If current gameplay code has a ship field that is not CMS-editable in this
phase, mark it explicitly server-owned/deferred. Do not drop it silently during
assembly.

Shop:

```text
product_id
product_type
display metadata
grant_target
price_policy
stock_policy
availability
```

## LC1 Example

Canonical future ID:

```text
item.laser.lc1
```

Current-compatible row may map to:

```text
laser_alpha_t1
```

Editable stats:

```text
attack_damage: 8
shield_damage: 5
range: 650
cooldown_ms: 1200
energy_cost: 4
rank_requirement: 1
rarity: common
```

Assembly maps these into `modules.StatModifier`, `modules.EnergyProfile`, and
`modules.Cooldown`.

## Validation

- item IDs unique
- module item ID exists as item definition
- ship IDs unique
- shop grant target exists
- display name is not raw snake_case ID unless allowlisted
- rank/tier positive
- cooldown positive
- active energy cost positive unless action is explicitly passive/upkeep-only
- passive upkeep energy cost non-negative
- stat values finite and within MVP bounds
- duplicate stat/cooldown keys rejected unless rule says stacking is allowed
- trade/bind flags known
- item `Source` version remains resolvable for stored inventory/cargo rows

## Runtime Touch Points

```text
internal/game/economy
internal/game/modules
internal/game/ships
internal/game/catalog
internal/game/server/content_registry.go
internal/game/server/economy_seed.go
internal/game/server/runtime.go
```

## Tests

Keep tests small:

- item row assembles into `economy.ItemDefinition`
- LC1 damage/range/cooldown from DB assembles into module catalog
- shop product grant target resolves
- inventory/loadout uses DB item definition
- old inventory/cargo item version resolves weight/display after publish
- equipped module behavior after stat/slot change follows documented version
  policy
- safe shop payload has no raw hidden data

## Done

- modules/ships/shop normal runtime source is CMS snapshot
- old hard-coded rows only seed default content
- changing DB-published LC1 stats changes runtime module definition after
  restart
