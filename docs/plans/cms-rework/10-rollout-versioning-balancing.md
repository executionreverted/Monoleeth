# Phase 10 - Rollout Versioning And Balancing Hardening

## Goal

Make CMS safe for balancing workflow and future hot reload.

MVP remains restart-based publish. This phase documents and tests rollout
guards.

## MVP Runtime Versioning

```text
publish content version
restart server
server loads current published version
new gameplay uses new catalog
```

Long-lived state keeps source refs:

```text
definition_id
content_version
```

Existing code already has `catalog.VersionedDefinition`. CMS must use it
consistently.

## Durable State Version Matrix

| Domain | Stored ref today | CMS rule |
| --- | --- | --- |
| Inventory/cargo item | item source + item id | old item version remains resolvable for weight/display |
| Loadout module | item id/current module lookup | publish must define whether equipped modules keep old stats or use new stats |
| Craft job | recipe source | old recipe version must complete or publish blocks |
| Planet building | production source | settlement must use stored version or publish blocks/migrates |
| Loot drop | concrete item/quantity | existing drops unchanged; new rolls use current table |
| Shop product | product id at command time | purchase uses current published product |
| NPC combat | live entity stats | existing live NPCs finish with spawned stats; new spawns use current version |

## Later Hot Reload Design

Future only:

```text
admin publish
runtime validates new snapshot
atomically swaps catalog pointer
new spawn/loot/craft uses new version
active combat/craft/route can finish with old snapshot
reconcile snapshots include content_version
```

No hot reload until restart-based flow is stable.

Publish gate must check active durable state before switching current version if
old snapshots are not retained.

## Balancing Workflow

Each publish requires:

```text
notes
balance_tag
actor
validation report
diff from previous published
rollback target
```

Useful tags:

```text
starter_balance
pvp_1_3_loot
crafting_alpha
shop_prices_alpha
npc_risk_low
production_alpha
```

## Observability

Metrics/logs should include content version for:

- server boot
- catalog validation failure
- combat damage calc
- loot roll
- craft start/complete
- production settlement
- shop purchase
- admin publish/rollback

No logs may contain secrets or hidden procedural seeds.

## Rollout Gates

Before public test:

- DB backup exists
- migration rollback plan documented
- current published snapshot export exists
- seed can recreate MVP from empty DB
- rollback creates new published version
- safe projection leak tests pass
- full verification passes

## Tests

- published version ID appears in runtime/server snapshot where safe
- LC1 stat change affects combat/module calc after restart
- loot table change affects new drop rolls after restart
- recipe change affects new craft starts after restart
- rollback restores previous values after restart
- invalid content never boots runtime
- old craft job/building version policy is enforced by completion, block, or
  migration test

## Done

- balancing publish/rollback loop proven
- restart-based runtime version switch documented
- hot reload deferred with clear design
- release gate can fail on CMS validation or projection leak
