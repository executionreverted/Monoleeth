# 2D Space MORPG - Architecture and Design Notes

Date: 2026-06-16

## Core Vision

The game is a browser-first and desktop-capable 2D space MORPG with a nerdy sci-fi feel: part real-time space action, part persistent strategy game, part console-like command interface.

The target mood is:

- Oldschool DarkOrbit-style 2D space movement and combat.
- OGame / Travian-style persistence, resource loops, timers, economy, alliances, and long-term planning.
- A more tactical, information-driven space game where sensors, radar/stealth
  visibility, range, cargo, gathering, loot, and map control matter.
- Pixel art or 16/32-bit inspired visuals rendered on a game canvas, with UI panels that can feel like a ship console.

The game should work in the browser and also as a packaged desktop client. The same client should ideally be reused for both.

## Non-Negotiable Technical Principles

The game must be server authoritative.

The client should never be trusted for:

- Position.
- Damage.
- Hit validation.
- Range checks.
- Line of sight.
- Visibility, radar, stealth, and hidden-data filtering.
- Loot ownership.
- Resource gathering progress.
- Cargo capacity.
- Portal/map transitions.

The client sends intent. The server owns truth.

Examples:

- Client says: `move_to(x, y)`.
- Server decides actual movement based on speed, map bounds, collision, current status, and elapsed simulation time.
- Client says: `attack(target_id)`.
- Server checks visibility, range, cooldown, weapon validity, line of sight, hit chance, and damage.
- Client says: `gather(resource_id)`.
- Server checks position, range, tool/state requirements, cargo capacity, and tick timing.

## Recommended Tech Stack

### Client

- TypeScript.
- PixiJS for 2D rendering.
- React, Solid, or another DOM UI layer for panels such as inventory, cargo, chat, station UI, market, quests, and terminal screens.
- Tauri for desktop packaging.

PixiJS is a good fit because this game needs a custom 2D render surface, sprite
control, radar/minimap overlays, effects, map layers, and a lot of UI
composition around the canvas. Phaser can work, but PixiJS is more flexible as
a rendering engine when the game logic is already server-owned.

### Realtime Network

- WebSocket for browser compatibility.
- Binary protocol for production: Protobuf, MessagePack, or a custom compact binary format later.
- JSON can be used during early prototyping, but it should not be the final protocol for high-frequency state replication.

### Game Server

- Go.
- Custom authoritative map/zone server.
- Actor-style map workers.
- Fixed tick simulation loop.
- In-memory spatial grid.
- In-memory delayed event scheduler per map.

The game server is not a regular backend API. It is a realtime simulation engine made of map actors.

### Backend Services

- PostgreSQL for persistent account, inventory, economy, market, progression, and world data.
- Redis for session/presence/cache/short-lived locks.
- NATS for service-to-service messaging.
- NATS JetStream for durable async jobs and event streams where persistence matters.

### Infrastructure

- Docker from day one.
- Docker Compose for local development.
- Kubernetes or another orchestrator later, when there are multiple zone servers, gateways, backend services, and observability requirements.
- Prometheus + Grafana + OpenTelemetry for metrics, traces, and debugging.

## High-Level Runtime Architecture

```text
Browser / Tauri Client
        |
   WebSocket Gateway
        |
   World Router
        |
 Map Worker A / B / C / ...
        |
 PostgreSQL / Redis / NATS
```

The socket should ideally connect to a gateway/session server, not directly to one map process forever.

The gateway owns the live client connection. The world router decides which map worker currently owns that player's simulation. When the player transitions from Map A to Map B, the socket does not need to disconnect; routing changes internally.

## Map Model

Example:

```text
Map A: 1000 x 1000
Portal at 312,564 -> Map B
Portal at 900,120 -> Map C
```

Each map has a worker that owns the complete authoritative state for that map.

The map worker is responsible for:

