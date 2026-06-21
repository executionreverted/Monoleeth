# Phase 04: Portals, Safe Zones, And PvP Policy

## Goal

Add server-authoritative portal travel, safe zones, spawn/portal protection, and
PvP policy for bounded 10000x10000 maps. Portal entry is always a client intent;
the server validates current map, proximity, cooldown, active transfer state,
ship state, destination spawn safety, and map policy before any handoff starts.

## Current state to replace/reuse, with exact file refs

Reuse:
- `docs/plans/ui-implementation/04-live-world-aoi-movement.md:59` states that
  movement targets are intents and the server resolves player/ship state.
- `docs/plans/ui-implementation/04-live-world-aoi-movement.md:62` already calls
  out movement validation for active ship state, path limits, server stats,
  visibility/routing rules, request id behavior, and movement rate limits.
- `internal/game/server/handlers.go:209` handles `move_to` as a server-validated
  intent.
- `internal/game/server/handlers.go:265` enforces movement distance and rate
  limiting before mutating worker state.
- `internal/game/server/handlers.go:372` rejects server-owned fields in client
  payloads.
- `docs/plans/modules/05-combat-damage-targeting.md:50` makes combat commands
  client intents.
- `docs/plans/modules/05-combat-damage-targeting.md:97` documents target
  validation for world membership, visibility, range, and friendly fire.
- `internal/game/combat/service.go:214` validates attacker and target state
  before combat mutation.
- `internal/game/combat/service.go:238` rejects actors in different world/zone.
- `internal/game/combat/service.go:241` rechecks visibility with
  `visibility.CanInteract`.
- `internal/game/combat/service.go:244` validates weapon range server-side.

Replace or extend:
- `client/src/protocol/envelope.ts:3` has no `portal.enter` operation.
- `client/src/protocol/envelope.ts:130` has no public `portal` entity type.
- `internal/game/server/handlers.go:112` has no portal command registration.
- `internal/game/server/runtime.go:1156` hard-codes one sector payload without
  map bounds, safe zone state, portal summary, or PvP policy.
- `internal/game/combat/service.go:238` checks world/zone only. It must become
  same active map plus PvP/safe-zone policy.
- Current movement validation does not clamp or reject targets outside
  10000x10000 map bounds.

## Target model

Portals are server-owned map entities. A portal can be visible through AOI like
other entities, but visibility does not grant travel permission by itself.

Portal validation order:

```text
authenticate session
resolve player active map
reject if transfer_state != active
resolve portal in current map
verify portal is active and visible/interactable
verify player proximity to portal radius
verify ship can travel
verify player+source-map+portal cooldown ready
verify destination map exists
choose safe destination spawn
start map transfer transaction
record player+source-map+portal cooldown
publish transfer events after commit
```

Portal cooldown default:

```text
30 seconds, keyed by player_id + source_map_id + portal_id
```

Safe zones are server-owned geometric regions inside a map. For MVP:
- safe zones block PvP initiation and PvP damage
- portal/spawn protection also blocks PvP
- NPC spawn tables should avoid safe zones
- PvE behavior inside safe zones should be an explicit map policy, not inferred
  from the PvP block

PvP policy is evaluated on every combat target validation and any future
area-of-effect damage. The combat service must never rely on the client to say
whether a target is PvP-allowed.

### Map Entry Gates

Map entry gates are map policy, not only portal policy. A map may require:

- minimum player rank
- faction/clan/alliance permission
- ship class or equipment requirement
- quest/event unlock
- safe-map graduation state
- premium/event access if later enabled

Portal validation must call map entry gates before transfer starts. A locked
map should return a safe `ERR_MAP_LOCKED`/`ERR_RANK_REQUIRED` style error and
must not reveal hidden destination internals.

### Portal Camping Protection Defaults

Default portal/spawn protection for MVP:

- duration: `10s` after destination attach
- blocks incoming PvP damage
- blocks the protected player from initiating PvP unless protection is broken
- breaks on PvP action, leaving the protected zone, changing ship, or explicit
  cancel if added later
- does not block PvE unless the map policy says starter/safe maps suppress NPC
  aggro near the spawn
- does not make the player invisible; visibility still follows radar/stealth

Protection timers are server state. The client may display server timestamps,
but client timers do not grant protection.

### Stations, Hangar, Repair, And Respawn Checkpoints

Starter and safe maps should include station/safe-zone definitions. Station
actions are server-authorized proximity interactions:

- hangar/ship swap
- repair quote and repair payment
- cargo unload/storage, if available
- marketplace/station services, if later map-localized
- respawn checkpoint registration

