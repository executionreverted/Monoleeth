# World AOI, Fog Of War, Visibility, And Security

Date: 2026-06-17

## Purpose

Bu modül oyuncunun hangi entity'leri görebileceğini, hangi entity'lerle etkileşime geçebileceğini ve client'e hangi bilgilerin asla gönderilmemesi gerektiğini tanımlar.

Core rule:

```text
Server tarafında oyuncu yakınında/görüşünde değilse client onu asla göremez.
```

Bu sadece UI kuralı değil, security kuralıdır.

## Owns

```text
AOIService
FogOfWarService
VisibilityService
ScannerVisibilityBridge
```

## Does Not Own

- Procedural generation truth
- Scan roll mechanics
- Combat damage
- Loot ownership
- Market intel sale

## Visibility Layers

Bir entity'nin client'e gönderilmesi için birden fazla katman geçilir:

```text
world/zone membership
AOI distance
radar/sensor detection
fog/intel permission
stealth/jammer modifiers
entity-specific visibility rules
```

Basit:

```go
func CanSendEntityToClient(viewer PlayerID, entity Entity) bool {
	if !SameWorldOrZone(viewer, entity) {
		return false
	}
	if !WithinAOI(viewer.Position, entity.Position) {
		return false
	}
	if !DetectionPasses(viewer, entity) {
		return false
	}
	return true
}
```

## AOI

AOI = active interest area.

Bu network optimization + security filter'dır.

AOI inputs:

- player position
- radar range
- entity type
- entity signature
- server zone

Spatial index:

```text
grid
quadtree
spatial hash
```

MVP için spatial hash yeterli.

## Spatial Hash Example

```go
func CellCoord(pos Vec2, cellSize float64) Cell {
	return Cell{
		X: int(math.Floor(pos.X / cellSize)),
		Y: int(math.Floor(pos.Y / cellSize)),
	}
}

func NearbyCells(center Cell, radius int) []Cell {
	// returns square around center; exact distance checked after
}
```

## Fog Of War Memory

Fog memory:

- discovered planets
- known coordinates
- last seen owner/status
- scanned anomalies
- known wormholes

Fog memory does not mean live visibility.

```text
Known planet can appear on map memory.
Live entity at that location still requires visibility to interact.
```

## Detection Rules

Detection can use:

```text
detection_score = viewer.radar_power
                + scanner_bonus
                - target.stealth
                - jammer_noise
                - distance_penalty
```

For non-hidden normal entities, distance may be enough.

For hidden planets:

- scan action required
- radar level requirement
- roll-based discovery

## Client Data Filtering

Never send:

- hidden planets
- hidden loot
- hidden NPC/player exact coordinates
- procedural gameplay seed
- future spawn candidates
- loot roll table
- scan roll result before server decides

Send:

- visible entity snapshots
- known fog memory summaries
- decorative visual seed only if not gameplay-relevant

## Interaction Validation

Every interaction must recheck visibility:

```text
attack
pickup
scan result claim
share intel
route create to unknown destination
open planet action panel
```

Example:

```go
func ValidateInteraction(viewer PlayerID, target EntityID, action ActionType) error {
	entity := world.GetEntity(target)
	if entity == nil {
		return ErrEntityNotFound
	}
	if !visibility.CanInteract(viewer, entity, action) {
		return ErrNotVisible
	}
	return nil
}
```

## Scanner Bridge

Scanner module produces events:

```text
scanner.pulse_started
scanner.pulse_resolved
scanner.signal_detected
scanner.planet_discovered
```

Visibility module updates:

```text
player_planet_intel
fog memory
temporary radar contacts
```

## AOI Snapshot Rate

Suggested:

```text
simulation tick: 20Hz
normal snapshots: 5-10Hz
combat snapshots: 10-20Hz if needed
```

Snapshot includes only visible entities.

## Events Emitted

```text
aoi.entity_entered
aoi.entity_left
visibility.entity_detected
visibility.entity_lost
fog.memory_updated
scanner.visibility_unlocked
```

## Edge Cases

- Entity visible by AOI but hidden by stealth.
- Entity was visible, then jammer hides it.
- Client targets entity that just left visibility.
- Fog memory shows planet but live owner changed.
- Drop public but outside visibility.
- Zone boundary transfer duplicates visibility updates.

## Abuse Vectors

### Packet Sniffing Hidden Data

Risk:

- Client receives hidden entity data and modified client displays it.

Defense:

- never send hidden gameplay entities
- server-side filtering before serialization

### Entity ID Memory Attack

Risk:

- Client remembers entity id and interacts later while hidden.

Defense:

- every command revalidates visibility/range
- stale entity ids fail safely

### Procedural Seed Leak

Risk:

- Client gets gameplay seed and predicts planets.

Defense:

- server-only gameplay seed
- client gets decorative seed only

### Radar Spoof

Risk:

- Client claims bigger radar range.

Defense:

- radar stats from server stat snapshot
- module equip server-side

## Testing Checklist

- Hidden entity never serialized.
- Entity leaving AOI sends left/despawn.
- Attack hidden target fails.
- Pickup hidden drop fails.
- Fog memory does not imply interaction permission.
- Scanner discovery writes intel.
- Gameplay seed not present in client payloads.
- AOI stress test with many entities.

## Implementation Notes

MVP:

- spatial hash AOI
- server-side visibility filter
- planet intel memory
- hidden planet scan reveal
- no client gameplay seeds