- Player positions.
- NPC/monster positions.
- Resource nodes.
- Projectiles.
- Loot drops.
- Portals.
- Safe zones.
- Static colliders.
- Fog/visibility rules.
- Combat and gathering validation.
- Delayed events like respawns and despawns.

## Bounded Multi-Map Frontier

The playable universe is composed of bounded maps connected by portals.

Conceptually:

```text
Map: 10000 x 10000 local coordinate space
Portal: server-owned transition from one map to another
Universe: graph of bounded maps with different profiles
```

Players begin in an entry map and move through portals into other maps. The goal
is to scan, discover planets, claim/build on them, create production routes, and
connect those planets into a personal or alliance network.

The key design hook:

```text
The universe can grow by adding maps, but each playable map stays bounded,
readable, and server-authoritative.
```

### Map Unlock Horizon

The game should not spawn meaningful content at arbitrary unbounded coordinates.

Instead, expansion is controlled by map catalog/profile rollout:

```text
map_catalog = explicit bounded maps
portal_graph = explicit transitions between maps
candidate_budget = per-map planet/resource/anomaly/enemy budget
```

New planets, resources, anomalies, NPC zones, and special points of interest are
generated or selected from the current map profile and its durable overlay.

This keeps the universe controlled, readable, and socially alive while still
allowing live ops to add new maps later.

Important distinction:

```text
Flying to the edge of a map does not create more world.
Portal unlocks, events, and map rollout expand the playable graph.
```

Valid map-expanding events can include:

- Discovering a new planet.
- Claiming or activating a planet.
- Building map infrastructure.
- Establishing a wormhole anchor.
- Scanning a major anomaly.
- Creating an outpost.
- Completing a frontier event.

This prevents one player from flying in a straight line for hours and forcing
the world generator to expand into absurd coordinates.

### Controlled Procedural Spawning

The world can still be procedural, but it should be procedural inside bounded
maps and server-owned map profiles.

Suggested model:

```text
chunk_size = fixed world-unit size
chunk_x = floor(x / chunk_size)
chunk_y = floor(y / chunk_size)
chunk_seed = world_seed + chunk_x + chunk_y
```

Chunks can be generated deterministically when needed, but meaningful entities become persistent only after interaction or discovery.

Examples:

```text
Unseen generated candidate:
  not stored permanently yet

Discovered planet:
  persisted to database
  gets identity, stats, ownership, production, buildings

Claimed planet:
  enters player/alliance network
  can host buildings, automation, routes, defenses, wormhole links
```

### Frontier Bands

The current discovery horizon can be divided into gameplay bands.

```text
Inner Space: 0% - 40% of horizon
  more settled
  safer
  lower yield
  starter-friendly

Mid Space: 40% - 75%
  normal PvE/resource loops
  trade routes
  moderate conflict

Frontier Band: 75% - 100%
  new planet discovery
  better resources
  higher NPC threat
  contested PvP pressure

Outer Noise: just beyond current horizon
  weak signals
  anomaly hints
  no fully spawned civilization yet
```

This creates a clear emotional geography:

```text
Center = civilization
Middle = economy
Frontier = opportunity and danger
Beyond = unknown
```

### Travel and Warp

Movement should be a real constraint, especially early.

Early ships have:

- Low movement speed.
- Low radar range.
- Low cargo.
- Weak capacitor.
- Limited ability to survive deep-space routes.

Except for energy-powered warp/wormhole travel between discovered or connected planets, players must physically fly through space.

Travel modes:

```text
Map Flight:
  Can move within the current bounded map.
  Slow, especially early.
  Exposes the player to radar-limited visibility, threats, cargo risk, and
  travel time.

Portal / Planet Warp / Wormhole:
  Works only through server-owned portals or discovered/claimed/linked
  infrastructure.
  Consumes energy.
  Requires infrastructure.
  Creates strategic transport networks.
```

This makes planet discovery and network-building central instead of decorative. A planet is not only a resource node; it is a logistics anchor.

