package aoi

import (
	"reflect"
	"sort"

	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

// StatusFlag is a client-safe public state label for a visible entity.
type StatusFlag string

// EntityDisplay is a client-safe public label for a visible entity.
type EntityDisplay struct {
	Label       string `json:"label,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

// EntityCombatStatus is client-safe combat state for a visible entity.
type EntityCombatStatus struct {
	HP        int    `json:"hp"`
	MaxHP     int    `json:"max_hp"`
	Shield    int    `json:"shield"`
	MaxShield int    `json:"max_shield"`
	Status    string `json:"status,omitempty"`
}

// EntityState is the server-owned AOI input for one live entity.
//
// Internal metadata fields are included to make the boundary explicit: callers
// may track hidden state here, but BuildVisibleSnapshot never copies it into a
// client payload.
type EntityState struct {
	Entity            world.Entity
	Signature         visibility.EntitySignature
	Hidden            bool
	PublicStatusFlags []StatusFlag
	PublicDisplay     *EntityDisplay
	PublicCombat      *EntityCombatStatus

	InternalMetadata map[string]string
	GameplaySeed     string
	FutureSpawnData  []string
}

// EntityPayload is the complete public payload for a visible entity.
type EntityPayload struct {
	ID          world.EntityID      `json:"entity_id"`
	Type        world.EntityType    `json:"entity_type"`
	Position    world.Vec2          `json:"position"`
	StatusFlags []StatusFlag        `json:"status_flags,omitempty"`
	Display     *EntityDisplay      `json:"display,omitempty"`
	Combat      *EntityCombatStatus `json:"combat,omitempty"`
}

// Snapshot is a deterministic client-safe AOI snapshot.
type Snapshot struct {
	Entities []EntityPayload `json:"entities"`
}

// Diff describes the visible entity changes between two snapshots.
type Diff struct {
	Entered []EntityPayload  `json:"entered,omitempty"`
	Updated []EntityPayload  `json:"updated,omitempty"`
	Left    []world.EntityID `json:"left,omitempty"`
}

// BuildVisibleSnapshot filters entities through visibility rules and returns a
// deterministic client-safe snapshot.
func BuildVisibleSnapshot(viewer visibility.Viewer, entities []EntityState) Snapshot {
	payloads := make([]EntityPayload, 0, len(entities))
	for _, state := range entities {
		if state.Entity.Type.Validate() != nil {
			continue
		}
		if !visibility.CanSendEntityToClient(viewer, visibility.Entity{
			WorldID:   state.Entity.WorldID,
			ZoneID:    state.Entity.ZoneID,
			ID:        state.Entity.ID,
			Position:  state.Entity.Position,
			Signature: state.Signature,
			Hidden:    state.Hidden,
		}) {
			continue
		}
		payloads = append(payloads, EntityPayload{
			ID:          state.Entity.ID,
			Type:        state.Entity.Type,
			Position:    state.Entity.Position,
			StatusFlags: cloneStatusFlags(state.PublicStatusFlags),
			Display:     cloneEntityDisplay(state.PublicDisplay),
			Combat:      cloneEntityCombatStatus(state.PublicCombat),
		})
	}
	sortEntityPayloads(payloads)
	return Snapshot{Entities: payloads}
}

// DiffSnapshots returns entered, updated, and left public entity changes from
// previous to current. Ordering is deterministic by entity id in every bucket.
func DiffSnapshots(previous Snapshot, current Snapshot) Diff {
	previousEntities := normalizedEntities(previous.Entities)
	currentEntities := normalizedEntities(current.Entities)

	previousByID := entityMap(previousEntities)
	currentByID := entityMap(currentEntities)

	diff := Diff{
		Entered: make([]EntityPayload, 0),
		Updated: make([]EntityPayload, 0),
		Left:    make([]world.EntityID, 0),
	}

	for _, entity := range currentEntities {
		previousEntity, existed := previousByID[entity.ID]
		if !existed {
			diff.Entered = append(diff.Entered, entity)
			continue
		}
		if !reflect.DeepEqual(previousEntity, entity) {
			diff.Updated = append(diff.Updated, entity)
		}
	}

	for _, entity := range previousEntities {
		if _, stillVisible := currentByID[entity.ID]; !stillVisible {
			diff.Left = append(diff.Left, entity.ID)
		}
	}

	return diff
}

func normalizedEntities(entities []EntityPayload) []EntityPayload {
	normalized := make([]EntityPayload, 0, len(entities))
	for _, entity := range entities {
		entity.StatusFlags = cloneStatusFlags(entity.StatusFlags)
		entity.Display = cloneEntityDisplay(entity.Display)
		entity.Combat = cloneEntityCombatStatus(entity.Combat)
		normalized = append(normalized, entity)
	}
	sortEntityPayloads(normalized)
	return normalized
}

func entityMap(entities []EntityPayload) map[world.EntityID]EntityPayload {
	byID := make(map[world.EntityID]EntityPayload, len(entities))
	for _, entity := range entities {
		if _, exists := byID[entity.ID]; !exists {
			byID[entity.ID] = entity
		}
	}
	return byID
}

func sortEntityPayloads(entities []EntityPayload) {
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].ID < entities[j].ID
	})
}

func cloneStatusFlags(flags []StatusFlag) []StatusFlag {
	if len(flags) == 0 {
		return nil
	}

	cloned := append([]StatusFlag(nil), flags...)
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i] < cloned[j]
	})
	return cloned
}

func cloneEntityDisplay(display *EntityDisplay) *EntityDisplay {
	if display == nil {
		return nil
	}
	cloned := *display
	if cloned.Label == "" && cloned.Disposition == "" {
		return nil
	}
	return &cloned
}

func cloneEntityCombatStatus(combat *EntityCombatStatus) *EntityCombatStatus {
	if combat == nil {
		return nil
	}
	cloned := *combat
	return &cloned
}
