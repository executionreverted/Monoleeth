package aoi

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"reflect"
	"sort"

	"gameproject/internal/game/foundation"
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

// EntityMovementStatus is client-safe movement timing for a visible entity.
type EntityMovementStatus struct {
	Moving      bool       `json:"moving"`
	Origin      world.Vec2 `json:"origin"`
	Target      world.Vec2 `json:"target"`
	Speed       float64    `json:"speed"`
	StartedAtMS int64      `json:"started_at_ms"`
	ArriveAtMS  int64      `json:"arrive_at_ms"`
}

// EntityState is the server-owned AOI input for one live entity.
//
// Internal metadata fields are included to make the boundary explicit: callers
// may track hidden state here, but BuildVisibleSnapshot never copies it into a
// client payload.
type EntityState struct {
	Entity            world.Entity
	PlayerID          foundation.PlayerID
	Signature         visibility.EntitySignature
	StealthScore      float64
	JammerStrength    float64
	Hidden            bool
	PublicStatusFlags []StatusFlag
	PublicDisplay     *EntityDisplay
	PublicCombat      *EntityCombatStatus
	PublicMovement    *EntityMovementStatus
	ProjectionSource  string

	InternalMetadata map[string]string
	GameplaySeed     string
	FutureSpawnData  []string
}

// EntityPayload is the complete public payload for a visible entity.
type EntityPayload struct {
	ID               world.EntityID        `json:"entity_id"`
	Type             world.EntityType      `json:"entity_type"`
	Position         world.Vec2            `json:"position"`
	Version          uint64                `json:"version"`
	StatusFlags      []StatusFlag          `json:"status_flags,omitempty"`
	Display          *EntityDisplay        `json:"display,omitempty"`
	Combat           *EntityCombatStatus   `json:"combat,omitempty"`
	Movement         *EntityMovementStatus `json:"movement,omitempty"`
	ProjectionSource string                `json:"projection_source,omitempty"`
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
			PlayerID:       state.PlayerID,
			WorldID:        state.Entity.WorldID,
			ZoneID:         state.Entity.ZoneID,
			ID:             state.Entity.ID,
			Position:       state.Entity.Position,
			Signature:      state.Signature,
			StealthScore:   state.StealthScore,
			JammerStrength: state.JammerStrength,
			Hidden:         state.Hidden,
		}) {
			continue
		}
		payload := EntityPayload{
			ID:               state.Entity.ID,
			Type:             state.Entity.Type,
			Position:         state.Entity.Position,
			StatusFlags:      cloneStatusFlags(state.PublicStatusFlags),
			Display:          cloneEntityDisplay(state.PublicDisplay),
			Combat:           cloneEntityCombatStatus(state.PublicCombat),
			Movement:         cloneEntityMovementStatus(state.PublicMovement),
			ProjectionSource: state.ProjectionSource,
		}
		payload.Version = PublicEntityVersion(payload)
		payloads = append(payloads, payload)
	}
	sortEntityPayloads(payloads)
	return Snapshot{Entities: payloads}
}

// PublicEntityVersion returns a stable opaque version for the client-safe entity
// payload. Callers use it to skip unchanged AOI serialization work.
func PublicEntityVersion(entity EntityPayload) uint64 {
	hash := fnv.New64a()
	writeString(hash, string(entity.ID))
	writeString(hash, string(entity.Type))
	writeFloat64(hash, entity.Position.X)
	writeFloat64(hash, entity.Position.Y)
	flags := cloneStatusFlags(entity.StatusFlags)
	for _, flag := range flags {
		writeString(hash, string(flag))
	}
	if entity.Display == nil {
		writeString(hash, "display:nil")
	} else {
		writeString(hash, entity.Display.Label)
		writeString(hash, entity.Display.Disposition)
	}
	if entity.Combat == nil {
		writeString(hash, "combat:nil")
	} else {
		writeInt64(hash, int64(entity.Combat.HP))
		writeInt64(hash, int64(entity.Combat.MaxHP))
		writeInt64(hash, int64(entity.Combat.Shield))
		writeInt64(hash, int64(entity.Combat.MaxShield))
		writeString(hash, entity.Combat.Status)
	}
	if entity.Movement == nil {
		writeString(hash, "movement:nil")
	} else {
		writeBool(hash, entity.Movement.Moving)
		writeFloat64(hash, entity.Movement.Origin.X)
		writeFloat64(hash, entity.Movement.Origin.Y)
		writeFloat64(hash, entity.Movement.Target.X)
		writeFloat64(hash, entity.Movement.Target.Y)
		writeFloat64(hash, entity.Movement.Speed)
		writeInt64(hash, entity.Movement.StartedAtMS)
		writeInt64(hash, entity.Movement.ArriveAtMS)
	}
	writeString(hash, entity.ProjectionSource)
	version := hash.Sum64()
	if version == 0 {
		return 1
	}
	return version
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
		if previousEntity.Version != 0 && entity.Version != 0 && previousEntity.Version == entity.Version {
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
		entity.Movement = cloneEntityMovementStatus(entity.Movement)
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

func cloneEntityMovementStatus(movement *EntityMovementStatus) *EntityMovementStatus {
	if movement == nil {
		return nil
	}
	cloned := *movement
	return &cloned
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeString(writer byteWriter, value string) {
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(value))
}

func writeBool(writer byteWriter, value bool) {
	if value {
		_, _ = writer.Write([]byte{1})
		return
	}
	_, _ = writer.Write([]byte{0})
}

func writeFloat64(writer byteWriter, value float64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], math.Float64bits(value))
	_, _ = writer.Write(encoded[:])
}

func writeInt64(writer byteWriter, value int64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], uint64(value))
	_, _ = writer.Write(encoded[:])
}