### Galaxy Network Gameplay

Players build a network of discovered and developed planets.

Example:

```text
Asterion-4:
  energy production

Kappa Mine:
  ore production

Silent Relay:
  radar and scan coverage

Veil Forge:
  crafting and module production

Wormhole links:
  Asterion-4 <-> Veil Forge
  Veil Forge <-> Kappa Mine
```

Automation can emerge from this network:

```text
Route ore from Kappa Mine to Veil Forge.
Route energy from Asterion-4 to wormhole upkeep.
Auto-produce laser cells.
Auto-upgrade radar relay.
Maintain defense budget on frontier planets.
```

Network upkeep and risk:

- Longer routes require more energy.
- Wormholes have upkeep.
- Cargo routes can be attacked in PvP zones.
- Frontier planets may need defense.
- Radar relays can become strategic targets.
- Deep-space infrastructure can be more valuable but harder to protect.

### Shared Planets and Personal Intel

Planets should exist in one shared global universe.

If a player discovers a planet at a coordinate, that planet is not a private instance. It is a real world object. Another player can later find, claim, colonize, raid, or defend that same planet.

The distinction:

```text
Planet existence:
  global

Planet ownership:
  global

Planet discovery knowledge:
  per player / alliance

Planet intel freshness:
  per player / alliance
```

Example:

```text
Player A flies inside map `1-1` from 3500,2400 toward 5200,6100.
Player A detects a planet.
The planet is added to Player A's known intel.

Later, Player B reaches the same planet and colonizes it.
Planet ownership changes globally.

Player A still remembers the planet exists,
but Player A's intel may be stale until they rescan it.
```

This creates an information economy:

- Scouts can discover valuable planets.
- Cartographers can sell coordinates.
- Alliances can share frontier intel.
- Pirates can buy leaked routes.
- Players can act on stale data and get surprised.
- Radar networks become economic infrastructure, not just combat utility.

Discovery should not automatically grant ownership. It grants knowledge and opportunity. To own a planet, the player must physically reach it and establish infrastructure.

### Planet Intel Records

Planet intel can be modeled separately from the planet itself.

```text
planets:
  id
  x
  y
  type
  base_energy
  resource_profile
  current_owner_id
  claim_state

planet_intel:
  owner_type: player | alliance
  owner_id
  planet_id
  first_seen_at
  last_seen_at
  known_owner_id
  known_energy
  known_defense
  known_buildings
  confidence
  stale_after
```

The player can know:

```text
Planet exists.
Last known type.
Last known owner.
Last known energy.
Last known defense.
Last known buildings.
Last seen timestamp.
Confidence / freshness.
```

But unless the planet is currently visible through ship radar, relay coverage, alliance intel, or another valid source, the data may be stale.

### Coordinate Shares and Intel Items

Planet coordinates can become tradeable knowledge.

There can be a "share" mechanic:

```text
Player A shares Planet ABC with Player B.
Player B receives an in-game mail or notification.
Planet ABC appears in Player B's known intel.
Player B gains a planet intel record.
```

To prevent spam and preserve value:

- Players may have a daily share limit.
- Alliance officers may have separate alliance-share limits.
- Sharing may require proximity, a communication relay, or a cartography module.
- High-value coordinates may require an itemized intel package instead of free sharing.

The game can represent coordinates as in-world items:

```text
Coordinate Scroll / Star Chart / Intel Packet
```

These can be:

- Created from discovered planet intel.
- Sent to another player.
- Shared with an alliance.
- Listed on the market.
- Bought by scouts, settlers, pirates, traders, or alliances.
- Consumed to reveal the planet in the buyer's known intel.

Possible item fields:

```text
intel_item:
  planet_id
  seller_id
  created_at
  snapshot_owner_id
  snapshot_type
  snapshot_rarity
  snapshot_energy
  snapshot_map_id
  snapshot_map_position
  expires_at
  validity_state
```

