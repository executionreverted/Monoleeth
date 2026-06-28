# Progression Economy And Content Review

Date: 2026-06-28

## Verdict

The systems can support a DarkOrbit-like loop, but current authored content and
balance are too skeletal to create long-term desire.

The current game can feel like:

```text
a polished vertical-slice spreadsheet
```

because catalogs, ledgers, stats, quests, shops, loot, market, auction, death,
and maps exist, but the actual player hunger is still thin.

## What Is Strong

### Server-Owned Economy Discipline

The project consistently treats client requests as intents. Wallet, inventory,
cargo, loadout, crafting, loot, market, and auction mutations are server-owned.

This is good. Keep it.

### Domain Breadth

The project has real foundations for:

- inventory
- cargo
- wallet/ledger
- hangar/loadout
- crafting
- progression/rank/skills
- quests
- shop
- market
- auction
- premium
- loot
- death/repair
- production/routes
- CMS/content publication

### Stat/Progression Direction

Equipment/stat aggregation and progression skills exist or are partly wired.
That is the right base for a buildcraft game.

Evidence areas:

- `internal/game/stats`
- `internal/game/progression`
- `internal/game/modules`
- `internal/game/ships`
- `internal/game/runtime/providers.go`

### Death/Danger Primitives

PvP cargo drop, death processing, repair, and protection rules exist. The game
has the primitives to make risk matter.

## Where It Fails DarkOrbit Feeling

### 1. The Upgrade Ladder Is Too Short

Current content is too narrow:

- few ships
- few modules
- few recipes
- limited loot tables
- limited enemy variety

DarkOrbit-like play needs repeated aspiration:

- first laser upgrade
- shield upgrades
- speed/generator decisions
- ammo pressure
- drone slots
- ship ladder
- better map access
- rare drops/fragments
- honor/rank/social rewards

### 2. The Equipment Chase Is Too Flat

If good items are mostly direct shop products or deterministic grants, the game
loses "one more run" energy.

Recommendation:

- starter gear can remain directly purchasable
- desirable gear should come from drops, crafting, market/auction, gates,
  limited stock, or risky zones
- direct unlimited shop access should not flatten the chase

### 3. Rewards Are Too Predictable

Fixed materials and formulaic quest rewards are easy to reason about, but they
do not create anticipation.

Add:

- rare drop rows
- visible but low-probability chase items
- gate fragments
- map-specific drops
- boss/event drops
- bonus-box/resource-node variance

Keep server-owned roll truth hidden.

### 4. Endgame / Repeatable Goal Is Missing

`docs/road-to-v1/11-first-endgame-signal-gate.md` is the missing retention
spine. A Galaxy-Gate-like signal gate does not need to be huge. One good
instanced wave loop can change the game's identity.

Recommended shape:

```text
drop fragments -> assemble gate -> enter wave instance -> defeat wave/boss ->
earn meaningful deterministic + rare rewards
```

### 5. DarkOrbit Flavor Is Not Started

`docs/road-to-v1/12-darkorbit-flavor.md` explicitly lists drones-lite,
P.E.T.-lite, ammo/consumables, and honor/leaderboard as not started.

These are not cosmetic:

- ammo changes combat economy immediately
- rockets add timing and burst
- drones add visible long-term gear chase
- honor makes PvP/social risk meaningful
- P.E.T. adds convenience/fantasy but has abuse risk

Recommended order:

```text
ammo -> rocket -> honor -> drone-lite -> P.E.T.-lite
```

### 6. Scarcity Is Too Dev-Seeded

Playtest seeds are useful, but live-feel must be judged without assuming:

- generous starter credits
- seeded X Core
- dev route materials
- static no-DB state
- deterministic shortcuts

Use dev seeds for tests. Do not let them define product feel.

## Recommended Early Ladder Contract

Author a concrete 3-5 hour progression spine:

```text
Phoenix starter
  -> kill passive/training contacts
  -> earn first LF/shield/cargo upgrade
  -> choose Vengeance/Bigboy/Goliath direction
  -> enter first risky map for better drops
  -> gather gate fragments / rare materials
  -> complete first mini-gate or boss event
```

This should be documented as tables:

- target kill times
- expected credits/hour
- expected drops/hour
- upgrade costs
- map danger
- ship/module unlock timing
- repair/death cost
- loot risk multiplier

## Content Expansion Minimum

Before claiming DarkOrbit-like early feel, add at least:

- 8-12 NPC profiles
- 4-6 loot tables
- 10-15 modules
- 6-10 recipes
- 3-5 ammo/consumable rows
- 1 rocket/burst weapon path
- 1 honor/reputation read model
- 1 gate-fragment loop
- distinct map resource identities

## Balance Principles

- Safe maps should be understandable but slower.
- Risk maps should materially outperform safe maps.
- Better rewards should require either danger, time, crafting, or market action.
- Repair/death costs should sting but not brick the player.
- Ammo should create tactical/economic choices, not just a tax.
- Rare drops should create anticipation, not mandatory frustration.

## Bottom Line

This is not a rewrite problem. It is a content and balance authorship problem on
top of a serious backend.

The next phase should be less "add another service" and more:

```text
make the existing services create hunger
```