Death/respawn policy should select a checkpoint in this order for MVP:

1. current map station/checkpoint if valid and safe
2. last registered checkpoint if still accessible
3. faction/starter map spawn
4. configured map default spawn

Respawn must clear or settle live combat state, apply spawn protection, and
avoid placing players inside hostile spawn areas unless the map explicitly
allows it.

### Safe/PvP Policy Consumers

Use one server-side policy facade for every consumer below. UI labels are only
projections of this policy.

| Consumer | Required policy checks |
| --- | --- |
| Combat initiation | same active map, visibility, range, attacker/target safe-zone state, protection, PvP mode, friendly-fire rule |
| Damage application | same policy as initiation; delayed/AOE damage must re-check before mutation |
| Death/cargo loss | map risk band, safe/PvP state, attacker type, anti-farming rules |
| Loot drop visibility/pickup | owner lock, current map, radar/AOI visibility, safe-zone pickup restrictions if any |
| NPC aggro/leash | current map, safe-zone attack policy, portal/spawn protection, leash boundary |
| Portal use | current map, proximity, transfer state, cooldown, map entry gate, destination spawn safety |
| Hangar/repair/station | current map, station/safe-zone proximity, combat state, ship state |
| Scan/claim | current map, scan/claim range, radar/scanner rules, safe/PvP interruption policy |
| Admin/debug | explicit dev/admin permission, target map resolution, no hidden data in public responses |

## Data structures/contracts to add or change

Add internal server types:

```text
PortalDefinition
  portal_id
  origin_map_id
  origin_position
  activation_radius
  destination_map_id
  destination_spawn_key
  cooldown = 30s
  active
  required_rank optional
  required_faction optional

PortalCooldown
  player_id
  source_map_id
  portal_id
  ready_at

SafeZoneDefinition
  zone_key
  internal_map_id
  shape circle|polygon|rect
  blocks_pvp
  grants_spawn_protection
  grants_portal_protection
  public_label optional

ProtectionState
  player_id
  internal_map_id
  reason spawn|portal|admin
  expires_at
  break_on_pvp_action
  break_on_leave_zone

PvPPolicy
  policy_key
  default_rule disabled|enabled|contested|faction_only
  safe_zone_override
  protection_override
  friendly_fire_rule
  aggression_flag_rule
```

Add operation:

```text
portal.enter
```

Request payload:

```json
{
  "portal_id": "portal_1_1_to_1_2"
}
```

Response examples:

```json
{
  "accepted": true,
  "status": "transfer_started",
  "cooldown_ready_at": 1782054000000
}
```

```json
{
  "accepted": false,
  "status": "cooldown",
  "cooldown_ready_at": 1782054000000
}
```

Add or extend events:

```text
map.transfer_started
map.transfer_completed
map.transfer_failed
player.protection_updated
```

Add public entity type and display contracts:

```text
portal
```

Portal entity payloads may include public display metadata such as label,
disposition `portal`, and public destination label. Do not send internal
destination map id, spawn key, worker id, cooldown table key, route seed, or
future spawn data.

Change `world.snapshot` or `map.snapshot` to include:

```json
{
  "map": {
    "map_key": "mmo-1",
    "bounds": { "width": 10000, "height": 10000 },
    "pvp_policy": "safe",
    "safe_zone": {
      "inside": true,
      "blocks_pvp": true,
      "protection_ends_at": 1782054000000
    }
  }
}
```

The snapshot can expose the viewer's own safe-zone/protection state. It must not
reveal hidden safe-zone internals or off-map portals.

Combat policy check must become:

```text
same active map
attacker alive and active
target alive and active
target visible to attacker
target in weapon/skill range
PvP/safe-zone/protection policy allows the action
cooldown and energy pass
```

## Implementation tasks in order

1. Add portal, safe zone, and PvP policy definitions to the map catalog.
2. Add the public `portal` entity type to protocol/client types and renderer
   contracts before exposing portal entities in snapshots.
3. Add map bounds validation to movement so `move_to` cannot target outside
   `0..10000` on either axis.
4. Add safe-zone geometry helpers and tests for circle, rectangle, and polygon
   point-in-zone checks if polygon is selected for MVP.
5. Add a server-side PvP policy evaluator. Keep it independent from UI and from
   raw combat damage math.
6. Add a shared map interaction policy facade for combat, delayed/AOE damage,
   death/cargo, loot, NPC aggro, portal, hangar/repair/station, scan/claim, and
   admin/debug map-targeted operations.
7. Update combat target validation to call the policy evaluator after same-map,
   visibility, and range checks but before energy/cooldown mutation.