The item does not need to reveal every stat. Low-tier intel may show only approximate coordinates and planet class. Higher-tier scans may include resource profile, energy production, defense estimate, and ownership snapshot.

### Intel Market Invalidation

Because planets are global, intel can become stale or invalid.

If a coordinate scroll is listed on the market and the planet is colonized before purchase, several rule options exist:

```text
Strict:
  Remove the listing automatically.
  Refund listing fee if needed.

Stale Intel:
  Keep the listing, but mark it as stale/changed.
  Buyer can still buy risky old intel at a discount.

Disclosure:
  Show "last verified at" and "status may have changed".
```

Recommended first version:

```text
If planet ownership changes while an intel item is listed,
mark the listing as stale and hide it from default market search.
Seller can relist after re-verifying the intel.
```

This keeps the market honest enough while preserving the fantasy of risky information trading.

Gameplay examples:

```text
Scout finds a rare frontier planet but cannot colonize it yet.
Scout creates an intel packet and lists it on the market.
An industrial player buys it and sends a hauler/settler ship.
A pirate buys leaked route intel and camps the travel path.
An alliance shares a batch of coordinates internally before a colonization push.
```

This makes information itself a resource.

## Map Worker Shape

```text
MapWorker
  owns map state
  has command mailbox
  has fixed tick loop
  has delayed task scheduler
  has spatial grid
  has radar/stealth visibility system
  has combat system
  has resource/gather system
  has loot system
  emits replication deltas
```

The worker should be the single owner of its map state. This avoids race conditions around:

- Two players trying to loot the same item.
- A monster dying while another attack lands.
- A player entering a portal while taking damage.
- A resource node being depleted by multiple gatherers.
- A delayed respawn firing after a map unload/reload.

Commands enter the worker through a mailbox/queue:

```text
MoveTo(player_id, x, y)
Attack(player_id, target_id, weapon_slot)
UseSkill(player_id, skill_id, target)
StartGather(player_id, resource_id)
PickupLoot(player_id, loot_id)
EnterPortal(player_id, portal_id)
Disconnect(player_id)
```

The worker processes commands and ticks the simulation in a controlled order.

## Tick Loop

Suggested starting point:

```text
Simulation tick: 20 Hz
Network snapshot: 10-20 Hz
AOI/visibility refresh: incremental every tick, full sanity refresh less frequently
```

Each tick:

```text
1. Drain input commands.
2. Resolve movement intents.
3. Update spatial grid membership.
4. Run combat, projectiles, gathering, and timed effects.
5. Process due delayed tasks.
6. Recompute visibility/AOI deltas.
7. Send replication deltas to clients through the gateway.
```

## Movement

The client does not send final position. It sends movement intent.

```text
client -> server:
  move_to(x, y, seq)
```

Server:

```text
current_position = server truth
target_position = requested destination
speed = server-owned stat
dt = fixed tick delta
direction = normalize(target - current)
new_position = current + direction * speed * dt
```

If the player reaches the target, movement stops. If a new movement command arrives, the target changes.

Good early movement command set:

- `move_to(x, y)`
- `stop()`
- `orbit(target_id, radius)`
- `follow(target_id)`
- `approach(target_id)`
- `keep_distance(target_id, distance)`

Client behavior:

- Predict only the local player's movement for responsiveness.
- Interpolate all remote entities from server snapshots.
- Smoothly correct local player position when server truth differs.

## Spatial Queries

The game needs frequent spatial checks:

- Nearby entities.
- Attack range.
- Gather range.
- Portal trigger area.
- Loot pickup range.
- Fog of war visibility.
- Monster aggro.
- Projectile collision.

For a 2D top-down space game, start with a uniform grid / spatial hash.

Example:

```text
cell_size = 256, 512, or 1024 world units
cell_x = floor(entity.x / cell_size)
cell_y = floor(entity.y / cell_size)
```

