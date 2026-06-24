# Phase 07 - Crafting Production And Building Content

## Goal

Move craft recipes and production/building definitions to DB-backed CMS.

No player craft job or planet state migration here. Only definitions move.

## Content Types

- craft recipes
- recipe input/output refs
- role/rank/location requirements
- craft duration/fees
- production building definitions
- building inputs/outputs/rates
- energy cost
- building cost/upgrade refs if currently hard-coded

## Editable Fields

Craft recipe:

```text
recipe_id
category
output kind/item/ship unlock
output quantity
inputs[item_id, quantity]
required_credits
required_rank
required_role_levels
required_location_type
required_building_type/level optional
craft_duration_ms
repeatable
enabled
```

Admin JSON must use CMS DTO names. Example: recipe duration is
`craft_duration_ms`, not raw Go `time.Duration` field names from
`crafting.RecipeDefinition`.

Admin edit rollout for recipes is staged:

- first safe numeric edit: `required_rank`
- optional later: existing `required_role_levels[].level` without add/remove or
  reorder
- defer `required_credits` until cancel/refund uses the job's stored recipe
  value, not the current catalog value
- defer input/output refs and quantities until active jobs complete from stored
  recipe snapshots or publish blocks incompatible changes
- defer `craft_duration_ms`, `enabled`, `recipe_id`, output kind, and
  location/building gates until active-job version policy is implemented

Production building:

```text
definition_id
building_type
category
level
inputs[item_id, amount_per_hour]
outputs[item_id, amount_per_hour]
energy_cost_per_hour
build_costs optional
upgrade_costs optional
enabled
```

## Validation

- recipe IDs unique
- recipe input/output item IDs exist
- output quantity positive
- input quantities positive
- duration positive
- required credits non-negative
- rank positive
- role requirements known
- location/building requirement known
- production input/output item IDs exist
- amount per hour positive
- production energy cost positive where current production validator requires it
- one production definition per building type + level

## Runtime Touch Points

```text
internal/game/crafting/catalog.go
internal/game/crafting/service.go
internal/game/production/catalog.go
internal/game/production/building_mutation.go
internal/game/production/settlement.go
internal/game/server/runtime.go
internal/game/server/planet_building_handlers.go
```

## Versioning Rule

Existing craft jobs must keep `RecipeSource`.

MVP behavior:

- new craft jobs use current published recipe
- existing jobs must either complete using their stored recipe version or
  publish must be blocked while changed recipes have active jobs
- implementation must not let `CompleteCraft` reject old jobs only because
  current catalog changed
- no hot reload in this phase
- publish must validate that fields affecting reservation, debit/refund,
  completion mint, duration, output kind, or location gate are compatible with
  active jobs before those fields become admin-editable

Later:

- active jobs can keep old snapshot object until completion

Production state has same rule:

- existing planet buildings keep source definition/version
- settlement/building transactions must use injected catalog or historical
  resolver
- no runtime path may call `production.MVPCatalog()` after CMS production phase

## Tests

- DB recipe controls `CraftingService.StartCraft`
- invalid missing input item rejected at publish/assembly
- recipe version written to craft job source
- production output rate from DB affects settlement
- old craft job completes or publish blocks before recipe version switch
- existing building settlement uses CMS/historical production catalog, not MVP
  helper
- duplicate building type/level rejected

## Done

- crafting and production definitions load from CMS snapshot
- old MVP definitions seed DB only
- changed recipe/production published content affects new runtime after restart
