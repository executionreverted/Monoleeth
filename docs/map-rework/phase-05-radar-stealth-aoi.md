# Phase 05: Radar, Stealth, Visibility, And AOI

## Goal

Replace fog-of-war wave assumptions with current-map radar visibility. A player
can see and action only entities in their active map and radar range. Radar
range, detection, stealth, scanner reveals, and jammer counterplay are
server-authoritative and may be affected by ship/module stats. Rare planet
scan/claim/production remains hidden until server-side scan and visibility rules
allow a public signal or known-intel entry.

## Current state to replace/reuse, with exact file refs

Reuse:
- `docs/plans/modules/14-world-aoi-fog-security.md:9` defines visibility as a
  security rule, not only a UI rule.
- `docs/plans/modules/14-world-aoi-fog-security.md:34` lists visibility layers:
  current-map membership, AOI distance, radar/sensor detection, known-intel
  permission,
  stealth/jammer modifiers, and entity-specific rules.
- `docs/plans/modules/14-world-aoi-fog-security.md:120` defines detection as
  radar/scanner minus stealth/jammer/distance.
- `docs/plans/modules/14-world-aoi-fog-security.md:140` lists data that must
  never be sent to the client.
- `docs/plans/modules/14-world-aoi-fog-security.md:158` requires every
  interaction to recheck visibility.
- `internal/game/world/visibility/visibility.go:19` wraps radar range as a
  server-calculated stat snapshot value.
- `internal/game/world/visibility/visibility.go:41` defines entity signature as
  server-owned detection input.
- `internal/game/world/visibility/visibility.go:56` defines server-owned viewer
  visibility state.
- `internal/game/world/visibility/visibility.go:80` defines server-owned entity
  visibility state.
- `internal/game/world/visibility/visibility.go:109` defines viewer-specific
  scanner witness state that must not be serialized.
- `internal/game/world/visibility/visibility.go:116` filters client-sendable
  entities by membership, radar range, and hidden-state permissions.
- `internal/game/world/visibility/visibility.go:140` uses generic not-visible
  errors to avoid hidden-data oracles.
- `internal/game/server/runtime.go:943` already builds a viewer-specific AOI
  snapshot before serialization.
- `internal/game/server/runtime.go:971` supports hidden players and
  viewer-specific witness reveals.
- `internal/game/server/runtime.go:1114` adds public entity metadata and marks
  the viewer's own stealthed state.
- `client/src/state/reducer.ts:435` strips `stealthed` from non-self public
  status flags.
- `client/src/state/reducer.ts:449` and `client/src/state/reducer.ts:464`
  maintain minimap live contacts from visible entities.

Replace or extend:
- `docs/plans/modules/14-world-aoi-fog-security.md` now supersedes old fog
  memory language with safe known-intel memory such as scanned planets, stale
  coordinate intel, and owned planet summaries.
- `internal/game/server/runtime.go:949` currently creates a stat snapshot with
  `runtimeLiveProjectionDiagonalRange`; radar must come from effective
  ship/module stats.
- `internal/game/server/runtime.go:962` fetches entities from a fixed projection
  window. The new AOI query should use current-map spatial index plus the
  viewer's authoritative radar/detection stats.
- `internal/game/server/runtime_world_snapshot.go` previously assigned a
  placeholder signature of `1`. TASK-0299 replaces this in runtime AOI/combat
  paths with content/stat-driven signatures.
- `client/src/protocol/envelope.ts:130` lacks portal and other map-object entity
  types that may need radar-visible public markers.
- `client/src/protocol/envelope.ts:207` should reject additional map, worker,
  transfer, detection, stealth, and jammer internals.

## Target model

Visibility pipeline:

```text
same active map
entity active and within map bounds
candidate within radar/AOI query window
detection score passes entity rules
viewer-specific witness/reveal applies if hidden
action-specific range and policy passes
client-safe projection is built
```

Current-map membership is the first hard gate. No entity from another map is
eligible for AOI, minimap, targeting, combat, loot pickup, portal interaction,
scan result claim, or planet action panels.

Radar is a stat, not a client input:
- base ship radar
- equipped module bonuses
- buffs/debuffs
- jammer penalties
- map effects
- temporary scan/reveal effects

Initial radar/stealth tier rule:

```text
effective_radar_tier =
  ship_radar_tier + radar_module_bonus + skill_bonus + temporary_scan_bonus
  - map_interference - jammer_penalty

effective_stealth_tier =
  stealth_module_tier + stealth_skill_level + active_stealth_bonus
  + map_stealth_bonus
```

Normal non-hidden entities can be visible by same-map range. Stealthed or hidden
entities require both:

```text
effective_radar_tier >= effective_stealth_tier
detection_score >= detection_threshold
```