To find entities around a player:

```text
1. Find player's cell.
2. Visit nearby cells based on query radius.
3. Collect candidate entities.
4. Run exact distance checks using squared distance.
```

Do not scan all entities on every query.

Quadtree can be considered later if entity density becomes extremely uneven, but a grid is simpler, faster to debug, and usually better for constantly moving entities.

## Fog of War and Visibility

Fog of war is not a visual-only system. It is a network permission system.

The server has:

```text
Truth State:
  Everything the server knows.

Client View State:
  Only the entities and intel this specific player is allowed to know.
```

The client should never receive hidden entities and merely hide them in the UI. If something is not visible or known, the client should not receive it.

Visibility pipeline:

```text
1. Use spatial grid to get nearby candidates.
2. Check exact distance.
3. Apply sensor range.
4. Apply stealth/signature.
5. Apply nebula/jammer/environment penalties.
6. Apply line of sight if the map has blockers.
7. Produce visible entity set.
8. Diff with previous visible set.
9. Send enter/update/leave deltas.
```

Possible visibility states:

```text
Unknown:
  Never discovered, not sent to client.

Explored:
  Previously discovered, can be represented as last-known/intel data.

Visible:
  Currently visible, receives live authoritative updates.
```

Important distinction:

```text
Live Entity:
  exact position, velocity, hp, current state

Intel Contact:
  approximate position, last seen time, confidence, maybe stale metadata
```

This supports gameplay around:

- Sensor range.
- Stealth.
- Signature radius.
- Scanner probes.
- Jammers.
- Nebula clouds.
- Decoys.
- Last-known fleet position.

## Portal and Map Transitions

When a player enters a portal/transition area:

```text
1. Map A validates the player is inside the portal trigger.
2. Map A checks requirements, cooldowns, combat restrictions, cargo rules, etc.
3. Player state is frozen/locked for transition.
4. World router finds or starts Map B worker.
5. Map A removes player from its simulation.
6. Map B inserts player at destination spawn point.
7. Gateway routes future player commands to Map B.
8. Client receives map_changed + initial_snapshot.
```

The client socket does not need to reconnect. The player changes map logically; the gateway stays connected.

## Combat

Combat is fully server authoritative.

Client request:

```text
attack(target_id, weapon_slot)
```

Server validation:

```text
1. Attacker exists and is alive.
2. Target exists and is alive.
3. Target is visible or sensor-locked.
4. Weapon exists and is equipped.
5. Cooldown is ready.
6. Target is in range.
7. Line of sight is valid if required.
8. PvP/faction/safe-zone rules allow the attack.
9. Hit chance / dodge / accuracy is calculated.
10. Damage is calculated.
11. HP/shield/armor changes are applied.
12. Combat events are replicated to relevant clients.
```

Auto combat can be represented as a server-side intent:

```text
AutoAttackIntent
  target_id
  weapon_slot
  repeat_until_invalid
```

Manual combat can use explicit commands:

```text
use_skill(skill_id, target_id or target_position)
fire_rocket(target_id)
activate_module(module_id)
```

Both paths still use the same server validation rules.

## Gathering and Idle Tasks

The player can move the ship to a location, choose a task, and let the ship perform it.

Examples:

- Gather asteroid ore.
- Salvage wreckage.
- Scan anomaly.
- Hack station debris.
- Mine gas cloud.

Server-owned gathering flow:

```text
1. Player requests StartGather(resource_id).
2. Server checks resource visibility, range, requirements, and cargo.
3. Server creates GatherIntent.
4. Every gather tick, server grants resources if still valid.
5. Cargo capacity is enforced by the server.
6. Resource node depletion is handled by the map worker.
7. Resource respawn is scheduled by the map worker.
```

Gathering should be interruptible by:

- Movement.
- Combat.
- Damage.
- Cargo full.
- Resource depleted.
- Player disconnect rules.

## Loot