8. Add `PortalCooldownStore`, keyed by
   `player_id + source_map_id + portal_id`, with the default 30 second cooldown
   from server time.
9. Add `portal.enter` operation constants and handler registration.
10. Implement `portal.enter` validation without trusting player id, position,
   map id, destination, cooldown, or spawn from the client.
11. Route accepted portal entry into the Phase 03 map transfer lifecycle.
12. Add destination spawn selection that prefers a spawn point inside a safe zone
    or with portal protection. If no safe spawn exists, reject the portal and do
    not start cooldown.
13. Apply spawn/portal protection after destination attach and publish
    `player.protection_updated` to the actor.
14. Add station/checkpoint policy for hangar, repair, and respawn fallback.
15. Update AOI serialization to expose visible portal entities only in the
    current map and radar range.
16. Update client reducer to handle transfer/protection events and to show
    server-owned cooldown/protection state only from responses/events.
17. Add abuse metrics for portal spam, invalid portal target, out-of-range
    portal entry, transfer-state rejection, and PvP blocked by safe zone.

## Tests to add/update

- `portal.enter` rejects unauthenticated requests.
- `portal.enter` rejects payloads containing player id, map id, position,
  destination, cooldown, or spawn fields.
- `portal.enter` rejects a portal entity not visible/interactable in the
  player's current map.
- `portal.enter` rejects when the player is outside the activation radius.
- `portal.enter` rejects while the player has an active transfer state.
- `portal.enter` starts a transfer when the player is in range and cooldown is
  ready.
- Portal cooldown is 30 seconds by default and keyed by player, source map, and
  portal id.
- Portal cooldown for portal A does not block portal B unless configured.
- Portal cooldown for player A does not block player B.
- Cooldown is not consumed when destination spawn validation fails.
- Player spawns at the destination safe spawn and receives portal protection.
- Protected players cannot receive PvP damage and cannot initiate PvP without
  breaking protection.
- Map entry gates reject rank/faction/quest/equipment-locked maps before
  transfer starts.
- Movement outside 10000x10000 map bounds is rejected.
- PvP attack in a safe zone is rejected before energy/cooldown/damage mutation.
- PvP attack against a portal-protected or spawn-protected player is rejected.
- Delayed/AOE damage rechecks safe-zone/protection policy before mutation.
- Death respawn selects the nearest valid station/checkpoint/fallback spawn and
  applies spawn protection.
- Hangar/repair station actions require current-map station/safe-zone access.
- PvE behavior in safe zones follows the explicit map policy.
- Same-map visible in-range PvP attack outside safe zones follows map PvP policy.
- Area damage cannot damage protected/safe-zone players.
- Client reducer stores cooldown/protection only from server payloads and clears
  stale origin-map combat/selection state on transfer.
- Portal entities from another map never appear in snapshots or minimap.

## Migration/doc updates

- Update transport docs with `portal.enter` as a handoff intent.
- Update combat module docs to include same-map and PvP/safe-zone checks in the
  target validation list.
- Update API/event docs with portal request/response errors:
  `ERR_PORTAL_NOT_FOUND`, `ERR_PORTAL_OUT_OF_RANGE`, `ERR_PORTAL_COOLDOWN`,
  `ERR_TRANSFER_ACTIVE`, `ERR_DESTINATION_UNAVAILABLE`, and
  `ERR_PVP_BLOCKED`.
- Update UI phase docs to add portal markers and safe-zone/protection HUD state
  as real server state only.
- Document the 30 second default portal cooldown and the
  `player_id + source_map_id + portal_id` key.
- Document map catalog authoring rules for portal destination safety.

## Risks and acceptance criteria

Risks:
- Portal cooldown can be consumed too early if written before destination spawn
  validation.
- PvP policy can be bypassed if only command handlers check it and combat tick
  or area effects skip it.
- Protection state can become hidden fake UI if the client invents timers.
- Portal entities can leak destination internals if public display and internal
  routing data are mixed.
- Safe-zone geometry can disagree between movement, combat, and rendering if
  duplicated.

Acceptance criteria:
- Portal entry is an intent and all authority comes from server state.
- Default portal cooldown is 30 seconds and keyed by player, source map, and
  portal id.
- Safe zones and spawn/portal protection block PvP both at initiation and damage
  application time.
- Combat checks same active map, visibility, range, safe-zone/PvP policy,
  cooldown, and energy before mutation.
- Destination attach starts from a safe spawn or rejects without partial handoff.
- Portal, safe-zone, and PvP state visible in the client comes only from
  authenticated server snapshots, responses, or events.