Lower-tier radar may still show vague, server-approved scan feedback if the map
profile allows it, but it must not reveal exact hidden player coordinates or
hidden planet candidates. Higher radar tier should increase range, detection
power, scan resolution, and resistance to jammer penalties. Higher stealth tier
should lower signature, increase detection threshold, or require stronger
scanner witness effects. All values are server-owned stat/module outputs.

Stealth is a server-owned state:
- the stealthed player can receive a self-only `stealthed` status flag
- other players do not receive hidden player coordinates unless detection or a
  viewer-specific witness allows it
- scanner/jammer counterplay updates server witness/reveal state
- toggling stealth may apply speed, capacitor, cooldown, and signature changes
  only through server mutation

Planet discovery:
- hidden planets and production data are not part of live AOI by default
- scan pulses can reveal a public `planet_signal` or write known planet intel
- known-intel minimap memory is safe and stale; it is not live interaction
  permission
- claim/production commands must recheck current map, radar/visibility or known
  intel permission, range, ownership, and server state

## Data structures/contracts to add or change

Add or extend internal server types:

```text
VisibilityContext
  viewer_player_id
  active_map_id
  position
  radar_range
  detection_power
  scanner_bonus
  jammer_resistance
  witnesses
  observed_at

VisibleEntityCandidate
  entity_id
  entity_type
  active_map_id
  position
  signature
  stealth_score
  jammer_strength
  hidden
  public_tags
  interaction_flags

RadarStats
  range
  detection_power
  scan_resolution
  jammer_resistance
  stealth_detection_bonus

StealthState
  player_id
  active_map_id
  active
  signature_modifier
  speed_modifier
  energy_drain_per_second
  cooldown_ready_at
  revealed_until optional

JammerEffect
  source_entity_id
  active_map_id
  radius
  strength
  expires_at

MapAOISubscription
  session_id
  player_id
  active_map_id
  last_snapshot
  snapshot_cursor

PlanetIntel
  player_id
  internal_map_id
  planet_id
  public_map_key
  last_known_position
  freshness
  claim_permission_state
```

Change `Visibility.Viewer` and `Visibility.Entity` equivalents from world/zone
membership to active map membership. If the old world/zone fields remain during
migration, they must be treated as internal implementation details and not as
the gameplay security boundary.

Update `stats.updated` to include public radar-facing fields only:

```json
{
  "speed": 420,
  "radar_range": 900,
  "weapon_range": 700,
  "detection_power": 12,
  "jammer_resistance": 3
}
```

Do not send detection rolls, stealth scores of other players, hidden target
metadata, witness expiry for other players, scan candidate data, procedural
seeds, or future spawn candidates.

`world.snapshot`/`map.snapshot` must include only:
- current map public metadata
- live `entities` visible now
- `minimap.live_contacts` built from the same visible entity set
- `minimap.remembered` containing safe known-intel records only
- a snapshot cursor scoped to the current map/subscription

AOI events remain:

```text
aoi.entity_entered
aoi.entity_updated
aoi.entity_left
visibility.entity_detected optional
visibility.entity_lost optional
```

`visibility.entity_detected` and `visibility.entity_lost` are optional sugar.
They must not carry data that would be forbidden in `aoi` events.

## Implementation tasks in order

1. Update visibility docs and code comments to say "current map membership"
   instead of "world/zone membership" as the gameplay rule.
2. Add bounded map coordinate validation before AOI and interaction checks.
3. Replace fixed projection range with effective radar range from the player's
   authoritative stat snapshot.
4. Extend stat aggregation to expose radar range, detection power, jammer
   resistance, and stealth-related modifiers from ships/modules.
5. Add radar tier and stealth tier aggregation so module/level progression can
   affect detection without exposing hidden rolls to the client.
6. Replace placeholder entity signatures with content-driven signatures for
   players, NPCs, loot, portals, planet signals, and other map objects.
7. Introduce a detection score function:

   ```text
   detection_score = viewer.detection_power
                   + scanner_bonus
                   + stealth_detection_bonus
                   + entity_signature
                   - target.stealth_score
                   - max(0, jammer_strength - viewer.jammer_resistance)
                   - distance_penalty
   ```

   Normal non-hidden entities can pass by distance until advanced detection
   tuning is enabled, but hidden/stealthed entities must require the detection
   path.
8. Keep scanner witness/reveal state server-side and map-scoped. Expire witness
   records by server time.
9. Update stealth toggle behavior so self receives public self-state, while
   other players receive either no entity or a sanitized detected entity based
   on server detection.
10. Add jammer effects as map-local visibility modifiers. They must never affect
   another map.
11. Update AOI spatial query to use the current map's index and radar query
    radius. Exact distance and detection checks still run after candidate fetch.