Monster or NPC death creates loot owned by the map worker.

Loot lifecycle:

```text
1. Monster dies.
2. Loot table is rolled server-side.
3. Loot entity is spawned at/near death position.
4. Loot is locked to the eligible player/party for X seconds.
5. After lock expires, loot becomes public for Y seconds.
6. If nobody picks it up, loot despawns after Z seconds.
```

Pickup validation:

```text
1. Loot exists.
2. Player is in pickup range.
3. Player can currently see/interact with it.
4. Lock allows this player/party.
5. Inventory/cargo has capacity.
6. Item is granted transactionally.
7. Loot entity is removed from map.
```

The map worker schedules:

- `loot_unlock_at`
- `loot_despawn_at`

## Delayed Events and Scheduling

Each map worker needs an in-memory scheduler for map-local delayed jobs.

Examples:

- Monster respawn.
- Resource node respawn.
- Loot unlock.
- Loot despawn.
- Portal cooldown.
- Temporary safe-zone state.
- Timed area event.

Implementation candidates:

- Min-heap priority queue by due time.
- Timing wheel if the volume of scheduled tasks becomes very high.

Do not send every small delayed map event through Redis/NATS. If the map worker owns the state, it should also own most local delayed tasks.

Use NATS/JetStream for durable cross-service or cross-worker events, not every tiny local simulation timer.

## Persistence

The authoritative live state is in memory inside the current map worker.

Persisted state should include:

- Account.
- Character/ship.
- Inventory/cargo.
- Currency.
- Progression.
- Skills/modules/equipment.
- Market orders.
- Player-owned structures if any.
- Long-term world data.
- Last safe position or logout position.

For frequently changing data, avoid writing PostgreSQL every tick.

Use patterns like:

- Dirty flags.
- Periodic persistence.
- Important transactional writes.
- Event log for high-value actions.
- Snapshot on logout/map transition.

High-value item/currency mutations should be transactional and auditable.

## Scaling Model

Do not think of "5000 players on one server" as "5000 players in one process all seeing each other."

Think:

```text
Physical server / node:
  many map workers
  many zones
  total 4000-5000 players if load allows

Map worker:
  owns one map or one shard/sector/instance
```

If a map becomes too hot:

- Split it into sectors.
- Use multiple instances.
- Reduce visibility radius.
- Limit high-frequency projectiles/effects.
- Partition NPC simulation.
- Move hot maps to dedicated nodes.

The most important scaling tools are:

- AOI/interest management.
- Server-side hidden-data filtering.
- Map/zone ownership.
- Efficient binary protocol.
- Avoiding global broadcasts.
- Avoiding database writes in the tick loop.

## Similar Game References

### DarkOrbit Reloaded

Reference: https://www.darkorbit.com/

Useful inspiration:

- Browser-first space MMO identity.
- 2D map-based space navigation.
- Portal/gate movement between maps.
- Laser-style target combat.
- PvE farming plus PvP danger.
- Loot/reward loops around kills and progression.

What to borrow:

- Simple readable space combat.
- Fast map-to-map travel through portals.
- Clear ship silhouettes and weapon feedback.
- Auto/default attack plus manual abilities.

What to avoid:

- Letting automation/botting dominate the core loop.
- Excessive pay-to-win progression pressure.
- Client-trust shortcuts that weaken combat and economy integrity.

### OGame

Reference: https://gameforge.com/en-GB/games/ogame.html

Useful inspiration:

- Persistent browser-based space empire.
- Resource mining.
- Research.
- Fleet construction.
- Alliances and diplomacy.
- Long-term planning around time, travel, and economy.

What to borrow:

- Strategic persistence outside moment-to-moment combat.
- Resource sinks and production timers.
- Research/upgrade progression.
- Alliance-scale objectives.

How to adapt:

- Instead of purely timer/report-based fleet action, keep the player's ship and nearby space realtime.
- Use OGame-like strategic depth as the macro layer, not as the entire interaction model.

### Travian / Travian Kingdoms

Reference: https://www.kingdoms.com/

Useful inspiration:

- Browser strategy pacing.
- Resource gathering and trading.
- Long server rounds.
- Alliances, diplomacy, and map control.
- Village/settlement growth over time.

What to borrow:

- Clear resource loops.
- Long-term server lifecycle and seasonal goals.
- Social pressure through alliances and territory.
- Economy pacing where time itself is part of strategy.

How to adapt:

- Replace villages with ships, stations, sectors, asteroid claims, or outposts.
- Replace troop travel with ship travel, cargo routes, scouting, and sensor intel.

### EVE Online

Reference: https://www.eveonline.com/

Useful inspiration:

- Space sandbox identity.
- PvP and PvE coexistence.
- Mining, trading, industry, exploration.
- Player economy.
- Group goals and territorial pressure.

What to borrow:

- Player-driven economy ambition.
- Risk/reward around cargo, travel, and dangerous space.
- Roles beyond combat: miner, trader, scout, hauler, pirate, fleet support.

How to adapt:

- Keep the interface and moment-to-moment play much lighter than EVE.
- Use EVE as inspiration for economy/social depth, not UI complexity.

### EVE Echoes

Reference: https://www.eveechoes.com/news/official/20200813/31611_898861.html

Useful inspiration:

- Space sandbox adapted to a more accessible client format.
- Mining, trading, industry, PvE, PvP, and player economy in a large persistent universe.

What to borrow:

- A readable, approachable version of big space MMO systems.
- Mobile/compact-client lessons for UI density.

## Design Direction Summary

The strongest design identity is:

```text
2D realtime space command MORPG
```

The game should not be a pure arcade shooter and should not be a pure spreadsheet strategy game. The sweet spot is:

- The player sees and moves their ship in realtime.
- Nearby space matters.
- Fog of war and sensors matter.
- Resources and cargo matter.
- Combat can be automated at a basic level, but manual actions create skill expression.
- The world persists beyond one fight.
- Economy and alliances give long-term purpose.

## Ship, Hangar, and Loadout Direction

The player controls one active ship at a time, similar to DarkOrbit-style ship identity. The account can own multiple ships, but only one ship is deployed into the world.

Ships are meaningful chassis choices, not just skins.

Example:

```text
Starter Scout:
  2 weapon slots
  1 shield slot
  2 utility slots
  high movement speed
  low cargo

Miner Frigate:
  1 weapon slot
  2 shield slots
  3 utility slots
  high cargo
  mining/gather bonus

Heavy Fighter:
  4 weapon slots
  3 shield slots
  2 utility slots
  expensive craft
  high combat pressure
  lower movement speed
```

Each ship can have its own saved loadout:

```text
ShipInstance
  ship_id
  chassis_type
  durability / repair_state
  loadout:
    weapon_slots[]
    shield_slots[]
    generator_slots[]
    radar_slots[]
    utility_slots[]
    cargo_modules[]
```

The hangar is the place where players:

- Swap active ship.
- Repair destroyed/damaged ships.
- Save and edit loadouts.
- Install weapons, shields, generators, radar modules, cargo modules, mining modules, and utilities.
- Compare chassis stats and slot layouts.

If the active ship is destroyed, the player cannot use that same ship until it is repaired. They can deploy another ship from the hangar if they own one.

This creates good long-term progression:

- Crafting a better ship matters because it unlocks more slots or different slot patterns.
- Losing a ship creates downtime/cost without necessarily locking the player out of the game.
- Older ships remain useful as backups, miners, scouts, or low-risk PvP ships.
- Loadouts can be specialized for combat, mining, cargo hauling, scouting, or deep-space exploration.

Ship chassis should support soft "holy trinity" roles without becoming rigid MMO classes. A ship can lean toward support, tank/hauler, fighter, scout, miner, or hybrid through base stats, slot layout, and passive bonuses.