12. Update minimap live contacts to be rebuilt only from visible current-map
    entities.
13. Keep `minimap.remembered` for safe known-intel data only. Rename references
    away from fog-wave language during migration where practical.
14. Make combat, loot, portal, scan, planet detail, claim, and production
    commands call the same visibility service or policy facade before mutation.
15. Add client protocol/parser forbidden keys for internal detection, jammer,
    stealth, map, and scan candidate data.
16. Add observability for AOI candidate count, visible count, hidden filtered
    count, radar range distribution, detection reveals, jammer hides, and
    cross-map filter rejections.

## Landed in TASK-0299

- `stats.ExplorationStats` now carries server-owned `detection_power`,
  `jammer_resistance`, and `stealth_detection_bonus` alongside existing radar,
  scanner, signature, stealth, and jammer fields.
- Runtime AOI, combat actor, scanner reveal, and loot pickup visibility inputs
  use content/ship-driven signatures for players, NPCs, loot, and map signals
  instead of the old `EntitySignature(1)` runtime placeholder.
- Hidden and stealthed entities still require current-map membership and radar
  range, then require self visibility, an active server-owned witness, or a
  passing server detection score. Normal non-hidden entities still pass by
  current-map radar range without requiring detection stats.
- Scanner hidden-player witness state remains current-map scoped and
  server-time expiring. Detection scores, signatures, stealth scores, jammer
  strengths, witness expiry, and hidden target ids remain internal-only.
- Command payload filtering rejects client-authored detection, signature,
  stealth-score, jammer, scan-power, and radar-range fields.

Deferred:

- Full jammer entity/effect ownership and map-local jammer lifecycle.
- Full radar/stealth tier progression and equip-driven balancing beyond stat
  aggregation fields and module stat-key plumbing.
- Browser HUD/protocol parser changes for displaying detection stats.
- Planet scan rarity/claim integration; Phase06 owns bounded scanner planets.

## Tests to add/update

- Player sees only entities in the same active map.
- Player cannot target, attack, loot, scan-claim, or open a planet panel for an
  entity in another map.
- Entity outside radar range is absent from snapshot, minimap, and AOI events.
- Radar range comes from server stats and cannot be enlarged by client payload.
- Radar tier and stealth tier are server-derived and affect hidden/stealthed
  detection according to the configured rule.
- Equipping a radar module changes server `stats.updated` and AOI reach.
- Hidden player is visible to self with a self-only `stealthed` flag.
- Hidden player is not serialized to other players without detection/witness.
- Scanner witness reveals a hidden player only to the viewer and only until the
  server-owned expiry.
- Client parser/reducer strips or rejects non-self stealth internals.
- Jammer effect can cause a previously visible hidden target to leave AOI.
- Jammer on map A has no effect on visibility in map B.
- Placeholder entity signatures are replaced by configured signatures.
- Known planet minimap memory does not grant live interaction permission.
- Scan pulse can create a visible `planet_signal` or known-intel record without
  leaking candidate seeds/rolls.
- Claim/production commands recheck current map and visibility/intel permission
  before mutation.
- AOI stress test with many entities on multiple maps does not leak cross-map
  contacts.

## Migration/doc updates

- Update `docs/plans/modules/14-world-aoi-fog-security.md` to remove fog-wave
  wording and retain only safe known-intel memory.
- Update world/AOI UI docs to say minimap live contacts are current-map radar
  contacts, not global world contacts.
- Update scanner/discovery docs so rare planet generation is map-scoped and
  hidden until server scan resolution.
- Update combat docs to require same active map and current visibility before
  PvP/PvE policy and range checks.
- Update protocol docs to include new stat fields and forbidden internal fields.
- Update client tests to cover transfer clearing plus radar/minimap rebuild from
  server snapshots.

## Risks and acceptance criteria

Risks:
- Leaving old fog-wave terminology in specs can cause future code to reintroduce
  global or stale visibility as live truth.
- Fixed projection constants can silently override module-driven radar stats.
- Stealth flags can leak if they are treated as ordinary public status flags.
- Known planet memory can become an interaction permission if command handlers
  skip live visibility/intel checks.
- Jammers can become global debuffs if not keyed by active map.

Acceptance criteria:
- Live visibility is exactly active map plus authoritative radar/detection.
- No fog-of-war wave is required for live gameplay visibility.
- Radar range and detection fields come from server stat/module state.
- Stealth and jammer counterplay are resolved server-side and filtered per
  viewer.
- Hidden planets, scan candidates, scan rolls, future spawns, procedural seeds,
  hidden player coordinates, and cross-map entities never reach the client.
- Every action path that depends on visibility rechecks the server visibility
  policy at mutation time.