Example role archetypes:

```text
Support / Capacitor Relay:
  low weapon slots
  medium shield
  strong radar/utility slots
  passive aura gives nearby party members capacitor/energy regen
  good for group PvE, wormhole logistics, and fleet sustain

Hauler / Bulwark:
  low weapon slots
  high shield
  high cargo
  slow movement
  passive cargo protection or reduced cargo drop chance
  good for mining routes, trade, and dangerous-zone hauling

Interceptor / Raider:
  high speed
  good weapon slots
  low cargo
  lower shield
  passive bonus to lock speed, pursuit, or disengage
  good for PvP players and hunting exposed targets

Explorer / Scanner:
  medium speed
  low-medium weapons
  strong radar slots
  passive scan range or anomaly detection bonus
  good for radar/scanner exploration and finding planets/resources

Miner / Industrial:
  mining laser bonuses
  high cargo
  utility slots
  weak combat profile
  passive gather efficiency or resource yield bonus
```

Aura and passive rules must stay server authoritative:

```text
Aura source:
  ship chassis or equipped module

Eligibility:
  party/alliance/fleet members only, depending on rule

Checks:
  same map
  within aura radius
  visible or sensor-linked if required
  not jammed/suppressed

Effect examples:
  + capacitor regen
  + shield regen
  + radar strength
  + mining efficiency
  + warp charge speed
  - incoming damage
```

Auras should be modest and tactical, not mandatory stack multipliers. The goal is to make group composition interesting while keeping solo play viable.

Server authority rules:

- The server owns the active ship state.
- The server validates slot compatibility.
- The server validates whether a module can be installed on a chassis.
- The server calculates final stats from chassis + modules + buffs.
- The server enforces repair state before deployment.
- The server rejects loadout swaps outside valid hangar/station contexts.

Good early loadout categories:

```text
Weapons:
  laser, rocket launcher, mining laser, special weapon

Shields:
  max shield, shield regen, damage-type resistance

Generators:
  max capacitor, capacitor regen, movement speed, efficiency

Radar:
  detection range, detection strength, signal classification, jammer resistance

Utility:
  cargo expanders, warp stabilizer, jammer, repair drone, scanner probe launcher
```

Energy/capacitor relationship:

```text
Planetary Energy:
  Produced by claimed/explored planets and infrastructure.
  Used for crafting, building, ammo production, wormholes, station upgrades, and strategic upkeep.

Ship Capacitor:
  Local combat/utility energy pool.
  Used by lasers, shields, radar pulses, warp bursts, jammers, and active modules.
  Determined by ship chassis and equipped generators.
```

This prevents planetary economy from turning directly into unlimited firing power while still making exploration and planet development important for combat progression.

## Open Questions

- Should known intel be per player, per party, per alliance, or per account?
- Should map visibility be circular sensor radius only, or should cones/scanner arcs exist later?
- Should combat be mostly target-lock based, skill-shot based, or hybrid?
- Should resource gathering continue while offline, or only while the ship is present and vulnerable?
- Should server worlds be seasonal like Travian or persistent like EVE?
- How punishing should death be: cargo drop, repair cost, item durability, full loot, or partial loot?
- Should map workers be one map each at first, or should large maps be split into sectors from day one?

## First Prototype Scope

A good first vertical slice:

```text
One server
One gateway
Two bounded maps: A and B
One portal from A to B
One player ship
One NPC monster type
One resource node type
One loot drop type
One cargo limit
Server-side radar/stealth visibility filtering
Server-side movement
Server-side attack range/damage
Server-side gather tick
Map worker delayed respawn/despawn
PixiJS client rendering
```

This proves the hardest architectural parts early:

- Authoritative movement.
- Map worker ownership.
- Portal transition.
- Spatial query.
- Fog-filtered replication.
- Combat validation.
- Loot lifecycle.
- Delayed map tasks.
